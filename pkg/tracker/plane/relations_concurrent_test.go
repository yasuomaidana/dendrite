package plane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetRelationsForRunsConcurrentlyAndSkipsFailures(t *testing.T) {
	var inFlight, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		if strings.Contains(r.URL.Path, "/bad/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"blocked_by":[{"id":"x","name":"[DEN-1] x"}]}`))
	}))
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL, WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "k"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	ids := []string{"bad"}
	for i := 0; i < 40; i++ {
		ids = append(ids, "i"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	start := time.Now()
	got, firstErr := c.GetRelationsFor(context.Background(), ids)
	elapsed := time.Since(start)

	if firstErr == nil {
		t.Error("expected the failing issue's error to be reported")
	}
	if _, ok := got["bad"]; ok || len(got) != 40 {
		t.Errorf("got %d relations (bad present: %v), want the 40 good ones only", len(got), ok)
	}
	if peak < 2 || peak > RelationFetchWorkers {
		t.Errorf("peak concurrency %d, want between 2 and %d", peak, RelationFetchWorkers)
	}
	if serial := 41 * 20 * time.Millisecond; elapsed > serial/2 {
		t.Errorf("took %v, not meaningfully faster than serial %v", elapsed, serial)
	}
	if empty, err := c.GetRelationsFor(context.Background(), nil); err != nil || len(empty) != 0 {
		t.Errorf("empty input = %v, %v", empty, err)
	}
}

func TestListPlaneIssuesDropsRowsRepeatedAcrossPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "c2" {
			_, _ = w.Write([]byte(`{"results":[{"id":"a","name":"[DEN-1] one"},{"id":"b","name":"[DEN-2] two"}],"next_page_results":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"id":"a","name":"[DEN-1] one"},{"id":"c","name":"[DEN-3] three"}],"next_page_results":true,"next_cursor":"c2"}`))
	}))
	defer srv.Close()
	c, err := NewClient(Config{BaseURL: srv.URL, WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "k"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	issues, err := c.ListPlaneIssues(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, i := range issues {
		ids = append(ids, i.ID)
	}
	if strings.Join(ids, ",") != "a,c,b" {
		t.Errorf("ids = %v, want a,c,b (first occurrence kept, order kept)", ids)
	}
}

func TestRelationWritesNotifyTheRelationsChangedHook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := NewClient(Config{BaseURL: srv.URL, WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "k"}, srv.Client())

	var got [][]string
	SetRelationsChangedHook(func(_ *Client, ids []string) { got = append(got, ids) })
	defer SetRelationsChangedHook(nil)

	if err := c.AddIssueRelation(context.Background(), "a", "blocked_by", []string{"b", "c"}); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveIssueRelation(context.Background(), "a", "blocked_by", "b"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || strings.Join(got[0], ",") != "a,b,c" || strings.Join(got[1], ",") != "a,b" {
		t.Errorf("hook calls = %v, want [a b c] then [a b]", got)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer failing.Close()
	fc, _ := NewClient(Config{BaseURL: failing.URL, WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "k"}, failing.Client())
	got = nil
	_ = fc.AddIssueRelation(context.Background(), "a", "blocked_by", []string{"b"})
	if len(got) != 0 {
		t.Errorf("a failed write still invalidated caches: %v", got)
	}
}
