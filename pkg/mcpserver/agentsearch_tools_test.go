package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDendriteAgentSearchMCPTool(t *testing.T) {
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	srv.RegisterDefaultTools(Dependencies{
		DevStatus: "DEV_STATUS.md",
		OllamaURL: "http://127.0.0.1:11434",
	})

	ctx := context.Background()

	// 1. Test search query
	reqJSON := `{
		"jsonrpc": "2.0",
		"id": 201,
		"method": "tools/call",
		"params": {
			"name": "dendrite_agent_search",
			"arguments": {
				"query": "refactor gateway limiter",
				"target_package": "pkg/gateway",
				"max_results": 2
			}
		}
	}`

	resp := srv.HandleRequest(ctx, []byte(reqJSON))
	if resp.Error != nil {
		t.Fatalf("unexpected error from dendrite_agent_search: %v", resp.Error)
	}

	resultMap, ok := resp.Result.(*CallToolResult)
	if !ok {
		t.Fatalf("expected *CallToolResult, got %T", resp.Result)
	}
	if len(resultMap.Content) == 0 {
		t.Fatalf("expected non-empty content list")
	}

	text := resultMap.Content[0].Text
	if !strings.Contains(text, "aider") && !strings.Contains(text, "openhands") && !strings.Contains(text, "agy") {
		t.Errorf("expected candidate agent in search results, got: %s", text)
	}

	// 2. Test route mode
	routeReq := `{
		"jsonrpc": "2.0",
		"id": 202,
		"method": "tools/call",
		"params": {
			"name": "dendrite_agent_search",
			"arguments": {
				"query": "write rapid unit test for AST",
				"require_local": true,
				"route": true
			}
		}
	}`

	routeResp := srv.HandleRequest(ctx, []byte(routeReq))
	if routeResp.Error != nil {
		t.Fatalf("unexpected error from route: %v", routeResp.Error)
	}

	rMap := routeResp.Result.(*CallToolResult)
	var decision map[string]interface{}
	if err := json.Unmarshal([]byte(rMap.Content[0].Text), &decision); err != nil {
		t.Fatalf("failed to unmarshal routing decision: %v", err)
	}

	agentObj, ok := decision["selected_agent"].(map[string]interface{})
	if !ok || agentObj["id"] == "" {
		t.Fatalf("expected valid selected_agent, got: %v", decision)
	}

	// 3. Test empty query error
	badReq := `{
		"jsonrpc": "2.0",
		"id": 203,
		"method": "tools/call",
		"params": {
			"name": "dendrite_agent_search",
			"arguments": {
				"query": ""
			}
		}
	}`
	badResp := srv.HandleRequest(ctx, []byte(badReq))
	badResult, isCallToolRes := badResp.Result.(CallToolResult)
	if !isCallToolRes || !badResult.IsError {
		t.Fatalf("expected IsError=true on empty query, got: %+v", badResp.Result)
	}
}
