package plane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stateBoard is a minimal in-memory Plane states endpoint supporting GET, POST and PATCH.
type stateBoard struct {
	states   []PlaneState
	projects []PlaneProject
	writes   []string
}

func (b *stateBoard) handler(t *testing.T) http.HandlerFunc {
	const prefix = "/api/v1/workspaces/ws/projects/"
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == prefix && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": b.projects})
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, prefix)
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) < 2 || parts[1] != "states" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]interface{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": b.states})
		case r.Method == http.MethodPost:
			b.writes = append(b.writes, "POST "+body["name"].(string))
			b.states = append(b.states, PlaneState{
				ID:       "new-" + body["name"].(string),
				Name:     body["name"].(string),
				Group:    body["group"].(string),
				Color:    body["color"].(string),
				Sequence: body["sequence"].(float64),
			})
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPatch && len(parts) == 3:
			b.writes = append(b.writes, "PATCH "+parts[2])
			for i := range b.states {
				if b.states[i].ID != parts[2] {
					continue
				}
				if v, ok := body["name"].(string); ok {
					b.states[i].Name = v
				}
				if v, ok := body["group"].(string); ok {
					b.states[i].Group = v
				}
				if v, ok := body["color"].(string); ok {
					b.states[i].Color = v
				}
				if v, ok := body["sequence"].(float64); ok {
					b.states[i].Sequence = v
				}
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func newBoardClient(t *testing.T, b *stateBoard) *Client {
	t.Helper()
	ts := httptest.NewServer(b.handler(t))
	t.Cleanup(ts.Close)
	c, err := NewClient(Config{BaseURL: ts.URL, WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "tok"}, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// planeDefaultBoard mirrors what Plane creates for a fresh project plus the legacy To Verify.
func planeDefaultBoard() []PlaneState {
	return []PlaneState{
		{ID: "b", Name: "Backlog", Group: "backlog", Color: "#A3A3A3", Sequence: 15000},
		{ID: "t", Name: "Todo", Group: "unstarted", Color: "#3A3A3A", Sequence: 25000},
		{ID: "p", Name: "In Progress", Group: "started", Color: "#F59E0B", Sequence: 35000},
		{ID: "d", Name: "Done", Group: "completed", Color: "#16A34A", Sequence: 45000},
		{ID: "c", Name: "Cancelled", Group: "cancelled", Color: "#EF4444", Sequence: 55000},
		{ID: "v", Name: "To Verify", Group: "started", Color: "#F59E0B", Sequence: 70000},
	}
}

func assertCanonical(t *testing.T, states []PlaneState) {
	t.Helper()
	byName := map[string]PlaneState{}
	for _, s := range states {
		byName[s.Name] = s
	}
	for _, spec := range CanonicalStates {
		got, ok := byName[spec.Name]
		if !ok {
			t.Errorf("missing canonical state %q", spec.Name)
			continue
		}
		if got.Group != spec.Group || !strings.EqualFold(got.Color, spec.Color) || got.Sequence != spec.Sequence {
			t.Errorf("state %q = %+v, want %+v", spec.Name, got, spec)
		}
	}
}

func TestReconcileStates_ConvergesSubProjectBoard(t *testing.T) {
	board := &stateBoard{states: planeDefaultBoard()}
	c := newBoardClient(t, board)

	rep, err := c.ReconcileStates(context.Background(), "p", false)
	if err != nil {
		t.Fatal(err)
	}
	assertCanonical(t, board.states)
	if len(rep.Created()) != 0 {
		t.Errorf("expected no creates, got %v", rep.Created())
	}
	var renamed bool
	for _, ch := range rep.Changes {
		if ch.Name == "To Do" && ch.From == "Todo" {
			renamed = true
		}
	}
	if !renamed {
		t.Errorf("expected Todo -> To Do rename in %+v", rep.Changes)
	}

	// Idempotent: a second pass writes nothing.
	board.writes = nil
	rep2, err := c.ReconcileStates(context.Background(), "p", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep2.Changes) != 0 || len(board.writes) != 0 {
		t.Errorf("second pass should be a no-op, got changes=%v writes=%v", rep2.Changes, board.writes)
	}
}

func TestReconcileStates_DENBoardKeepsUnmanagedAndFillsBlankColors(t *testing.T) {
	board := &stateBoard{states: []PlaneState{
		{ID: "b", Name: "Backlog", Group: "backlog", Sequence: 1},
		{ID: "t", Name: "To Do", Group: "unstarted", Sequence: 15001},
		{ID: "p", Name: "In Progress", Group: "started", Sequence: 30001},
		{ID: "d", Name: "Done", Group: "completed", Sequence: 45001},
		{ID: "v", Name: "To Verify", Group: "started", Color: "#fcb900", Sequence: 60001},
		{ID: "r", Name: "Relocated", Group: "cancelled", Color: "#9AA4BC", Sequence: 75001},
	}}
	c := newBoardClient(t, board)

	rep, err := c.ReconcileStates(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	assertCanonical(t, board.states)
	if got := rep.Created(); len(got) != 1 || got[0] != "Cancelled" {
		t.Errorf("expected Cancelled created, got %v", got)
	}
	if len(rep.Unmanaged) != 1 || rep.Unmanaged[0] != "Relocated" {
		t.Errorf("expected Relocated reported unmanaged, got %v", rep.Unmanaged)
	}
	for _, s := range board.states {
		if s.Name == "Relocated" && s.Color != "#9AA4BC" {
			t.Errorf("unmanaged state must not be touched: %+v", s)
		}
	}
}

func TestReconcileStates_ExactNameBeatsAlias(t *testing.T) {
	board := &stateBoard{states: []PlaneState{
		{ID: "legacy", Name: "Todo", Group: "unstarted"},
		{ID: "real", Name: "To Do", Group: "unstarted"},
	}}
	c := newBoardClient(t, board)
	rep, err := c.ReconcileStates(context.Background(), "p", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Unmanaged) != 1 || rep.Unmanaged[0] != "Todo" {
		t.Errorf("expected legacy Todo left unmanaged, got %v", rep.Unmanaged)
	}
	for _, ch := range rep.Changes {
		if ch.Name == "To Do" && ch.From != "" {
			t.Errorf("To Do must not be renamed from %q", ch.From)
		}
	}
}

func TestReconcileStates_DryRunWritesNothing(t *testing.T) {
	board := &stateBoard{states: planeDefaultBoard()[:2]}
	c := newBoardClient(t, board)
	rep, err := c.ReconcileStates(context.Background(), "p", true)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || len(rep.Changes) == 0 {
		t.Fatalf("expected a non-empty dry-run plan, got %+v", rep)
	}
	if len(board.writes) != 0 {
		t.Errorf("dry run must not write, got %v", board.writes)
	}
}

func TestReconcileStates_Errors(t *testing.T) {
	c := newBoardClient(t, &stateBoard{})
	c.projectID = ""
	if _, err := c.ReconcileStates(context.Background(), "", false); !errors.Is(err, ErrEmptyProjectID) {
		t.Errorf("expected ErrEmptyProjectID, got %v", err)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[{"id":"t","name":"Todo","group":"unstarted"}]`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	fc, _ := NewClient(Config{BaseURL: failing.URL, WorkspaceSlug: "ws", ProjectID: "p", DirectToken: "tok"}, failing.Client())
	if _, err := fc.ReconcileStates(context.Background(), "p", false); err == nil || !strings.Contains(err.Error(), "Backlog") {
		t.Errorf("expected create failure naming Backlog, got %v", err)
	}
}

func TestClientForProject_ResolvesIdentifierOrUUID(t *testing.T) {
	board := &stateBoard{projects: []PlaneProject{
		{ID: "uuid-den", Identifier: "DEN"},
		{ID: "uuid-git", Identifier: "DGIT"},
	}}
	base := newBoardClient(t, board)
	ctx := context.Background()

	scoped, err := ClientForProject(ctx, base, "dgit")
	if err != nil {
		t.Fatal(err)
	}
	if scoped.ProjectID() != "uuid-git" || base.ProjectID() != "p" {
		t.Errorf("scoped=%s base=%s", scoped.ProjectID(), base.ProjectID())
	}
	if p, err := base.ResolveProject(ctx, "uuid-den"); err != nil || p.Identifier != "DEN" {
		t.Errorf("resolve by uuid: %+v %v", p, err)
	}
	if _, err := ClientForProject(ctx, base, "NOPE"); err == nil {
		t.Error("expected not-found error")
	}
	if _, err := base.ResolveProject(ctx, " "); !errors.Is(err, ErrEmptyProjectID) {
		t.Errorf("expected ErrEmptyProjectID, got %v", err)
	}
}
