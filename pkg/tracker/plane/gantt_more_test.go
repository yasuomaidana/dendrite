package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ganttPlane serves modules, issues and per-module issues, and records what the sync changes.
type ganttPlane struct {
	*httptest.Server
	mu           sync.Mutex
	modules      []PlaneModule
	issues       []PlaneIssue
	inModule     map[string][]PlaneIssue
	failModules  bool
	failIssues   bool
	issuePatches []map[string]interface{}
}

func newGanttPlane(t *testing.T, modules []PlaneModule, issues []PlaneIssue) *ganttPlane {
	t.Helper()
	g := &ganttPlane{modules: modules, issues: issues, inModule: map[string][]PlaneIssue{}}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/modules/"):
			if g.failModules {
				w.WriteHeader(500)
				return
			}
			_ = json.NewEncoder(w).Encode(g.modules)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/issues/"):
			if g.failIssues {
				w.WriteHeader(500)
				return
			}
			_ = json.NewEncoder(w).Encode(g.issues)
		case r.Method == http.MethodGet && strings.Contains(path, "/module-issues/"):
			parts := strings.Split(path, "/")
			_ = json.NewEncoder(w).Encode(g.inModule[parts[len(parts)-3]])
		case r.Method == http.MethodPatch && strings.Contains(path, "/issues/"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			g.issuePatches = append(g.issuePatches, body)
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(g.Close)
	return g
}

func TestGanttSyncStopsWhenTheBoardCannotBeRead(t *testing.T) {
	ctx := context.Background()
	for name, set := range map[string]func(*ganttPlane){
		"modules": func(g *ganttPlane) { g.failModules = true },
		"issues":  func(g *ganttPlane) { g.failIssues = true },
	} {
		t.Run(name, func(t *testing.T) {
			g := newGanttPlane(t, nil, nil)
			set(g)
			if _, err := SyncPlaneGanttAndRelations(ctx, planeAt(t, g.URL)); err == nil {
				t.Fatal("expected the board error")
			}
		})
	}
	t.Run("an empty board", func(t *testing.T) {
		g := newGanttPlane(t, nil, nil)
		report, err := SyncPlaneGanttAndRelations(ctx, planeAt(t, g.URL))
		if err != nil || report.TotalModules != 0 || report.TotalTickets != 0 {
			t.Fatalf("report = %+v, err = %v", report, err)
		}
	})
}

func TestGanttSyncPlacesIssuesThatBelongToNoModule(t *testing.T) {
	modules := []PlaneModule{
		{ID: "b", Name: "Milestone 2: B"},
		{ID: "a", Name: "Milestone 1: A"},
		{ID: "empty", Name: "Milestone 3: Empty"},
		{ID: "n2", Name: "Beta"},
		{ID: "n1", Name: "Alpha"},
	}
	issues := []PlaneIssue{
		{ID: "by-name", Name: "Work for Milestone 2", SequenceID: 7},
		{ID: "no-seq", Name: "Unnumbered", SequenceID: 0, Priority: "none"},
		{ID: "by-seq", Name: "Plain", SequenceID: 3, Priority: "high"},
	}
	g := newGanttPlane(t, modules, issues)
	report, err := SyncPlaneGanttAndRelations(context.Background(), planeAt(t, g.URL))
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalTickets != 3 || report.UpdatedTickets != 3 {
		t.Fatalf("report = %+v, want all three tickets scheduled", report)
	}
	if report.UpdatedModules != 5 {
		t.Errorf("modules without dates must all be updated, got %d", report.UpdatedModules)
	}
	prios := map[interface{}]int{}
	for _, p := range g.issuePatches {
		prios[p["priority"]]++
	}
	if prios["medium"] != 2 || prios["high"] != 1 {
		t.Errorf("priorities = %v, want none and blank turned into medium", prios)
	}
}

func TestGanttSyncLeavesDatedIssuesAlone(t *testing.T) {
	d := "2026-01-01"
	g := newGanttPlane(t,
		[]PlaneModule{{ID: "m", Name: "Milestone 1", StartDate: &d, TargetDate: &d}},
		[]PlaneIssue{{ID: "i", Name: "x", SequenceID: 1, StartDate: &d, TargetDate: &d}})
	g.inModule["m"] = g.issues
	report, err := SyncPlaneGanttAndRelations(context.Background(), planeAt(t, g.URL))
	if err != nil || report.UpdatedModules != 0 || report.UpdatedTickets != 0 {
		t.Fatalf("report = %+v, err = %v; dated work must not be rewritten", report, err)
	}
}
