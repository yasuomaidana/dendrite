package codetools_test

import (
	"Dendrite/pkg/mcpserver/catalog"
	"Dendrite/pkg/mcpserver/codetools"
	"Dendrite/pkg/mcpserver/mcptest"
	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/query/reuse"
	"Dendrite/pkg/query/slice"
	"Dendrite/pkg/sidecar"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "Dendrite/pkg/mcpserver"
)

func TestMain(m *testing.M) { mcptest.Main(m) }

// fixedSlicer answers every slice request with the same result.
type fixedSlicer struct{ result *slice.SliceResult }

func (f fixedSlicer) Slice(context.Context, slice.SliceRequest) (*slice.SliceResult, error) {
	return f.result, nil
}

func goProject(t *testing.T, module string, requires ...string) string {
	t.Helper()
	dir := t.TempDir()
	mod := "module " + module + "\n\ngo 1.22\n"
	for _, r := range requires {
		mod += "\nrequire " + r + " v0.0.0\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
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

// ownRefs points the reference tools at a refs file in the test's temporary directory.
func ownRefs(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "refs.json")
	old := codetools.DefaultMCPRefMgr
	codetools.DefaultMCPRefMgr = reuse.NewRefManager(path)
	t.Cleanup(func() { codetools.DefaultMCPRefMgr = old })
	return path
}

func ragDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package demo\n\n// LocateBinary finds the sidecar executable.\nfunc LocateBinary(name string) string { return name }\n"
	if err := os.WriteFile(filepath.Join(dir, "pkg", "demo", "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

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

// coreAllows reports whether the core profile may expose the tool.
func coreAllows(name string) bool {
	if CoreToolNames[name] {
		return true
	}
	for _, p := range CoreToolPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// checkSeen reports each name whose presence among seen differs from wantSeen.
func checkSeen(t *testing.T, seen map[string]bool, names []string, wantSeen bool) {
	t.Helper()
	for _, n := range names {
		if seen[n] != wantSeen {
			t.Errorf("core exposure of %s = %v, want %v", n, seen[n], wantSeen)
		}
	}
}

func TestCoreProfileExposesOnlyRAGGraphPlane(t *testing.T) {
	src := mcptest.NewDefaultServerForProfileTest()
	core := ApplyProfile(src, ProfileCore)
	tools := core.ListTools()
	if len(tools) == 0 || len(tools) >= len(src.ListTools()) {
		t.Fatalf("core has %d of %d tools", len(tools), len(src.ListTools()))
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool.Name] = true
		if !coreAllows(tool.Name) {
			t.Errorf("core exposes %s", tool.Name)
		}
		if _, has := core.Handler(tool.Name); !has {
			t.Errorf("core tool %s lost its handler", tool.Name)
		}
	}
	checkSeen(t, seen, []string{"dendrite_rag_retrieve", "dendrite_rag_show", "dendrite_rag_ask", "dendrite_plane_issues", "dendrite_graph_explain", "dendrite_ticket_smart_select"}, true)
	checkSeen(t, seen, []string{"dendrite_store_rm", "dendrite_service_move", "dendrite_dist_spawn"}, false)
}

func TestAgentSearchFailsWithoutTheSidecar(t *testing.T) {
	t.Setenv(sidecar.EnvVar("dendrite-rag"), filepath.Join(t.TempDir(), "no-such-binary"))
	srv := mcptest.ToolsWith(t, nil, "")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "search", Tool: "dendrite_agent_search", Args: `{"query":"find the parser"}`, Env: nil, Want: "agent search failed"},
		{Name: "route", Tool: "dendrite_agent_search", Args: `{"query":"find the parser","route":true}`, Env: nil, Want: "agent routing failed"},
	})
}

func TestGraphAndDocusaurusToolsReportFailures(t *testing.T) {
	mcptest.NoEnvPlane(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	dir := testGraphProject(t)
	missing := filepath.Join(t.TempDir(), "missing")
	auditFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(auditFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "communities", Tool: "dendrite_graph_communities", Args: `{"project_path":"` + dir + `"}`, Env: map[string]string{"STANDIN_FAIL": "graph.communities"}, Want: "graph communities failed"},
		{Name: "god nodes", Tool: "dendrite_graph_god_nodes", Args: `{"project_path":"` + dir + `"}`, Env: map[string]string{"STANDIN_FAIL": "graph.god_nodes"}, Want: "graph god nodes failed"},
		{Name: "path", Tool: "dendrite_graph_path", Args: `{"project_path":"` + dir + `","from":"a","to":"b"}`, Env: map[string]string{"STANDIN_FAIL": "graph.path"}, Want: "graph path failed"},
		{Name: "explain", Tool: "dendrite_graph_explain", Args: `{"project_path":"` + dir + `","target":"a"}`, Env: map[string]string{"STANDIN_FAIL": "graph.explain"}, Want: "graph explain failed"},
		{Name: "a site with an invalid port", Tool: "dendrite_docusaurus_generate", Args: `{"port":70000}`, Env: nil, Want: "invalid site configuration"},
		{Name: "a site from missing docs", Tool: "dendrite_docusaurus_generate", Args: `{"docs_dir":"` + missing + `","target_dir":"` + filepath.Join(t.TempDir(), "site") + `"}`, Env: nil, Want: "docusaurus generation failed"},
		{Name: "a bundle from a missing site", Tool: "dendrite_docusaurus_share", Args: `{"site_dir":"` + missing + `"}`, Env: nil, Want: "failed creating sharing bundle"},
		{Name: "an audit of a project that is a file", Tool: "dendrite_docusaurus_audit", Args: `{"project_root":"` + auditFile + `","docs_dir":"` + auditFile + `"}`, Env: nil, Want: "documentation audit failed"},
	})

	h, _ := srv.Handler("dendrite_graph_path")
	if _, err := h(context.Background(), map[string]interface{}{"project_path": dir, "from": "a", "to": "b", "kinds": []string{"calls"}, "max_hops": 3}); err != nil {
		t.Fatalf("typed kinds and an integer hop limit: %v", err)
	}
}

// agentSearchCall sends one dendrite_agent_search request and fails the test when the server answers with an error.
func agentSearchCall(t *testing.T, srv *Server, ctx context.Context, id int, args string) *JSONRPCResponse {
	t.Helper()
	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"dendrite_agent_search","arguments":%s}}`, id, args)
	resp := srv.HandleRequest(ctx, []byte(req))
	if resp.Error != nil {
		t.Fatalf("unexpected error from dendrite_agent_search: %v", resp.Error)
	}
	return resp
}

func TestTools_AgentSearch(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	req := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_agent_search", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, req, "expected error for missing query in agent_search")
	mcptest.ExpectCalls(t, srv, "dendrite_agent_search",
		`{"query": "refactor mcp server", "target_package": "pkg/mcpserver", "target_symbols": ["NewServer"], "require_local": true, "max_cost": 0.05, "max_results": 3, "route": false}`,
		`{"query": "refactor mcp server", "route": true}`)
}

func TestTools_TestGen(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. Missing file parameter (error)
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_test_auto", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, req1, "expected error for missing file in test_auto")

	// 2. Valid Go file
	tmpDir := t.TempDir()
	sourceFile := filepath.Join(tmpDir, "calculator.go")
	code := "package calc\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n"
	if err := os.WriteFile(sourceFile, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_test_auto", "arguments": {"file": %q, "with_testcontainers": false, "max_iterations": 1}}}`, sourceFile)
	mcptest.RequireResponse(t, srv, req2, "expected response for test_auto")
}

func TestDendriteRAGTools(t *testing.T) {
	tmpDir := testDendriteRAGToolsStep1(t)
	testDendriteRAGToolsStep2(t, tmpDir)
	srv := testDendriteRAGToolsStep3(t, tmpDir)
	testDendriteRAGToolsStep4(t, tmpDir, srv)
	respBad := testDendriteRAGToolsStep5(t, srv)
	testDendriteRAGToolsStep6(t, respBad)
	testDendriteRAGToolsStep7(t, srv)
}

func testDendriteRAGToolsStep1(t *testing.T) string {
	tmpDir := t.TempDir()
	return tmpDir
}

func testDendriteRAGToolsStep2(t *testing.T, tmpDir string) {
	docFile := filepath.Join(tmpDir, "architecture.md")
	docContent := `# System Architecture
Dendrite connects IDEs and autonomous agents through Model Context Protocol and AST reachability analysis.
`
	if err := os.WriteFile(docFile, []byte(docContent), 0644); err != nil {
		t.Fatalf("failed to write doc: %v", err)
	}
}

func testDendriteRAGToolsStep3(t *testing.T, tmpDir string) *Server {
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
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return srv
}

// testDendriteRAGToolsStep4: 1. Test dendrite_rag_retrieve tool
func testDendriteRAGToolsStep4(t *testing.T, tmpDir string, srv *Server) {
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
}

// testDendriteRAGToolsStep5: 2. Error case: missing question mcptest.On dendrite_rag_ask
func testDendriteRAGToolsStep5(t *testing.T, srv *Server) *JSONRPCResponse {
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
	return respBad
}

// testDendriteRAGToolsStep6: Handler error wraps as CallToolResult with IsError: true
func testDendriteRAGToolsStep6(t *testing.T, respBad *JSONRPCResponse) {
	resBad, isCallRes := respBad.Result.(CallToolResult)
	if isCallRes {
		if !resBad.IsError {
			t.Errorf("expected IsError=true mcptest.On missing question")
		}
	} else if respBad.Error == nil {
		t.Errorf("expected error or IsError mcptest.On missing question")
	}
}

// testDendriteRAGToolsStep7: 3. Error case: missing query mcptest.On dendrite_rag_retrieve
func testDendriteRAGToolsStep7(t *testing.T, srv *Server) {
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
			t.Errorf("expected IsError=true mcptest.On missing query")
		}
	} else if respBadRet.Error == nil {
		t.Errorf("expected error or IsError mcptest.On missing query")
	}
}

func TestDendriteRAGMapThenShow(t *testing.T) {
	dir := t.TempDir()
	src := "package demo\n\n// Locate finds the sidecar.\nfunc Locate() string { return \"x\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	catalog.RegisterDefaultTools(srv, Dependencies{})
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

func TestRAGAskArgumentsAndFailures(t *testing.T) {
	mcptest.NoEnvPlane(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	dir := ragDir(t)

	cases := []struct {
		name string
		args string
		want string
		fail bool
	}{
		{"a paid model, an explicit top_k and a share", `{"question":"LocateBinary","project_path":"` + dir + `","paid_model":"some-model","top_k":3,"docs_share":0.5}`, "local answer", false},
		{"a share without top_k asks for the context pack", `{"question":"LocateBinary","project_path":"` + dir + `","docs_share":0.5}`, "local answer", false},
		{"a share out of range", `{"question":"q","project_path":"` + dir + `","docs_share":2}`, "docs_share must be from 0 to 1", true},
		{"the tickets corpus needs Plane", `{"question":"q","project_path":"` + dir + `","corpus":"tickets"}`, "needs Plane", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, isErr := mcptest.CallTool(t, srv, "dendrite_rag_ask", c.args)
			if isErr != c.fail || !strings.Contains(text, c.want) {
				t.Fatalf("text = %q (error %v), want %q (error %v)", text, isErr, c.want, c.fail)
			}
		})
	}

	t.Run("integer top_k from a Go caller", func(t *testing.T) {
		h, _ := srv.Handler("dendrite_rag_ask")
		if _, err := h(context.Background(), map[string]interface{}{"question": "LocateBinary", "project_path": dir, "top_k": 2}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("the sidecar fails", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", "ask")
		if text, isErr := mcptest.CallTool(t, srv, "dendrite_rag_ask", `{"question":"q","project_path":"`+dir+`"}`); !isErr || !strings.Contains(text, "rag ask failed") {
			t.Fatalf("text = %q (error %v)", text, isErr)
		}
	})
}

func TestRAGRetrieveArgumentsAndFailures(t *testing.T) {
	mcptest.NoEnvPlane(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	dir := ragDir(t)

	if text, isErr := mcptest.CallTool(t, srv, "dendrite_rag_retrieve", `{"query":"LocateBinary","project_path":"`+dir+`","detail":"map","token_budget":900}`); isErr {
		t.Fatalf("detail with a budget: %s", text)
	}
	h, _ := srv.Handler("dendrite_rag_retrieve")
	if _, err := h(context.Background(), map[string]interface{}{"query": "LocateBinary", "project_path": dir, "top_k": 2}); err != nil {
		t.Fatalf("integer top_k: %v", err)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_rag_retrieve", `{"query":"q","project_path":"`+dir+`","corpus":"tickets"}`); !isErr || !strings.Contains(text, "needs Plane") {
		t.Fatalf("tickets corpus: %q (error %v)", text, isErr)
	}
	t.Run("the sidecar fails", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", "search")
		if text, isErr := mcptest.CallTool(t, srv, "dendrite_rag_retrieve", `{"query":"q","project_path":"`+dir+`"}`); !isErr || !strings.Contains(text, "rag retrieval failed") {
			t.Fatalf("text = %q (error %v)", text, isErr)
		}
	})
}

func TestRAGShowArgumentsAndFailures(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	dir := ragDir(t)
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_rag_show", `{"ids":["",3]}`); !isErr || !strings.Contains(text, "ids is required") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
	t.Run("the sidecar fails", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", "chunks.get")
		if text, isErr := mcptest.CallTool(t, srv, "dendrite_rag_show", `{"ids":["pkg/demo/demo.go#1"],"project_path":"`+dir+`"}`); !isErr || !strings.Contains(text, "rag show failed") {
			t.Fatalf("text = %q (error %v)", text, isErr)
		}
	})
}

func TestGraphTools(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	dir := testGraphProject(t)
	ctx := context.Background()
	for _, c := range []struct {
		tool    string
		args    map[string]interface{}
		wantErr bool
	}{
		{"dendrite_graph_communities", map[string]interface{}{"limit": 10}, false},
		{"dendrite_graph_god_nodes", map[string]interface{}{"limit": 10}, false},
		{"dendrite_graph_path", map[string]interface{}{"from": "Run", "to": "LocateBinary", "kinds": []interface{}{"call"}}, false},
		{"dendrite_graph_path", map[string]interface{}{"from": ""}, true},
		{"dendrite_graph_explain", map[string]interface{}{"target": "LocateBinary"}, false},
		{"dendrite_graph_explain", map[string]interface{}{"target": ""}, true},
	} {
		c.args["project_path"] = dir
		res, err := invokeTool(ctx, srv, c.tool, c.args)
		switch {
		case c.wantErr && err == nil:
			t.Errorf("%s %v: expected an error", c.tool, c.args)
		case !c.wantErr && err != nil:
			t.Fatalf("%s: %v", c.tool, err)
		case !c.wantErr && (len(res.Content) == 0 || res.Content[0].Text == ""):
			t.Errorf("%s: empty response", c.tool)
		}
	}
}

func TestTools_Pool(t *testing.T) {
	srv, tmpDir := testTools_PoolStep1(t)
	testTools_PoolStep2(t, srv)
	projA, projB := testTools_PoolStep3(t, tmpDir)
	testTools_PoolStep4(t, srv, projA, projB)
	testTools_PoolStep5(t, srv)
	testTools_PoolStep6(t, srv)
	testTools_PoolStep7(t, srv, projA)
	testTools_PoolStep8(t, srv, projA)
	testTools_PoolStep9(t, srv, tmpDir, projA)
}

func testTools_PoolStep1(t *testing.T) (*Server, string) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	tmpDir := t.TempDir()
	return srv, tmpDir
}

// testTools_PoolStep2: 1. dendrite_pool_resolve without directories (empty pool)
func testTools_PoolStep2(t *testing.T, srv *Server) {
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_pool_resolve", "arguments": {}}}`
	mcptest.MustHandle(t, srv, req1, "pool_resolve empty failed")
}

// testTools_PoolStep3: 2. dendrite_pool_resolve with directories
func testTools_PoolStep3(t *testing.T, tmpDir string) (string, string) {
	projA := filepath.Join(tmpDir, "projA")
	projB := filepath.Join(tmpDir, "projB")
	_ = os.MkdirAll(projA, 0755)
	_ = os.MkdirAll(projB, 0755)
	_ = os.WriteFile(filepath.Join(projA, "go.mod"), []byte("module projA\n\ngo 1.25\n"), 0644)
	_ = os.WriteFile(filepath.Join(projB, "go.mod"), []byte("module projB\n\ngo 1.25\n"), 0644)
	return projA, projB
}

func testTools_PoolStep4(t *testing.T, srv *Server, projA string, projB string) {
	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_pool_resolve", "arguments": {"directories": [%q, %q]}}}`, projA, projB)
	mcptest.MustHandle(t, srv, req2, "pool_resolve with dirs failed")
}

// testTools_PoolStep5: 3. dendrite_pool_list
func testTools_PoolStep5(t *testing.T, srv *Server) {
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_pool_list", "arguments": {}}}`
	mcptest.MustHandle(t, srv, req3, "pool_list failed")
}

// testTools_PoolStep6: 4. dendrite_pool_add: empty directories (error)
func testTools_PoolStep6(t *testing.T, srv *Server) {
	req4a := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_pool_add", "arguments": {"directories": []}}}`
	mcptest.ExpectToolError(t, srv, req4a, "expected error for empty directories in pool_add")
}

// testTools_PoolStep7: 5. dendrite_pool_add: valid directories
func testTools_PoolStep7(t *testing.T, srv *Server, projA string) {
	req4b := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_pool_add", "arguments": {"directories": [%q]}}}`, projA)
	mcptest.RequireResponse(t, srv, req4b, "expected response for pool_add")
}

// testTools_PoolStep8: 6. dendrite_pool_switch
func testTools_PoolStep8(t *testing.T, srv *Server, projA string) {
	req5 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_pool_switch", "arguments": {"target_project_dir": %q, "dependency_name": "projB", "mode": "published", "version": "v1.0.0"}}}`, projA)
	mcptest.RequireResponse(t, srv, req5, "expected response for pool_switch")
}

// testTools_PoolStep9: 7. dendrite_pool_split
func testTools_PoolStep9(t *testing.T, srv *Server, tmpDir string, projA string) {
	splitTarget := filepath.Join(tmpDir, "splitProj")
	req6 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_pool_split", "arguments": {"source_project_dir": %q, "module_subpath": "sub", "new_member_name": "splitProj", "target_directory": %q, "wire_local_mode": false, "register_in_pool": false}}}`, projA, splitTarget)
	mcptest.RequireResponse(t, srv, req6, "expected response for pool_split")
}

func TestTools_Query(t *testing.T) {
	srv, tmpDir := testTools_QueryStep1(t)
	testTools_QueryStep2(t, srv, tmpDir)
	testTools_QueryStep3(t, srv, tmpDir)
	testTools_QueryStep4(t, srv, tmpDir)
	testTools_QueryStep5(t, srv, tmpDir)
	testTools_QueryStep6(t, srv, tmpDir)
	testTools_QueryStep7(t, srv)
	testTools_QueryStep8(t, srv)
	testTools_QueryStep9(t, srv)
	testTools_QueryStep10(t, srv)
}

// testTools_QueryStep1: The reuse audit records ADRs under docs/adr relative to the CWD; keep them out of the package dir.
func testTools_QueryStep1(t *testing.T) (*Server, string) {
	t.Chdir(t.TempDir())
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	tmpDir := t.TempDir()
	return srv, tmpDir
}

// testTools_QueryStep2: 1. dendrite_query_slice
func testTools_QueryStep2(t *testing.T, srv *Server, tmpDir string) {
	sliceReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_query_slice", "arguments": {"sources_path": %q, "entrypoint_symbol": "main", "intent": "extract core", "direction": "forward", "output_dir": %q}}}`, tmpDir, filepath.Join(tmpDir, "slice_out"))
	mcptest.RequireResponse(t, srv, sliceReq, "expected response for query_slice")
}

// testTools_QueryStep3: 2. dendrite_query_swap
func testTools_QueryStep3(t *testing.T, srv *Server, tmpDir string) {
	swapReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_query_swap", "arguments": {"sources_path": %q, "from_subsystem": "opengl", "to_subsystem": "wgpu", "target_language": "rust", "output_dir": %q}}}`, tmpDir, filepath.Join(tmpDir, "swap_out"))
	mcptest.RequireResponse(t, srv, swapReq, "expected response for query_swap")
}

// testTools_QueryStep4: 3. dendrite_query_transpile
func testTools_QueryStep4(t *testing.T, srv *Server, tmpDir string) {
	transReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_query_transpile", "arguments": {"sources_artifact": %q, "target_language": "rust", "no_std": true, "audit_library_reuse": true, "prompt": "safe rust", "output_dir": %q}}}`, tmpDir, filepath.Join(tmpDir, "trans_out"))
	mcptest.RequireResponse(t, srv, transReq, "expected response for query_transpile")
}

// testTools_QueryStep5: 4. dendrite_query_fuse: < 2 sources (error)
func testTools_QueryStep5(t *testing.T, srv *Server, tmpDir string) {
	fuseReq1 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_query_fuse", "arguments": {"sources": [%q], "fused_member_name": "fused", "target_directory": %q}}}`, tmpDir, filepath.Join(tmpDir, "fuse_out"))
	mcptest.ExpectToolError(t, srv, fuseReq1, "expected error for < 2 sources in query_fuse")
}

// testTools_QueryStep6: 5. dendrite_query_fuse: valid sources
func testTools_QueryStep6(t *testing.T, srv *Server, tmpDir string) {
	fuseReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_query_fuse", "arguments": {"sources": [%q, %q], "fused_member_name": "fused", "target_directory": %q, "target_language": "go", "register_in_pool": false}}}`, tmpDir, tmpDir, filepath.Join(tmpDir, "fuse_out"))
	mcptest.RequireResponse(t, srv, fuseReq2, "expected response for query_fuse")
}

// testTools_QueryStep7: 6. dendrite_query_reuse_audit
func testTools_QueryStep7(t *testing.T, srv *Server) {
	reuseReq := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_query_reuse_audit", "arguments": {"capability": "linear_algebra", "language": "rust", "ticket_id": "DEN-001", "intent": "matrix ops"}}}`
	mcptest.RequireResponse(t, srv, reuseReq, "expected response for query_reuse_audit")
}

// testTools_QueryStep8: 7. dendrite_ref_list
func testTools_QueryStep8(t *testing.T, srv *Server) {
	refListReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_ref_list", "arguments": {}}}`
	mcptest.RequireResponse(t, srv, refListReq, "expected response for ref_list")
}

// testTools_QueryStep9: 8. dendrite_ref_add (with name and without name)
func testTools_QueryStep9(t *testing.T, srv *Server) {
	refAddReq1 := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_ref_add", "arguments": {"id": "ref-wgpu", "name": "wgpu-rs", "repository": "https://github.com/gfx-rs/wgpu", "language": "rust", "capabilities": ["graphics", "compute"], "license": "MIT"}}}`
	mcptest.RequireResponse(t, srv, refAddReq1, "expected response for ref_add with name")
}

func testTools_QueryStep10(t *testing.T, srv *Server) {
	refAddReq2 := `{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": {"name": "dendrite_ref_add", "arguments": {"id": "ref-turso", "repository": "https://github.com/tursodatabase/libsql"}}}`
	mcptest.RequireResponse(t, srv, refAddReq2, "expected response for ref_add without name")
}

func TestPoolResolveSkipsNonStringsAndReportsACycle(t *testing.T) {
	a := goProject(t, "example.com/a", "example.com/b")
	b := goProject(t, "example.com/b", "example.com/a")
	srv := mcptest.ToolsWith(t, nil, "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_pool_resolve", `{"directories":[5,"`+a+`","`+b+`"]}`); isErr && !strings.Contains(text, "resolution failed") {
		t.Errorf("text = %q", text)
	}
}

func TestPoolAddReportsAConfigItCannotSave(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := codetools.DefaultMCPPoolManager
	codetools.DefaultMCPPoolManager = dag.NewPoolManager(filepath.Join(blocker, "pool.json"))
	t.Cleanup(func() { codetools.DefaultMCPPoolManager = old })
	srv := mcptest.ToolsWith(t, nil, "")
	dir := goProject(t, "example.com/c")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_pool_add", `{"directories":["`+dir+`"]}`); !isErr || !strings.Contains(text, "saving pool config") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestPoolSplitExtractsAModule(t *testing.T) {
	src := goProject(t, "example.com/src")
	mod := filepath.Join(src, "internal", "matcher")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "m.go"), []byte("package matcher\n\nfunc Match(a, b string) bool { return a == b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "m_test.go"), []byte("package matcher\n\nimport \"testing\"\n\nfunc TestMatch(t *testing.T) {\n\tif !Match(\"a\", \"a\") {\n\t\tt.Fatal()\n\t}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := mcptest.ToolsWith(t, nil, "")
	target := filepath.Join(t.TempDir(), "matcher")
	text, isErr := mcptest.CallTool(t, srv, "dendrite_pool_split", `{"source_project_dir":"`+src+`","module_subpath":"internal/matcher","new_member_name":"matcher","target_directory":"`+target+`"}`)
	t.Logf("split: %q (error %v)", text, isErr)
}

func TestQuerySliceOfASmallProject(t *testing.T) {
	src := goProject(t, "example.com/slice")
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc used() int { return 1 }\n\nfunc unused() int { return 2 }\n\nfunc main() { _ = used() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := mcptest.ToolsWith(t, nil, "")
	text, isErr := mcptest.CallTool(t, srv, "dendrite_query_slice", `{"sources_path":"`+src+`","entrypoint_symbol":"main","direction":"forward"}`)
	t.Logf("slice: %q (error %v)", text, isErr)
}

func TestQualityAndQueryToolsReportFailures(t *testing.T) {
	mcptest.InEmptyDir(t)
	refs := ownRefs(t)
	srv := mcptest.ToolsWith(t, nil, "")
	missing := filepath.Join(t.TempDir(), "missing")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "quality audit without a package", Tool: "dendrite_quality_audit", Args: `{}`, Env: nil, Want: "missing pkg"},
		{Name: "slice", Tool: "dendrite_query_slice", Args: `{"sources_path":"` + missing + `","entrypoint_symbol":"Main"}`, Env: nil, Want: "Slicing failed"},
		{Name: "swap without sources", Tool: "dendrite_query_swap", Args: `{"sources_path":"","from_subsystem":"opengl","to_subsystem":"wgpu"}`, Env: nil, Want: "Subsystem swap failed"},
		{Name: "transpile", Tool: "dendrite_query_transpile", Args: `{"sources_artifact":""}`, Env: nil, Want: "Transpilation failed"},
		{Name: "fuse with fewer than two sources", Tool: "dendrite_query_fuse", Args: `{"sources":["a"],"fused_member_name":"m","target_directory":"t"}`, Env: nil, Want: "at least two"},
		{Name: "fuse without a target", Tool: "dendrite_query_fuse", Args: `{"sources":["a","b"],"fused_member_name":"m","target_directory":""}`, Env: nil, Want: "Architecture fusion failed"},
		{Name: "reuse audit without a capability", Tool: "dendrite_query_reuse_audit", Args: `{"capability":""}`, Env: nil, Want: "reuse audit failed"},
		{Name: "ref add twice", Tool: "dendrite_ref_add", Args: `{"id":"ref-oxipix","repository":"https://example.com/r.git"}`, Env: nil, Want: "Failed to register ref"},
	})
	if err := os.WriteFile(refs, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{{Name: "ref list of a corrupt file", Tool: "dendrite_ref_list", Args: `{}`, Env: nil, Want: "Failed to list refs"}})
}

func TestPoolAndSchedulerToolsReportFailures(t *testing.T) {
	mcptest.InEmptyDir(t)
	srv := mcptest.ToolsWith(t, nil, "")
	missing := filepath.Join(t.TempDir(), "missing")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "resolve a missing directory", Tool: "dendrite_pool_resolve", Args: `{"directories":["` + missing + `",5]}`, Env: nil, Want: "scanning"},
		{Name: "add a missing directory", Tool: "dendrite_pool_add", Args: `{"directories":["` + missing + `"]}`, Env: nil, Want: "Failed adding projects"},
		{Name: "switch to an unknown mode", Tool: "dendrite_pool_switch", Args: `{"target_project_dir":"` + missing + `","dependency_name":"d","mode":"bogus"}`, Env: nil, Want: "mode switch failed"},
		{Name: "split a missing project", Tool: "dendrite_pool_split", Args: `{"source_project_dir":"` + missing + `","module_subpath":"x","new_member_name":"n","target_directory":"` + filepath.Join(t.TempDir(), "t") + `"}`, Env: nil, Want: "split failed"},
		{Name: "a policy that is not valid", Tool: "dendrite_analyze_policy", Args: `{"reserve_floor":7}`, Env: nil, Want: "updating scheduler policy"},
		{Name: "trigger without a chunk", Tool: "dendrite_analyze_trigger", Args: `{}`, Env: nil, Want: "missing chunk_id"},
	})
}

func TestDistAndPerfToolsReportFailures(t *testing.T) {
	mcptest.InEmptyDir(t)
	mcptest.NoEnvPlane(t)
	srv := mcptest.ToolsWith(t, nil, "")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "coordinate without a project", Tool: "dendrite_dist_interop", Args: `{"action":"coordinate"}`, Env: nil, Want: "interop coordination failed"},
		{Name: "spawn without a name", Tool: "dendrite_dist_spawn", Args: `{"project_name":"","create_plane_project":false,"init_git":false}`, Env: nil, Want: "spawning project failed"},
	})
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_dist_sources", `{"language":"go","priority":1}`); isErr {
		t.Errorf("sources with filters: %s", text)
	}
}

func TestQuerySliceAnswersWithTheSlice(t *testing.T) {
	old := codetools.DefaultMCPQuerySlicer
	codetools.DefaultMCPQuerySlicer = fixedSlicer{result: &slice.SliceResult{}}
	t.Cleanup(func() { codetools.DefaultMCPQuerySlicer = old })
	srv := mcptest.ToolsWith(t, nil, "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_query_slice", `{"sources_path":".","entrypoint_symbol":"main"}`); isErr || !strings.Contains(text, "{") {
		t.Fatalf("slice: %q (error %v)", text, isErr)
	}
}

func TestDendriteAgentSearchMCPTool(t *testing.T) {
	srv, ctx := testDendriteAgentSearchMCPToolStep1(t)
	resp := testDendriteAgentSearchMCPToolStep2(t, srv, ctx)
	ok := testDendriteAgentSearchMCPToolStep3(t, resp)
	routeResp := testDendriteAgentSearchMCPToolStep4(t, srv, ctx)
	testDendriteAgentSearchMCPToolStep5(t, ok, routeResp)
	testDendriteAgentSearchMCPToolStep6(t, srv, ctx)
}

func testDendriteAgentSearchMCPToolStep1(t *testing.T) (*Server, context.Context) {
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	catalog.RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
		OllamaURL: "http://127.0.0.1:11434",
	})
	ctx := context.Background()
	return srv, ctx
}

// testDendriteAgentSearchMCPToolStep2: 1. Test search query
func testDendriteAgentSearchMCPToolStep2(t *testing.T, srv *Server, ctx context.Context) *JSONRPCResponse {
	return agentSearchCall(t, srv, ctx, 201, `{"query": "refactor gateway limiter","target_package": "pkg/gateway","max_results": 2}`)
}

func testDendriteAgentSearchMCPToolStep3(t *testing.T, resp *JSONRPCResponse) bool {
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
	return ok
}

// testDendriteAgentSearchMCPToolStep4: 2. Test route mode
func testDendriteAgentSearchMCPToolStep4(t *testing.T, srv *Server, ctx context.Context) *JSONRPCResponse {
	return agentSearchCall(t, srv, ctx, 202, `{"query": "write rapid unit test for AST","require_local": true,"route": true}`)
}

func testDendriteAgentSearchMCPToolStep5(t *testing.T, ok bool, routeResp *JSONRPCResponse) {
	rMap := routeResp.Result.(*CallToolResult)
	var decision map[string]interface{}
	if err := json.Unmarshal([]byte(rMap.Content[0].Text), &decision); err != nil {
		t.Fatalf("failed to unmarshal routing decision: %v", err)
	}
	agentObj, ok := decision["selected_agent"].(map[string]interface{})
	if !ok || agentObj["id"] == "" {
		t.Fatalf("expected valid selected_agent, got: %v", decision)
	}
}

// testDendriteAgentSearchMCPToolStep6: 3. Test empty query error
func testDendriteAgentSearchMCPToolStep6(t *testing.T, srv *Server, ctx context.Context) {
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
		t.Fatalf("expected IsError=true mcptest.On empty query, got: %+v", badResp.Result)
	}
}

func TestDendriteTestAutoMCPTool(t *testing.T) {
	srv, ctx := testDendriteTestAutoMCPToolStep1(t)
	sampleSrc := testDendriteTestAutoMCPToolStep2(t)
	resp := testDendriteTestAutoMCPToolStep3(t, srv, ctx, sampleSrc)
	testDendriteTestAutoMCPToolStep4(t, resp)
	testDendriteTestAutoMCPToolStep5(t, srv, ctx)
}

func testDendriteTestAutoMCPToolStep1(t *testing.T) (*Server, context.Context) {
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	catalog.RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
		OllamaURL: "http://127.0.0.1:11434",
	})
	ctx := context.Background()
	return srv, ctx
}

// testDendriteTestAutoMCPToolStep2: Create a sample source file in a temp dir
func testDendriteTestAutoMCPToolStep2(t *testing.T) string {
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
	return sampleSrc
}

// testDendriteTestAutoMCPToolStep3: 1. Test test synthesis mcptest.On the source file
func testDendriteTestAutoMCPToolStep3(t *testing.T, srv *Server, ctx context.Context, sampleSrc string) *JSONRPCResponse {
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
	return resp
}

func testDendriteTestAutoMCPToolStep4(t *testing.T, resp *JSONRPCResponse) {
	var ContentText string
	switch r := resp.Result.(type) {
	case *CallToolResult:
		if len(r.Content) > 0 {
			ContentText = r.Content[0].Text
		}
	case CallToolResult:
		if len(r.Content) > 0 {
			ContentText = r.Content[0].Text
		}
	default:
		t.Fatalf("unexpected result type: %T", resp.Result)
	}
	if ContentText == "" {
		t.Fatalf("expected non-empty content text")
	}
	if strings.HasPrefix(ContentText, "Tool error:") {
		t.Fatalf("Tool error: %s", ContentText)
	}
	var report map[string]interface{}
	if err := json.Unmarshal([]byte(ContentText), &report); err != nil {
		t.Fatalf("failed to parse report: %v", err)
	}
	if report["state"] != "Verified" {
		t.Errorf("expected state Verified, got: %v", report["state"])
	}
}

// testDendriteTestAutoMCPToolStep5: 2. Test empty file error
func testDendriteTestAutoMCPToolStep5(t *testing.T, srv *Server, ctx context.Context) {
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
		t.Errorf("expected IsError mcptest.On empty file")
	}
	if !strings.Contains(badRes.Content[0].Text, "file parameter is required") {
		t.Errorf("unexpected error message: %s", badRes.Content[0].Text)
	}
}

func TestTools_DistCodebase(t *testing.T) {
	srv := testTools_DistCodebaseStep1(t)
	testTools_DistCodebaseStep2(t, srv)
	testTools_DistCodebaseStep3(t, srv)
	testTools_DistCodebaseStep4(t, srv)
	testTools_DistCodebaseStep5(t, srv)
	testTools_DistCodebaseStep6(t, srv)
	testTools_DistCodebaseStep7(t, srv)
	testTools_DistCodebaseStep8(t, srv)
	testTools_DistCodebaseStep9(t, srv)
	testTools_DistCodebaseStep10(t, srv)
	testTools_DistCodebaseStep11(t, srv)
}

func testTools_DistCodebaseStep1(t *testing.T) *Server {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return srv
}

// testTools_DistCodebaseStep2: 1. dendrite_dist_sources: query matching
func testTools_DistCodebaseStep2(t *testing.T, srv *Server) {
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_dist_sources", "arguments": {"query": "wgpu", "language": "rust"}}}`
	mcptest.MustHandle(t, srv, req1, "dist_sources matching query failed")
}

// testTools_DistCodebaseStep3: 2. dendrite_dist_sources: filters (priority & language)
func testTools_DistCodebaseStep3(t *testing.T, srv *Server) {
	req2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_dist_sources", "arguments": {"priority": 1, "language": "rust"}}}`
	mcptest.MustHandle(t, srv, req2, "dist_sources with filters failed")
}

// testTools_DistCodebaseStep4: 3. dendrite_dist_decompose: valid
func testTools_DistCodebaseStep4(t *testing.T, srv *Server) {
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_dist_decompose", "arguments": {"project_name": "carla-rust", "modules": ["engine", "physics"], "user_custom_overrides": ["physics"]}}}`
	mcptest.MustHandle(t, srv, req3, "dist_decompose failed")
}

// testTools_DistCodebaseStep5: 4. dendrite_dist_decompose: missing project_name (error)
func testTools_DistCodebaseStep5(t *testing.T, srv *Server) {
	req3b := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_dist_decompose", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, req3b, "expected error for decompose with missing project_name")
}

// testTools_DistCodebaseStep6: 5. dendrite_dist_interop: action "coordinate"
func testTools_DistCodebaseStep6(t *testing.T, srv *Server) {
	coordReq := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "coordinate", "requester_project": "docula", "dependency_project": "oxipix", "feature_requested": "svg_render", "allow_fallback": true}}}`
	mcptest.MustHandle(t, srv, coordReq, "interop coordinate failed")
}

// testTools_DistCodebaseStep7: 6. dendrite_dist_interop: action "list"
func testTools_DistCodebaseStep7(t *testing.T, srv *Server) {
	listReq := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "list"}}}`
	mcptest.MustHandle(t, srv, listReq, "interop list failed")
}

// testTools_DistCodebaseStep8: 7. dendrite_dist_interop: action "complete" & "reconcile" with invalid coordination_id (error)
func testTools_DistCodebaseStep8(t *testing.T, srv *Server) {
	compReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "complete", "coordination_id": "non-existent"}}}`
	mcptest.ExpectToolError(t, srv, compReq, "expected error for complete non-existent coordination")
}

func testTools_DistCodebaseStep9(t *testing.T, srv *Server) {
	recReq := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "reconcile", "coordination_id": "non-existent"}}}`
	mcptest.ExpectToolError(t, srv, recReq, "expected error for reconcile non-existent coordination")
}

// testTools_DistCodebaseStep10: 8. dendrite_dist_interop: unknown action (error)
func testTools_DistCodebaseStep10(t *testing.T, srv *Server) {
	unknownReq := `{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": {"name": "dendrite_dist_interop", "arguments": {"action": "unknown_action"}}}`
	mcptest.ExpectToolError(t, srv, unknownReq, "expected error for unknown interop action")
}

// testTools_DistCodebaseStep11: 9. dendrite_dist_spawn: valid spawn
func testTools_DistCodebaseStep11(t *testing.T, srv *Server) {
	spawnDir := filepath.Join(t.TempDir(), "subproj")
	spawnReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 10, "method": "tools/call", "params": {"name": "dendrite_dist_spawn", "arguments": {"project_name": "subproj", "identifier": "SUBP", "target_dir": %q, "language": "go", "is_satellite": true, "create_plane_project": false, "init_git": true, "description": "Sub project"}}}`, spawnDir)
	mcptest.MustHandle(t, srv, spawnReq, "dist_spawn failed")
}

func TestDistCodebaseMCPTools(t *testing.T) {
	srv, ctx := testDistCodebaseMCPToolsStep1(t)
	callTool := testDistCodebaseMCPToolsStep2(t, srv, ctx)
	err := testDistCodebaseMCPToolsStep3(t, callTool)
	testDistCodebaseMCPToolsStep4(t, callTool, err)
	testDistCodebaseMCPToolsStep5(t, callTool, err)
	coordID := testDistCodebaseMCPToolsStep6(t, callTool, err)
	testDistCodebaseMCPToolsStep7(t, callTool, err, coordID)
	testDistCodebaseMCPToolsStep8(t, callTool, err, coordID)
	testDistCodebaseMCPToolsStep9(t, callTool, err, coordID)
}

func testDistCodebaseMCPToolsStep1(t *testing.T) (*Server, context.Context) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})
	ctx := context.Background()
	return srv, ctx
}

func testDistCodebaseMCPToolsStep2(t *testing.T, srv *Server, ctx context.Context) func(name string, args map[string]interface{}) (*CallToolResult, error) {
	callTool := func(name string, args map[string]interface{}) (*CallToolResult, error) {
		reqBytes, _ := json.Marshal(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      name,
				"arguments": args,
			},
		})
		resp := srv.HandleRequest(ctx, reqBytes)
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error: %s", resp.Error.Message)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok {
			return nil, fmt.Errorf("unexpected result type: %T", resp.Result)
		}
		return res, nil
	}
	return callTool
}

// testDistCodebaseMCPToolsStep3: 1. Test dendrite_dist_sources: list all
func testDistCodebaseMCPToolsStep3(t *testing.T, callTool func(name string, args map[string]interface{}) (*CallToolResult, error)) error {
	res, err := callTool("dendrite_dist_sources", map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to call dendrite_dist_sources: %v", err)
	}
	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "wgpu") {
		t.Errorf("expected wgpu in dist sources output, got: %v", res.Content)
	}
	return err
}

// testDistCodebaseMCPToolsStep4: Test query resolution: "cte" -> diesel-cte-ext
func testDistCodebaseMCPToolsStep4(t *testing.T, callTool func(name string, args map[string]interface{}) (*CallToolResult, error), err error) {
	mcptest.ExpectContent(t, "failed to resolve query in dendrite_dist_sources", func() (*CallToolResult, error) {
		return callTool("dendrite_dist_sources", map[string]interface{}{
			"query": "cte",
		})
	}, "diesel-cte-ext")
}

// testDistCodebaseMCPToolsStep5: 2. Test dendrite_dist_decompose
func testDistCodebaseMCPToolsStep5(t *testing.T, callTool func(name string, args map[string]interface{}) (*CallToolResult, error), err error) {
	mcptest.ExpectContent(t, "failed to call dendrite_dist_decompose", func() (*CallToolResult, error) {
		return callTool("dendrite_dist_decompose", map[string]interface{}{
			"project_name": "CarlaEngine",
			"language":     "rust",
			"modules": []interface{}{
				"render-pipeline",
				"storage-engine",
			},
		})
	}, "wgpu")
}

// testDistCodebaseMCPToolsStep6: 3. Test dendrite_dist_interop
func testDistCodebaseMCPToolsStep6(t *testing.T, callTool func(name string, args map[string]interface{}) (*CallToolResult, error), err error) string {
	resInterop, err := callTool("dendrite_dist_interop", map[string]interface{}{
		"action":             "coordinate",
		"requester_project":  "Docula",
		"dependency_project": "Oxipix",
		"feature_requested":  "simd_blend_rgba",
		"allow_fallback":     true,
	})
	if err != nil {
		t.Fatalf("failed to coordinate interop: %v", err)
	}
	if !strings.Contains(resInterop.Content[0].Text, "TODO(interop:Oxipix)") {
		t.Errorf("expected Oxipix TODO in interop, got: %s", resInterop.Content[0].Text)
	}
	var coordMap map[string]interface{}
	_ = json.Unmarshal([]byte(resInterop.Content[0].Text), &coordMap)
	coordObj := coordMap["coordination"].(map[string]interface{})
	coordID := coordObj["id"].(string)
	return coordID
}

// interopCaller calls an MCP tool by name.
type interopCaller = func(name string, args map[string]interface{}) (*CallToolResult, error)

// interopAction runs one dendrite_dist_interop action on a coordination and requires want in the answer.
func interopAction(t *testing.T, callTool interopCaller, coordID, action, want string) {
	t.Helper()
	mcptest.ExpectContent(t, "failed to "+action+" interop", func() (*CallToolResult, error) {
		return callTool("dendrite_dist_interop", map[string]interface{}{"action": action, "coordination_id": coordID})
	}, want)
}

// testDistCodebaseMCPToolsStep7: Complete dependency feature
func testDistCodebaseMCPToolsStep7(t *testing.T, callTool interopCaller, err error, coordID string) {
	interopAction(t, callTool, coordID, "complete", "dependency_completed")
}

// testDistCodebaseMCPToolsStep8: Reconcile
func testDistCodebaseMCPToolsStep8(t *testing.T, callTool interopCaller, err error, coordID string) {
	interopAction(t, callTool, coordID, "reconcile", "successfully_reconciled")
}

// testDistCodebaseMCPToolsStep9: List
func testDistCodebaseMCPToolsStep9(t *testing.T, callTool func(name string, args map[string]interface{}) (*CallToolResult, error), err error, coordID string) {
	resList, err := callTool("dendrite_dist_interop", map[string]interface{}{
		"action": "list",
	})
	if err != nil || !strings.Contains(resList.Content[0].Text, coordID) {
		t.Errorf("expected coordination in list, got: %v, err: %v", resList, err)
	}
}
