package opstools_test

import (
	"Dendrite/pkg/agent"
	"Dendrite/pkg/env/bootstrap"
	"Dendrite/pkg/mcpserver/catalog"
	"Dendrite/pkg/mcpserver/mcptest"
	"Dendrite/pkg/mcpserver/opstools"
	"Dendrite/pkg/personalcontext/greeting"
	"Dendrite/pkg/store"
	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"
	"Dendrite/pkg/story/fork"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/hitl"
	"Dendrite/pkg/tracker/hitl/hitltest"
	"Dendrite/pkg/tracker/plane"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "Dendrite/pkg/mcpserver"
)

func TestMain(m *testing.M) { mcptest.Main(m) }

// chdirToScratchRepo runs the rest of the test inside a throwaway git checkout (mcptest.On a
// feature branch, with an origin) so tools that act mcptest.On "." (branch guard, audit,
// remote sync) can never switch, rebase or commit in the real repository.
func chdirToScratchRepo(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	origin, repo := filepath.Join(root, "origin.git"), filepath.Join(root, "repo")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(root, "init", "-q", "--bare", "-b", "develop", origin)
	git(root, "clone", "-q", origin, repo)
	git(repo, "checkout", "-q", "-b", "develop")
	git(repo, "commit", "-q", "--allow-empty", "-m", "init")
	git(repo, "push", "-q", "-u", "origin", "develop")
	git(repo, "checkout", "-q", "-b", "feature/x")
	t.Chdir(repo)
}

func closedURL() string {
	s := httptest.NewServer(http.NotFoundHandler())
	s.Close()
	return s.URL
}

func gitIn(t *testing.T, dir string, args ...string) { testkit.Git(t, dir, args...) }

// offlineHITL points the GitHub/HITL tools at a local Plane server and a scripted gh,
// so tests never reach the real Plane instance or GitHub.
func offlineHITL(t *testing.T) (*hitltest.PlaneServer, *hitltest.GHScript) {
	t.Helper()
	ps := hitltest.NewPlaneServer(
		plane.PlaneIssue{ID: "u-1", Name: "[DEN-1] feat(cli): new", State: "st-verify"},
		plane.PlaneIssue{ID: "u-2", Name: "[DEN-2] approved", State: "st-verify"},
	)
	t.Cleanup(ps.Close)
	gh := (&hitltest.GHScript{}).
		On("gh issue list", hitltest.IssueListJSON(hitltest.IssueRow{Number: 5, Title: "[DEN-2] approved", State: "CLOSED", StateReason: "COMPLETED", Labels: []string{github.LabelToVerify, github.LabelHITL}}), nil).
		On("gh issue create", "https://github.com/o/r/issues/6", nil)
	prev := opstools.NewHITLSyncer
	opstools.NewHITLSyncer = func(_ context.Context, _ Dependencies, repo string) (*hitl.Syncer, error) {
		if repo == "fail/me" {
			return nil, errors.New("no plane")
		}
		s := hitl.New(ps.Client(), github.NewGitHubClient("o/r").WithExecutor(gh), ".")
		s.Git = gh
		return s, nil
	}
	t.Cleanup(func() { opstools.NewHITLSyncer = prev })
	return ps, gh
}

// ownStore points the store tools at a store inside the test's temporary home and returns it.
func ownStore(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	st, err := segmented.NewSegmentedStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldStore, oldInspector := opstools.DefaultMCPStore, opstools.DefaultMCPStoreInspector
	opstools.DefaultMCPStore, opstools.DefaultMCPStoreInspector = st, status.NewStoreInspector(st)
	t.Cleanup(func() { opstools.DefaultMCPStore, opstools.DefaultMCPStoreInspector = oldStore, oldInspector })
	return dir
}

// resetScheduler puts the shared scheduler back to automatic mode with a full quota.
func resetScheduler(t *testing.T, srv *Server) {
	t.Helper()
	t.Cleanup(func() {
		mcptest.CallTool(t, srv, "dendrite_analyze_policy", `{"mode":"auto","allow_zero":false}`)
		mcptest.CallTool(t, srv, "dendrite_analyze_status", `{"remaining_quota":1}`)
	})
}

// storyRepo points the story tools at a temporary git repository and its own worktree directory.
func storyRepo(t *testing.T) string {
	t.Helper()
	repo, stories := t.TempDir(), filepath.Join(t.TempDir(), "stories")
	testkit.Git(t, repo, "init", "-q")
	testkit.Git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "base")
	old := opstools.DefaultForkController
	opstools.DefaultForkController = fork.NewForkController(repo, stories, fork.NewOSGitExecutor(), timedHarness{}, store.NewStore(filepath.Join(repo, "store"), filepath.Join(repo, "adr")))
	t.Cleanup(func() { opstools.DefaultForkController = old })
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
	return stories
}

// timedHarness measures a variant by returning the same fixed numbers for each.
type timedHarness struct{}

func (timedHarness) BenchmarkVariant(_ context.Context, v *fork.Variant, _ fork.BenchmarkOptions) (*fork.VariantMetrics, error) {
	return &fork.VariantMetrics{Variant: v.Name, TTFT: time.Millisecond, TPS: 10, MemoryMB: 5}, nil
}

func TestDefaultTools_Execution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})

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
	setupDir, _ := json.Marshal(t.TempDir())
	setupReq := `{"jsonrpc": "2.0", "id": 15, "method": "tools/call", "params": {"name": "dendrite_jetbrains_setup", "arguments": {"project_only": true, "project_dir": ` + string(setupDir) + `}}}`
	setupResp := srv.HandleRequest(context.Background(), []byte(setupReq))
	if setupResp.Error != nil {
		t.Fatalf("dendrite_jetbrains_setup error: %v", setupResp.Error)
	}
}

func TestTriageTierAndEngineNames(t *testing.T) {
	srv := mcptest.ToolsWith(t, nil, "")
	for _, engine := range []string{"ollama", "ollama-validated", "huggingface", "hf", "candle", "oxillama", ""} {
		for _, tier := range []string{"local", "hybrid", "cloud", ""} {
			mcptest.CallTool(t, srv, "dendrite_local_agent_triage", `{"prompt":"fix a typo","target_engine":"`+engine+`","force_tier":"`+tier+`"}`)
		}
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_local_agent_triage", `{"prompt":"x","execute_local":true}`); isErr {
		t.Errorf("text = %q", text)
	}
}

func TestSmallToolsReportTheirFailures(t *testing.T) {
	srv := mcptest.ToolsWith(t, nil, "")
	missing := filepath.Join(t.TempDir(), "missing")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "env prepare mcptest.On a file", Tool: "dendrite_env_prepare", Args: `{"project_path":"` + file + `","dry_run":false}`, Env: nil, Want: "environment preparation failed"},
		{Name: "migrate to an unreachable host", Tool: "dendrite_service_move", Args: `{"target_host":"127.0.0.1","ssh_port":1,"timeout_seconds":1}`, Env: nil, Want: "migration failed"},
		{Name: "test synthesis for a missing file", Tool: "dendrite_test_auto", Args: `{"file":"` + missing + `"}`, Env: nil, Want: "test synthesis error"},
		{Name: "inference migration without a model", Tool: "dendrite_inference_migrate_model", Args: `{"model":" "}`, Env: nil, Want: "model name is required"},
		{Name: "inference migration of a model that is not there", Tool: "dendrite_inference_migrate_model", Args: `{"model":"no-such-model:1b","dry_run":true}`, Env: nil, Want: "model not found"},
		{Name: "penpot sync of a missing file", Tool: "dendrite_ui_penpot_sync", Args: `{"file":"` + missing + `","dry_run":true}`, Env: nil, Want: ""},
	})
}

func TestTools_Perf(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

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
	catalog.RegisterDefaultTools(srv, Dependencies{})

	for _, format := range []string{"json", "markdown", "summary", "default"} {
		mcptest.AssertToolCallSuccess(t, srv, "dendrite_rea_showcase", fmt.Sprintf(`{"target": "all", "format": %q}`, format))
	}
}

func TestTools_Docusaurus(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

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
	catalog.RegisterDefaultTools(srv, Dependencies{OllamaURL: "http://127.0.0.1:11434"})

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

func TestBenchToolDefaultsAndFailures(t *testing.T) {
	srv := mcptest.ToolsWith(t, nil, "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_bench_run", `{"mode":"stress"}`); isErr {
		t.Errorf("a stress run with the default users: %s", text)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_bench_run", `{"mode":"inference","engine_id":"ollama-metal","duration_seconds":1,"virtual_users":1}`); isErr && !strings.Contains(text, "benchmark execution failed") {
		t.Errorf("a known engine supplies its endpoint: %s", text)
	}
	dead := closedURL()
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_bench_run", `{"mode":"inference","endpoint":"`+dead+`","duration_seconds":1,"virtual_users":1}`); isErr && !strings.Contains(text, "benchmark execution failed") {
		t.Errorf("an endpoint that is down: %s", text)
	}
}

func TestTokenStatusToolReadsItsFiles(t *testing.T) {
	dir := t.TempDir()
	spend := filepath.Join(dir, "spend.json")
	policy := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(spend, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := mcptest.ToolsWith(t, nil, "")
	text, isErr := mcptest.CallTool(t, srv, "dendrite_tokens_status", `{"spend_file":"`+spend+`","policy_file":"`+policy+`","model":"m"}`)
	if isErr && !strings.Contains(text, "evaluating token status") {
		t.Errorf("text = %q", text)
	}
}

func TestBenchToolReportsAK6RunThatFails(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "k6"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	srv := mcptest.ToolsWith(t, nil, "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_bench_run", `{"mode":"inference","endpoint":"http://127.0.0.1:1","duration_seconds":1,"virtual_users":1}`); !isErr || !strings.Contains(text, "benchmark execution failed") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestAnalyzeTriggerReportsWhyAChunkCannotRun(t *testing.T) {
	srv := mcptest.ToolsWith(t, nil, "")
	resetScheduler(t, srv)

	if text, isErr := mcptest.CallTool(t, srv, "dendrite_analyze_policy", `{"mode":"paused"}`); isErr {
		t.Fatalf("pausing: %s", text)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_analyze_trigger", `{"chunk_id":"c1"}`); !isErr || !strings.Contains(text, "processing chunk") {
		t.Errorf("a paused scheduler: %q (error %v)", text, isErr)
	}

	mcptest.CallTool(t, srv, "dendrite_analyze_policy", `{"mode":"auto"}`)
	mcptest.CallTool(t, srv, "dendrite_analyze_status", `{"remaining_quota":0.01}`)
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_analyze_trigger", `{"chunk_id":"c2","simulate_sprint":true}`); !isErr || !strings.Contains(text, "processing chunk") {
		t.Errorf("the quota governor refuses the cloud: %q (error %v)", text, isErr)
	}
}

func TestSchedulerTools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// Verify tool registration
	registered := make(map[string]bool)
	for _, tool := range srv.ListTools() {
		registered[tool.Name] = true
	}

	expectedTools := []string{
		"dendrite_analyze_policy",
		"dendrite_analyze_status",
		"dendrite_analyze_trigger",
		"dendrite_analyze_switch",
		"dendrite_tokens_status",
	}

	for _, toolName := range expectedTools {
		if !registered[toolName] {
			t.Errorf("expected tool %s to be registered", toolName)
		}
	}

	ctx := context.Background()

	// 1. Test dendrite_analyze_policy query
	policyHandler, _ := srv.Handler("dendrite_analyze_policy")
	res, err := policyHandler(ctx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("policyHandler query failed: %v", err)
	}
	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "night_window") {
		t.Errorf("expected night_window in policy response: %+v", res.Content)
	}

	// 2. Test dendrite_analyze_policy update
	resUpdate, err := policyHandler(ctx, map[string]interface{}{
		"night_window":        "22:00-05:00",
		"weekly_reset":        "MON 00:00",
		"sprint_before_reset": "24h",
		"reserve_floor":       0.15,
		"allow_zero":          true,
		"mode":                "auto",
	})
	if err != nil {
		t.Fatalf("policyHandler update failed: %v", err)
	}
	if !strings.Contains(resUpdate.Content[0].Text, `"updated": true`) {
		t.Errorf("expected updated: true in response: %s", resUpdate.Content[0].Text)
	}

	// 3. Test dendrite_analyze_status
	statusHandler, _ := srv.Handler("dendrite_analyze_status")
	resStatus, err := statusHandler(ctx, map[string]interface{}{
		"remaining_quota": 0.85,
	})
	if err != nil {
		t.Fatalf("statusHandler failed: %v", err)
	}
	if len(resStatus.Content) == 0 || !strings.Contains(resStatus.Content[0].Text, "active_tier") {
		t.Errorf("expected active_tier in status response: %+v", resStatus.Content)
	}

	// 4. Test dendrite_analyze_trigger
	triggerHandler, _ := srv.Handler("dendrite_analyze_trigger")
	resTrigger, err := triggerHandler(ctx, map[string]interface{}{
		"chunk_id": "pkg/store/store.go",
	})
	if err != nil {
		t.Fatalf("triggerHandler failed: %v", err)
	}
	if len(resTrigger.Content) == 0 || !strings.Contains(resTrigger.Content[0].Text, "chunk-") && !strings.Contains(resTrigger.Content[0].Text, "pkg/store/store.go") {
		t.Errorf("expected chunk in trigger response: %+v", resTrigger.Content)
	}

	// 5. Test dendrite_analyze_trigger with simulated sprint
	resSprint, err := triggerHandler(ctx, map[string]interface{}{
		"chunk_id":        "pkg/ast/parser.go",
		"simulate_sprint": true,
	})
	if err != nil {
		t.Fatalf("triggerHandler sprint failed: %v", err)
	}
	if !strings.Contains(resSprint.Content[0].Text, "CLOUD_SPRINT") {
		t.Errorf("expected CLOUD_SPRINT in simulated sprint trigger response: %s", resSprint.Content[0].Text)
	}

	// 6. Test dendrite_analyze_trigger with simulated night
	resNight, err := triggerHandler(ctx, map[string]interface{}{
		"chunk_id":       "pkg/ast/parser.go",
		"simulate_night": true,
	})
	if err != nil {
		t.Fatalf("triggerHandler night failed: %v", err)
	}
	if !strings.Contains(resNight.Content[0].Text, "NIGHT_LOCAL") {
		t.Errorf("expected NIGHT_LOCAL in simulated night trigger response: %s", resNight.Content[0].Text)
	}

	// 7. Test dendrite_analyze_switch
	switchHandler, _ := srv.Handler("dendrite_analyze_switch")
	resSwitch, err := switchHandler(ctx, map[string]interface{}{
		"allow_zero": true,
		"mode":       "always",
	})
	if err != nil {
		t.Fatalf("switchHandler failed: %v", err)
	}
	if !strings.Contains(resSwitch.Content[0].Text, `"allow_zero_drain": true`) {
		t.Errorf("expected allow_zero_drain in switch response: %s", resSwitch.Content[0].Text)
	}

	// 8. Test dendrite_tokens_status
	tokensHandler, _ := srv.Handler("dendrite_tokens_status")
	resTokens, err := tokensHandler(ctx, map[string]interface{}{
		"model": "gemini-3.8-flash-high",
	})
	if err != nil {
		t.Fatalf("tokensHandler failed: %v", err)
	}
	if len(resTokens.Content) == 0 || !strings.Contains(resTokens.Content[0].Text, "day_window") {
		t.Errorf("expected day_window in tokens response: %+v", resTokens.Content)
	}
	if !strings.Contains(resTokens.Content[0].Text, "week_window") {
		t.Errorf("expected week_window in tokens response: %+v", resTokens.Content)
	}
	if !strings.Contains(resTokens.Content[0].Text, "agy_status") {
		t.Errorf("expected agy_status in tokens response: %+v", resTokens.Content)
	}
}

func TestContractTools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// Verify tool registration
	registered := false
	for _, tool := range srv.ListTools() {
		if tool.Name == "dendrite_contract_drift_check" {
			registered = true
			break
		}
	}
	if !registered {
		t.Fatalf("expected dendrite_contract_drift_check to be registered")
	}

	tmpDir, err := os.MkdirTemp("", "mcp_contract_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	schemaDir := filepath.Join(tmpDir, "schemas")
	codeDir := filepath.Join(tmpDir, "models")
	_ = os.MkdirAll(schemaDir, 0755)
	_ = os.MkdirAll(codeDir, 0755)

	protoFile := filepath.Join(schemaDir, "test.proto")
	_ = os.WriteFile(protoFile, []byte("message User {\n  string id = 1;\n  int32 age = 2;\n}"), 0644)

	goFile := filepath.Join(codeDir, "user.go")
	_ = os.WriteFile(goFile, []byte("package main\ntype User struct {\n\tID string `json:\"id\"`\n\tAge string `json:\"age\"`\n}\n"), 0644)

	handler, _ := srv.Handler("dendrite_contract_drift_check")
	ctx := context.Background()

	res, err := handler(ctx, map[string]interface{}{
		"schema_dirs": []interface{}{schemaDir},
		"code_dirs": map[string]interface{}{
			"go": codeDir,
		},
	})
	if err != nil {
		t.Fatalf("handler execution failed: %v", err)
	}

	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "TYPE_MISMATCH") {
		t.Errorf("expected TYPE_MISMATCH in drift check response: %+v", res.Content)
	}
}

func TestREATools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})

	tools := srv.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "dendrite_rea_showcase" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dendrite_rea_showcase to be registered among tools")
	}

	handler, exists := srv.Handler("dendrite_rea_showcase")
	if !exists {
		t.Fatalf("handler for dendrite_rea_showcase not found")
	}

	ctx := context.Background()

	// 1. Execute summary mode
	res, err := handler(ctx, map[string]interface{}{
		"target": "all",
		"format": "summary",
	})
	if err != nil {
		t.Fatalf("dendrite_rea_showcase failed: %v", err)
	}
	if len(res.Content) != 2 {
		t.Fatalf("expected 2 content items in summary mode, got %d", len(res.Content))
	}
	if !strings.Contains(res.Content[0].Text, "REA Showcase Verification") {
		t.Errorf("expected summary text, got: %s", res.Content[0].Text)
	}

	// 2. Execute json mode
	resJSON, err := handler(ctx, map[string]interface{}{
		"target": "th04",
		"format": "json",
	})
	if err != nil {
		t.Fatalf("dendrite_rea_showcase json failed: %v", err)
	}
	if len(resJSON.Content) != 1 || !strings.Contains(resJSON.Content[0].Text, `"target": "th04"`) {
		t.Errorf("unexpected json result: %+v", resJSON.Content)
	}

	// 3. Execute markdown mode
	resMD, err := handler(ctx, map[string]interface{}{
		"target": "dxball",
		"format": "markdown",
	})
	if err != nil {
		t.Fatalf("dendrite_rea_showcase markdown failed: %v", err)
	}
	if len(resMD.Content) != 1 || !strings.Contains(resMD.Content[0].Text, "# Open-Source REA Showcase") {
		t.Errorf("unexpected markdown result: %+v", resMD.Content)
	}

	// 4. Unknown target error handling
	_, err = handler(ctx, map[string]interface{}{
		"target": "nonexistent_target_xyz",
	})
	if err == nil {
		t.Fatalf("expected error mcptest.On unknown target, got nil")
	}
}

func TestDendriteLocalAgentTriageTool(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:7b"}]}`))
	}))
	defer ts.Close()

	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	catalog.RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
		OllamaURL: ts.URL,
	})

	// 1. Test chore task with forced local tier
	reqJSON := `{
		"jsonrpc": "2.0",
		"id": 101,
		"method": "tools/call",
		"params": {
			"name": "dendrite_local_agent_triage",
			"arguments": {
				"prompt": "Fix linter errors and format this file",
				"force_tier": "local",
				"execute_local": true
			}
		}
	}`

	resp := srv.HandleRequest(context.Background(), []byte(reqJSON))
	if resp.Error != nil {
		t.Fatalf("unexpected error from dendrite_local_agent_triage: %v", resp.Error)
	}

	resultMap, ok := resp.Result.(*CallToolResult)
	if !ok {
		t.Fatalf("expected *CallToolResult, got %T", resp.Result)
	}
	if len(resultMap.Content) == 0 {
		t.Fatalf("expected non-empty content list")
	}

	text := resultMap.Content[0].Text
	if !strings.Contains(text, `"tier": "local"`) {
		t.Errorf("expected tier 'local' in output, got: %s", text)
	}
	if !strings.Contains(text, `"token_savings_pct": 100`) {
		t.Errorf("expected 100%% token savings in output, got: %s", text)
	}

	// 2. Test hybrid task with skill matching
	hybridReq := `{
		"jsonrpc": "2.0",
		"id": 102,
		"method": "tools/call",
		"params": {
			"name": "dendrite_local_agent_triage",
			"arguments": {
				"prompt": "Evaluate ticket DEN-050 and check cyclomatic complexity before refactoring",
				"force_tier": "hybrid"
			}
		}
	}`

	hybridResp := srv.HandleRequest(context.Background(), []byte(hybridReq))
	if hybridResp.Error != nil {
		t.Fatalf("unexpected error from hybrid triage: %v", hybridResp.Error)
	}

	var parsed struct {
		Tier           string   `json:"tier"`
		SelectedSkills []string `json:"selected_skills"`
		EnhancedPrompt string   `json:"enhanced_prompt"`
	}
	hResultMap := hybridResp.Result.(*CallToolResult)
	hContent := hResultMap.Content[0].Text
	if err := json.Unmarshal([]byte(hContent), &parsed); err != nil {
		t.Fatalf("failed to parse triage result json: %v", err)
	}

	if parsed.Tier != "hybrid" {
		t.Errorf("expected tier 'hybrid', got %s", parsed.Tier)
	}
	if len(parsed.SelectedSkills) == 0 {
		t.Errorf("expected selected skills, got empty list")
	}
}

func TestDocusaurusTools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})

	expectedTools := []string{
		"dendrite_docusaurus_generate",
		"dendrite_docusaurus_share",
		"dendrite_docusaurus_audit",
	}

	tools := srv.ListTools()
	for _, expected := range expectedTools {
		found := false
		for _, tool := range tools {
			if tool.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected tool %s to be registered", expected)
		}
	}

	ctx := context.Background()

	// Setup temporary dirs
	tempSrc, err := os.MkdirTemp("", "mcp-docusaurus-src-*")
	if err != nil {
		t.Fatalf("failed to create temp src: %v", err)
	}
	defer os.RemoveAll(tempSrc)

	tempDest, err := os.MkdirTemp("", "mcp-docusaurus-dest-*")
	if err != nil {
		t.Fatalf("failed to create temp dest: %v", err)
	}
	defer os.RemoveAll(tempDest)

	_ = os.WriteFile(filepath.Join(tempSrc, "FEAT-100-test.md"), []byte("# FEAT-100\n## Acceptance Criteria\n- [x] pass\n"), 0644)

	// 1. Test dendrite_docusaurus_generate
	genHandler, _ := srv.Handler("dendrite_docusaurus_generate")
	resGen, err := genHandler(ctx, map[string]interface{}{
		"docs_dir":     tempSrc,
		"target_dir":   tempDest,
		"title":        "Test Docs Portal",
		"port":         3050,
		"sharing_tier": "federated",
	})
	if err != nil {
		t.Fatalf("dendrite_docusaurus_generate failed: %v", err)
	}
	if len(resGen.Content) == 0 || !strings.Contains(resGen.Content[0].Text, "site_directory") {
		t.Errorf("unexpected generation result: %v", resGen.Content)
	}

	// 2. Test dendrite_docusaurus_share
	shareHandler, _ := srv.Handler("dendrite_docusaurus_share")
	resShare, err := shareHandler(ctx, map[string]interface{}{
		"site_dir": tempDest,
	})
	if err != nil {
		t.Fatalf("dendrite_docusaurus_share failed: %v", err)
	}
	if len(resShare.Content) == 0 || !strings.Contains(resShare.Content[0].Text, "manifest_version") {
		t.Errorf("unexpected share result: %v", resShare.Content)
	}

	// 3. Test dendrite_docusaurus_audit
	auditHandler, _ := srv.Handler("dendrite_docusaurus_audit")
	resAudit, err := auditHandler(ctx, map[string]interface{}{
		"project_root": tempSrc,
		"docs_dir":     tempSrc,
	})
	if err != nil {
		t.Fatalf("dendrite_docusaurus_audit failed: %v", err)
	}
	if len(resAudit.Content) == 0 || !strings.Contains(resAudit.Content[0].Text, "total_features") {
		t.Errorf("unexpected audit result: %v", resAudit.Content)
	}
}

func TestWorkingTreeToolsReportANonRepository(t *testing.T) {
	mcptest.InEmptyDir(t)
	mcptest.NoEnvPlane(t)
	srv := mcptest.ToolsWith(t, nil, "")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "audit changes", Tool: "dendrite_audit_changes", Args: `{"branch":"x","dry_run":true,"max_tokens":100,"model":"m"}`, Env: nil, Want: "audit changes failed"},
	})
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_vcs_guard", `{}`); isErr || !strings.Contains(text, "ALERT") {
		t.Errorf("a guard outside a repository alerts: %q (error %v)", text, isErr)
	}
}

func TestCanvasToolsReportBadRequests(t *testing.T) {
	mcptest.NoEnvPlane(t)
	srv := mcptest.ToolsWith(t, nil, "")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "an unknown action", Tool: "dendrite_canvas_action_dispatch", Args: `{"action":"no-such-action","prompt":"p"}`, Env: nil, Want: "unknown"},
		{Name: "a relation without a ticket", Tool: "dendrite_canvas_penpot_bridge", Args: `{"subaction":"relate"}`, Env: nil, Want: ""},
		{Name: "a state update without a ticket", Tool: "dendrite_canvas_penpot_bridge", Args: `{"subaction":"update_state"}`, Env: nil, Want: ""},
	})
}

func TestStoreStatusReportsMissingCheckpoints(t *testing.T) {
	dir := ownStore(t)
	srv := mcptest.ToolsWith(t, nil, "")
	if out, err := exec.Command("sqlite3", filepath.Join(dir, "meta.db"), "DROP TABLE checkpoints").CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, out)
	}
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{{Name: "status", Tool: "dendrite_store_status", Args: `{"project":"p"}`, Env: nil, Want: "listing checkpoints"}})
}

func TestPenpotSyncAndModelMigrationDryRuns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv := mcptest.ToolsWith(t, nil, "")

	design := filepath.Join(t.TempDir(), "Button.tsx")
	if err := os.WriteFile(design, []byte("export const Button = () => null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_ui_penpot_sync", `{"file":"`+design+`","dry_run":true,"comment_plane":false}`); isErr {
		t.Errorf("penpot sync: %s", text)
	}

	manifest := filepath.Join(home, ".ollama", "models", "manifests", "registry.ollama.ai", "library", "tiny")
	if err := os.MkdirAll(manifest, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]interface{}{"schemaVersion": 2, "config": map[string]interface{}{"digest": "sha256:aa", "size": 2}, "layers": []interface{}{}})
	if err := os.WriteFile(filepath.Join(manifest, "latest"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_inference_migrate_model", `{"model":"tiny","dry_run":true}`); isErr {
		t.Errorf("model migration: %s", text)
	}
}

func TestRemoteSyncReportsAConflictingRebase(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	local := filepath.Join(root, "local")
	other := filepath.Join(root, "other")
	gitIn(t, root, "init", "-q", "--bare", "-b", "main", remote)
	gitIn(t, root, "clone", "-q", remote, local)
	if err := os.WriteFile(filepath.Join(local, "f.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, local, "add", ".")
	gitIn(t, local, "commit", "-q", "-m", "base")
	gitIn(t, local, "push", "-q", "origin", "HEAD:main")
	gitIn(t, local, "branch", "-M", "main")
	gitIn(t, local, "branch", "--set-upstream-to=origin/main", "main")

	gitIn(t, root, "clone", "-q", remote, other)
	if err := os.WriteFile(filepath.Join(other, "f.txt"), []byte("remote change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, other, "commit", "-q", "-am", "remote")
	gitIn(t, other, "push", "-q", "origin", "HEAD:main")

	if err := os.WriteFile(filepath.Join(local, "f.txt"), []byte("local change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, local, "commit", "-q", "-am", "local")

	t.Chdir(local)
	srv := mcptest.ToolsWith(t, nil, "")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{{Name: "remote sync", Tool: "dendrite_remote_sync", Args: `{"branch":"main"}`, Env: nil, Want: "remote sync failed"}})
	gitIn(t, local, "rebase", "--abort")
}

func TestQualityAuditReportsAScanThatCannotRun(t *testing.T) {
	t.Chdir(t.TempDir()) // not a git repository
	srv := mcptest.ToolsWith(t, nil, "")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{{Name: "quality audit", Tool: "dendrite_quality_audit", Args: `{"pkg":"pkg/none"}`, Env: nil, Want: "anti-mock scan failed"}})
}

func TestTools_Store(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_store_status
	stReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_store_status", "arguments": {"project": "proj-store"}}}`
	stResp := srv.HandleRequest(context.Background(), []byte(stReq))
	if stResp == nil || stResp.Error != nil {
		t.Fatalf("store_status failed: %v", stResp)
	}

	// 2. dendrite_store_commit_chunk (type: "file", is_final: false)
	chunkReq1 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_store_commit_chunk", "arguments": {"project": "proj-store", "chunk_id": "chunk-1", "identifier": "main.go", "data": "package main", "type": "file", "is_final": false}}}`
	chunkResp1 := srv.HandleRequest(context.Background(), []byte(chunkReq1))
	if chunkResp1 == nil || chunkResp1.Error != nil {
		t.Fatalf("commit_chunk 1 failed: %v", chunkResp1)
	}

	// 3. dendrite_store_commit_chunk (type: "", is_final: true)
	chunkReq2 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_store_commit_chunk", "arguments": {"project": "proj-store", "chunk_id": "chunk-2", "identifier": "helper.go", "data": "package helper", "is_final": true}}}`
	chunkResp2 := srv.HandleRequest(context.Background(), []byte(chunkReq2))
	if chunkResp2 == nil || chunkResp2.Error != nil {
		t.Fatalf("commit_chunk 2 failed: %v", chunkResp2)
	}

	// 4. dendrite_store_query_partial
	qpReq := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_store_query_partial", "arguments": {"project": "proj-store"}}}`
	qpResp := srv.HandleRequest(context.Background(), []byte(qpReq))
	if qpResp == nil || qpResp.Error != nil {
		t.Fatalf("query_partial failed: %v", qpResp)
	}

	// 5. dendrite_store_inspect
	inspReq := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_store_inspect", "arguments": {"store_id": "proj-store"}}}`
	inspResp := srv.HandleRequest(context.Background(), []byte(inspReq))
	if inspResp == nil {
		t.Fatal("expected response for store_inspect")
	}

	// 6. dendrite_store_rm (force: false and force: true)
	rmReq1 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_store_rm", "arguments": {"store_id": "proj-store", "force": false}}}`
	rmResp1 := srv.HandleRequest(context.Background(), []byte(rmReq1))
	if rmResp1 == nil {
		t.Fatal("expected response for store_rm force:false")
	}

	rmReq2 := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_store_rm", "arguments": {"store_id": "proj-store", "force": true}}}`
	rmResp2 := srv.HandleRequest(context.Background(), []byte(rmReq2))
	if rmResp2 == nil {
		t.Fatal("expected response for store_rm force:true")
	}
}

func TestTools_Scheduler(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_analyze_policy: sprint_before_reset invalid duration error
	polReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_analyze_policy", "arguments": {"sprint_before_reset": "invalid-dur"}}}`
	polResp1 := srv.HandleRequest(context.Background(), []byte(polReq1))
	if res, ok := polResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for invalid duration in policy, got: %#v", polResp1)
	}

	// 2. dendrite_analyze_policy: valid options
	polReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_analyze_policy", "arguments": {"night_window": "23:00-06:00", "weekly_reset": "SUN 23:59", "sprint_before_reset": "48h", "reserve_floor": 0.20, "allow_zero": true, "mode": "auto"}}}`
	polResp2 := srv.HandleRequest(context.Background(), []byte(polReq2))
	if polResp2 == nil || polResp2.Error != nil {
		t.Fatalf("analyze_policy failed: %v", polResp2)
	}

	// 3. dendrite_analyze_status
	statReq := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_analyze_status", "arguments": {"remaining_quota": 0.75}}}`
	statResp := srv.HandleRequest(context.Background(), []byte(statReq))
	if statResp == nil || statResp.Error != nil {
		t.Fatalf("analyze_status failed: %v", statResp)
	}

	// 4. dendrite_analyze_trigger: missing chunk_id error
	trigReq1 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {}}}`
	trigResp1 := srv.HandleRequest(context.Background(), []byte(trigReq1))
	if res, ok := trigResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing chunk_id, got: %#v", trigResp1)
	}

	// 5. dendrite_analyze_trigger: simulate_night and simulate_sprint
	trigReq2 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {"chunk_id": "chunk-101", "simulate_night": true}}}`
	trigResp2 := srv.HandleRequest(context.Background(), []byte(trigReq2))
	if trigResp2 == nil || trigResp2.Error != nil {
		t.Fatalf("analyze_trigger simulate_night failed: %v", trigResp2)
	}

	trigReq3 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {"chunk_id": "chunk-102", "simulate_sprint": true}}}`
	trigResp3 := srv.HandleRequest(context.Background(), []byte(trigReq3))
	if trigResp3 == nil || trigResp3.Error != nil {
		t.Fatalf("analyze_trigger simulate_sprint failed: %v", trigResp3)
	}

	// 6. dendrite_analyze_switch
	swReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_analyze_switch", "arguments": {"allow_zero": true, "mode": "always"}}}`
	swResp := srv.HandleRequest(context.Background(), []byte(swReq))
	if swResp == nil || swResp.Error != nil {
		t.Fatalf("analyze_switch failed: %v", swResp)
	}

	// 7. dendrite_tokens_status
	tokReq := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_tokens_status", "arguments": {"model": "gemini-3.8-flash-high", "probe": false}}}`
	tokResp := srv.HandleRequest(context.Background(), []byte(tokReq))
	if tokResp == nil || tokResp.Error != nil {
		t.Fatalf("tokens_status failed: %v", tokResp)
	}
}

func TestTools_Contract(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. Missing schema_dirs error
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_contract_drift_check", "arguments": {}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if res, ok := resp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing schema_dirs, got: %#v", resp1)
	}

	// 2. Valid contract drift check
	tmpDir := t.TempDir()
	schemaDir := filepath.Join(tmpDir, "schemas")
	_ = os.MkdirAll(schemaDir, 0755)
	_ = os.WriteFile(filepath.Join(schemaDir, "test.proto"), []byte(`syntax = "proto3"; message Test { string id = 1; }`), 0644)

	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_contract_drift_check", "arguments": {"schema_dirs": [%q], "code_dirs": {"go": %q}}}}`, schemaDir, tmpDir)
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("contract_drift_check failed: %v", resp2)
	}
}

func TestTools_Story(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_story_fork: missing story_id
	fReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_story_fork", "arguments": {"variants": ["v1", "v2"]}}}`
	fResp1 := srv.HandleRequest(context.Background(), []byte(fReq1))
	if res, ok := fResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing story_id in story_fork, got: %#v", fResp1)
	}

	// 2. dendrite_story_fork: < 2 variants
	fReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_story_fork", "arguments": {"story_id": "DEN-064", "variants": ["v1"]}}}`
	fResp2 := srv.HandleRequest(context.Background(), []byte(fReq2))
	if res, ok := fResp2.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for < 2 variants in story_fork, got: %#v", fResp2)
	}

	// 3. dendrite_story_benchmark: missing story_id
	bReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_story_benchmark", "arguments": {}}}`
	bResp1 := srv.HandleRequest(context.Background(), []byte(bReq1))
	if res, ok := bResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing story_id in story_benchmark, got: %#v", bResp1)
	}

	// 4. dendrite_story_resolve: missing story_id or winner
	rReq1 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_story_resolve", "arguments": {"story_id": "DEN-064"}}}`
	rResp1 := srv.HandleRequest(context.Background(), []byte(rReq1))
	if res, ok := rResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing winner in story_resolve, got: %#v", rResp1)
	}

	// 5. dendrite_story_abort: missing story_id
	aReq1 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_story_abort", "arguments": {}}}`
	aResp1 := srv.HandleRequest(context.Background(), []byte(aReq1))
	if res, ok := aResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing story_id in story_abort, got: %#v", aResp1)
	}
}

func TestTools_Audit(t *testing.T) {
	chdirToScratchRepo(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_vcs_guard (STANDARD and DEGRADED_BUDGET)
	gReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_vcs_guard", "arguments": {"mode": "STANDARD"}}}`
	gResp1 := srv.HandleRequest(context.Background(), []byte(gReq1))
	if gResp1 == nil || gResp1.Error != nil {
		t.Fatalf("vcs_guard STANDARD failed: %v", gResp1)
	}

	gReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_vcs_guard", "arguments": {"mode": "DEGRADED_BUDGET"}}}`
	gResp2 := srv.HandleRequest(context.Background(), []byte(gReq2))
	if gResp2 == nil || gResp2.Error != nil {
		t.Fatalf("vcs_guard DEGRADED_BUDGET failed: %v", gResp2)
	}

	// 2. dendrite_quality_audit: missing pkg error
	qaReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_quality_audit", "arguments": {}}}`
	qaResp1 := srv.HandleRequest(context.Background(), []byte(qaReq1))
	if res, ok := qaResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing pkg, got: %#v", qaResp1)
	}

	// 3. dendrite_quality_audit: valid
	qaReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_quality_audit", "arguments": {"pkg": "pkg/mcpserver"}}}`
	qaResp2 := srv.HandleRequest(context.Background(), []byte(qaReq2))
	if qaResp2 == nil {
		t.Fatal("expected response for quality_audit")
	}

	// 4. dendrite_audit_changes
	acReq := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_audit_changes", "arguments": {"dry_run": true, "max_tokens": 2048, "model": "qwen3.8:27b"}}}`
	acResp := srv.HandleRequest(context.Background(), []byte(acReq))
	if acResp == nil {
		t.Fatal("expected response for audit_changes")
	}

	// 5. dendrite_hitl_sync (offline: local Plane server and scripted gh)
	offlineHITL(t)
	hitlReq := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_hitl_sync", "arguments": {}}}`
	hitlResp := srv.HandleRequest(context.Background(), []byte(hitlReq))
	if hitlResp == nil {
		t.Fatal("expected response for hitl_sync")
	}

	// 6. dendrite_remote_sync
	rsReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_remote_sync", "arguments": {"branch": "develop"}}}`
	rsResp := srv.HandleRequest(context.Background(), []byte(rsReq))
	if rsResp == nil {
		t.Fatal("expected response for remote_sync")
	}

	// 7. dendrite_github_sync
	ghReq := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_github_sync", "arguments": {"repo": "yasuomaidana/dendrite"}}}`
	ghResp := srv.HandleRequest(context.Background(), []byte(ghReq))
	if ghResp == nil {
		t.Fatal("expected response for github_sync")
	}
}

func TestTools_InferenceAndPenpot(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_inference_migrate_model: missing model error
	mReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_inference_migrate_model", "arguments": {}}}`
	mResp1 := srv.HandleRequest(context.Background(), []byte(mReq1))
	if res, ok := mResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing model, got: %#v", mResp1)
	}

	// 2. dendrite_inference_migrate_model: valid
	mReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_inference_migrate_model", "arguments": {"model": "qwen2.5:7b", "source_engine": "ollama", "target_engine": "oxillama", "delete_source": false, "dry_run": true}}}`
	mResp2 := srv.HandleRequest(context.Background(), []byte(mReq2))
	if mResp2 == nil {
		t.Fatal("expected response for migrate_model")
	}

	// 3. dendrite_ui_penpot_sync: missing file error
	pReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_ui_penpot_sync", "arguments": {}}}`
	pResp1 := srv.HandleRequest(context.Background(), []byte(pReq1))
	if res, ok := pResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing file, got: %#v", pResp1)
	}

	// 4. dendrite_ui_penpot_sync: valid
	pReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_ui_penpot_sync", "arguments": {"file": "Button.svelte", "ticket_id": "DEN-810", "dry_run": true, "comment_plane": false}}}`
	pResp2 := srv.HandleRequest(context.Background(), []byte(pReq2))
	if pResp2 == nil {
		t.Fatal("expected response for ui_penpot_sync")
	}
}

func TestTools_MigrateGreetingEnv(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_service_move: missing target_host error
	smReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {}}}`
	smResp1 := srv.HandleRequest(context.Background(), []byte(smReq1))
	if res, ok := smResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing target_host, got: %#v", smResp1)
	}

	// 2. dendrite_service_move: valid dry_run
	smReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {"target_host": "127.0.0.1", "services": ["plane"], "ssh_port": 22, "target_ram_mb": 4096, "target_disk_gb": 500, "dry_run": true, "prune_local": false, "timeout_seconds": 60}}}`
	smResp2 := srv.HandleRequest(context.Background(), []byte(smReq2))
	if smResp2 == nil {
		t.Fatal("expected response for service_move")
	}

	// 3. dendrite_service_plan
	planReq := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_service_plan", "arguments": {"ram_mb": 4096, "disk_gb": 1000}}}`
	planResp := srv.HandleRequest(context.Background(), []byte(planReq))
	if planResp == nil || planResp.Error != nil {
		t.Fatalf("service_plan failed: %v", planResp)
	}

	// 4. dendrite_pcontext_greeting_generate
	gReq := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_pcontext_greeting_generate", "arguments": {"topic": "Refactoring", "decision": "architecture", "milestone": "M1", "milestone_achieved": true, "event": "Launch", "days_until_event": 5, "unfinished_step": "test", "specific_challenge": "speed", "curiosity_angle": "wasm", "days_since_last_active": 2, "turn_count": 3, "high_stress": false, "low_pressure_hobby": true, "options": ["A", "B"], "recent_styles": ["G-01"]}}}`
	gResp := srv.HandleRequest(context.Background(), []byte(gReq))
	if gResp == nil || gResp.Error != nil {
		t.Fatalf("greeting_generate failed: %v", gResp)
	}

	// 5. dendrite_env_prepare (dry_run: true)
	tmpDir := t.TempDir()
	envReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_env_prepare", "arguments": {"project_path": %q, "default_tool": "copilot", "enforce_cicd": true, "dry_run": true}}}`, tmpDir)
	envResp := srv.HandleRequest(context.Background(), []byte(envReq))
	if envResp == nil || envResp.Error != nil {
		t.Fatalf("env_prepare failed: %v", envResp)
	}
}

func TestTools_CanvasAndRAG(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_canvas_action_dispatch: missing action error
	cReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_canvas_action_dispatch", "arguments": {"prompt": "write test"}}}`
	cResp1 := srv.HandleRequest(context.Background(), []byte(cReq1))
	if res, ok := cResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing action, got: %#v", cResp1)
	}

	// 2. dendrite_canvas_action_dispatch: valid
	cReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_canvas_action_dispatch", "arguments": {"action": "WRITE_PLAN", "mode": "DRAFT", "title": "Test Plan", "prompt": "Outline tasks"}}}`
	cResp2 := srv.HandleRequest(context.Background(), []byte(cReq2))
	if cResp2 == nil || cResp2.Error != nil {
		t.Fatalf("canvas action dispatch failed: %v", cResp2)
	}

	// 3. dendrite_canvas_penpot_bridge subactions
	// relate
	subReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "relate", "ticket_id": "DEN-100", "feature_name": "Canvas", "file_path": "App.svelte"}}}`
	subResp1 := srv.HandleRequest(context.Background(), []byte(subReq1))
	if subResp1 == nil || subResp1.Error != nil {
		t.Fatalf("penpot bridge relate failed: %v", subResp1)
	}

	// sync
	subReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "sync", "ticket_id": "DEN-100", "file_path": "App.svelte", "dry_run": true}}}`
	subResp2 := srv.HandleRequest(context.Background(), []byte(subReq2))
	if subResp2 == nil || subResp2.Error != nil {
		t.Fatalf("penpot bridge sync failed: %v", subResp2)
	}

	// update_state
	subReq3 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "update_state", "ticket_id": "DEN-100", "target_state": "Done", "push_to_plane": false}}}`
	subResp3 := srv.HandleRequest(context.Background(), []byte(subReq3))
	if subResp3 == nil || subResp3.Error != nil {
		t.Fatalf("penpot bridge update_state failed: %v", subResp3)
	}

	// list
	subReq4 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "list"}}}`
	subResp4 := srv.HandleRequest(context.Background(), []byte(subReq4))
	if subResp4 == nil || subResp4.Error != nil {
		t.Fatalf("penpot bridge list failed: %v", subResp4)
	}

	// unknown subaction error
	subReq5 := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "invalid_subaction"}}}`
	subResp5 := srv.HandleRequest(context.Background(), []byte(subReq5))
	if res, ok := subResp5.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for unknown subaction, got: %#v", subResp5)
	}

	// 4. dendrite_rag_ask: missing question error
	tmpDir := t.TempDir()
	rReq1 := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_rag_ask", "arguments": {}}}`
	rResp1 := srv.HandleRequest(context.Background(), []byte(rReq1))
	if res, ok := rResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing question in rag_ask, got: %#v", rResp1)
	}

	// dendrite_rag_ask: valid question
	rReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": {"name": "dendrite_rag_ask", "arguments": {"question": "How does server work?", "project_path": %q, "auto_approve_paid": false, "top_k": 2}}}`, tmpDir)
	rResp2 := srv.HandleRequest(context.Background(), []byte(rReq2))
	if rResp2 == nil {
		t.Fatal("expected response for rag_ask")
	}

	// 5. dendrite_rag_retrieve: missing query error
	retReq1 := `{"jsonrpc": "2.0", "id": 10, "method": "tools/call", "params": {"name": "dendrite_rag_retrieve", "arguments": {}}}`
	retResp1 := srv.HandleRequest(context.Background(), []byte(retReq1))
	if res, ok := retResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing query in rag_retrieve, got: %#v", retResp1)
	}

	// dendrite_rag_retrieve: valid query
	retReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 11, "method": "tools/call", "params": {"name": "dendrite_rag_retrieve", "arguments": {"query": "Server", "project_path": %q, "top_k": 2, "min_score": 0.001}}}`, tmpDir)
	retResp2 := srv.HandleRequest(context.Background(), []byte(retReq2))
	if retResp2 == nil {
		t.Fatal("expected response for rag_retrieve")
	}
}

func TestMCP_CanvasTools(t *testing.T) {
	ctx := context.Background()

	// Mock Plane Server
	planeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/states/") {
			states := []plane.PlaneState{
				{ID: "state-progress", Name: "In Progress", Group: "started"},
				{ID: "state-verify", Name: "To Verify", Group: "started"},
			}
			_ = json.NewEncoder(w).Encode(states)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/issues/") {
			issues := []plane.PlaneIssue{
				{ID: "issue-807", Name: "[DEN-807] Canvas Interactive Writing"},
			}
			_ = json.NewEncoder(w).Encode(issues)
			return
		}
		if strings.Contains(r.URL.Path, "/comments/") {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "cmt-1"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "ok"})
	}))
	defer planeServer.Close()

	planeClient, _ := plane.NewClient(plane.Config{
		BaseURL:       planeServer.URL,
		WorkspaceSlug: "dendrite-ws",
		ProjectID:     "proj-test-1",
		DirectToken:   "plane_token",
	}, nil)

	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	deps := Dependencies{
		PlaneClient: planeClient,
	}
	opstools.RegisterCanvasTools(srv, deps)

	// 1. Test dendrite_canvas_action_dispatch (WRITE_PLAN)
	t.Run("Dispatch_WritePlan", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_action_dispatch",
				"arguments": {
					"action": "WRITE_PLAN",
					"title": "UI Architecture Rollout",
					"prompt": "Deliver Penpot Bridge and Plane state synchronization",
					"mode": "DRAFT"
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call dendrite_canvas_action_dispatch failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "UI Architecture Rollout") {
			t.Errorf("expected plan output, got: %v", resp.Result)
		}
	})

	// 2. Test dendrite_canvas_action_dispatch (DESIGN_UI_COMPONENT)
	t.Run("Dispatch_DesignUI", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_action_dispatch",
				"arguments": {
					"action": "DESIGN_UI_COMPONENT",
					"prompt": "Interactive Canvas Workspace with side drawer",
					"parameters": {
						"framework": "svelte",
						"component_name": "WorkspaceCanvas"
					}
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call design UI failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "WorkspaceCanvas") {
			t.Errorf("expected WorkspaceCanvas in output: %v", resp.Result)
		}
	})

	// 3. Test dendrite_canvas_penpot_bridge (relate)
	t.Run("PenpotBridge_Relate", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 3,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_penpot_bridge",
				"arguments": {
					"subaction": "relate",
					"ticket_id": "DEN-807",
					"feature_name": "Canvas Workspace",
					"comment": "Binding Plane ticket to Penpot design frame"
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call relate failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "DEN-807") {
			t.Errorf("expected DEN-807 in output: %v", resp.Result)
		}
	})

	// 4. Test dendrite_canvas_penpot_bridge (sync)
	t.Run("PenpotBridge_Sync", func(t *testing.T) {
		tmpDir := t.TempDir()
		uiPath := filepath.Join(tmpDir, "Header.svelte")
		_ = os.WriteFile(uiPath, []byte(`<header class="bg-slate-900 p-4 text-white">Header</header>`), 0644)

		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 4,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_penpot_bridge",
				"arguments": {
					"subaction": "sync",
					"ticket_id": "DEN-807",
					"file_path": "` + uiPath + `",
					"dry_run": true
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call sync failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "Header") {
			t.Errorf("expected Header in sync output: %v", resp.Result)
		}
	})

	// 5. Test dendrite_canvas_penpot_bridge (update_state)
	t.Run("PenpotBridge_UpdateState", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 5,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_penpot_bridge",
				"arguments": {
					"subaction": "update_state",
					"ticket_id": "DEN-807",
					"target_state": "To Verify",
					"comment": "Verification completed in Penpot frame",
					"push_to_plane": true
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call update_state failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "To Verify") {
			t.Errorf("expected To Verify in output: %v", resp.Result)
		}
	})

	// 6. Test dendrite_canvas_penpot_bridge (list)
	t.Run("PenpotBridge_List", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 6,
			"method": "tools/call",
			"params": {
				"name": "dendrite_canvas_penpot_bridge",
				"arguments": {
					"subaction": "list"
				}
			}
		}`
		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("call list failed: %v", resp.Error)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok || !strings.Contains(res.Content[0].Text, "DEN-807") {
			t.Errorf("expected list to contain DEN-807: %v", resp.Result)
		}
	})
}

func TestStoryForkTools_RegistrationAndExecution(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	expectedTools := []string{
		"dendrite_story_fork",
		"dendrite_story_benchmark",
		"dendrite_story_resolve",
		"dendrite_story_abort",
	}

	registered := make(map[string]bool)
	for _, tool := range srv.ListTools() {
		registered[tool.Name] = true
	}

	for _, name := range expectedTools {
		if !registered[name] {
			t.Errorf("expected MCP tool %s to be registered", name)
		}
	}

	ctx := context.Background()

	// Test validation errors mcptest.On handler calls
	forkHandler, _ := srv.Handler("dendrite_story_fork")
	_, err := forkHandler(ctx, map[string]interface{}{"story_id": ""})
	if err == nil {
		t.Error("expected error mcptest.On missing story_id")
	}

	benchHandler, _ := srv.Handler("dendrite_story_benchmark")
	_, err = benchHandler(ctx, map[string]interface{}{"story_id": ""})
	if err == nil {
		t.Error("expected error mcptest.On missing story_id")
	}

	resolveHandler, _ := srv.Handler("dendrite_story_resolve")
	_, err = resolveHandler(ctx, map[string]interface{}{"story_id": "", "winner": ""})
	if err == nil {
		t.Error("expected error mcptest.On missing parameters")
	}

	abortHandler, _ := srv.Handler("dendrite_story_abort")
	_, err = abortHandler(ctx, map[string]interface{}{"story_id": ""})
	if err == nil {
		t.Error("expected error mcptest.On missing story_id")
	}

	// Abort mcptest.On nonexistent story should succeed cleanly
	res, err := abortHandler(ctx, map[string]interface{}{"story_id": "nonexistent-story-123"})
	if err != nil {
		t.Fatalf("abort handler failed: %v", err)
	}
	if len(res.Content) == 0 {
		t.Error("expected content in abort response")
	}
}

func TestStoryLifecycleThroughTheTools(t *testing.T) {
	storyRepo(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	text, isErr := mcptest.CallTool(t, srv, "dendrite_story_fork", `{"story_id":"s1","variants":["v1","","v2",3]}`)
	if isErr || !strings.Contains(text, "v1") || !strings.Contains(text, "v2") {
		t.Fatalf("fork: %s (error %v)", text, isErr)
	}
	if text, isErr = mcptest.CallTool(t, srv, "dendrite_story_benchmark", `{"story_id":"s1","iterations":5,"virtual_users":2}`); isErr || !strings.Contains(text, "recommended_winner") {
		t.Fatalf("benchmark: %s (error %v)", text, isErr)
	}
	if text, isErr = mcptest.CallTool(t, srv, "dendrite_story_resolve", `{"story_id":"s1","winner":"v1"}`); isErr {
		t.Fatalf("resolve: %s", text)
	}
}

func TestStoryToolsReportWhatTheControllerRefuses(t *testing.T) {
	storyRepo(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "fork with one variant", Tool: "dendrite_story_fork", Args: `{"story_id":"s","variants":["only-one"]}`, Env: nil, Want: "at least 2"},
		{Name: "fork with a repeated variant", Tool: "dendrite_story_fork", Args: `{"story_id":"s","variants":["a","a"]}`, Env: nil, Want: "story fork failed"},
		{Name: "benchmark of an unknown story", Tool: "dendrite_story_benchmark", Args: `{"story_id":"never-forked"}`, Env: nil, Want: "story benchmark failed"},
		{Name: "resolve of an unknown story", Tool: "dendrite_story_resolve", Args: `{"story_id":"never-forked","winner":"v1"}`, Env: nil, Want: "story resolve failed"},
	})
}

func TestStoryAbortReportsADirectoryItCannotRemove(t *testing.T) {
	stories := storyRepo(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	// A story with a manifest is aborted by removing its directory.
	if _, isErr := mcptest.CallTool(t, srv, "dendrite_story_fork", `{"story_id":"stuck2","variants":["a","b"]}`); isErr {
		t.Fatal("fork failed")
	}
	if err := os.Chmod(filepath.Join(stories, "stuck2"), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(stories, "stuck2"), 0o755) })
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_story_abort", `{"story_id":"stuck2"}`); !isErr || !strings.Contains(text, "story abort failed") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestStoreToolsWithoutAStoreRegisterNothing(t *testing.T) {
	old := opstools.DefaultMCPStore
	opstools.DefaultMCPStore = nil
	t.Cleanup(func() { opstools.DefaultMCPStore = old })
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	for _, tool := range srv.ListTools() {
		if strings.HasPrefix(tool.Name, "dendrite_store_") {
			t.Errorf("%s must not be registered without a store", tool.Name)
		}
	}
}

func TestStoreToolsCountChunksAndSegments(t *testing.T) {
	ownStore(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	for _, args := range []string{
		`{"project":"p","chunk_id":"c1","identifier":"a.go","data":"package a","is_final":true}`,
		`{"project":"p","chunk_id":"c2","identifier":"b.go","data":"package b","type":"function"}`,
	} {
		if text, isErr := mcptest.CallTool(t, srv, "dendrite_store_commit_chunk", args); isErr {
			t.Fatalf("commit: %s", text)
		}
	}
	text, isErr := mcptest.CallTool(t, srv, "dendrite_store_status", `{"project":"p"}`)
	if isErr || !strings.Contains(text, `"stored_chunks": 1`) || !strings.Contains(text, `"partial_chunks": 1`) || !strings.Contains(text, `"total_segments": 2`) {
		t.Fatalf("status: %s", text)
	}
	if text, isErr = mcptest.CallTool(t, srv, "dendrite_store_query_partial", `{"project":"p"}`); isErr {
		t.Fatalf("query: %s", text)
	}
}

func TestStoreToolsReportAStoreThatCannotBeRead(t *testing.T) {
	dir := ownStore(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	// A database path that is a directory cannot be opened.
	db := filepath.Join(dir, "meta.db")
	if err := os.RemoveAll(db); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(db, 0o755); err != nil {
		t.Fatal(err)
	}
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "status", Tool: "dendrite_store_status", Args: `{"project":"p"}`, Env: nil, Want: "querying segments"},
		{Name: "commit", Tool: "dendrite_store_commit_chunk", Args: `{"project":"p","chunk_id":"c","identifier":"i","data":"d"}`, Env: nil, Want: "Failed to commit chunk"},
		{Name: "query", Tool: "dendrite_store_query_partial", Args: `{"project":"p"}`, Env: nil, Want: "querying partial segments"},
		{Name: "inspect", Tool: "dendrite_store_inspect", Args: `{"store_id":"p"}`, Env: nil, Want: "Inspect failed"},
		{Name: "remove", Tool: "dendrite_store_rm", Args: `{"store_id":"p","force":true}`, Env: nil, Want: "Removal failed"},
	})
}

func TestAgentRunReportListsTheSession(t *testing.T) {
	got := opstools.AgentRunReport(&agent.SessionSummary{
		SessionID: "s1", AgentType: agent.AgentAider, TicketID: "DEN-1", Duration: 3 * time.Second,
		TokensConsumed: 120, EstimatedCostUSD: 0.5, CommitsGenerated: []string{"abc"}, Status: "ok",
	}, []string{"aider", "--yes"}, map[string]string{"K": "V"})
	if got["command"] != "aider --yes" || got["duration"] != "3s" || got["tokens"] != 120 || got["status"] != "ok" {
		t.Fatalf("report = %v", got)
	}
}

func TestAgentRunRejectsIncompleteAndFallsBackToSandboxForOpenHands(t *testing.T) {
	srv := mcptest.ToolsWith(t, nil, "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_agent_run", `{"agent_type":"aider","task":"t","spend_limit":-1}`); !isErr || !strings.Contains(text, "failed to build agent env") {
		t.Errorf("a negative spend limit: %q (error %v)", text, isErr)
	}
	t.Setenv("PATH", t.TempDir()) // no openhands binary to run mcptest.On the host
	mcptest.CallTool(t, srv, "dendrite_agent_run", `{"agent_type":"openhands","task":"t","sandbox":false}`)
}

func TestInferenceModelsToolAssessesTheMachine(t *testing.T) {
	srv := mcptest.ToolsWith(t, nil, "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_inference_models", `{}`); isErr || !strings.Contains(text, "budget_bytes") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestAgentRunReportsASessionThatSucceeds(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "aider"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	workdir := t.TempDir()
	t.Chdir(workdir)
	srv := mcptest.ToolsWith(t, nil, "")
	text, isErr := mcptest.CallTool(t, srv, "dendrite_agent_run", `{"agent_type":"aider","task":"say hi","ticket_id":"DEN-1","model":"m"}`)
	if isErr || !strings.Contains(text, `"session_id"`) {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestMCP_MigrateTools(t *testing.T) {
	srv := NewServer(ServerConfig{
		Name:    "test-dendrite",
		Version: "0.1.0",
	})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	ctx := context.Background()

	// 1. Test dendrite_service_plan
	planReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_service_plan", "arguments": {"ram_mb": 4096, "disk_gb": 1000}}}`
	respPlan := srv.HandleRequest(ctx, []byte(planReq))
	if respPlan == nil || respPlan.Error != nil {
		t.Fatalf("unexpected error calling dendrite_service_plan: %v", respPlan)
	}

	callRes, ok := respPlan.Result.(*CallToolResult)
	if !ok || len(callRes.Content) == 0 {
		t.Fatalf("expected valid CallToolResult with content")
	}

	var planData map[string]interface{}
	if err := json.Unmarshal([]byte(callRes.Content[0].Text), &planData); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if recCmd, ok := planData["recommended_action"].(string); !ok || !strings.Contains(recCmd, "dendrite translocate run") {
		t.Errorf("expected recommended action with dendrite move, got %v", recCmd)
	}

	// 2. Test dendrite_service_move in dry-run mode
	moveReq := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {"services": ["plane"], "target_host": "user@debian-server", "dry_run": true}}}`
	respMove := srv.HandleRequest(ctx, []byte(moveReq))
	if respMove == nil || respMove.Error != nil {
		t.Fatalf("unexpected error calling dendrite_service_move: %v", respMove)
	}

	callMoveRes, ok := respMove.Result.(*CallToolResult)
	if !ok || len(callMoveRes.Content) == 0 {
		t.Fatalf("expected valid CallToolResult for move")
	}

	var moveData map[string]interface{}
	if err := json.Unmarshal([]byte(callMoveRes.Content[0].Text), &moveData); err != nil {
		t.Fatalf("failed to parse json move response: %v", err)
	}
	if success, ok := moveData["success"].(bool); !ok || !success {
		t.Errorf("expected dry-run move to succeed, got %v", moveData)
	}
}

func TestMCP_GreetingTools(t *testing.T) {
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	catalog.RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
	})
	ctx := context.Background()

	t.Run("generate greeting via MCP", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 101,
			"method": "tools/call",
			"params": {
				"name": "dendrite_pcontext_greeting_generate",
				"arguments": {
					"topic": "Distributed Mesh",
					"milestone": "Phase 1 Complete",
					"milestone_achieved": true
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

		var gen greeting.GeneratedGreeting
		if err := json.Unmarshal([]byte(res.Content[0].Text), &gen); err != nil {
			t.Fatalf("failed to unmarshal output: %v", err)
		}

		if gen.StyleID != greeting.StyleProgressCelebration {
			t.Errorf("expected G-02, got %s", gen.StyleID)
		}
		if !strings.HasPrefix(gen.Headline, "# ") {
			t.Errorf("expected headline to start with '# ', got %s", gen.Headline)
		}
	})

	t.Run("suppressed style via MCP", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 102,
			"method": "tools/call",
			"params": {
				"name": "dendrite_pcontext_greeting_generate",
				"arguments": {
					"topic": "Distributed Mesh",
					"milestone": "Phase 1 Complete",
					"milestone_achieved": true,
					"recent_styles": ["G-02"]
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

		var gen greeting.GeneratedGreeting
		if err := json.Unmarshal([]byte(res.Content[0].Text), &gen); err != nil {
			t.Fatalf("failed to unmarshal output: %v", err)
		}

		if gen.StyleID == greeting.StyleProgressCelebration {
			t.Errorf("expected G-02 to be suppressed, got %s", gen.StyleID)
		}
	})

	t.Run("all styles suppressed relaxes suppression instead of failing", func(t *testing.T) {
		reqJSON := `{
			"jsonrpc": "2.0",
			"id": 103,
			"method": "tools/call",
			"params": {
				"name": "dendrite_pcontext_greeting_generate",
				"arguments": {
					"topic": "Distributed Mesh",
					"recent_styles": ["G-01", "G-02", "G-03", "G-04", "G-05", "G-06", "G-07", "G-08", "G-09", "G-10"]
				}
			}
		}`

		resp := srv.HandleRequest(ctx, []byte(reqJSON))
		if resp.Error != nil {
			t.Fatalf("HandleRequest failed: %v", resp.Error)
		}

		res, ok := resp.Result.(*CallToolResult)
		if !ok {
			t.Fatalf("expected *CallToolResult, got %T", resp.Result)
		}
		if res.IsError {
			t.Fatalf("expected a fallback greeting when every style is recent, got error %q", res.Content[0].Text)
		}
	})
}

func TestGitHubSyncAndHITLSyncTools(t *testing.T) {
	ps, gh := offlineHITL(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	text, isErr := mcptest.CallTool(t, srv, "dendrite_github_sync", `{"dry_run":true}`)
	if isErr || !strings.Contains(text, `"kind": "create"`) || !strings.Contains(text, `"dry_run": true`) {
		t.Fatalf("dry run: %s", text)
	}
	if gh.Called("gh issue create") != 0 {
		t.Fatal("dry run created an issue")
	}
	if text, _ = mcptest.CallTool(t, srv, "dendrite_github_sync", `{"dry_run":true,"tickets":["DEN-2",7]}`); strings.Contains(text, `"ticket_id": "DEN-1"`) {
		t.Fatalf("tickets filter ignored: %s", text)
	}
	if text, isErr = mcptest.CallTool(t, srv, "dendrite_github_sync", `{}`); isErr || gh.Called("gh issue create") != 1 {
		t.Fatalf("apply: %s", text)
	}

	text, isErr = mcptest.CallTool(t, srv, "dendrite_hitl_sync", `{"dry_run":true}`)
	var results []hitl.HITLResult
	if isErr || json.Unmarshal([]byte(text), &results) != nil || len(results) != 2 || len(ps.Patches) != 0 {
		t.Fatalf("hitl dry run: %s", text)
	}
	if _, isErr = mcptest.CallTool(t, srv, "dendrite_hitl_sync", `{}`); isErr || ps.Patches["u-2"] != "st-done" {
		t.Fatalf("hitl apply: patches %v", ps.Patches)
	}

	for _, tool := range []string{"dendrite_github_sync", "dendrite_hitl_sync"} {
		if text, isErr := mcptest.CallTool(t, srv, tool, `{"repo":"fail/me"}`); !isErr || !strings.Contains(text, "no plane") {
			t.Errorf("%s setup failure: %s", tool, text)
		}
	}
	gh.On("gh issue list", "", errors.New("offline"))
	for _, tool := range []string{"dendrite_github_sync", "dendrite_hitl_sync"} {
		if text, isErr := mcptest.CallTool(t, srv, tool, `{}`); !isErr || !strings.Contains(text, "offline") {
			t.Errorf("%s sync failure: %s", tool, text)
		}
	}
}

func TestHITLSyncToolReturnsEmptyList(t *testing.T) {
	ps := hitltest.NewPlaneServer()
	defer ps.Close()
	prev := opstools.NewHITLSyncer
	opstools.NewHITLSyncer = func(context.Context, Dependencies, string) (*hitl.Syncer, error) {
		return hitl.New(ps.Client(), github.NewGitHubClient("o/r").WithExecutor(&hitltest.GHScript{}), "."), nil
	}
	defer func() { opstools.NewHITLSyncer = prev }()
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_hitl_sync", `{}`); isErr || strings.TrimSpace(text) != "[]" {
		t.Fatalf("empty result = %q", text)
	}
}

func TestNewHITLSyncerUsesServerPlaneClient(t *testing.T) {
	ps := hitltest.NewPlaneServer()
	defer ps.Close()
	ctx := context.Background()
	s, err := opstools.NewHITLSyncer(ctx, Dependencies{PlaneClient: ps.Client()}, "o/explicit")
	if err != nil || s.GitHub.Repo != "o/explicit" {
		t.Fatalf("explicit repo: %+v %v", s, err)
	}
	// The repository's own origin remote is detected when no repo is given.
	if s, err := opstools.NewHITLSyncer(ctx, Dependencies{PlaneClient: ps.Client()}, ""); err != nil || !strings.Contains(s.GitHub.Repo, "/") {
		t.Fatalf("detected repo: %+v %v", s, err)
	}
	t.Chdir(t.TempDir())
	if _, err := opstools.NewHITLSyncer(ctx, Dependencies{PlaneClient: ps.Client()}, ""); err == nil {
		t.Error("expected detection failure outside a git repository")
	}
	t.Setenv("PLANE_API_KEY", "test-token")
	if s, err := opstools.NewHITLSyncer(ctx, Dependencies{}, "o/r"); err != nil || s.GitHub.Repo != "o/r" {
		t.Fatalf("env Plane client: %v", err)
	}
}

func TestTools_DendriteAgentRun(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. Invalid agent type
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_agent_run", "arguments": {"agent_type": "invalid_agent", "task": "test"}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if res, ok := resp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for invalid agent type, got: %#v", resp1)
	}

	// 2. Multiple valid agent types (model default branches, sandbox, spend_limit)
	agents := []string{"aider", "openhands", "opencode", "agy", "copilot"}
	for _, a := range agents {
		req := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_agent_run", "arguments": {"agent_type": "%s", "task": "run chore", "ticket_id": "DEN-001", "sandbox": true, "spend_limit": 1.50}}}`, a)
		resp := srv.HandleRequest(context.Background(), []byte(req))
		if resp == nil {
			t.Fatalf("expected response for agent %s", a)
		}
	}
}

func TestTools_DendriteTelemetryAndInference(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// dendrite_telemetry
	tReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_telemetry", "arguments": {}}}`
	tResp := srv.HandleRequest(context.Background(), []byte(tReq))
	if tResp == nil || tResp.Error != nil {
		t.Fatalf("telemetry error: %v", tResp)
	}

	// dendrite_inference_list
	lReq := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_inference_list", "arguments": {}}}`
	lResp := srv.HandleRequest(context.Background(), []byte(lReq))
	if lResp == nil || lResp.Error != nil {
		t.Fatalf("inference list error: %v", lResp)
	}

	// dendrite_inference_probe all
	pReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {}}}`
	pResp1 := srv.HandleRequest(context.Background(), []byte(pReq1))
	if pResp1 == nil || pResp1.Error != nil {
		t.Fatalf("inference probe all error: %v", pResp1)
	}

	// dendrite_inference_probe single engine
	pReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {"engine_id": "oxillama-rust"}}}`
	pResp2 := srv.HandleRequest(context.Background(), []byte(pReq2))
	if pResp2 == nil || pResp2.Error != nil {
		t.Fatalf("inference probe single error: %v", pResp2)
	}

	// dendrite_inference_probe non-existent engine
	pReq3 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {"engine_id": "non-existent-engine"}}}`
	pResp3 := srv.HandleRequest(context.Background(), []byte(pReq3))
	if res, ok := pResp3.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for non-existent engine, got: %#v", pResp3)
	}
}

func TestMCP_EnvPrepareTools(t *testing.T) {
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	catalog.RegisterDefaultTools(srv, Dependencies{
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
