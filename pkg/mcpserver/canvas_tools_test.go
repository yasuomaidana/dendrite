package mcpserver

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

func TestMCP_CanvasTools(t *testing.T) {
	ctx := context.Background()

	// Mock Plane Server
	planeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/states/") {
			states := []plane.PlaneState{
				{ID: "state-progress", Name: "In Progress", Group: "started"},
				{ID: "state-verify", Name: "To Verify", Group: "started"},
			}
			_ = json.NewEncoder(w).Encode(states)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/issues/") {
			issues := []plane.PlaneIssue{
				{ID: "issue-807", Name: "[DEN-807] Canvas Interactive Writing"},
			}
			_ = json.NewEncoder(w).Encode(issues)
			return
		}
		if strings.Contains(r.URL.Path, "/comments/") {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "cmt-1"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "ok"})
	}))
	defer planeServer.Close()

	planeClient, _ := plane.NewClient(plane.Config{
		BaseURL:       planeServer.URL,
		WorkspaceSlug: "dendrite-ws",
		ProjectID:     "proj-test-1",
		DirectToken:   "plane_token",
	}, nil)

	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	deps := Dependencies{
		PlaneClient: planeClient,
	}
	registerCanvasTools(srv, deps)

	// 1. Test dendrite_canvas_action_dispatch (WRITE_PLAN)
	t.Run("Dispatch_WritePlan", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_action_dispatch",
				"arguments": {
					"action": "WRITE_PLAN",
					"title": "UI Architecture Rollout",
					"prompt": "Deliver Penpot Bridge and Plane state synchronization",
					"mode": "DRAFT"
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call dendrite_canvas_action_dispatch failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "UI Architecture Rollout") {
			t.Errorf("expected plan output, got: %v", resp.Result)
		}
	})

	// 2. Test dendrite_canvas_action_dispatch (DESIGN_UI_COMPONENT)
	t.Run("Dispatch_DesignUI", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_action_dispatch",
				"arguments": {
					"action": "DESIGN_UI_COMPONENT",
					"prompt": "Interactive Canvas Workspace with side drawer",
					"parameters": {
						"framework": "svelte",
						"component_name": "WorkspaceCanvas"
					}
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call design UI failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "WorkspaceCanvas") {
			t.Errorf("expected WorkspaceCanvas in output: %v", resp.Result)
		}
	})

	// 3. Test dendrite_canvas_penpot_bridge (relate)
	t.Run("PenpotBridge_Relate", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 3,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_penpot_bridge",
				"arguments": {
					"subaction": "relate",
					"ticket_id": "DEN-807",
					"feature_name": "Canvas Workspace",
					"comment": "Binding Plane ticket to Penpot design frame"
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call relate failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "DEN-807") {
			t.Errorf("expected DEN-807 in output: %v", resp.Result)
		}
	})

	// 4. Test dendrite_canvas_penpot_bridge (sync)
	t.Run("PenpotBridge_Sync", func(t *testing.T) {
		tmpDir := t.TempDir()
		uiPath := filepath.Join(tmpDir, "Header.svelte")
		_ = os.WriteFile(uiPath, []byte(`<header class="bg-slate-900 p-4 text-white">Header</header>`), 0644)

		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 4,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_penpot_bridge",
				"arguments": {
					"subaction": "sync",
					"ticket_id": "DEN-807",
					"file_path": "` + uiPath + `",
					"dry_run": true
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call sync failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "Header") {
			t.Errorf("expected Header in sync output: %v", resp.Result)
		}
	})

	// 5. Test dendrite_canvas_penpot_bridge (update_state)
	t.Run("PenpotBridge_UpdateState", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 5,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_penpot_bridge",
				"arguments": {
					"subaction": "update_state",
					"ticket_id": "DEN-807",
					"target_state": "To Verify",
					"comment": "Verification completed in Penpot frame",
					"push_to_plane": true
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call update_state failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "To Verify") {
			t.Errorf("expected To Verify in output: %v", resp.Result)
		}
	})

	// 6. Test dendrite_canvas_penpot_bridge (list)
	t.Run("PenpotBridge_List", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 6,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_penpot_bridge",
				"arguments": {
					"subaction": "list"
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call list failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "DEN-807") {
			t.Errorf("expected list to contain DEN-807: %v", resp.Result)
		}
	})
}
