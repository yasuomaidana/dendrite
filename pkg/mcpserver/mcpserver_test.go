package mcpserver_test

import (
	"Dendrite/pkg/mcpserver/catalog"
	"Dendrite/pkg/mcpserver/mcptest"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "Dendrite/pkg/mcpserver"
)

func TestMain(m *testing.M) { mcptest.Main(m) }

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func peersRepo(t *testing.T, root, name string) (dir, id string) {
	t.Helper()
	dir = filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := projectid.Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, id
}

func repoWithIdentity(t *testing.T, withID bool) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if withID {
		if _, err := projectid.Ensure(dir); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// synapseFixture saves a two-member synapse whose members are real directories with project IDs.
func synapseFixture(t *testing.T) (*synapse.Store, map[string]string) {
	t.Helper()
	root := t.TempDir()
	ids := map[string]string{"api": "dpid_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "web": "dpid_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	dirs := map[string]string{}
	var members []synapse.Member
	for _, alias := range []string{"api", "web"} {
		dir := filepath.Join(root, alias)
		if err := os.MkdirAll(filepath.Join(dir, ".dendrite"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".dendrite", "project.id"), []byte(ids[alias]+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dirs[alias] = dir
		members = append(members, synapse.Member{ProjectID: ids[alias], Alias: alias, Role: synapse.RolePeer, PathHint: dir})
	}
	st := synapse.NewStore(filepath.Join(root, "synapses"))
	err := st.Save(&synapse.Synapse{
		ID: "web-api", Name: "Web API", State: synapse.StateActive, Members: members,
		Contracts: []synapse.Contract{{Kind: "openapi", Producer: "api", Path: "openapi.yaml", Consumers: []synapse.Consumer{{Alias: "web", Path: "src/types.ts"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, dirs
}

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
	catalog.RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})

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

func TestServer_ConfigDefaults(t *testing.T) {
	// Empty config defaults
	srv := NewServer(ServerConfig{})
	if srv.Info().Name != "Dendrite Control Plane MCP Server" {
		t.Errorf("expected default name, got %q", srv.Info().Name)
	}
	if srv.Info().Version != "0.1.0-alpha" {
		t.Errorf("expected default version, got %q", srv.Info().Version)
	}

	// Custom config
	srv2 := NewServer(ServerConfig{Name: "Custom", Version: "2.0.0"})
	if srv2.Info().Name != "Custom" || srv2.Info().Version != "2.0.0" {
		t.Errorf("expected custom config, got name=%q version=%q", srv2.Info().Name, srv2.Info().Version)
	}
}

func TestServer_InvalidParams(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterTool(Tool{Name: "tool1"}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		return &CallToolResult{}, nil
	})

	// tools/call with invalid params (not a JSON object)
	req := `{"jsonrpc": "2.0", "id": 99, "method": "tools/call", "params": "not-an-object"}`
	resp := srv.HandleRequest(context.Background(), []byte(req))
	if resp == nil || resp.Error == nil || resp.Error.Code != ErrCodeInvalidParams {
		t.Fatalf("expected ErrCodeInvalidParams, got: %v", resp)
	}
}

func TestServer_ToolHandlerError(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterTool(Tool{Name: "failing_tool"}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		return nil, errors.New("simulated tool failure")
	})

	req := `{"jsonrpc": "2.0", "id": 100, "method": "tools/call", "params": {"name": "failing_tool"}}`
	resp := srv.HandleRequest(context.Background(), []byte(req))
	if resp == nil {
		t.Fatal("expected response")
	}
	res, ok := resp.Result.(CallToolResult)
	if !ok || !res.IsError || len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "simulated tool failure") {
		t.Fatalf("expected CallToolResult with error, got: %#v", resp.Result)
	}
}

func TestServer_ServeStdio_Cancellation(t *testing.T) {
	srv := NewServer(ServerConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	in := strings.NewReader(`{"jsonrpc": "2.0", "id": 1, "method": "ping"}` + "\n")
	out := &bytes.Buffer{}

	err := srv.ServeStdio(ctx, in, out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestServer_ServeStdio_EmptyLine(t *testing.T) {
	srv := NewServer(ServerConfig{})
	input := "\n\n" + `{"jsonrpc": "2.0", "id": 1, "method": "ping"}` + "\n\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}

	err := srv.ServeStdio(context.Background(), in, out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}
}

func TestServer_ServeStdio_WriteError(t *testing.T) {
	srv := NewServer(ServerConfig{})
	in := strings.NewReader(`{"jsonrpc": "2.0", "id": 1, "method": "ping"}` + "\n")
	pr, pw := io.Pipe()
	_ = pr.Close()
	_ = pw.Close() // closing pipe writer forces fmt.Fprintf(out, ...) to fail

	err := srv.ServeStdio(context.Background(), in, pw)
	if err == nil || !strings.Contains(err.Error(), "failed to write response") {
		t.Fatalf("expected failed to write response error, got: %v", err)
	}
}

func TestServer_ServeStdio_ScannerError(t *testing.T) {
	srv := NewServer(ServerConfig{})
	pr, pw := io.Pipe()
	_ = pw.CloseWithError(errors.New("simulated pipe read error"))

	out := &bytes.Buffer{}
	err := srv.ServeStdio(context.Background(), pr, out)
	if err == nil || !strings.Contains(err.Error(), "simulated pipe read error") {
		t.Fatalf("expected simulated pipe read error, got: %v", err)
	}
}

func TestServer_RegisterDefaultTools_EmptyDependencies(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	tools := srv.ListTools()
	if len(tools) == 0 {
		t.Fatal("expected registered tools with empty dependencies")
	}
}

func TestParseProfile(t *testing.T) {
	cases := []struct {
		in   string
		want Profile
		err  bool
	}{
		{"", ProfileFull, false}, {"core", ProfileCore, false}, {" FULL ", ProfileFull, false}, {"tiny", "", true},
	}
	for _, c := range cases {
		got, err := ParseProfile(c.in, ProfileFull)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("ParseProfile(%q) = %q, %v", c.in, got, err)
		}
	}
}

func TestProfileEnv(t *testing.T) {
	t.Setenv(ProfileEnv, "core")
	if p, err := ProfileFromEnv(ProfileFull); err != nil || p != ProfileCore {
		t.Fatalf("env core = %q, %v", p, err)
	}
	t.Setenv(ProfileEnv, "bogus")
	if _, err := ProfileFromEnv(ProfileFull); err == nil {
		t.Fatal("bogus profile accepted")
	}
}

func TestFullProfileKeepsEveryTool(t *testing.T) {
	src := mcptest.NewDefaultServerForProfileTest()
	if got := ApplyProfile(src, ProfileFull); got != src {
		t.Fatal("full profile must return the server unchanged")
	}
}

func TestProbeToolsClassifiesOutcomes(t *testing.T) {
	s := NewServer(ServerConfig{})
	reg := func(name string, h ToolHandler) { s.RegisterTool(Tool{Name: name}, h) }
	reg("ok", func(context.Context, map[string]any) (*CallToolResult, error) {
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: "fine"}}}, nil
	})
	reg("err", func(context.Context, map[string]any) (*CallToolResult, error) {
		return nil, errors.New("plane unreachable\nstack")
	})
	reg("tool-error", func(context.Context, map[string]any) (*CallToolResult, error) {
		return &CallToolResult{IsError: true, Content: []ContentItem{{Type: "text", Text: "bad input"}}}, nil
	})
	reg("slow", func(ctx context.Context, _ map[string]any) (*CallToolResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	reg("panics", func(context.Context, map[string]any) (*CallToolResult, error) { panic("boom") })

	got := ProbeTools(context.Background(), s, []ProbeSpec{
		{Tool: "ok"}, {Tool: "err"}, {Tool: "tool-error"},
		{Tool: "slow", Timeout: 20 * time.Millisecond}, {Tool: "panics"}, {Tool: "missing"},
	})
	want := map[string][2]any{ // ok, skipped
		"ok": {true, false}, "err": {false, false}, "tool-error": {false, false},
		"slow": {false, false}, "panics": {false, false}, "missing": {false, true},
	}
	for _, r := range got {
		w := want[r.Tool]
		if r.OK != w[0] || r.Skipped != w[1] {
			t.Errorf("%s = ok:%v skipped:%v (%s)", r.Tool, r.OK, r.Skipped, r.Detail)
		}
	}
	var buf bytes.Buffer
	if fails := WriteProbeMatrix(&buf, got); fails != 4 {
		t.Errorf("fails = %d\n%s", fails, buf.String())
	}
	for _, s := range []string{"PASS", "FAIL", "SKIP", "plane unreachable", "timeout after"} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("matrix lacks %q:\n%s", s, buf.String())
		}
	}
}

func TestDefaultProbeSpecsOnlyReadOnlyRegisteredTools(t *testing.T) {
	src := mcptest.NewDefaultServerForProfileTest()
	writers := []string{"issue_create", "push_status", "doc_sync", "ticket_update", "store_rm", "service_move", "penpot_sync", "relation_link", "commit"}
	for _, spec := range DefaultProbeSpecs() {
		if _, ok := src.Handler(spec.Tool); !ok {
			t.Errorf("probe spec %s is not a registered tool", spec.Tool)
		}
		for _, w := range writers {
			if strings.Contains(spec.Tool, w) {
				t.Errorf("probe spec %s is a writer", spec.Tool)
			}
		}
	}
}

func TestProfilesOtherThanCoreExposeEverything(t *testing.T) {
	if !ProfileFull.InProfile("any_tool_at_all") {
		t.Error("the full profile exposes every tool")
	}
	if ProfileCore.InProfile("a_tool_outside_core") {
		t.Error("the core profile hides tools outside its list")
	}
}

func TestProbeReportsAHandlerThatReturnsNothing(t *testing.T) {
	got := ProbeOne(context.Background(), ProbeSpec{Tool: "silent"}, func(context.Context, map[string]interface{}) (*CallToolResult, error) {
		return nil, nil
	}, time.Second)
	if got.OK || got.Detail != "nil result" {
		t.Fatalf("probe = %+v", got)
	}
}

func TestSynapsePeersTool_ListsPeersAndIsEmptyOutsideSynapses(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	root := t.TempDir()
	docula, doculaID := peersRepo(t, root, "docula")
	oxipix, oxipixID := peersRepo(t, root, "oxipix")
	lone, _ := peersRepo(t, root, "lone")

	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&synapse.Synapse{ID: "mesh", Name: "mesh", State: synapse.StateActive, Members: []synapse.Member{
		{ProjectID: doculaID, Alias: "docula", Role: synapse.RoleConsumer, PathHint: docula, PlaneProject: "DOC"},
		{ProjectID: oxipixID, Alias: "oxipix", Role: synapse.RoleProducer, PathHint: oxipix, PlaneProject: "OXI"},
	}}); err != nil {
		t.Fatal(err)
	}

	s := mcptest.NewDefaultServerForProfileTest()
	call := func(dir string) string {
		resp := mcptest.Rpc(t, s, "tools/call", CallToolParams{Name: "dendrite_synapse_peers", Arguments: map[string]interface{}{"dir": dir}})
		return mustJSON(t, resp.Result)
	}

	got := call(filepath.Join(docula)) // from the repository root; a subdirectory resolves the same way
	for _, want := range []string{`mesh`, `oxipix`, `producer`, `OXI`, `OK`} {
		if !strings.Contains(got, want) {
			t.Errorf("peers of docula lack %q: %s", want, got)
		}
	}
	if strings.Contains(got, `\"alias\":\"docula\"`) {
		t.Errorf("a project is not its own peer: %s", got)
	}
	if got := call(lone); !strings.Contains(got, `[]`) || strings.Contains(got, "isError\":true") {
		t.Errorf("a repository in no synapse gets an empty list: %s", got)
	}
	if got := call(t.TempDir()); !strings.Contains(got, `"isError":true`) {
		t.Errorf("a directory outside any repository is an error: %s", got)
	}

	// The tool is read-only and cheap: the core profile keeps it.
	core := ApplyProfile(s, ProfileCore)
	if _, ok := core.Handler("dendrite_synapse_peers"); !ok {
		t.Error("the core profile must expose dendrite_synapse_peers")
	}
}

func TestPlaneClientOfBuildsFromTheEnvironment(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "token")
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("PLANE_PROJECT_ID", "proj")
	c, err := PlaneClientOf(Dependencies{})
	if err != nil || c == nil {
		t.Fatalf("client = %v, err = %v", c, err)
	}
}

func TestSynapseServerExposesOnlySynapseTools(t *testing.T) {
	st, _ := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	tools := s.ListTools()
	if len(tools) == 0 {
		t.Fatal("scoped server has no tools")
	}
	for _, tool := range tools {
		if !strings.HasPrefix(tool.Name, "synapse_") {
			t.Fatalf("scoped server leaked non-synapse tool %q", tool.Name)
		}
	}
	init := mcptest.Rpc(t, s, "initialize", map[string]interface{}{})
	if !strings.Contains(mustJSON(t, init.Result), "web-api") {
		t.Fatalf("server name should identify the synapse: %s", mustJSON(t, init.Result))
	}
}

func TestNewSynapseServerUnknownID(t *testing.T) {
	st, _ := synapseFixture(t)
	if _, err := NewSynapseServer(SynapseScope{Store: st, ID: "nope"}, "test"); err == nil {
		t.Fatal("expected error for unknown synapse")
	}
}

func TestSynapseInfo(t *testing.T) {
	st, dirs := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	call := func() map[string]interface{} {
		resp := mcptest.Rpc(t, s, "tools/call", CallToolParams{Name: "synapse_info", Arguments: map[string]interface{}{}})
		var res CallToolResult
		if err := json.Unmarshal([]byte(mustJSON(t, resp.Result)), &res); err != nil {
			t.Fatal(err)
		}
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
			t.Fatalf("synapse_info text is not JSON: %v\n%s", err, res.Content[0].Text)
		}
		out["isError"] = res.IsError
		return out
	}

	out := call()
	if out["id"] != "web-api" || out["state"] != "active" {
		t.Fatalf("unexpected info: %v", out)
	}
	members := out["members"].([]interface{})
	if members[0].(map[string]interface{})["status"] != "OK" {
		t.Fatalf("member status = %v", members[0])
	}

	// Health is re-read mcptest.On every call.
	if err := os.RemoveAll(dirs["web"]); err != nil {
		t.Fatal(err)
	}
	members = call()["members"].([]interface{})
	if members[1].(map[string]interface{})["status"] != "MISSING" {
		t.Fatalf("web status after removal = %v", members[1])
	}

	// Manifest deleted under a running server -> tool-level error, not a crash.
	if err := st.Delete("web-api"); err != nil {
		t.Fatal(err)
	}
	resp := mcptest.Rpc(t, s, "tools/call", CallToolParams{Name: "synapse_info", Arguments: map[string]interface{}{}})
	if !strings.Contains(mustJSON(t, resp.Result), `"isError":true`) {
		t.Fatalf("expected isError result, got %s", mustJSON(t, resp.Result))
	}
}

func TestConfineToMember(t *testing.T) {
	st, dirs := synapseFixture(t)
	syn, err := st.Load("web-api")
	if err != nil {
		t.Fatal(err)
	}
	ok := []struct{ alias, rel, want string }{
		{"api", "", dirs["api"]},
		{"api", "openapi.yaml", filepath.Join(dirs["api"], "openapi.yaml")},
		{"web", "src/../src/types.ts", filepath.Join(dirs["web"], "src", "types.ts")},
	}
	for _, c := range ok {
		got, err := ConfineToMember(syn, c.alias, c.rel)
		if err != nil || got != c.want {
			t.Fatalf("ConfineToMember(%s, %q) = %q, %v; want %q", c.alias, c.rel, got, err, c.want)
		}
	}
	bad := []struct{ alias, rel string }{
		{"nope", "x"},
		{"api", "/etc/passwd"},
		{"api", "../web/src/types.ts"},
		{"api", ".."},
	}
	for _, c := range bad {
		if _, err := ConfineToMember(syn, c.alias, c.rel); err == nil {
			t.Fatalf("ConfineToMember(%s, %q) should be rejected", c.alias, c.rel)
		}
	}
	if err := os.RemoveAll(dirs["api"]); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfineToMember(syn, "api", "x"); err == nil || !strings.Contains(err.Error(), "heal") {
		t.Fatalf("unresolved member should point at heal, got %v", err)
	}
}

func TestJSONResultUnmarshalableValue(t *testing.T) {
	if _, err := JsonResult(map[string]interface{}{"c": make(chan int)}); err == nil {
		t.Fatal("expected marshal error for a channel value")
	}
}

func TestSynapseSyncStatus(t *testing.T) {
	st, dirs := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	call := func(args map[string]interface{}) CallToolResult {
		resp := mcptest.Rpc(t, s, "tools/call", CallToolParams{Name: "synapse_sync_status", Arguments: args})
		var res CallToolResult
		if err := json.Unmarshal([]byte(mustJSON(t, resp.Result)), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Fixture members are plain directories, not git checkouts: each reports an error.
	res := call(map[string]interface{}{"fetch": true})
	var rep synapse.SyncReport
	if err := json.Unmarshal([]byte(res.Content[0].Text), &rep); err != nil {
		t.Fatalf("not a sync report: %v\n%s", err, res.Content[0].Text)
	}
	if rep.Synapse != "web-api" || len(rep.Members) != len(dirs) || rep.Members[0].Error == "" {
		t.Fatalf("report = %+v", rep)
	}
	// Read-only: the manifest mcptest.On disk is unchanged.
	if syn, _ := st.Load("web-api"); syn.State != synapse.StateActive {
		t.Fatalf("manifest state changed to %s", syn.State)
	}

	if err := st.Delete("web-api"); err != nil {
		t.Fatal(err)
	}
	if res := call(map[string]interface{}{}); !res.IsError {
		t.Fatal("expected isError after manifest deletion")
	}
}

func TestSynapseSearchToolValidatesInputAndReportsUnsearchableMembers(t *testing.T) {
	st, _ := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	h, ok := s.Handler("synapse_search")
	if !ok {
		t.Fatal("synapse_search is not registered mcptest.On the synapse server")
	}
	res, err := h(context.Background(), map[string]any{"query": "  "})
	if err != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "empty query") {
		t.Errorf("empty query: %+v, %v", res, err)
	}
	res, err = h(context.Background(), map[string]any{"query": "x", "members": []any{"nobody"}})
	if err != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "not a member") {
		t.Errorf("unknown member: %+v, %v", res, err)
	}
}

func TestSynapseToolsReportASynapseThatIsGone(t *testing.T) {
	st, _ := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(st.Dir()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"synapse_sync_status", "synapse_search"} {
		h, _ := s.Handler(name)
		res, err := h(context.Background(), map[string]any{"query": "x"})
		if err != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "loading synapse") {
			t.Errorf("%s: %+v, %v", name, res, err)
		}
	}
}

func TestSynapseSearchAnswersWithHits(t *testing.T) {
	st, dirs := synapseFixture(t)
	for _, dir := range dirs {
		if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\n// LocateBinary finds it.\nfunc LocateBinary() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := s.Handler("synapse_search")
	res, err := h(context.Background(), map[string]any{"query": "LocateBinary", "top_k": float64(2)})
	if err != nil || res.IsError || !strings.Contains(res.Content[0].Text, "hits") {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestSynapsePeersToolHandlesEveryRepositoryShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	srv := NewServer(ServerConfig{})
	RegisterSynapsePeersTool(srv)

	if text, isErr := mcptest.CallTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+t.TempDir()+`"}`); !isErr || !strings.Contains(text, "not inside a git repository") {
		t.Errorf("outside a repository: %q (error %v)", text, isErr)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+repoWithIdentity(t, false)+`"}`); isErr || strings.TrimSpace(text) != "[]" {
		t.Errorf("a repository without an identity is in no synapse: %q (error %v)", text, isErr)
	}
	identified := repoWithIdentity(t, true)
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+identified+`"}`); isErr || strings.TrimSpace(text) != "[]" {
		t.Errorf("an identified repository with no synapse: %q (error %v)", text, isErr)
	}

	if err := os.WriteFile(filepath.Join(home, "synapses"), []byte("x"), 0o644); err == nil {
		// DefaultStore keeps its manifests under home/synapses; a file there cannot be listed.
		if text, isErr := mcptest.CallTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+identified+`"}`); !isErr || !strings.Contains(text, "listing peers") {
			t.Errorf("an unreadable store: %q (error %v)", text, isErr)
		}
	}

	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+identified+`"}`); !isErr || !strings.Contains(text, "opening the synapse store") {
		t.Errorf("no home directory: %q (error %v)", text, isErr)
	}
}
