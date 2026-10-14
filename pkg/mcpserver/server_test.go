package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestServer_Initialize(t *testing.T) {
	srv := NewServer(ServerConfig{
		Name:    "Test MCP Server",
		Version: "1.0.0",
	})

	req := `{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}}`
	resp := srv.HandleRequest(context.Background(), []byte(req))

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	initRes, ok := resp.Result.(InitializeResult)
	if !ok {
		t.Fatalf("expected InitializeResult, got: %T", resp.Result)
	}
	if initRes.ServerInfo.Name != "Test MCP Server" {
		t.Errorf("expected name 'Test MCP Server', got: %s", initRes.ServerInfo.Name)
	}
	if initRes.Capabilities.Tools == nil {
		t.Error("expected tools capability to be present")
	}
}

func TestServer_Ping(t *testing.T) {
	srv := NewServer(ServerConfig{})
	req := `{"jsonrpc": "2.0", "id": "p1", "method": "ping"}`
	resp := srv.HandleRequest(context.Background(), []byte(req))

	if resp == nil || resp.Error != nil {
		t.Fatalf("ping failed: %v", resp)
	}
	resMap, ok := resp.Result.(map[string]interface{})
	if !ok || resMap["status"] != "pong" {
		t.Errorf("expected pong, got: %v", resp.Result)
	}
}

func TestServer_ToolsListAndCall(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterTool(Tool{
		Name:        "echo_tool",
		Description: "Echo input message",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		msg, _ := args["msg"].(string)
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: "echo: " + msg}},
		}, nil
	})

	// 1. tools/list
	listReq := `{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}`
	listResp := srv.HandleRequest(context.Background(), []byte(listReq))
	if listResp == nil || listResp.Error != nil {
		t.Fatalf("tools/list failed: %v", listResp)
	}
	toolsRes, ok := listResp.Result.(ListToolsResult)
	if !ok || len(toolsRes.Tools) != 1 || toolsRes.Tools[0].Name != "echo_tool" {
		t.Fatalf("unexpected tools result: %v", listResp.Result)
	}

	// 2. tools/call
	callReq := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "echo_tool", "arguments": {"msg": "hello mcp"}}}`
	callResp := srv.HandleRequest(context.Background(), []byte(callReq))
	if callResp == nil || callResp.Error != nil {
		t.Fatalf("tools/call failed: %v", callResp)
	}
	callRes, ok := callResp.Result.(*CallToolResult)
	if !ok {
		t.Fatalf("expected *CallToolResult, got: %T", callResp.Result)
	}
	if len(callRes.Content) == 0 || callRes.Content[0].Text != "echo: hello mcp" {
		t.Errorf("unexpected content: %v", callRes.Content)
	}
}

func TestServer_UnknownToolAndErrors(t *testing.T) {
	srv := NewServer(ServerConfig{})

	// Unknown tool
	req := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "non_existent"}}`
	resp := srv.HandleRequest(context.Background(), []byte(req))
	if resp == nil || resp.Error == nil || resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("expected ErrCodeMethodNotFound, got: %v", resp)
	}

	// Unknown method
	req2 := `{"jsonrpc": "2.0", "id": 5, "method": "unknown_method"}`
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error == nil || resp2.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("expected method not found error, got: %v", resp2)
	}

	// Bad JSON
	resp3 := srv.HandleRequest(context.Background(), []byte(`{invalid json`))
	if resp3 == nil || resp3.Error == nil || resp3.Error.Code != ErrCodeParseError {
		t.Errorf("expected parse error, got: %v", resp3)
	}
}

func TestServer_ServeStdio(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{DevStatus: "../../DEV_STATUS.md"})

	input := strings.Join([]string{
		`{"jsonrpc": "2.0", "id": 1, "method": "initialize"}`,
		`{"jsonrpc": "2.0", "method": "notifications/initialized"}`,
		`{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}`,
	}, "\n") + "\n"

	in := strings.NewReader(input)
	out := &bytes.Buffer{}

	err := srv.ServeStdio(context.Background(), in, out)
	if err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}

	outputLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(outputLines) != 2 {
		t.Fatalf("expected 2 responses (notification ignored), got %d: %s", len(outputLines), out.String())
	}

	var r1, r2 JSONRPCResponse
	if err := json.Unmarshal([]byte(outputLines[0]), &r1); err != nil {
		t.Fatalf("failed unmarshaling r1: %v", err)
	}
	if err := json.Unmarshal([]byte(outputLines[1]), &r2); err != nil {
		t.Fatalf("failed unmarshaling r2: %v", err)
	}

	if r1.ID != float64(1) || r2.ID != float64(2) {
		t.Errorf("mismatched response IDs: r1=%v, r2=%v", r1.ID, r2.ID)
	}
}

func TestDefaultTools_Execution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{DevStatus: "../../DEV_STATUS.md"})

	tools := srv.ListTools()
	if len(tools) < 8 {
		t.Fatalf("expected at least 8 default tools, got %d", len(tools))
	}

	// 1. dendrite_gateway_render
	callReq := `{"jsonrpc": "2.0", "id": 10, "method": "tools/call", "params": {"name": "dendrite_gateway_render", "arguments": {"scenario": "local-hybrid"}}}`
	resp := srv.HandleRequest(context.Background(), []byte(callReq))
	if resp.Error != nil {
		t.Fatalf("dendrite_gateway_render error: %v", resp.Error)
	}

	// 2. dendrite_agent_run
	agentReq := `{"jsonrpc": "2.0", "id": 11, "method": "tools/call", "params": {"name": "dendrite_agent_run", "arguments": {"agent_type": "aider", "task": "refactor mcp", "ticket_id": "DEN-034"}}}`
	agentResp := srv.HandleRequest(context.Background(), []byte(agentReq))
	if agentResp.Error != nil {
		t.Fatalf("dendrite_agent_run error: %v", agentResp.Error)
	}

	// 3. dendrite_telemetry
	telemReq := `{"jsonrpc": "2.0", "id": 12, "method": "tools/call", "params": {"name": "dendrite_telemetry", "arguments": {}}}`
	telemResp := srv.HandleRequest(context.Background(), []byte(telemReq))
	if telemResp.Error != nil {
		t.Fatalf("dendrite_telemetry error: %v", telemResp.Error)
	}

	// 4. dendrite_inference_list
	infListReq := `{"jsonrpc": "2.0", "id": 13, "method": "tools/call", "params": {"name": "dendrite_inference_list", "arguments": {}}}`
	infListResp := srv.HandleRequest(context.Background(), []byte(infListReq))
	if infListResp.Error != nil {
		t.Fatalf("dendrite_inference_list error: %v", infListResp.Error)
	}

	// 5. dendrite_inference_probe
	infProbeReq := `{"jsonrpc": "2.0", "id": 14, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {"engine_id": "oxillama-rust"}}}`
	infProbeResp := srv.HandleRequest(context.Background(), []byte(infProbeReq))
	if infProbeResp.Error != nil {
		t.Fatalf("dendrite_inference_probe error: %v", infProbeResp.Error)
	}

	// 6. dendrite_jetbrains_setup
	setupReq := `{"jsonrpc": "2.0", "id": 15, "method": "tools/call", "params": {"name": "dendrite_jetbrains_setup", "arguments": {"project_only": true}}}`
	setupResp := srv.HandleRequest(context.Background(), []byte(setupReq))
	if setupResp.Error != nil {
		t.Fatalf("dendrite_jetbrains_setup error: %v", setupResp.Error)
	}
}
