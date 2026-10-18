package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/env/bootstrap"
)

func TestMCP_EnvPrepareTools(t *testing.T) {
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
	})
	ctx := context.Background()

	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module testmcp\n\ngo 1.24\n"), 0644)

	t.Run("dry run prepare via MCP", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 201,
			"method": "tools/call",
			"params": {
				"name": "dendrite_env_prepare",
				"arguments": {
					"project_path": "` + tmpDir + `",
					"default_tool": "copilot",
					"dry_run": true
				}
			}
		}`

		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("HandleRequest failed: %v", resp.Error)
		}

		res, ok := resp.Result.(*CallToolResult)
		if !ok || len(res.Content) == 0 {
			t.Fatalf("expected *CallToolResult with content")
		}

		var rep bootstrap.PrepareReport
		if err := json.Unmarshal([]byte(res.Content[0].Text), &rep); err != nil {
			t.Fatalf("failed to unmarshal output: %v", err)
		}

		if !rep.Success {
			t.Errorf("expected success true")
		}
		if rep.Stack.PrimaryStack != bootstrap.StackGo {
			t.Errorf("expected StackGo, got %s", rep.Stack.PrimaryStack)
		}
		if rep.CICD.DefaultTool != "copilot" {
			t.Errorf("expected copilot default, got %s", rep.CICD.DefaultTool)
		}
	})

	t.Run("real prepare via MCP", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 202,
			"method": "tools/call",
			"params": {
				"name": "dendrite_env_prepare",
				"arguments": {
					"project_path": "` + tmpDir + `",
					"default_tool": "copilot",
					"dry_run": false
				}
			}
		}`

		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("HandleRequest failed: %v", resp.Error)
		}

		res, ok := resp.Result.(*CallToolResult)
		if !ok || len(res.Content) == 0 {
			t.Fatalf("expected *CallToolResult with content")
		}

		// Verify files written
		if _, err := os.Stat(filepath.Join(tmpDir, ".github", "copilot-instructions.md")); err != nil {
			t.Errorf("expected copilot-instructions.md to exist: %v", err)
		}
	})
}
