package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTools_Perf(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_bench_run mode "stress"
	stressReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_bench_run", "arguments": {"mode": "stress", "virtual_users": 2}}}`
	stressResp := srv.HandleRequest(context.Background(), []byte(stressReq))
	if stressResp == nil || stressResp.Error != nil {
		t.Fatalf("bench_run stress failed: %v", stressResp)
	}

	// 2. dendrite_bench_run mode "inference" with httptest server
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"response"}}]}`))
	}))
	defer mockLLM.Close()

	for _, fault := range []string{"none", "rate-limit-429", "node-glitch-503"} {
		infReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_bench_run", "arguments": {"mode": "inference", "endpoint": %q, "virtual_users": 1, "duration_seconds": 1, "fault_injection": %q}}}`, mockLLM.URL, fault)
		infResp := srv.HandleRequest(context.Background(), []byte(infReq))
		if infResp == nil || infResp.Error != nil {
			t.Fatalf("bench_run inference failed for fault %s: %v", fault, infResp)
		}
	}
}

func TestTools_REA(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	for _, format := range []string{"json", "markdown", "summary", "default"} {
		req := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_rea_showcase", "arguments": {"target": "all", "format": %q}}}`, format)
		resp := srv.HandleRequest(context.Background(), []byte(req))
		if resp == nil || resp.Error != nil {
			t.Fatalf("rea_showcase format %s failed: %v", format, resp)
		}
	}
}

func TestTools_Docusaurus(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	tmpDir := t.TempDir()
	docsDir := filepath.Join(tmpDir, "docs")
	_ = os.MkdirAll(docsDir, 0755)
	_ = os.WriteFile(filepath.Join(docsDir, "intro.md"), []byte("# Intro\nWelcome\n"), 0644)
	siteDir := filepath.Join(tmpDir, "website")

	// 1. dendrite_docusaurus_generate
	genReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_docusaurus_generate", "arguments": {"docs_dir": %q, "target_dir": %q, "title": "My Site", "port": 3000, "sharing_tier": "federated"}}}`, docsDir, siteDir)
	genResp := srv.HandleRequest(context.Background(), []byte(genReq))
	if genResp == nil || genResp.Error != nil {
		t.Fatalf("docusaurus_generate failed: %v", genResp)
	}

	// 2. dendrite_docusaurus_share
	bundlePath := filepath.Join(tmpDir, "bundle.tar.gz")
	shareReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_docusaurus_share", "arguments": {"site_dir": %q, "bundle_path": %q}}}`, siteDir, bundlePath)
	shareResp := srv.HandleRequest(context.Background(), []byte(shareReq))
	if shareResp == nil || shareResp.Error != nil {
		t.Fatalf("docusaurus_share failed: %v", shareResp)
	}

	// 3. dendrite_docusaurus_audit
	auditReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_docusaurus_audit", "arguments": {"project_root": %q, "docs_dir": %q}}}`, tmpDir, docsDir)
	auditResp := srv.HandleRequest(context.Background(), []byte(auditReq))
	if auditResp == nil || auditResp.Error != nil {
		t.Fatalf("docusaurus_audit failed: %v", auditResp)
	}
}

func TestTools_Triage(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{OllamaURL: "http://127.0.0.1:11434"})

	// 1. Missing prompt (error)
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_local_agent_triage", "arguments": {}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if res, ok := resp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing prompt in triage, got: %#v", resp1)
	}

	// 2. Valid triage across force_tier options and engines
	tiers := []string{"local", "hybrid", "cloud"}
	engines := []string{"ollama", "ollama-validated", "huggingface", "candle", "oxillama"}

	for i, tier := range tiers {
		eng := engines[i%len(engines)]
		req := fmt.Sprintf(`{"jsonrpc": "2.0", "id": %d, "method": "tools/call", "params": {"name": "dendrite_local_agent_triage", "arguments": {"prompt": "Implement a binary search tree", "active_file": "bst.go", "file_content": "package bst", "force_tier": %q, "target_engine": %q, "execute_local": true}}}`, 10+i, tier, eng)
		resp := srv.HandleRequest(context.Background(), []byte(req))
		if resp == nil || resp.Error != nil {
			t.Fatalf("triage failed for tier %s engine %s: %v", tier, eng, resp)
		}
	}
}

func TestTools_AgentSearch(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// 1. Missing query (error)
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_agent_search", "arguments": {}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if res, ok := resp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing query in agent_search, got: %#v", resp1)
	}

	// 2. Search mode (route = false)
	req2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_agent_search", "arguments": {"query": "refactor mcp server", "target_package": "pkg/mcpserver", "target_symbols": ["NewServer"], "require_local": true, "max_cost": 0.05, "max_results": 3, "route": false}}}`
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("agent_search search mode failed: %v", resp2)
	}

	// 3. Route mode (route = true)
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_agent_search", "arguments": {"query": "refactor mcp server", "route": true}}}`
	resp3 := srv.HandleRequest(context.Background(), []byte(req3))
	if resp3 == nil || resp3.Error != nil {
		t.Fatalf("agent_search route mode failed: %v", resp3)
	}
}

func TestTools_TestGen(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// 1. Missing file parameter (error)
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_test_auto", "arguments": {}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if res, ok := resp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing file in test_auto, got: %#v", resp1)
	}

	// 2. Valid Go file
	tmpDir := t.TempDir()
	sourceFile := filepath.Join(tmpDir, "calculator.go")
	code := "package calc\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	if err := os.WriteFile(sourceFile, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_test_auto", "arguments": {"file": %q, "with_testcontainers": false, "max_iterations": 1}}}`, sourceFile)
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil {
		t.Fatal("expected response for test_auto")
	}
}
