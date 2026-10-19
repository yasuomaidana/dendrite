package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMCP_MigrateTools(t *testing.T) {
	srv := NewServer(ServerConfig{
		Name:    "test-dendrite",
		Version: "0.1.0",
	})
	RegisterDefaultTools(srv, Dependencies{})

	ctx := context.Background()

	// 1. Test dendrite_service_plan
	planReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_service_plan", "arguments": {"ram_mb": 4096, "disk_gb": 1000}}}`
	respPlan := srv.HandleRequest(ctx, []byte(planReq))
	if respPlan == nil || respPlan.Error != nil {
		t.Fatalf("unexpected error calling dendrite_service_plan: %v", respPlan)
	}

	callRes, ok := respPlan.Result.(*CallToolResult)
	if !ok || len(callRes.Content) == 0 {
		t.Fatalf("expected valid CallToolResult with content")
	}

	var planData map[string]interface{}
	if err := json.Unmarshal([]byte(callRes.Content[0].Text), &planData); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if recCmd, ok := planData["recommended_action"].(string); !ok || !strings.Contains(recCmd, "dendrite translocate run") {
		t.Errorf("expected recommended action with dendrite move, got %v", recCmd)
	}

	// 2. Test dendrite_service_move in dry-run mode
	moveReq := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {"services": ["plane"], "target_host": "user@debian-server", "dry_run": true}}}`
	respMove := srv.HandleRequest(ctx, []byte(moveReq))
	if respMove == nil || respMove.Error != nil {
		t.Fatalf("unexpected error calling dendrite_service_move: %v", respMove)
	}

	callMoveRes, ok := respMove.Result.(*CallToolResult)
	if !ok || len(callMoveRes.Content) == 0 {
		t.Fatalf("expected valid CallToolResult for move")
	}

	var moveData map[string]interface{}
	if err := json.Unmarshal([]byte(callMoveRes.Content[0].Text), &moveData); err != nil {
		t.Fatalf("failed to parse json move response: %v", err)
	}
	if success, ok := moveData["success"].(bool); !ok || !success {
		t.Errorf("expected dry-run move to succeed, got %v", moveData)
	}
}
