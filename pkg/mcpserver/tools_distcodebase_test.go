package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestTools_DistCodebase(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_dist_sources: query matching
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_dist_sources", "arguments": {"query": "wgpu", "language": "rust"}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("dist_sources matching query failed: %v", resp1)
	}

	// 2. dendrite_dist_sources: filters (priority & language)
	req2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_dist_sources", "arguments": {"priority": 1, "language": "rust"}}}`
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("dist_sources with filters failed: %v", resp2)
	}

	// 3. dendrite_dist_decompose: valid
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_dist_decompose", "arguments": {"project_name": "carla-rust", "modules": ["engine", "physics"], "user_custom_overrides": ["physics"]}}}`
	resp3 := srv.HandleRequest(context.Background(), []byte(req3))
	if resp3 == nil || resp3.Error != nil {
		t.Fatalf("dist_decompose failed: %v", resp3)
	}

	// 4. dendrite_dist_decompose: missing project_name (error)
	req3b := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_dist_decompose", "arguments": {}}}`
	resp3b := srv.HandleRequest(context.Background(), []byte(req3b))
	if res, ok := resp3b.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for decompose with missing project_name, got: %#v", resp3b)
	}

	// 5. dendrite_dist_interop: action "coordinate"
	coordReq := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "coordinate", "requester_project": "docula", "dependency_project": "oxipix", "feature_requested": "svg_render", "allow_fallback": true}}}`
	coordResp := srv.HandleRequest(context.Background(), []byte(coordReq))
	if coordResp == nil || coordResp.Error != nil {
		t.Fatalf("interop coordinate failed: %v", coordResp)
	}

	// 6. dendrite_dist_interop: action "list"
	listReq := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "list"}}}`
	listResp := srv.HandleRequest(context.Background(), []byte(listReq))
	if listResp == nil || listResp.Error != nil {
		t.Fatalf("interop list failed: %v", listResp)
	}

	// 7. dendrite_dist_interop: action "complete" & "reconcile" with invalid coordination_id (error)
	compReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "complete", "coordination_id": "non-existent"}}}`
	compResp := srv.HandleRequest(context.Background(), []byte(compReq))
	if res, ok := compResp.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for complete non-existent coordination, got: %#v", compResp)
	}

	recReq := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "reconcile", "coordination_id": "non-existent"}}}`
	recResp := srv.HandleRequest(context.Background(), []byte(recReq))
	if res, ok := recResp.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for reconcile non-existent coordination, got: %#v", recResp)
	}

	// 8. dendrite_dist_interop: unknown action (error)
	unknownReq := `{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "unknown_action"}}}`
	unknownResp := srv.HandleRequest(context.Background(), []byte(unknownReq))
	if res, ok := unknownResp.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for unknown interop action, got: %#v", unknownResp)
	}

	// 9. dendrite_dist_spawn: valid spawn
	spawnDir := filepath.Join(t.TempDir(), "subproj")
	spawnReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 10, "method": "tools/call", "params": {"name": "dendrite_dist_spawn", "arguments": {"project_name": "subproj", "identifier": "SUBP", "target_dir": %q, "language": "go", "is_satellite": true, "create_plane_project": false, "init_git": true, "description": "Sub project"}}}`, spawnDir)
	spawnResp := srv.HandleRequest(context.Background(), []byte(spawnReq))
	if spawnResp == nil || spawnResp.Error != nil {
		t.Fatalf("dist_spawn failed: %v", spawnResp)
	}
}
