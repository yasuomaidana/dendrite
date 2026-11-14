package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// statesPlane serves one project's states: GET lists them, POST creates one with the wrong
// sequence (as Plane does), PATCH answers patchStatus when it is set.
type statesPlane struct {
	*scriptedPlane
	existing    []PlaneState
	postStatus  int
	patchStatus int
	patched     []string
}

func newStatesPlane(t *testing.T, existing ...PlaneState) *statesPlane {
	t.Helper()
	sp := &statesPlane{scriptedPlane: newScriptedPlane(t), existing: existing}
	sp.HandleFunc("", "", func(w http.ResponseWriter, r *http.Request) {
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
	})
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
	allDrifted := func() []PlaneState {
		all := make([]PlaneState, len(CanonicalStates))
		for i, s := range CanonicalStates {
			all[i] = PlaneState{ID: s.Name, Name: s.Name, Group: "wrong", Color: s.Color, Sequence: s.Sequence}
		}
		return all
	}
	tests := []struct {
		name     string
		existing []PlaneState
		setErr   func(sp *statesPlane)
		wantErr  string
	}{
		{
			name:    "create",
			setErr:  func(sp *statesPlane) { sp.postStatus = 500 },
			wantErr: "create state",
		},
		{
			name:    "order",
			setErr:  func(sp *statesPlane) { sp.patchStatus = 500 },
			wantErr: "order state",
		},
		{
			name:     "update",
			existing: allDrifted(),
			setErr:   func(sp *statesPlane) { sp.patchStatus = 500 },
			wantErr:  "update state",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sp := newStatesPlane(t, tc.existing...)
			tc.setErr(sp)
			if _, err := planeAt(t, sp.URL).ReconcileStates(ctx, "", false); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	t.Run("an unreadable state list", func(t *testing.T) {
		s := statusServer(500, "")
		defer s.Close()
		if _, err := planeAt(t, s.URL).ResolveProject(ctx, "DEN"); err == nil {
			t.Fatal("expected the project list error")
		}
	})
}
