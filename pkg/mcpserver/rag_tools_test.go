package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDendriteRAGTools(t *testing.T) {
	tmpDir := t.TempDir()

	docFile := filepath.Join(tmpDir, "architecture.md")
	docContent := `# System Architecture
Dendrite connects IDEs and autonomous agents through Model Context Protocol and AST reachability analysis.
`
	if err := os.WriteFile(docFile, []byte(docContent), 0644); err != nil {
		t.Fatalf("failed to write doc: %v", err)
	}

	codeFile := filepath.Join(tmpDir, "pipeline.go")
	codeContent := `package pipeline

// PromptEnhancer provides context enrichment.
type PromptEnhancer struct {
	MaxTokens int
}

// Enhance adds AST chunks to prompt.
func (p *PromptEnhancer) Enhance(prompt string) string {
	return prompt + " [enhanced]"
}
`
	if err := os.WriteFile(codeFile, []byte(codeContent), 0644); err != nil {
		t.Fatalf("failed to write code: %v", err)
	}

	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	RegisterDefaultTools(srv, Dependencies{})

	// 1. Test dendrite_rag_retrieve tool
	retrieveJSON := `{
		"jsonrpc": "2.0",
		"id": 201,
		"method": "tools/call",
		"params": {
			"name": "dendrite_rag_retrieve",
			"arguments": {
				"query": "PromptEnhancer Enhance AST",
				"project_path": "` + tmpDir + `",
				"top_k": 2
			}
		}
	}`

	resp := srv.HandleRequest(context.Background(), []byte(retrieveJSON))
	if resp.Error != nil {
		t.Fatalf("unexpected error from dendrite_rag_retrieve: %v", resp.Error)
	}

	resultMap, ok := resp.Result.(*CallToolResult)
	if !ok || len(resultMap.Content) == 0 {
		t.Fatalf("expected valid CallToolResult with content")
	}

	text := resultMap.Content[0].Text
	if !strings.Contains(text, "PromptEnhancer") && !strings.Contains(text, "Enhance") {
		t.Errorf("expected retrieved content to contain AST symbols, got: %s", text)
	}

	// 2. Error case: missing question on dendrite_rag_ask
	badAskJSON := `{
		"jsonrpc": "2.0",
		"id": 202,
		"method": "tools/call",
		"params": {
			"name": "dendrite_rag_ask",
			"arguments": {}
		}
	}`
	respBad := srv.HandleRequest(context.Background(), []byte(badAskJSON))
	if respBad == nil {
		t.Fatalf("expected non-nil response for bad ask")
	}

	// Handler error wraps as CallToolResult with IsError: true
	resBad, isCallRes := respBad.Result.(CallToolResult)
	if isCallRes {
		if !resBad.IsError {
			t.Errorf("expected IsError=true on missing question")
		}
	} else if respBad.Error == nil {
		t.Errorf("expected error or IsError on missing question")
	}

	// 3. Error case: missing query on dendrite_rag_retrieve
	badRetJSON := `{
		"jsonrpc": "2.0",
		"id": 203,
		"method": "tools/call",
		"params": {
			"name": "dendrite_rag_retrieve",
			"arguments": {}
		}
	}`
	respBadRet := srv.HandleRequest(context.Background(), []byte(badRetJSON))
	if respBadRet == nil {
		t.Fatalf("expected non-nil response for bad retrieve")
	}
	resBadRet, isRetCallRes := respBadRet.Result.(CallToolResult)
	if isRetCallRes {
		if !resBadRet.IsError {
			t.Errorf("expected IsError=true on missing query")
		}
	} else if respBadRet.Error == nil {
		t.Errorf("expected error or IsError on missing query")
	}
}

func TestDendriteRAGMapThenShow(t *testing.T) {
	dir := t.TempDir()
	src := "package demo\n\n// Locate finds the sidecar.\nfunc Locate() string { return \"x\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	RegisterDefaultTools(srv, Dependencies{})
	call := func(name, args string) string {
		t.Helper()
		resp := srv.HandleRequest(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`))
		if resp.Error != nil {
			t.Fatalf("%s: %v", name, resp.Error)
		}
		data, _ := json.Marshal(resp.Result)
		return string(data)
	}
	mapped := call("dendrite_rag_retrieve", `{"query":"Locate sidecar","project_path":"`+dir+`","detail":"map","corpus":"code"}`)
	if !strings.Contains(mapped, `\"outline\": true`) || strings.Contains(mapped, "return") {
		t.Fatalf("map = %s", mapped)
	}
	resp := srv.HandleRequest(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"dendrite_rag_retrieve","arguments":{"query":"Locate","project_path":"`+dir+`","detail":"auto","docs_share":1.5}}}`))
	if data, _ := json.Marshal(resp); resp.Error == nil && !strings.Contains(string(data), "docs_share must be from 0 to 1") {
		t.Fatalf("docs_share 1.5 was accepted: %s", data)
	}
	shown := call("dendrite_rag_show", `{"ids":["demo.go#1"],"project_path":"`+dir+`"}`)
	if !strings.Contains(shown, "return") {
		t.Fatalf("show = %s", shown)
	}
}
