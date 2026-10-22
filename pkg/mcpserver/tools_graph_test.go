package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func testGraphProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "package demo\n\nfunc LocateBinary(name string) string { return name }\n"
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "demo", "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func invokeTool(ctx context.Context, srv *Server, name string, args map[string]interface{}) (*CallToolResult, error) {
	reqMap := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}
	raw, err := json.Marshal(reqMap)
	if err != nil {
		return nil, err
	}
	resp := srv.HandleRequest(ctx, raw)
	if resp == nil {
		return nil, fmt.Errorf("no response")
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	callRes, ok := resp.Result.(*CallToolResult)
	if !ok {
		return nil, fmt.Errorf("expected *CallToolResult, got %T", resp.Result)
	}
	return callRes, nil
}

func TestGraphTools(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	dir := testGraphProject(t)

	ctx := context.Background()

	// 1. dendrite_graph_communities
	res, err := invokeTool(ctx, srv, "dendrite_graph_communities", map[string]interface{}{
		"project_path": dir,
		"limit":        10,
	})
	if err != nil {
		t.Fatalf("communities error: %v", err)
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Errorf("empty communities response")
	}

	// 2. dendrite_graph_god_nodes
	res, err = invokeTool(ctx, srv, "dendrite_graph_god_nodes", map[string]interface{}{
		"project_path": dir,
		"limit":        10,
	})
	if err != nil {
		t.Fatalf("god nodes error: %v", err)
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Errorf("empty god nodes response")
	}

	// 3. dendrite_graph_path
	res, err = invokeTool(ctx, srv, "dendrite_graph_path", map[string]interface{}{
		"project_path": dir,
		"from":         "Run",
		"to":           "LocateBinary",
		"kinds":        []interface{}{"call"},
	})
	if err != nil {
		t.Fatalf("path error: %v", err)
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Errorf("empty path response")
	}

	// 3b. dendrite_graph_path missing required args
	_, err = invokeTool(ctx, srv, "dendrite_graph_path", map[string]interface{}{
		"project_path": dir,
		"from":         "",
	})
	if err == nil {
		t.Errorf("expected error for missing 'to' in path")
	}

	// 4. dendrite_graph_explain
	res, err = invokeTool(ctx, srv, "dendrite_graph_explain", map[string]interface{}{
		"project_path": dir,
		"target":       "LocateBinary",
	})
	if err != nil {
		t.Fatalf("explain error: %v", err)
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Errorf("empty explain response")
	}

	// 4b. dendrite_graph_explain missing target
	_, err = invokeTool(ctx, srv, "dendrite_graph_explain", map[string]interface{}{
		"project_path": dir,
		"target":       "",
	})
	if err == nil {
		t.Errorf("expected error for empty target in explain")
	}
}
