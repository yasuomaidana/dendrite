package ticketindex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/tracker/plane"
)

type board struct {
	mu     sync.Mutex
	issues string
	hits   int
}

func (b *board) set(issues string) { b.mu.Lock(); b.issues = issues; b.mu.Unlock() }

func newBoard(t *testing.T) (*board, *plane.Client) {
	t.Helper()
	b := &board{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch p := r.URL.Path; {
		case strings.HasSuffix(p, "/states/"):
			_, _ = w.Write([]byte(`{"results":[{"id":"s-prog","name":"In Progress","group":"started"},{"id":"s-block","name":"Blocked","group":"started"}]}`))
		case strings.Contains(p, "/issue-relation/"):
			if strings.Contains(p, "/i2/") {
				_, _ = w.Write([]byte(`{"blocked_by":[{"id":"i1","name":"[DEN-1] Build the index"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(p, "/issues/"):
			b.mu.Lock()
			b.hits++
			out := b.issues
			b.mu.Unlock()
			_, _ = w.Write([]byte(out))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := plane.NewClient(plane.Config{BaseURL: srv.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "k"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return b, c
}

func service() *planesvc.Service {
	return planesvc.New(sidecar.Locator{LookPath: func(string) (string, error) { return "", os.ErrNotExist }})
}

const twoTickets = `{"results":[
 {"id":"i1","sequence_id":1,"state":"s-prog","name":"[DEN-1] Build the index","priority":"high","estimate_point":3,
  "description_html":"<p>Render tickets &amp; index them.</p><ul><li>one file per ticket</li><li>skip archived</li></ul>"},
 {"id":"i2","sequence_id":2,"state":"s-block","name":"[DEN-2] Federate search","priority":"medium","description_html":""}
],"next_page_results":false}`

func TestSyncRendersTicketsWithStatusRelationsAndDescription(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	b, c := newBoard(t)
	b.set(twoTickets)
	svc := service()
	defer svc.Close()

	res, err := Sync(context.Background(), svc, c, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tickets != 2 || res.Written != 2 || res.Removed != 0 {
		t.Fatalf("result = %+v", res)
	}
	one, _ := os.ReadFile(filepath.Join(res.Dir, "DEN-1.md"))
	for _, want := range []string{"# DEN-1 Build the index", "- Status: In Progress", "- Priority: high", "- Estimate: 3 points", "Render tickets & index them.", "- one file per ticket"} {
		if !strings.Contains(string(one), want) {
			t.Errorf("DEN-1.md lacks %q:\n%s", want, one)
		}
	}
	two, _ := os.ReadFile(filepath.Join(res.Dir, "DEN-2.md"))
	for _, want := range []string{"- Status: Blocked", "- Blocked by: DEN-1 Build the index"} {
		if !strings.Contains(string(two), want) {
			t.Errorf("DEN-2.md lacks %q:\n%s", want, two)
		}
	}
	if strings.Contains(string(two), "## Description") {
		t.Error("an empty description produced a section")
	}
}

func TestSyncRewritesOnlyChangesRemovesGoneTicketsAndSkipsArchived(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	b, c := newBoard(t)
	b.set(twoTickets)
	svc := service()
	defer svc.Close()
	ctx := context.Background()
	first, _ := Sync(ctx, svc, c, true)

	again, err := Sync(ctx, svc, c, true)
	if err != nil || again.Written != 0 || again.Tickets != 2 {
		t.Fatalf("unchanged board: %+v, %v (nothing should be rewritten)", again, err)
	}

	// DEN-2 disappears, DEN-3 is archived, DEN-1 changes state.
	b.set(`{"results":[
 {"id":"i1","sequence_id":1,"state":"s-block","name":"[DEN-1] Build the index","description_html":""},
 {"id":"i3","sequence_id":3,"state":"s-prog","name":"[DEN-3] Old work","archived_at":"2026-10-01T00:00:00Z"}
],"next_page_results":false}`)
	res, err := Sync(ctx, svc, c, true)
	if err != nil || res.Written != 1 || res.Removed != 1 || res.Tickets != 1 {
		t.Fatalf("changed board: %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(first.Dir, "DEN-2.md")); !os.IsNotExist(err) {
		t.Error("the file of a ticket that no longer exists was kept")
	}
	if _, err := os.Stat(filepath.Join(first.Dir, "DEN-3.md")); !os.IsNotExist(err) {
		t.Error("an archived ticket was indexed")
	}
	if one, _ := os.ReadFile(filepath.Join(first.Dir, "DEN-1.md")); !strings.Contains(string(one), "Status: Blocked") {
		t.Errorf("DEN-1 not updated:\n%s", one)
	}
}

func TestSyncReusesAFreshIndexAndUntitledTicketsGetAFileToo(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	b, c := newBoard(t)
	b.set(`{"results":[{"id":"i9","sequence_id":9,"state":"s-prog","name":"No id in the title"}],"next_page_results":false}`)
	svc := service()
	defer svc.Close()
	ctx := context.Background()
	res, err := Sync(ctx, svc, c, false)
	if err != nil || res.Reused || res.Written != 1 {
		t.Fatalf("first: %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(res.Dir, "plane-9.md")); err != nil {
		t.Errorf("untitled ticket file: %v", err)
	}
	hits := b.hits
	res, err = Sync(ctx, svc, c, false)
	if err != nil || !res.Reused || b.hits != hits {
		t.Errorf("a fresh index was not reused: %+v (list calls %d -> %d)", res, hits, b.hits)
	}
	old := time.Now().Add(-2 * FreshFor)
	_ = os.Chtimes(filepath.Join(res.Dir, stampFile), old, old)
	if res, _ = Sync(ctx, svc, c, false); res.Reused {
		t.Error("a stale index was reused")
	}
}

func TestTextTurnsPlaneHTMLIntoReadableText(t *testing.T) {
	got := Text(`<h2>Goal</h2><p>Fast &amp; cheap<br/>lookups</p><ul><li>a</li><li>b &lt;c&gt;</li></ul>`)
	for _, want := range []string{"Goal", "Fast & cheap", "lookups", "- a", "- b <c>"} {
		if !strings.Contains(got, want) {
			t.Errorf("Text lacks %q: %q", want, got)
		}
	}
	if strings.Contains(got, "<") && !strings.Contains(got, "<c>") || Text("  ") != "" {
		t.Errorf("tags leaked or blank input not empty: %q", got)
	}
}

func TestResolvePassesOtherCorporaThroughUnchanged(t *testing.T) {
	p, c, err := Resolve(context.Background(), "/some/project", "docs")
	if err != nil || p != "/some/project" || c != "docs" {
		t.Errorf("Resolve = %q %q %v", p, c, err)
	}
}
