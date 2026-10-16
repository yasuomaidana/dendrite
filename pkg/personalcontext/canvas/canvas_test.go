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

func TestRegistry_All15WritingActions(t *testing.T) {
	ctx := context.Background()
	reg := NewDefaultRegistry(nil)

	actions := reg.ListActions()
	if len(actions) < 19 {
		t.Fatalf("expected at least 19 actions registered, got %d", len(actions))
	}

	writingActions := []ActionID{
		ActionWriteEmail, ActionWriteStory, ActionWriteReview,
		ActionWriteSpeech, ActionWriteEssay, ActionWriteOutline,
		ActionWriteFeedback, ActionWriteReport, ActionWriteLessonPlan,
		ActionWriteCoverLetter, ActionWriteResume, ActionWriteScript,
		ActionWritePoem, ActionWritePlan, ActionWritePresentation,
	}

	for _, act := range writingActions {
		t.Run(string(act), func(t *testing.T) {
			h, ok := reg.Get(act)
			if !ok {
				t.Fatalf("action %s not registered", act)
			}
			if h.ActionID() != act {
				t.Errorf("mismatched ActionID: %s != %s", h.ActionID(), act)
			}

			req := ActionRequest{
				Action: act,
				Mode:   ModeDraft,
				Title:  "Test Item",
				Prompt: "Design an interactive system",
			}
			resp, err := reg.Dispatch(ctx, req)
			if err != nil {
				t.Fatalf("dispatch failed for %s: %v", act, err)
			}
			if resp.Status != "success" {
				t.Errorf("expected success status, got %s", resp.Status)
			}
			if resp.Content == "" {
				t.Errorf("expected non-empty content for %s", act)
			}
			if len(resp.Notes) == 0 {
				t.Errorf("expected notes for %s", act)
			}
		})
	}
}

func TestRegistry_WritingModes(t *testing.T) {
	ctx := context.Background()
	reg := NewDefaultRegistry(nil)

	modes := []WritingMode{
		ModeDraft, ModeRefine, ModeExpand, ModeCondense,
		ModeTone, ModeCritique, ModeFormat, WritingMode("UNKNOWN_MODE"),
	}

	for _, m := range modes {
		t.Run(string(m), func(t *testing.T) {
			req := ActionRequest{
				Action:         ActionWriteReport,
				Mode:           m,
				Title:          "Telemetry",
				Prompt:         "Analyze system throughput",
				CurrentContent: "Initial baseline telemetry data.",
				Parameters: map[string]interface{}{
					"tone": "academic",
				},
			}
			resp, err := reg.Dispatch(ctx, req)
			if err != nil {
				t.Fatalf("dispatch mode %s failed: %v", m, err)
			}
			if resp.Content == "" {
				t.Errorf("expected non-empty content for mode %s", m)
			}
			if (m == ModeRefine || m == ModeExpand) && resp.Diff == "" {
				t.Errorf("expected non-empty diff for mode %s", m)
			}
		})
	}
}

func TestRegistry_ErrorsAndValidation(t *testing.T) {
	ctx := context.Background()
	reg := NewDefaultRegistry(nil)

	// Missing action
	_, err := reg.Dispatch(ctx, ActionRequest{})
	if err == nil || !strings.Contains(err.Error(), "action is required") {
		t.Errorf("expected ErrMissingAction, got: %v", err)
	}

	// Unknown action
	_, err = reg.Dispatch(ctx, ActionRequest{Action: ActionID("NON_EXISTENT")})
	if err == nil || !strings.Contains(err.Error(), "unknown canvas action") {
		t.Errorf("expected ErrUnknownAction, got: %v", err)
	}

	// Missing prompt
	_, err = reg.Dispatch(ctx, ActionRequest{Action: ActionWriteEmail})
	if err == nil || !strings.Contains(err.Error(), "prompt is required") {
		t.Errorf("expected ErrMissingPrompt, got: %v", err)
	}

	// Nil bridge UI actions
	_, err = reg.Dispatch(ctx, ActionRequest{Action: ActionSyncPenpot, Prompt: "foo"})
	if err == nil || !strings.Contains(err.Error(), "penpot ui bridge is not configured") {
		t.Errorf("expected ErrNilBridge, got: %v", err)
	}

	_, err = reg.Dispatch(ctx, ActionRequest{Action: ActionRelateTicket, Prompt: "foo"})
	if err == nil || !strings.Contains(err.Error(), "penpot ui bridge is not configured") {
		t.Errorf("expected ErrNilBridge, got: %v", err)
	}

	_, err = reg.Dispatch(ctx, ActionRequest{Action: ActionUpdateState, Prompt: "foo"})
	if err == nil || !strings.Contains(err.Error(), "penpot ui bridge is not configured") {
		t.Errorf("expected ErrNilBridge, got: %v", err)
	}
}

func TestActionDesignUI(t *testing.T) {
	ctx := context.Background()
	reg := NewDefaultRegistry(nil)

	t.Run("Svelte Framework", func(t *testing.T) {
		resp, err := reg.Dispatch(ctx, ActionRequest{
			Action: ActionDesignUI,
			Prompt: "Navigation header with responsive drawer",
			Parameters: map[string]interface{}{
				"framework":      "svelte",
				"component_name": "NavBar",
			},
		})
		if err != nil {
			t.Fatalf("design ui svelte failed: %v", err)
		}
		if !strings.Contains(resp.Content, "<script lang=\"ts\">") {
			t.Errorf("expected svelte script tag in content: %s", resp.Content)
		}
		if !strings.Contains(resp.Content, "export let title") {
			t.Errorf("expected svelte export let in content: %s", resp.Content)
		}
	})

	t.Run("React Framework", func(t *testing.T) {
		resp, err := reg.Dispatch(ctx, ActionRequest{
			Action: ActionDesignUI,
			Prompt: "Metric Card widget",
			Parameters: map[string]interface{}{
				"framework":      "react",
				"component_name": "MetricCard",
			},
		})
		if err != nil {
			t.Fatalf("design ui react failed: %v", err)
		}
		if !strings.Contains(resp.Content, "import React from 'react';") {
			t.Errorf("expected react import: %s", resp.Content)
		}
		if !strings.Contains(resp.Content, "interface MetricCardProps") {
			t.Errorf("expected react props interface: %s", resp.Content)
		}
	})

	t.Run("Default Framework Fallback", func(t *testing.T) {
		resp, err := reg.Dispatch(ctx, ActionRequest{
			Action: ActionDesignUI,
			Prompt: "Basic box layout",
			Parameters: map[string]interface{}{
				"framework": "html",
			},
		})
		if err != nil {
			t.Fatalf("design ui html failed: %v", err)
		}
		if !strings.Contains(resp.Content, "<div class=") {
			t.Errorf("expected html div: %s", resp.Content)
		}
	})

	t.Run("Missing Prompt Error", func(t *testing.T) {
		_, err := reg.Dispatch(ctx, ActionRequest{Action: ActionDesignUI})
		if err == nil {
			t.Errorf("expected error on missing prompt")
		}
	})
}

func TestPenpotUIBridge_RelateAndList(t *testing.T) {
	ctx := context.Background()

	// Mock Plane Server
	planeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/comments/") {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "cmt-1"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer planeServer.Close()

	planeClient, err := plane.NewClient(plane.Config{
		BaseURL:       planeServer.URL,
		WorkspaceSlug: "dendrite-ws",
		ProjectID:     "proj-test-1",
		DirectToken:   "plane_token",
	}, nil)
	if err != nil {
		t.Fatalf("failed to init plane client: %v", err)
	}

	bridge := NewPenpotUIBridge(planeClient, "http://localhost:9000")
	if bridge == nil {
		t.Fatal("expected non-nil bridge")
	}

	// Test missing ticket ID
	err = bridge.RelateTicketToFeature(ctx, TicketFeatureRelation{FeatureName: "Foo"})
	if err != ErrMissingTicketID {
		t.Errorf("expected ErrMissingTicketID, got: %v", err)
	}

	// Test missing feature name
	err = bridge.RelateTicketToFeature(ctx, TicketFeatureRelation{TicketID: "DEN-807"})
	if err != ErrMissingFeatureName {
		t.Errorf("expected ErrMissingFeatureName, got: %v", err)
	}

	// Relate valid ticket
	err = bridge.RelateTicketToFeature(ctx, TicketFeatureRelation{
		TicketID:      "DEN-807",
		FeatureName:   "Canvas Interactive Writing",
		Description:   "Interactive Writing & Direct Creative Action Handlers",
		ComponentPath: "web/src/Canvas.svelte",
	})
	if err != nil {
		t.Fatalf("relate ticket failed: %v", err)
	}

	// Fetch relation
	rel, ok := bridge.GetTicketFeatureRelation("DEN-807")
	if !ok {
		t.Fatal("expected to find DEN-807 relation")
	}
	if rel.FeatureName != "Canvas Interactive Writing" {
		t.Errorf("unexpected feature name: %s", rel.FeatureName)
	}
	if !strings.Contains(rel.PenpotFrameURL, "canvas-interactive-writing") {
		t.Errorf("unexpected penpot frame URL: %s", rel.PenpotFrameURL)
	}

	// List relations
	list := bridge.ListTicketRelations()
	if len(list) != 1 {
		t.Errorf("expected 1 relation in list, got %d", len(list))
	}

	// Non-existent relation
	_, ok = bridge.GetTicketFeatureRelation("NON-EXISTENT")
	if ok {
		t.Errorf("expected false for NON-EXISTENT relation")
	}
}

func TestPenpotUIBridge_SyncComponentWithPenpot(t *testing.T) {
	ctx := context.Background()

	// Mock Penpot server
	penpotServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer penpotServer.Close()

	// Mock Plane Server
	planeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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

	bridge := NewPenpotUIBridge(planeClient, penpotServer.URL)

	// Create temp Svelte file
	tmpDir := t.TempDir()
	uiFile := filepath.Join(tmpDir, "CanvasButton.svelte")
	content := `<script lang="ts">
  export let label: string = "Submit";
</script>
<button class="bg-indigo-600 hover:bg-indigo-500 text-white p-4 rounded-lg shadow-md flex items-center">
  {label}
</button>`
	if err := os.WriteFile(uiFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write tmp ui file: %v", err)
	}

	// Missing file path error
	_, err := bridge.SyncComponentWithPenpot(ctx, "DEN-807", "", false)
	if err != ErrMissingFilePath {
		t.Errorf("expected ErrMissingFilePath, got: %v", err)
	}

	// Non-existent file error
	_, err = bridge.SyncComponentWithPenpot(ctx, "DEN-807", filepath.Join(tmpDir, "missing.svelte"), false)
	if err == nil {
		t.Errorf("expected error for non-existent file")
	}

	// Sync component successfully
	res, err := bridge.SyncComponentWithPenpot(ctx, "DEN-807", uiFile, false)
	if err != nil {
		t.Fatalf("sync component failed: %v", err)
	}

	if res.FeatureName != "CanvasButton" {
		t.Errorf("unexpected component name: %s", res.FeatureName)
	}
	if !res.PlaneUpdated {
		t.Errorf("expected plane to be updated")
	}
	if len(res.Tokens) == 0 {
		t.Errorf("expected extracted tokens, got empty")
	}

	// Verify relation was created / updated
	rel, ok := bridge.GetTicketFeatureRelation("DEN-807")
	if !ok {
		t.Fatal("expected relation to be created for DEN-807")
	}
	if rel.ComponentPath != uiFile {
		t.Errorf("mismatched component path: %s", rel.ComponentPath)
	}

	// Dry-run sync
	resDry, err := bridge.SyncComponentWithPenpot(ctx, "DEN-807", uiFile, true)
	if err != nil {
		t.Fatalf("dry run sync failed: %v", err)
	}
	if resDry.PlaneUpdated {
		t.Errorf("expected plane_updated to be false on dry run")
	}
}

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
