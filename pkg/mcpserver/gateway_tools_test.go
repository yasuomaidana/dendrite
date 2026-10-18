package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDendriteGatewayUpdate_Tool(t *testing.T) {
	srv := NewServer(ServerConfig{
		Name:    "Test MCP Server",
		Version: "1.0.0",
	})
	RegisterDefaultTools(srv, Dependencies{})

	callReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_gateway_update", "arguments": {"dry_run": true, "file": "/tmp/litellm_test_mcp.yaml", "use_container_host": false}}}`
	resp := srv.HandleRequest(context.Background(), []byte(callReq))
	if resp == nil || resp.Error != nil {
		t.Fatalf("unexpected error calling dendrite_gateway_update: %v", resp)
	}

	callRes, ok := resp.Result.(*CallToolResult)
	if !ok || len(callRes.Content) == 0 {
		t.Fatalf("expected non-empty CallToolResult, got: %T", resp.Result)
	}

	var report map[string]interface{}
	if err := json.Unmarshal([]byte(callRes.Content[0].Text), &report); err != nil {
		t.Fatalf("failed to decode report json: %v", err)
	}

	if dryRun, ok := report["dry_run"].(bool); !ok || !dryRun {
		t.Errorf("expected dry_run to be true in report, got %v", report["dry_run"])
	}

	if totalModels, ok := report["total_models_configured"].(float64); !ok || totalModels == 0 {
		t.Errorf("expected positive total_models_configured, got %v", report["total_models_configured"])
	}
}
