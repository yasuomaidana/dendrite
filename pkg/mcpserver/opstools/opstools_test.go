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
	mcptest.MustHandle(t, srv, stressReq, "bench_run stress failed")

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
	mcptest.MustHandle(t, srv, genReq, "docusaurus_generate failed")

	// 2. dendrite_docusaurus_share
	bundlePath := filepath.Join(tmpDir, "bundle.tar.gz")
	shareReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_docusaurus_share", "arguments": {"site_dir": %q, "bundle_path": %q}}}`, siteDir, bundlePath)
	mcptest.MustHandle(t, srv, shareReq, "docusaurus_share failed")

	// 3. dendrite_docusaurus_audit
	auditReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_docusaurus_audit", "arguments": {"project_root": %q, "docs_dir": %q}}}`, tmpDir, docsDir)
	mcptest.MustHandle(t, srv, auditReq, "docusaurus_audit failed")
}

func TestTools_Triage(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{OllamaURL: "http://127.0.0.1:11434"})

	// 1. Missing prompt (error)
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_local_agent_triage", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, req1, "expected error for missing prompt in triage")

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
	srv, registered := testSchedulerTools_RegistrationAndExecutionStep1(t)
	expectedTools := testSchedulerTools_RegistrationAndExecutionStep2(t)
	ctx := testSchedulerTools_RegistrationAndExecutionStep3(t, registered, expectedTools)
	policyHandler, err := testSchedulerTools_RegistrationAndExecutionStep4(t, srv, ctx)
	testSchedulerTools_RegistrationAndExecutionStep5(t, ctx, policyHandler, err)
	testSchedulerTools_RegistrationAndExecutionStep6(t, srv, ctx, err)
	triggerHandler := testSchedulerTools_RegistrationAndExecutionStep7(t, srv, ctx, err)
	testSchedulerTools_RegistrationAndExecutionStep8(t, ctx, err, triggerHandler)
	testSchedulerTools_RegistrationAndExecutionStep9(t, ctx, err, triggerHandler)
	testSchedulerTools_RegistrationAndExecutionStep10(t, srv, ctx, err)
	testSchedulerTools_RegistrationAndExecutionStep11(t, srv, ctx, err)
}

func testSchedulerTools_RegistrationAndExecutionStep1(t *testing.T) (*Server, map[string]bool) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	registered := make(map[string]bool)
	for _, tool := range srv.ListTools() {
		registered[tool.Name] = true
	}
	return srv, registered
}

func testSchedulerTools_RegistrationAndExecutionStep2(t *testing.T) []string {
	expectedTools := []string{
		"dendrite_analyze_policy",
		"dendrite_analyze_status",
		"dendrite_analyze_trigger",
		"dendrite_analyze_switch",
		"dendrite_tokens_status",
	}
	return expectedTools
}

func testSchedulerTools_RegistrationAndExecutionStep3(t *testing.T, registered map[string]bool, expectedTools []string) context.Context {
	for _, toolName := range expectedTools {
		if !registered[toolName] {
			t.Errorf("expected tool %s to be registered", toolName)
		}
	}
	ctx := context.Background()
	return ctx
}

// testSchedulerTools_RegistrationAndExecutionStep4: 1. Test dendrite_analyze_policy query
func testSchedulerTools_RegistrationAndExecutionStep4(t *testing.T, srv *Server, ctx context.Context) (ToolHandler, error) {
	policyHandler, _ := srv.Handler("dendrite_analyze_policy")
	res, err := policyHandler(ctx, map[string]interface{}{})
	if err != nil {
		t.Fatalf("policyHandler query failed: %v", err)
	}
	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "night_window") {
		t.Errorf("expected night_window in policy response: %+v", res.Content)
	}
	return policyHandler, err
}

// testSchedulerTools_RegistrationAndExecutionStep5: 2. Test dendrite_analyze_policy update
func testSchedulerTools_RegistrationAndExecutionStep5(t *testing.T, ctx context.Context, policyHandler ToolHandler, err error) {
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
}

// testSchedulerTools_RegistrationAndExecutionStep6: 3. Test dendrite_analyze_status
func testSchedulerTools_RegistrationAndExecutionStep6(t *testing.T, srv *Server, ctx context.Context, err error) {
	statusHandler, _ := srv.Handler("dendrite_analyze_status")
	mcptest.ExpectContent(t, "statusHandler failed", func() (*CallToolResult, error) {
		return statusHandler(ctx, map[string]interface{}{
			"remaining_quota": 0.85,
		})
	}, "active_tier")
}

// testSchedulerTools_RegistrationAndExecutionStep7: 4. Test dendrite_analyze_trigger
func testSchedulerTools_RegistrationAndExecutionStep7(t *testing.T, srv *Server, ctx context.Context, err error) ToolHandler {
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
	return triggerHandler
}

// testSchedulerTools_RegistrationAndExecutionStep8: 5. Test dendrite_analyze_trigger with simulated sprint
func testSchedulerTools_RegistrationAndExecutionStep8(t *testing.T, ctx context.Context, err error, triggerHandler ToolHandler) {
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
}

// testSchedulerTools_RegistrationAndExecutionStep9: 6. Test dendrite_analyze_trigger with simulated night
func testSchedulerTools_RegistrationAndExecutionStep9(t *testing.T, ctx context.Context, err error, triggerHandler ToolHandler) {
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
}

// testSchedulerTools_RegistrationAndExecutionStep10: 7. Test dendrite_analyze_switch
func testSchedulerTools_RegistrationAndExecutionStep10(t *testing.T, srv *Server, ctx context.Context, err error) {
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
}

// testSchedulerTools_RegistrationAndExecutionStep11: 8. Test dendrite_tokens_status
func testSchedulerTools_RegistrationAndExecutionStep11(t *testing.T, srv *Server, ctx context.Context, err error) {
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
	srv := testREATools_RegistrationAndExecutionStep1(t)
	handler, ctx := testREATools_RegistrationAndExecutionStep2(t, srv)
	err := testREATools_RegistrationAndExecutionStep3(t, handler, ctx)
	testREATools_RegistrationAndExecutionStep4(t, handler, ctx, err)
	testREATools_RegistrationAndExecutionStep5(t, handler, ctx, err)
	testREATools_RegistrationAndExecutionStep6(t, handler, ctx, err)
}

func testREATools_RegistrationAndExecutionStep1(t *testing.T) *Server {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})
	return srv
}

func testREATools_RegistrationAndExecutionStep2(t *testing.T, srv *Server) (ToolHandler, context.Context) {
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
	return handler, ctx
}

// testREATools_RegistrationAndExecutionStep3: 1. Execute summary mode
func testREATools_RegistrationAndExecutionStep3(t *testing.T, handler ToolHandler, ctx context.Context) error {
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
	return err
}

// testREATools_RegistrationAndExecutionStep4: 2. Execute json mode
func testREATools_RegistrationAndExecutionStep4(t *testing.T, handler ToolHandler, ctx context.Context, err error) {
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
}

// testREATools_RegistrationAndExecutionStep5: 3. Execute markdown mode
func testREATools_RegistrationAndExecutionStep5(t *testing.T, handler ToolHandler, ctx context.Context, err error) {
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
}

// testREATools_RegistrationAndExecutionStep6: 4. Unknown target error handling
func testREATools_RegistrationAndExecutionStep6(t *testing.T, handler ToolHandler, ctx context.Context, err error) {
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
	srv := testDocusaurusTools_RegistrationAndExecutionStep1(t)
	expectedTools := testDocusaurusTools_RegistrationAndExecutionStep2(t)
	ctx := testDocusaurusTools_RegistrationAndExecutionStep3(t, srv, expectedTools)
	tempSrc, err := testDocusaurusTools_RegistrationAndExecutionStep4(t)
	tempDest := testDocusaurusTools_RegistrationAndExecutionStep5(t, tempSrc, err)
	testDocusaurusTools_RegistrationAndExecutionStep6(t, srv, ctx, tempSrc, err, tempDest)
	testDocusaurusTools_RegistrationAndExecutionStep7(t, srv, ctx, err, tempDest)
	testDocusaurusTools_RegistrationAndExecutionStep8(t, srv, ctx, tempSrc, err)
}

func testDocusaurusTools_RegistrationAndExecutionStep1(t *testing.T) *Server {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{DevStatus: "../../DEV_STATUS.md"})
	return srv
}

func testDocusaurusTools_RegistrationAndExecutionStep2(t *testing.T) []string {
	expectedTools := []string{
		"dendrite_docusaurus_generate",
		"dendrite_docusaurus_share",
		"dendrite_docusaurus_audit",
	}
	return expectedTools
}

func testDocusaurusTools_RegistrationAndExecutionStep3(t *testing.T, srv *Server, expectedTools []string) context.Context {
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
	return ctx
}

// testDocusaurusTools_RegistrationAndExecutionStep4: Setup temporary dirs
func testDocusaurusTools_RegistrationAndExecutionStep4(t *testing.T) (string, error) {
	tempSrc, err := os.MkdirTemp("", "mcp-docusaurus-src-*")
	if err != nil {
		t.Fatalf("failed to create temp src: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempSrc) })
	return tempSrc, err
}

func testDocusaurusTools_RegistrationAndExecutionStep5(t *testing.T, tempSrc string, err error) string {
	tempDest, err := os.MkdirTemp("", "mcp-docusaurus-dest-*")
	if err != nil {
		t.Fatalf("failed to create temp dest: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDest) })
	_ = os.WriteFile(filepath.Join(tempSrc, "FEAT-100-test.md"), []byte("# FEAT-100\n## Acceptance Criteria\n- [x] pass\n"), 0644)
	return tempDest
}

// testDocusaurusTools_RegistrationAndExecutionStep6: 1. Test dendrite_docusaurus_generate
func testDocusaurusTools_RegistrationAndExecutionStep6(t *testing.T, srv *Server, ctx context.Context, tempSrc string, err error, tempDest string) {
	genHandler, _ := srv.Handler("dendrite_docusaurus_generate")
	mcptest.ExpectContent(t, "dendrite_docusaurus_generate failed", func() (*CallToolResult, error) {
		return genHandler(ctx, map[string]interface{}{
			"docs_dir":     tempSrc,
			"target_dir":   tempDest,
			"title":        "Test Docs Portal",
			"port":         3050,
			"sharing_tier": "federated",
		})
	}, "site_directory")
}

// testDocusaurusTools_RegistrationAndExecutionStep7: 2. Test dendrite_docusaurus_share
func testDocusaurusTools_RegistrationAndExecutionStep7(t *testing.T, srv *Server, ctx context.Context, err error, tempDest string) {
	shareHandler, _ := srv.Handler("dendrite_docusaurus_share")
	mcptest.ExpectContent(t, "dendrite_docusaurus_share failed", func() (*CallToolResult, error) {
		return shareHandler(ctx, map[string]interface{}{
			"site_dir": tempDest,
		})
	}, "manifest_version")
}

// testDocusaurusTools_RegistrationAndExecutionStep8: 3. Test dendrite_docusaurus_audit
func testDocusaurusTools_RegistrationAndExecutionStep8(t *testing.T, srv *Server, ctx context.Context, tempSrc string, err error) {
	auditHandler, _ := srv.Handler("dendrite_docusaurus_audit")
	mcptest.ExpectContent(t, "dendrite_docusaurus_audit failed", func() (*CallToolResult, error) {
		return auditHandler(ctx, map[string]interface{}{
			"project_root": tempSrc,
			"docs_dir":     tempSrc,
		})
	}, "total_features")
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
	srv := testTools_StoreStep1(t)
	testTools_StoreStep2(t, srv)
	testTools_StoreStep3(t, srv)
	testTools_StoreStep4(t, srv)
	testTools_StoreStep5(t, srv)
	testTools_StoreStep6(t, srv)
	testTools_StoreStep7(t, srv)
	testTools_StoreStep8(t, srv)
}

func testTools_StoreStep1(t *testing.T) *Server {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return srv
}

// testTools_StoreStep2: 1. dendrite_store_status
func testTools_StoreStep2(t *testing.T, srv *Server) {
	stReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_store_status", "arguments": {"project": "proj-store"}}}`
	mcptest.MustHandle(t, srv, stReq, "store_status failed")
}

// testTools_StoreStep3: 2. dendrite_store_commit_chunk (type: "file", is_final: false)
func testTools_StoreStep3(t *testing.T, srv *Server) {
	chunkReq1 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_store_commit_chunk", "arguments": {"project": "proj-store", "chunk_id": "chunk-1", "identifier": "main.go", "data": "package main", "type": "file", "is_final": false}}}`
	mcptest.MustHandle(t, srv, chunkReq1, "commit_chunk 1 failed")
}

// testTools_StoreStep4: 3. dendrite_store_commit_chunk (type: "", is_final: true)
func testTools_StoreStep4(t *testing.T, srv *Server) {
	chunkReq2 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_store_commit_chunk", "arguments": {"project": "proj-store", "chunk_id": "chunk-2", "identifier": "helper.go", "data": "package helper", "is_final": true}}}`
	mcptest.MustHandle(t, srv, chunkReq2, "commit_chunk 2 failed")
}

// testTools_StoreStep5: 4. dendrite_store_query_partial
func testTools_StoreStep5(t *testing.T, srv *Server) {
	qpReq := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_store_query_partial", "arguments": {"project": "proj-store"}}}`
	mcptest.MustHandle(t, srv, qpReq, "query_partial failed")
}

// testTools_StoreStep6: 5. dendrite_store_inspect
func testTools_StoreStep6(t *testing.T, srv *Server) {
	inspReq := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_store_inspect", "arguments": {"store_id": "proj-store"}}}`
	mcptest.RequireResponse(t, srv, inspReq, "expected response for store_inspect")
}

// testTools_StoreStep7: 6. dendrite_store_rm (force: false and force: true)
func testTools_StoreStep7(t *testing.T, srv *Server) {
	rmReq1 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_store_rm", "arguments": {"store_id": "proj-store", "force": false}}}`
	mcptest.RequireResponse(t, srv, rmReq1, "expected response for store_rm force:false")
}

func testTools_StoreStep8(t *testing.T, srv *Server) {
	rmReq2 := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_store_rm", "arguments": {"store_id": "proj-store", "force": true}}}`
	mcptest.RequireResponse(t, srv, rmReq2, "expected response for store_rm force:true")
}

func TestTools_Scheduler(t *testing.T) {
	srv := testTools_SchedulerStep1(t)
	testTools_SchedulerStep2(t, srv)
	testTools_SchedulerStep3(t, srv)
	testTools_SchedulerStep4(t, srv)
	testTools_SchedulerStep5(t, srv)
	testTools_SchedulerStep6(t, srv)
	testTools_SchedulerStep7(t, srv)
	testTools_SchedulerStep8(t, srv)
	testTools_SchedulerStep9(t, srv)
}

func testTools_SchedulerStep1(t *testing.T) *Server {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return srv
}

// testTools_SchedulerStep2: 1. dendrite_analyze_policy: sprint_before_reset invalid duration error
func testTools_SchedulerStep2(t *testing.T, srv *Server) {
	polReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_analyze_policy", "arguments": {"sprint_before_reset": "invalid-dur"}}}`
	mcptest.ExpectToolError(t, srv, polReq1, "expected error for invalid duration in policy")
}

// testTools_SchedulerStep3: 2. dendrite_analyze_policy: valid options
func testTools_SchedulerStep3(t *testing.T, srv *Server) {
	polReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_analyze_policy", "arguments": {"night_window": "23:00-06:00", "weekly_reset": "SUN 23:59", "sprint_before_reset": "48h", "reserve_floor": 0.20, "allow_zero": true, "mode": "auto"}}}`
	mcptest.MustHandle(t, srv, polReq2, "analyze_policy failed")
}

// testTools_SchedulerStep4: 3. dendrite_analyze_status
func testTools_SchedulerStep4(t *testing.T, srv *Server) {
	statReq := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_analyze_status", "arguments": {"remaining_quota": 0.75}}}`
	mcptest.MustHandle(t, srv, statReq, "analyze_status failed")
}

// testTools_SchedulerStep5: 4. dendrite_analyze_trigger: missing chunk_id error
func testTools_SchedulerStep5(t *testing.T, srv *Server) {
	trigReq1 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, trigReq1, "expected error for missing chunk_id")
}

// testTools_SchedulerStep6: 5. dendrite_analyze_trigger: simulate_night and simulate_sprint
func testTools_SchedulerStep6(t *testing.T, srv *Server) {
	trigReq2 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {"chunk_id": "chunk-101", "simulate_night": true}}}`
	mcptest.MustHandle(t, srv, trigReq2, "analyze_trigger simulate_night failed")
}

func testTools_SchedulerStep7(t *testing.T, srv *Server) {
	trigReq3 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {"chunk_id": "chunk-102", "simulate_sprint": true}}}`
	mcptest.MustHandle(t, srv, trigReq3, "analyze_trigger simulate_sprint failed")
}

// testTools_SchedulerStep8: 6. dendrite_analyze_switch
func testTools_SchedulerStep8(t *testing.T, srv *Server) {
	swReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_analyze_switch", "arguments": {"allow_zero": true, "mode": "always"}}}`
	mcptest.MustHandle(t, srv, swReq, "analyze_switch failed")
}

// testTools_SchedulerStep9: 7. dendrite_tokens_status
func testTools_SchedulerStep9(t *testing.T, srv *Server) {
	tokReq := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_tokens_status", "arguments": {"model": "gemini-3.8-flash-high", "probe": false}}}`
	mcptest.MustHandle(t, srv, tokReq, "tokens_status failed")
}

func TestTools_Contract(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. Missing schema_dirs error
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_contract_drift_check", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, req1, "expected error for missing schema_dirs")

	// 2. Valid contract drift check
	tmpDir := t.TempDir()
	schemaDir := filepath.Join(tmpDir, "schemas")
	_ = os.MkdirAll(schemaDir, 0755)
	_ = os.WriteFile(filepath.Join(schemaDir, "test.proto"), []byte(`syntax = "proto3"; message Test { string id = 1; }`), 0644)

	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_contract_drift_check", "arguments": {"schema_dirs": [%q], "code_dirs": {"go": %q}}}}`, schemaDir, tmpDir)
	mcptest.MustHandle(t, srv, req2, "contract_drift_check failed")
}

func TestTools_Story(t *testing.T) {
	srv := testTools_StoryStep1(t)
	testTools_StoryStep2(t, srv)
	testTools_StoryStep3(t, srv)
	testTools_StoryStep4(t, srv)
	testTools_StoryStep5(t, srv)
	testTools_StoryStep6(t, srv)
}

func testTools_StoryStep1(t *testing.T) *Server {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return srv
}

// testTools_StoryStep2: 1. dendrite_story_fork: missing story_id
func testTools_StoryStep2(t *testing.T, srv *Server) {
	fReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_story_fork", "arguments": {"variants": ["v1", "v2"]}}}`
	mcptest.ExpectToolError(t, srv, fReq1, "expected error for missing story_id in story_fork")
}

// testTools_StoryStep3: 2. dendrite_story_fork: < 2 variants
func testTools_StoryStep3(t *testing.T, srv *Server) {
	fReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_story_fork", "arguments": {"story_id": "DEN-064", "variants": ["v1"]}}}`
	mcptest.ExpectToolError(t, srv, fReq2, "expected error for < 2 variants in story_fork")
}

// testTools_StoryStep4: 3. dendrite_story_benchmark: missing story_id
func testTools_StoryStep4(t *testing.T, srv *Server) {
	bReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_story_benchmark", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, bReq1, "expected error for missing story_id in story_benchmark")
}

// testTools_StoryStep5: 4. dendrite_story_resolve: missing story_id or winner
func testTools_StoryStep5(t *testing.T, srv *Server) {
	rReq1 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_story_resolve", "arguments": {"story_id": "DEN-064"}}}`
	mcptest.ExpectToolError(t, srv, rReq1, "expected error for missing winner in story_resolve")
}

// testTools_StoryStep6: 5. dendrite_story_abort: missing story_id
func testTools_StoryStep6(t *testing.T, srv *Server) {
	aReq1 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_story_abort", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, aReq1, "expected error for missing story_id in story_abort")
}

func TestTools_Audit(t *testing.T) {
	srv := testTools_AuditStep1(t)
	testTools_AuditStep2(t, srv)
	testTools_AuditStep3(t, srv)
	testTools_AuditStep4(t, srv)
	testTools_AuditStep5(t, srv)
	testTools_AuditStep6(t, srv)
	testTools_AuditStep7(t, srv)
	testTools_AuditStep8(t, srv)
	testTools_AuditStep9(t, srv)
}

func testTools_AuditStep1(t *testing.T) *Server {
	chdirToScratchRepo(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return srv
}

// testTools_AuditStep2: 1. dendrite_vcs_guard (STANDARD and DEGRADED_BUDGET)
func testTools_AuditStep2(t *testing.T, srv *Server) {
	gReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_vcs_guard", "arguments": {"mode": "STANDARD"}}}`
	mcptest.MustHandle(t, srv, gReq1, "vcs_guard STANDARD failed")
}

func testTools_AuditStep3(t *testing.T, srv *Server) {
	gReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_vcs_guard", "arguments": {"mode": "DEGRADED_BUDGET"}}}`
	mcptest.MustHandle(t, srv, gReq2, "vcs_guard DEGRADED_BUDGET failed")
}

// testTools_AuditStep4: 2. dendrite_quality_audit: missing pkg error
func testTools_AuditStep4(t *testing.T, srv *Server) {
	qaReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_quality_audit", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, qaReq1, "expected error for missing pkg")
}

// testTools_AuditStep5: 3. dendrite_quality_audit: valid
func testTools_AuditStep5(t *testing.T, srv *Server) {
	qaReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_quality_audit", "arguments": {"pkg": "pkg/mcpserver"}}}`
	mcptest.RequireResponse(t, srv, qaReq2, "expected response for quality_audit")
}

// testTools_AuditStep6: 4. dendrite_audit_changes
func testTools_AuditStep6(t *testing.T, srv *Server) {
	acReq := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_audit_changes", "arguments": {"dry_run": true, "max_tokens": 2048, "model": "qwen3.8:27b"}}}`
	mcptest.RequireResponse(t, srv, acReq, "expected response for audit_changes")
}

// testTools_AuditStep7: 5. dendrite_hitl_sync (offline: local Plane server and scripted gh)
func testTools_AuditStep7(t *testing.T, srv *Server) {
	offlineHITL(t)
	hitlReq := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_hitl_sync", "arguments": {}}}`
	mcptest.RequireResponse(t, srv, hitlReq, "expected response for hitl_sync")
}

// testTools_AuditStep8: 6. dendrite_remote_sync
func testTools_AuditStep8(t *testing.T, srv *Server) {
	rsReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_remote_sync", "arguments": {"branch": "develop"}}}`
	mcptest.RequireResponse(t, srv, rsReq, "expected response for remote_sync")
}

// testTools_AuditStep9: 7. dendrite_github_sync
func testTools_AuditStep9(t *testing.T, srv *Server) {
	ghReq := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_github_sync", "arguments": {"repo": "yasuomaidana/dendrite"}}}`
	mcptest.RequireResponse(t, srv, ghReq, "expected response for github_sync")
}

func TestTools_InferenceAndPenpot(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_inference_migrate_model: missing model error
	mReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_inference_migrate_model", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, mReq1, "expected error for missing model")

	// 2. dendrite_inference_migrate_model: valid
	mReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_inference_migrate_model", "arguments": {"model": "qwen2.5:7b", "source_engine": "ollama", "target_engine": "oxillama", "delete_source": false, "dry_run": true}}}`
	mcptest.RequireResponse(t, srv, mReq2, "expected response for migrate_model")

	// 3. dendrite_ui_penpot_sync: missing file error
	pReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_ui_penpot_sync", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, pReq1, "expected error for missing file")

	// 4. dendrite_ui_penpot_sync: valid
	pReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_ui_penpot_sync", "arguments": {"file": "Button.svelte", "ticket_id": "DEN-810", "dry_run": true, "comment_plane": false}}}`
	mcptest.RequireResponse(t, srv, pReq2, "expected response for ui_penpot_sync")
}

func TestTools_MigrateGreetingEnv(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// 1. dendrite_service_move: missing target_host error
	smReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, smReq1, "expected error for missing target_host")

	// 2. dendrite_service_move: valid dry_run
	smReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {"target_host": "127.0.0.1", "services": ["plane"], "ssh_port": 22, "target_ram_mb": 4096, "target_disk_gb": 500, "dry_run": true, "prune_local": false, "timeout_seconds": 60}}}`
	mcptest.RequireResponse(t, srv, smReq2, "expected response for service_move")

	// 3. dendrite_service_plan
	planReq := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_service_plan", "arguments": {"ram_mb": 4096, "disk_gb": 1000}}}`
	mcptest.MustHandle(t, srv, planReq, "service_plan failed")

	// 4. dendrite_pcontext_greeting_generate
	gReq := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_pcontext_greeting_generate", "arguments": {"topic": "Refactoring", "decision": "architecture", "milestone": "M1", "milestone_achieved": true, "event": "Launch", "days_until_event": 5, "unfinished_step": "test", "specific_challenge": "speed", "curiosity_angle": "wasm", "days_since_last_active": 2, "turn_count": 3, "high_stress": false, "low_pressure_hobby": true, "options": ["A", "B"], "recent_styles": ["G-01"]}}}`
	mcptest.MustHandle(t, srv, gReq, "greeting_generate failed")

	// 5. dendrite_env_prepare (dry_run: true)
	tmpDir := t.TempDir()
	envReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_env_prepare", "arguments": {"project_path": %q, "default_tool": "copilot", "enforce_cicd": true, "dry_run": true}}}`, tmpDir)
	mcptest.MustHandle(t, srv, envReq, "env_prepare failed")
}

func TestTools_CanvasAndRAG(t *testing.T) {
	srv := testTools_CanvasAndRAGStep1(t)
	testTools_CanvasAndRAGStep2(t, srv)
	testTools_CanvasAndRAGStep3(t, srv)
	testTools_CanvasAndRAGStep4(t, srv)
	testTools_CanvasAndRAGStep5(t, srv)
	testTools_CanvasAndRAGStep6(t, srv)
	testTools_CanvasAndRAGStep7(t, srv)
	testTools_CanvasAndRAGStep8(t, srv)
	tmpDir := testTools_CanvasAndRAGStep9(t, srv)
	testTools_CanvasAndRAGStep10(t, srv, tmpDir)
	testTools_CanvasAndRAGStep11(t, srv)
	testTools_CanvasAndRAGStep12(t, srv, tmpDir)
}

func testTools_CanvasAndRAGStep1(t *testing.T) *Server {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return srv
}

// testTools_CanvasAndRAGStep2: 1. dendrite_canvas_action_dispatch: missing action error
func testTools_CanvasAndRAGStep2(t *testing.T, srv *Server) {
	cReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_canvas_action_dispatch", "arguments": {"prompt": "write test"}}}`
	mcptest.ExpectToolError(t, srv, cReq1, "expected error for missing action")
}

// testTools_CanvasAndRAGStep3: 2. dendrite_canvas_action_dispatch: valid
func testTools_CanvasAndRAGStep3(t *testing.T, srv *Server) {
	cReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_canvas_action_dispatch", "arguments": {"action": "WRITE_PLAN", "mode": "DRAFT", "title": "Test Plan", "prompt": "Outline tasks"}}}`
	mcptest.MustHandle(t, srv, cReq2, "canvas action dispatch failed")
}

// testTools_CanvasAndRAGStep4: 3. dendrite_canvas_penpot_bridge subactions
// relate
func testTools_CanvasAndRAGStep4(t *testing.T, srv *Server) {
	subReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "relate", "ticket_id": "DEN-100", "feature_name": "Canvas", "file_path": "App.svelte"}}}`
	mcptest.MustHandle(t, srv, subReq1, "penpot bridge relate failed")
}

// testTools_CanvasAndRAGStep5: sync
func testTools_CanvasAndRAGStep5(t *testing.T, srv *Server) {
	subReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "sync", "ticket_id": "DEN-100", "file_path": "App.svelte", "dry_run": true}}}`
	mcptest.MustHandle(t, srv, subReq2, "penpot bridge sync failed")
}

// testTools_CanvasAndRAGStep6: update_state
func testTools_CanvasAndRAGStep6(t *testing.T, srv *Server) {
	subReq3 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "update_state", "ticket_id": "DEN-100", "target_state": "Done", "push_to_plane": false}}}`
	mcptest.MustHandle(t, srv, subReq3, "penpot bridge update_state failed")
}

// testTools_CanvasAndRAGStep7: list
func testTools_CanvasAndRAGStep7(t *testing.T, srv *Server) {
	subReq4 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "list"}}}`
	mcptest.MustHandle(t, srv, subReq4, "penpot bridge list failed")
}

// testTools_CanvasAndRAGStep8: unknown subaction error
func testTools_CanvasAndRAGStep8(t *testing.T, srv *Server) {
	subReq5 := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "invalid_subaction"}}}`
	mcptest.ExpectToolError(t, srv, subReq5, "expected error for unknown subaction")
}

// testTools_CanvasAndRAGStep9: 4. dendrite_rag_ask: missing question error
func testTools_CanvasAndRAGStep9(t *testing.T, srv *Server) string {
	tmpDir := t.TempDir()
	rReq1 := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_rag_ask", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, rReq1, "expected error for missing question in rag_ask")
	return tmpDir
}

// testTools_CanvasAndRAGStep10: dendrite_rag_ask: valid question
func testTools_CanvasAndRAGStep10(t *testing.T, srv *Server, tmpDir string) {
	rReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": {"name": "dendrite_rag_ask", "arguments": {"question": "How does server work?", "project_path": %q, "auto_approve_paid": false, "top_k": 2}}}`, tmpDir)
	mcptest.RequireResponse(t, srv, rReq2, "expected response for rag_ask")
}

// testTools_CanvasAndRAGStep11: 5. dendrite_rag_retrieve: missing query error
func testTools_CanvasAndRAGStep11(t *testing.T, srv *Server) {
	retReq1 := `{"jsonrpc": "2.0", "id": 10, "method": "tools/call", "params": {"name": "dendrite_rag_retrieve", "arguments": {}}}`
	mcptest.ExpectToolError(t, srv, retReq1, "expected error for missing query in rag_retrieve")
}

// testTools_CanvasAndRAGStep12: dendrite_rag_retrieve: valid query
func testTools_CanvasAndRAGStep12(t *testing.T, srv *Server, tmpDir string) {
	retReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 11, "method": "tools/call", "params": {"name": "dendrite_rag_retrieve", "arguments": {"query": "Server", "project_path": %q, "top_k": 2, "min_score": 0.001}}}`, tmpDir)
	mcptest.RequireResponse(t, srv, retReq2, "expected response for rag_retrieve")
}

// fixtureMCPCanvasTools is the setup TestMCP_CanvasTools and its subtests share.
func fixtureMCPCanvasTools(t *testing.T) (context.Context, *Server) {
	t.Helper()
	ctx := context.Background()
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
	t.Cleanup(func() { planeServer.Close() })
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
	_ = planeServer
	_ = planeClient
	_ = deps
	return ctx, srv
}

func TestMCP_CanvasTools_DispatchWritePlan(t *testing.T) {
	ctx, srv := fixtureMCPCanvasTools(t)
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
}

func TestMCP_CanvasTools_DispatchDesignUI(t *testing.T) {
	ctx, srv := fixtureMCPCanvasTools(t)
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
}

func TestMCP_CanvasTools_PenpotBridgeRelate(t *testing.T) {
	ctx, srv := fixtureMCPCanvasTools(t)
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
}

func TestMCP_CanvasTools_PenpotBridgeSync(t *testing.T) {
	ctx, srv := fixtureMCPCanvasTools(t)
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
}

func TestMCP_CanvasTools_PenpotBridgeUpdateState(t *testing.T) {
	ctx, srv := fixtureMCPCanvasTools(t)
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
}

func TestMCP_CanvasTools_PenpotBridgeList(t *testing.T) {
	ctx, srv := fixtureMCPCanvasTools(t)
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
	srv, ctx := testMCP_MigrateToolsStep1(t)
	callRes, ok := testMCP_MigrateToolsStep2(t, srv, ctx)
	testMCP_MigrateToolsStep3(t, callRes)
	callMoveRes := testMCP_MigrateToolsStep4(t, srv, ctx, ok)
	testMCP_MigrateToolsStep5(t, callMoveRes)
}

func testMCP_MigrateToolsStep1(t *testing.T) (*Server, context.Context) {
	srv := NewServer(ServerConfig{
		Name:    "test-dendrite",
		Version: "0.1.0",
	})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	ctx := context.Background()
	return srv, ctx
}

// testMCP_MigrateToolsStep2: 1. Test dendrite_service_plan
func testMCP_MigrateToolsStep2(t *testing.T, srv *Server, ctx context.Context) (*CallToolResult, bool) {
	planReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_service_plan", "arguments": {"ram_mb": 4096, "disk_gb": 1000}}}`
	respPlan := srv.HandleRequest(ctx, []byte(planReq))
	if respPlan == nil || respPlan.Error != nil {
		t.Fatalf("unexpected error calling dendrite_service_plan: %v", respPlan)
	}
	callRes, ok := respPlan.Result.(*CallToolResult)
	if !ok || len(callRes.Content) == 0 {
		t.Fatalf("expected valid CallToolResult with content")
	}
	return callRes, ok
}

func testMCP_MigrateToolsStep3(t *testing.T, callRes *CallToolResult) {
	var planData map[string]interface{}
	if err := json.Unmarshal([]byte(callRes.Content[0].Text), &planData); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if recCmd, ok := planData["recommended_action"].(string); !ok || !strings.Contains(recCmd, "dendrite translocate run") {
		t.Errorf("expected recommended action with dendrite move, got %v", recCmd)
	}
}

// testMCP_MigrateToolsStep4: 2. Test dendrite_service_move in dry-run mode
func testMCP_MigrateToolsStep4(t *testing.T, srv *Server, ctx context.Context, ok bool) *CallToolResult {
	moveReq := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {"services": ["plane"], "target_host": "user@debian-server", "dry_run": true}}}`
	respMove := srv.HandleRequest(ctx, []byte(moveReq))
	if respMove == nil || respMove.Error != nil {
		t.Fatalf("unexpected error calling dendrite_service_move: %v", respMove)
	}
	callMoveRes, ok := respMove.Result.(*CallToolResult)
	if !ok || len(callMoveRes.Content) == 0 {
		t.Fatalf("expected valid CallToolResult for move")
	}
	return callMoveRes
}

func testMCP_MigrateToolsStep5(t *testing.T, callMoveRes *CallToolResult) {
	var moveData map[string]interface{}
	if err := json.Unmarshal([]byte(callMoveRes.Content[0].Text), &moveData); err != nil {
		t.Fatalf("failed to parse json move response: %v", err)
	}
	if success, ok := moveData["success"].(bool); !ok || !success {
		t.Errorf("expected dry-run move to succeed, got %v", moveData)
	}
}

// fixtureMCPGreetingTools is the setup TestMCP_GreetingTools and its subtests share.
func fixtureMCPGreetingTools(t *testing.T) (*Server, context.Context) {
	t.Helper()
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	catalog.RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
	})
	ctx := context.Background()
	return srv, ctx
}

func TestMCP_GreetingTools_GenerateGreetingViaMCP(t *testing.T) {
	srv, ctx := fixtureMCPGreetingTools(t)
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
}

func TestMCP_GreetingTools_SuppressedStyleViaMCP(t *testing.T) {
	srv, ctx := fixtureMCPGreetingTools(t)
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
}

func TestMCP_GreetingTools_AllStylesSuppressedRelaxesSuppressionInsteadOfFailing(t *testing.T) {
	srv, ctx := fixtureMCPGreetingTools(t)
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
}

func TestGitHubSyncAndHITLSyncTools(t *testing.T) {
	ps, gh, srv := testGitHubSyncAndHITLSyncToolsStep1(t)
	text, isErr := testGitHubSyncAndHITLSyncToolsStep2(t, gh, srv)
	testGitHubSyncAndHITLSyncToolsStep3(t, ps, srv, text, isErr)
	testGitHubSyncAndHITLSyncToolsStep4(t, gh, srv)
}

func testGitHubSyncAndHITLSyncToolsStep1(t *testing.T) (*hitltest.PlaneServer, *hitltest.GHScript, *Server) {
	ps, gh := offlineHITL(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return ps, gh, srv
}

func testGitHubSyncAndHITLSyncToolsStep2(t *testing.T, gh *hitltest.GHScript, srv *Server) (string, bool) {
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
	return text, isErr
}

func testGitHubSyncAndHITLSyncToolsStep3(t *testing.T, ps *hitltest.PlaneServer, srv *Server, text string, isErr bool) {
	text, isErr = mcptest.CallTool(t, srv, "dendrite_hitl_sync", `{"dry_run":true}`)
	var results []hitl.HITLResult
	if isErr || json.Unmarshal([]byte(text), &results) != nil || len(results) != 2 || len(ps.Patches) != 0 {
		t.Fatalf("hitl dry run: %s", text)
	}
	if _, isErr = mcptest.CallTool(t, srv, "dendrite_hitl_sync", `{}`); isErr || ps.Patches["u-2"] != "st-done" {
		t.Fatalf("hitl apply: patches %v", ps.Patches)
	}
}

func testGitHubSyncAndHITLSyncToolsStep4(t *testing.T, gh *hitltest.GHScript, srv *Server) {
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
	mcptest.ExpectToolError(t, srv, req1, "expected error for invalid agent type")

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
	srv := testTools_DendriteTelemetryAndInferenceStep1(t)
	testTools_DendriteTelemetryAndInferenceStep2(t, srv)
	testTools_DendriteTelemetryAndInferenceStep3(t, srv)
	testTools_DendriteTelemetryAndInferenceStep4(t, srv)
	testTools_DendriteTelemetryAndInferenceStep5(t, srv)
	testTools_DendriteTelemetryAndInferenceStep6(t, srv)
}

func testTools_DendriteTelemetryAndInferenceStep1(t *testing.T) *Server {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})
	return srv
}

// testTools_DendriteTelemetryAndInferenceStep2: dendrite_telemetry
func testTools_DendriteTelemetryAndInferenceStep2(t *testing.T, srv *Server) {
	tReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_telemetry", "arguments": {}}}`
	mcptest.MustHandle(t, srv, tReq, "telemetry error")
}

// testTools_DendriteTelemetryAndInferenceStep3: dendrite_inference_list
func testTools_DendriteTelemetryAndInferenceStep3(t *testing.T, srv *Server) {
	lReq := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_inference_list", "arguments": {}}}`
	mcptest.MustHandle(t, srv, lReq, "inference list error")
}

// testTools_DendriteTelemetryAndInferenceStep4: dendrite_inference_probe all
func testTools_DendriteTelemetryAndInferenceStep4(t *testing.T, srv *Server) {
	pReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {}}}`
	mcptest.MustHandle(t, srv, pReq1, "inference probe all error")
}

// testTools_DendriteTelemetryAndInferenceStep5: dendrite_inference_probe single engine
func testTools_DendriteTelemetryAndInferenceStep5(t *testing.T, srv *Server) {
	pReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {"engine_id": "oxillama-rust"}}}`
	mcptest.MustHandle(t, srv, pReq2, "inference probe single error")
}

// testTools_DendriteTelemetryAndInferenceStep6: dendrite_inference_probe non-existent engine
func testTools_DendriteTelemetryAndInferenceStep6(t *testing.T, srv *Server) {
	pReq3 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {"engine_id": "non-existent-engine"}}}`
	mcptest.ExpectToolError(t, srv, pReq3, "expected error for non-existent engine")
}

// fixtureMCPEnvPrepareTools is the setup TestMCP_EnvPrepareTools and its subtests share.
func fixtureMCPEnvPrepareTools(t *testing.T) (*Server, context.Context, string) {
	t.Helper()
	srv := NewServer(ServerConfig{Name: "dendrite", Version: "1.0.0"})
	catalog.RegisterDefaultTools(srv, Dependencies{
		DevStatus: "DEV_STATUS.md",
	})
	ctx := context.Background()
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module testmcp\n\ngo 1.24\n"), 0644)
	return srv, ctx, tmpDir
}

func TestMCP_EnvPrepareTools_DryRunPrepareViaMCP(t *testing.T) {
	srv, ctx, tmpDir := fixtureMCPEnvPrepareTools(t)
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
}

func TestMCP_EnvPrepareTools_RealPrepareViaMCP(t *testing.T) {
	srv, ctx, tmpDir := fixtureMCPEnvPrepareTools(t)
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

	if _, err := os.Stat(filepath.Join(tmpDir, ".github", "copilot-instructions.md")); err != nil {
		t.Errorf("expected copilot-instructions.md to exist: %v", err)
	}
}
