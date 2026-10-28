package planesvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// localPlane is a small real HTTP server speaking the Plane routes Dendrite uses: three pages of issues (one issue repeated across pages, as Plane does
// for an issue in two modules), canonical states, and relations for the open issues.
type localPlane struct {
	*httptest.Server
	mu        sync.Mutex
	relCalls  []string
	listCalls int
	patched   []string
}

func newLocalPlane(t *testing.T) *localPlane {
	t.Helper()
	f := &localPlane{}
	issue := func(id string, seq int, state, name string) string {
		return fmt.Sprintf(`{"id":%q,"sequence_id":%d,"state":%q,"name":%q,"priority":"high"}`, id, seq, state, name)
	}
	pages := map[string]string{
		"":   `{"results":[` + issue("i1", 1, "s-todo", "[DEN-1] one") + `,` + issue("i2", 2, "s-prog", "[DEN-2] two") + `],"next_page_results":true,"next_cursor":"p2"}`,
		"p2": `{"results":[` + issue("i2", 2, "s-prog", "[DEN-2] two") + `,` + issue("i3", 3, "s-done", "[DEN-3] three") + `,` + issue("i4", 4, "s-block", "[DEN-4] four") + `],"next_page_results":false}`,
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/states/"):
			_, _ = w.Write([]byte(`{"results":[{"id":"s-todo","name":"To Do","group":"unstarted"},{"id":"s-prog","name":"In Progress","group":"started"},{"id":"s-block","name":"Blocked","group":"started"},{"id":"s-done","name":"Done","group":"completed"}]}`))
		case strings.Contains(p, "/issue-relation/"):
			parts := strings.Split(p, "/")
			id := parts[len(parts)-3]
			f.mu.Lock()
			f.relCalls = append(f.relCalls, id)
			f.mu.Unlock()
			if id == "i2" {
				_, _ = w.Write([]byte(`{"blocked_by":[{"id":"i1","name":"[DEN-1] one"}],"blocking":[],"duplicate":[],"relates_to":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"blocked_by":[],"blocking":[],"duplicate":[],"relates_to":[]}`))
		case r.Method == http.MethodPatch && strings.Contains(p, "/issues/"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.patched = append(f.patched, strings.Split(p, "/")[len(strings.Split(p, "/"))-2]+"="+body["state"])
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(p, "/issues/"):
			f.mu.Lock()
			f.listCalls++
			f.mu.Unlock()
			_, _ = w.Write([]byte(pages[r.URL.Query().Get("cursor")]))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *localPlane) client(t *testing.T) *plane.Client {
	t.Helper()
	c, err := plane.NewClient(plane.Config{BaseURL: f.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "k"}, f.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func absentService() *Service {
	return New(sidecar.Locator{LookPath: func(string) (string, error) { return "", os.ErrNotExist }})
}

func realService() *Service { return New(sidecar.DefaultLocator()) }

func checkSnapshot(t *testing.T, snap *Snapshot) {
	t.Helper()
	var ids []string
	for _, iss := range snap.Issues {
		ids = append(ids, iss.ID)
	}
	if !reflect.DeepEqual(ids, []string{"i1", "i2", "i3", "i4"}) {
		t.Errorf("issues = %v, want the four distinct ones in order", ids)
	}
	wantStatus := map[string]tracker.Status{"i1": tracker.StatusTodo, "i2": tracker.StatusInProgress, "i3": tracker.StatusDone, "i4": tracker.StatusBlocked}
	if !reflect.DeepEqual(snap.Statuses, wantStatus) {
		t.Errorf("statuses = %v, want %v", snap.Statuses, wantStatus)
	}
	var withRels []string
	for id := range snap.Relations {
		withRels = append(withRels, id)
	}
	sort.Strings(withRels)
	if !reflect.DeepEqual(withRels, []string{"i1", "i2", "i4"}) {
		t.Errorf("relations for %v, want the open issues only (not the Done i3)", withRels)
	}
	if got := snap.Relations["i2"].BlockedBy; len(got) != 1 || got[0].ID != "i1" {
		t.Errorf("i2 blocked_by = %+v", got)
	}
}

func TestSnapshotComesFromTheSidecar(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newLocalPlane(t)
	svc := realService()
	defer svc.Close()

	snap, out, err := svc.Snapshot(context.Background(), f.client(t), true)
	if err != nil {
		t.Fatal(err)
	}
	if out.Origin != sidecar.OriginSidecar {
		t.Errorf("outcome = %+v", out)
	}
	checkSnapshot(t, snap)
	for _, id := range f.relCalls {
		if id == "i3" {
			t.Error("relations were requested for a Done ticket")
		}
	}
}

func TestMissingSidecarIsAnErrorThatNamesTheFix(t *testing.T) {
	t.Setenv(sidecar.EnvVar(BinaryName), "")
	f := newLocalPlane(t)
	svc := absentService()
	defer svc.Close()
	_, _, err := svc.Snapshot(context.Background(), f.client(t), false)
	if !errors.Is(err, ErrSidecarMissing) || !strings.Contains(err.Error(), "dendrite install") {
		t.Fatalf("err = %v, want ErrSidecarMissing naming dendrite install", err)
	}
	if _, err := svc.SetStatus(context.Background(), f.client(t), "DEN-1", tracker.StatusInProgress); !errors.Is(err, ErrSidecarMissing) {
		t.Errorf("SetStatus err = %v, want ErrSidecarMissing", err)
	}
}

func TestPlaneErrorsSurfaceWithTheirSentinel(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	c, _ := plane.NewClient(plane.Config{BaseURL: srv.URL, WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "k"}, srv.Client())
	svc := realService()
	defer svc.Close()
	if _, _, err := svc.Snapshot(context.Background(), c, false); !errors.Is(err, plane.ErrUnauthorized) {
		t.Errorf("err = %v, want plane.ErrUnauthorized", err)
	}
}

func TestSetStatusMovesTheTicketThroughTheSidecar(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newLocalPlane(t)
	svc := realService()
	defer svc.Close()
	if _, err := svc.SetStatus(context.Background(), f.client(t), "DEN-1", tracker.StatusInProgress); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.patched, []string{"i1=s-prog"}) {
		t.Errorf("patched = %v, want i1 moved to the In Progress state", f.patched)
	}
}

func TestSetStatusUnknownTicketIsIssueNotFound(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "k")
	f := newLocalPlane(t)
	svc := realService()
	defer svc.Close()
	if _, err := svc.SetStatus(context.Background(), f.client(t), "DEN-99", tracker.StatusInProgress); !errors.Is(err, plane.ErrIssueNotFound) {
		t.Errorf("err = %v, want plane.ErrIssueNotFound", err)
	}
}

func ids(s *Snapshot) []string {
	var out []string
	for _, i := range s.Issues {
		out = append(out, i.ID)
	}
	return out
}

func relIDs(r plane.IssueRelationsResponse) []string {
	var out []string
	for _, group := range [][]plane.IssueRelation{r.BlockedBy, r.Blocking, r.Duplicate, r.RelatesTo} {
		for _, x := range group {
			out = append(out, x.ID)
		}
		out = append(out, "|")
	}
	return out
}
