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
