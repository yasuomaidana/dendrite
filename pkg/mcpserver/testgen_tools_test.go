package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDendriteTestAutoMCPTool(t *testing.T) {
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	srv.RegisterDefaultTools(Dependencies{
		DevStatus: "DEV_STATUS.md",
		OllamaURL: "http://127.0.0.1:11434",
	})

	ctx := context.Background()

	// Create a sample source file in a temp dir
	tmpDir := t.TempDir()
	sampleSrc := filepath.Join(tmpDir, "calculator.go")
	code := `package calc

func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(sampleSrc, []byte(code), 0644); err != nil {
		t.Fatalf("failed to write sample source: %v", err)
	}

	// 1. Test test synthesis on the source file
	reqJSON := fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": 301,
		"method": "tools/call",
		"params": {
			"name": "dendrite_test_auto",
			"arguments": {
				"file": %q
			}
		}
	}`, sampleSrc)

	resp := srv.HandleRequest(ctx, []byte(reqJSON))
	if resp.Error != nil {
		t.Fatalf("unexpected error from dendrite_test_auto: %v", resp.Error)
	}

	var contentText string
	switch r := resp.Result.(type) {
	case *CallToolResult:
		if len(r.Content) > 0 {
			contentText = r.Content[0].Text
		}
	case CallToolResult:
		if len(r.Content) > 0 {
			contentText = r.Content[0].Text
		}
	default:
		t.Fatalf("unexpected result type: %T", resp.Result)
	}

	if contentText == "" {
		t.Fatalf("expected non-empty content text")
	}

	if strings.HasPrefix(contentText, "Tool error:") {
		t.Fatalf("Tool error: %s", contentText)
	}

	var report map[string]interface{}
	if err := json.Unmarshal([]byte(contentText), &report); err != nil {
		t.Fatalf("failed to parse report: %v", err)
	}

	if report["state"] != "Verified" {
		t.Errorf("expected state Verified, got: %v", report["state"])
	}

	// 2. Test empty file error
	badReq := `{
		"jsonrpc": "2.0",
		"id": 302,
		"method": "tools/call",
		"params": {
			"name": "dendrite_test_auto",
			"arguments": {
				"file": ""
			}
		}
	}`
	badResp := srv.HandleRequest(ctx, []byte(badReq))
	badRes, isCallToolRes := badResp.Result.(CallToolResult)
	if !isCallToolRes || !badRes.IsError {
		t.Errorf("expected IsError on empty file")
	}
	if !strings.Contains(badRes.Content[0].Text, "file parameter is required") {
		t.Errorf("unexpected error message: %s", badRes.Content[0].Text)
	}
}
