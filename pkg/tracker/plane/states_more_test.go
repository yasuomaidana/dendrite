package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// statesPlane serves one project's states: GET lists them, POST creates one with the wrong
// sequence (as Plane does), PATCH answers patchStatus when it is set.
type statesPlane struct {
	*httptest.Server
	existing    []PlaneState
	postStatus  int
	patchStatus int
	patched     []string
}

func newStatesPlane(t *testing.T, existing ...PlaneState) *statesPlane {
	t.Helper()
	sp := &statesPlane{existing: existing}
	sp.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(sp.existing)
		case http.MethodPost:
			if sp.postStatus != 0 {
				w.WriteHeader(sp.postStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(PlaneState{ID: "created", Sequence: 1})
		case http.MethodPatch:
			sp.patched = append(sp.patched, r.URL.Path)
			if sp.patchStatus != 0 {
				w.WriteHeader(sp.patchStatus)
				return
			}
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(sp.Close)
	return sp
}

func TestReconcileStatesFixesDriftAndOrdersNewStates(t *testing.T) {
	spec := CanonicalStates[1] // To Do
	sp := newStatesPlane(t, PlaneState{ID: "s1", Name: spec.Name, Group: "wrong", Color: spec.Color, Sequence: spec.Sequence})
	report, err := planeAt(t, sp.URL).ReconcileStates(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	var fields []string
	for _, ch := range report.Changes {
		if ch.Action == "update" {
			fields = ch.Fields
		}
	}
	if len(fields) != 1 || fields[0] != "group" {
		t.Errorf("update fields = %v, want only the group", fields)
	}
	// Six states are created and each is then moved into place, plus the group fix.
	if len(sp.patched) != 7 {
		t.Errorf("patches = %d, want 7: %v", len(sp.patched), sp.patched)
	}
}

func TestReconcileStatesReportsEachWriteThatFails(t *testing.T) {
	ctx := context.Background()
	t.Run("create", func(t *testing.T) {
		sp := newStatesPlane(t)
		sp.postStatus = 500
		if _, err := planeAt(t, sp.URL).ReconcileStates(ctx, "", false); err == nil || !strings.Contains(err.Error(), "create state") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("order", func(t *testing.T) {
		sp := newStatesPlane(t)
		sp.patchStatus = 500
		if _, err := planeAt(t, sp.URL).ReconcileStates(ctx, "", false); err == nil || !strings.Contains(err.Error(), "order state") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("update", func(t *testing.T) {
		all := make([]PlaneState, len(CanonicalStates))
		for i, s := range CanonicalStates {
			all[i] = PlaneState{ID: s.Name, Name: s.Name, Group: "wrong", Color: s.Color, Sequence: s.Sequence}
		}
		sp := newStatesPlane(t, all...)
		sp.patchStatus = 500
		if _, err := planeAt(t, sp.URL).ReconcileStates(ctx, "", false); err == nil || !strings.Contains(err.Error(), "update state") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("an unreadable state list", func(t *testing.T) {
		s := statusServer(500, "")
		defer s.Close()
		if _, err := planeAt(t, s.URL).ResolveProject(ctx, "DEN"); err == nil {
			t.Fatal("expected the project list error")
		}
	})
}
