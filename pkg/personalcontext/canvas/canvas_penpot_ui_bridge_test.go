package canvas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/plane"
)

func TestPenpotUIBridge_UpdateProjectState(t *testing.T) {
	ctx := context.Background()

	// Mock Plane Server tracking state updates
	var lastUpdatedState string
	planeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/states/") {
			states := []plane.PlaneState{
				{ID: "state-backlog", Name: "Backlog", Group: "backlog"},
				{ID: "state-todo", Name: "To Do", Group: "unstarted"},
				{ID: "state-progress", Name: "In Progress", Group: "started"},
				{ID: "state-verify", Name: "To Verify", Group: "started"},
				{ID: "state-done", Name: "Done", Group: "completed"},
			}
			_ = json.NewEncoder(w).Encode(states)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/issues/") {
			issues := []plane.PlaneIssue{
				{ID: "issue-123", Name: "[DEN-807] Canvas Interactive Writing"},
			}
			_ = json.NewEncoder(w).Encode(issues)
			return
		}
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/issues/") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if s, ok := body["state"].(string); ok {
				lastUpdatedState = s
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "issue-123"})
			return
		}
		if strings.Contains(r.URL.Path, "/comments/") {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "cmt-1"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer planeServer.Close()

	planeClient, _ := plane.NewClient(plane.Config{
		BaseURL:       planeServer.URL,
		WorkspaceSlug: "dendrite-ws",
		ProjectID:     "proj-test-1",
		DirectToken:   "plane_token",
	}, nil)

	bridge := NewPenpotUIBridge(planeClient, "http://localhost:9000")

	// Missing ticket error
	_, err := bridge.UpdateProjectState(ctx, ProjectStateUpdateRequest{TargetState: "To Verify"})
	if err != ErrMissingTicketID {
		t.Errorf("expected ErrMissingTicketID, got: %v", err)
	}

	// Missing target state error
	_, err = bridge.UpdateProjectState(ctx, ProjectStateUpdateRequest{TicketID: "DEN-807"})
	if err != ErrMissingTargetState {
		t.Errorf("expected ErrMissingTargetState, got: %v", err)
	}

	// Update project state with Plane sync
	req := ProjectStateUpdateRequest{
		TicketID:       "DEN-807",
		TargetState:    "To Verify",
		Comment:        "Design reviewed and verified in Penpot",
		PenpotFrameURL: "http://localhost:9000/#/workspace/dendrite/project/ui?frame=canvas",
		PushToPlane:    true,
	}

	resp, err := bridge.UpdateProjectState(ctx, req)
	if err != nil {
		t.Fatalf("update project state failed: %v", err)
	}

	if !resp.PlaneSynced {
		t.Errorf("expected PlaneSynced to be true")
	}
	if resp.NewState != "To Verify" {
		t.Errorf("unexpected new state: %s", resp.NewState)
	}
	if lastUpdatedState != "state-verify" {
		t.Errorf("expected Plane state to be patched to 'state-verify', got: %s", lastUpdatedState)
	}

	// Verify relation updated in memory
	rel, ok := bridge.GetTicketFeatureRelation("DEN-807")
	if !ok || rel.CurrentState != "To Verify" {
		t.Errorf("expected relation current state to be 'To Verify'")
	}
}

func TestRegistry_UIHandlersViaDispatch(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	uiFile := filepath.Join(tmpDir, "Widget.svelte")
	_ = os.WriteFile(uiFile, []byte(`<div class="bg-blue-500 p-2">Widget</div>`), 0644)

	bridge := NewPenpotUIBridge(nil, "http://localhost:9000")
	reg := NewDefaultRegistry(bridge)

	if reg.Bridge() != bridge {
		t.Errorf("mismatched bridge in registry")
	}

	// 1. Relate ticket action
	relateResp, err := reg.Dispatch(ctx, ActionRequest{
		Action: ActionRelateTicket,
		Title:  "DEN-807",
		Prompt: "Canvas Workspace UI",
		Parameters: map[string]interface{}{
			"component_path": uiFile,
		},
	})
	if err != nil {
		t.Fatalf("dispatch ActionRelateTicket failed: %v", err)
	}
	if relateResp.UIBridgeResult == nil || relateResp.UIBridgeResult.TicketID != "DEN-807" {
		t.Errorf("unexpected UIBridgeResult in relateResp")
	}

	// 2. Sync Penpot action
	syncResp, err := reg.Dispatch(ctx, ActionRequest{
		Action: ActionSyncPenpot,
		Prompt: uiFile,
		Parameters: map[string]interface{}{
			"ticket_id": "DEN-807",
			"dry_run":   true,
		},
	})
	if err != nil {
		t.Fatalf("dispatch ActionSyncPenpot failed: %v", err)
	}
	if syncResp.UIBridgeResult == nil || syncResp.UIBridgeResult.FeatureName != "Widget" {
		t.Errorf("unexpected UIBridgeResult in syncResp")
	}

	// 3. Update state action
	updateResp, err := reg.Dispatch(ctx, ActionRequest{
		Action: ActionUpdateState,
		Title:  "DEN-807",
		Prompt: "To Verify",
	})
	if err != nil {
		t.Fatalf("dispatch ActionUpdateState failed: %v", err)
	}
	if updateResp.UIBridgeResult == nil || updateResp.UIBridgeResult.ProjectState != "To Verify" {
		t.Errorf("unexpected UIBridgeResult in updateResp")
	}
}

func TestPenpotUIBridge_Persistence(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	storageFile := filepath.Join(tmpDir, "relations.json")

	bridge1 := NewPenpotUIBridge(nil, "http://localhost:9000")
	bridge1.SetStoragePath(storageFile)

	err := bridge1.RelateTicketToFeature(ctx, TicketFeatureRelation{
		TicketID:    "DEN-807",
		FeatureName: "Persistence Test Feature",
	})
	if err != nil {
		t.Fatalf("relate failed: %v", err)
	}

	// Create bridge2 loading same file
	bridge2 := NewPenpotUIBridge(nil, "http://localhost:9000")
	bridge2.SetStoragePath(storageFile)

	rel, ok := bridge2.GetTicketFeatureRelation("DEN-807")
	if !ok {
		t.Fatal("expected DEN-807 to be loaded from storage file")
	}
	if rel.FeatureName != "Persistence Test Feature" {
		t.Errorf("unexpected feature name: %s", rel.FeatureName)
	}
}
