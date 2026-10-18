package devtool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/plane"
)

func isolateEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:59999")
	t.Setenv("PLANE_API_KEY", "isolated-plane-key")
	t.Setenv("PLANE_WORKSPACE_SLUG", "isolated-ws")
	t.Setenv("PLANE_PROJECT_ID", "isolated-proj")
	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", "http://127.0.0.1:59997")
	t.Setenv("DENDRITE_COMMIT_PREFER_OXILLAMA", "false")
}

func TestFormatTicketCommit(t *testing.T) {
	tests := []struct {
		ticket   string
		message  string
		expected string
	}{
		{"DEN-098", "feat: implement commit engine", "[DEN-098] feat: implement commit engine"},
		{"DEN-098", "[DEN-098] feat: already formatted", "[DEN-098] feat: already formatted"},
		{"", "chore: no ticket ID", "chore: no ticket ID"},
		{"  DEN-098  ", "fix: spacing check", "[DEN-098] fix: spacing check"},
	}

	for _, tt := range tests {
		got := FormatTicketCommit(tt.ticket, tt.message)
		if got != tt.expected {
			t.Errorf("FormatTicketCommit(%q, %q) = %q; want %q", tt.ticket, tt.message, got, tt.expected)
		}
	}
}

func TestCleanCommitText(t *testing.T) {
	raw := "```markdown\nfeat(plane): fix doc sync\n```"
	cleaned := cleanCommitText(raw)
	if strings.Contains(cleaned, "```") {
		t.Errorf("cleanCommitText did not remove code fences: %s", cleaned)
	}
	if !strings.Contains(cleaned, "feat(plane): fix doc sync") {
		t.Errorf("cleanCommitText dropped actual content: %s", cleaned)
	}
}

func TestTruncateString(t *testing.T) {
	s := "hello world from dendrite"
	trunc := truncateString(s, 5)
	if !strings.HasPrefix(trunc, "hello") || !strings.Contains(trunc, "[truncated]") {
		t.Errorf("truncateString failed: %s", trunc)
	}

	short := truncateString(s, 50)
	if short != s {
		t.Errorf("truncateString altered string within limit: %s", short)
	}
}

func TestGenerateLocalCommitMessage_Ollama(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"response": "feat(doc): support taxonomy guard",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	msg, modelUsed, err := GenerateLocalCommitMessage(
		context.Background(),
		"1 file changed",
		"+ test code",
		"DEN-098",
		"test-qwen",
		ts.URL,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(msg, "[DEN-098] feat(doc): support taxonomy guard") {
		t.Errorf("unexpected message: %s", msg)
	}
	if !strings.Contains(modelUsed, "Ollama") {
		t.Errorf("expected Ollama model used, got %s", modelUsed)
	}
}

func TestGenerateLocalCommitMessage_Fallback(t *testing.T) {
	msg, modelUsed, err := GenerateLocalCommitMessage(
		context.Background(),
		"pkg/devtool/commit.go | 100 +++\n",
		"+ func ExecuteCommit",
		"DEN-098",
		"qwen3.8:27b",
		"http://127.0.0.1:59999", // dead endpoint
		"http://127.0.0.1:59998", // dead endpoint
		"",
	)
	if err != nil {
		t.Fatalf("fallback should not return fatal error: %v", err)
	}
	if !strings.HasPrefix(msg, "[DEN-098] chore:") {
		t.Errorf("expected deterministic fallback with ticket ID, got: %s", msg)
	}
	if modelUsed != "deterministic-fallback" {
		t.Errorf("expected deterministic-fallback, got %s", modelUsed)
	}
}

func TestExecuteCommit_DryRunAndActual(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize git repo in tempDir
	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = tempDir
	if err := cmdInit.Run(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	cmdConfig1 := exec.Command("git", "config", "user.name", "Dendrite Bot")
	cmdConfig1.Dir = tempDir
	_ = cmdConfig1.Run()

	cmdConfig2 := exec.Command("git", "config", "user.email", "bot@dendrite.ai")
	cmdConfig2.Dir = tempDir
	_ = cmdConfig2.Run()

	// Create test file
	testFile := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(testFile, []byte("initial commit content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 1. Dry run test
	dryRes, err := ExecuteCommit(context.Background(), CommitOptions{
		TicketID:      "DEN-098",
		Message:       "feat: test commit execution",
		StageAll:      true,
		DryRun:        true,
		RepoDir:       tempDir,
		UseLocalModel: false,
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if !dryRes.DryRun || dryRes.CommitSHA != "dry-run-preview-sha" {
		t.Errorf("unexpected dry run result: %+v", dryRes)
	}

	// 2. Real commit execution
	res, err := ExecuteCommit(context.Background(), CommitOptions{
		TicketID:      "DEN-098",
		Message:       "feat: test real commit execution",
		StageAll:      true,
		DryRun:        false,
		RepoDir:       tempDir,
		UseLocalModel: false,
	})
	if err != nil {
		t.Fatalf("real commit failed: %v", err)
	}
	if res.CommitSHA == "" || res.CommitSHA == "unknown-sha" {
		t.Errorf("expected valid commit SHA, got %s", res.CommitSHA)
	}
	if len(res.Files) == 0 {
		t.Errorf("expected staged files, got 0")
	}

	// 3. Commit with no changes should return error
	_, noChangesErr := ExecuteCommit(context.Background(), CommitOptions{
		TicketID: "DEN-098",
		RepoDir:  tempDir,
	})
	if noChangesErr == nil {
		t.Errorf("expected error when nothing to commit")
	}
}

func TestGenerateLocalCommitMessage_OxiLLaMaHTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"content": "feat(commit): dynamic oxillama http synthesis"}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", ts.URL)
	t.Setenv("DENDRITE_COMMIT_PREFER_OXILLAMA", "true")

	msg, modelUsed, err := GenerateLocalCommitMessage(
		context.Background(),
		"1 file changed",
		"+ test code",
		"DEN-817",
		"oxillama/qwen3.8:27b",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(msg, "[DEN-817] feat(commit): dynamic oxillama http synthesis") {
		t.Errorf("unexpected message: %s", msg)
	}
	if !strings.Contains(modelUsed, "OxiLLaMa HTTP") {
		t.Errorf("expected OxiLLaMa HTTP model used, got %s", modelUsed)
	}
}

func TestGenerateLocalCommitMessage_LiteLLM(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"content": "feat(proxy): synthesized via litellm"}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	msg, modelUsed, err := GenerateLocalCommitMessage(
		context.Background(),
		"1 file changed",
		"+ test code",
		"DEN-817",
		"cloud-model",
		"http://127.0.0.1:59999", // dead ollama
		ts.URL,
		"test-key",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(msg, "[DEN-817] feat(proxy): synthesized via litellm") {
		t.Errorf("unexpected message: %s", msg)
	}
	if !strings.Contains(modelUsed, "LiteLLM") {
		t.Errorf("expected LiteLLM model used, got %s", modelUsed)
	}
}

func TestExecuteCommit_UseLocalModel_And_SpecificFiles(t *testing.T) {
	tempDir := t.TempDir()

	_ = exec.Command("git", "-C", tempDir, "init").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Test").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@example.com").Run()

	filePath := filepath.Join(tempDir, "file1.txt")
	_ = os.WriteFile(filePath, []byte("hello file 1"), 0644)

	// 1. Commit with specific files and UseLocalModel = true
	res, err := ExecuteCommit(context.Background(), CommitOptions{
		TicketID:        "DEN-817",
		Files:           []string{"file1.txt"},
		UseLocalModel:   true,
		RepoDir:         tempDir,
		OllamaEndpoint:  "http://127.0.0.1:59999", // fallback
		LiteLLMEndpoint: "http://127.0.0.1:59998", // fallback
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CommitSHA == "" {
		t.Errorf("expected valid SHA")
	}
	if !strings.HasPrefix(res.Message, "[DEN-817]") {
		t.Errorf("expected ticket ID prefix, got: %s", res.Message)
	}

	// 2. Commit with Message = "" and UseLocalModel = false
	filePath2 := filepath.Join(tempDir, "file2.txt")
	_ = os.WriteFile(filePath2, []byte("hello file 2"), 0644)

	res2, err2 := ExecuteCommit(context.Background(), CommitOptions{
		TicketID:      "DEN-817",
		Files:         []string{"file2.txt"},
		UseLocalModel: false,
		RepoDir:       tempDir,
	})
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if !strings.Contains(res2.Message, "chore: repository update") {
		t.Errorf("expected default chore message, got: %s", res2.Message)
	}
}

func TestGenerateLocalCommitMessage_OxiLLaMaCLI(t *testing.T) {
	tempDir := t.TempDir()
	modelsDir := filepath.Join(tempDir, "models")
	modelDir := filepath.Join(modelsDir, "qwen_test")
	_ = os.MkdirAll(modelDir, 0755)

	scriptPath := filepath.Join(tempDir, "oxillama")
	scriptContent := "#!/bin/sh\necho 'INFO loading model'\necho '✓ ready'\necho 'feat(cli): generated commit from oxillama cli'\n"
	_ = os.WriteFile(scriptPath, []byte(scriptContent), 0755)

	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", "http://127.0.0.1:59999") // dead HTTP so it falls back to CLI
	t.Setenv("DENDRITE_OXILLAMA_BIN", scriptPath)
	t.Setenv("DENDRITE_OXILLAMA_MODELS_DIR", modelsDir)

	msg, modelUsed, err := GenerateLocalCommitMessage(
		context.Background(),
		"1 file changed",
		"+ test code",
		"DEN-817",
		"oxillama/qwen:test",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(msg, "[DEN-817] feat(cli): generated commit from oxillama cli") {
		t.Errorf("unexpected message: %s", msg)
	}
	if !strings.Contains(modelUsed, "OxiLLaMa Pure-Rust Zero-FFI") {
		t.Errorf("expected OxiLLaMa CLI model used, got %s", modelUsed)
	}
}

func TestDevTool_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	// ProbeOllama with unreachable port
	_, _, err := ProbeOllama(ctx, "http://127.0.0.1:59999")
	if err == nil {
		t.Errorf("expected error probing dead ollama")
	}

	// UpdateTicketInMarkdown with non-existent path
	err = UpdateTicketInMarkdown(ctx, "/nonexistent/DEV_STATUS.md", "DEN-1", "Done")
	if err == nil {
		t.Errorf("expected error updating non-existent markdown file")
	}

	// ExecuteCommit with invalid repo dir
	_, err = ExecuteCommit(ctx, CommitOptions{
		RepoDir: "/nonexistent/git/repo",
	})
	if err == nil {
		t.Errorf("expected error executing commit in non-existent repo")
	}

	// ExecuteCommit with invalid staged files
	tempDir := t.TempDir()
	_ = exec.Command("git", "-C", tempDir, "init").Run()
	_, err = ExecuteCommit(ctx, CommitOptions{
		RepoDir: tempDir,
		Files:   []string{"nonexistent-file.go"},
	})
	if err == nil {
		t.Errorf("expected error staging non-existent file")
	}
}

func TestIsCannedOrInvalidCommitMessage(t *testing.T) {
	canned := []string{
		"OxiLLaMa (pure-Rust zero-FFI runner) responded via 'qwen3.8:27b': processed your request safely with zero cloud tokens consumed.",
		"processed your request safely with zero cloud tokens consumed",
		"pure-Rust zero-FFI runner responded via qwen",
		"As an AI, here is the commit message: feat(repo): update",
		"Here is the commit message for you",
		"",
		"   ",
		"random non-conventional commit text without type",
	}
	for _, c := range canned {
		if !isCannedOrInvalidCommitMessage(c) {
			t.Errorf("expected isCannedOrInvalidCommitMessage(%q) = true, got false", c)
		}
	}

	valid := []string{
		"feat(commit): implement smart commit strategy",
		"fix(gateway): handle timeout error",
		"docs: update installation instructions",
		"refactor(tracker): extract common client logic",
		"[DEN-817] feat(devtool): decouple git commit engine with oxillama http adapter",
		"chore: update dependencies",
		"test(mcpserver): add integration tests for plane tools",
	}
	for _, v := range valid {
		if isCannedOrInvalidCommitMessage(v) {
			t.Errorf("expected isCannedOrInvalidCommitMessage(%q) = false, got true", v)
		}
	}
}

func TestResolveSmartTicket_FileCorrelationAndFallbacks(t *testing.T) {
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:59999")
	tempDir := t.TempDir()

	// Write mock DEV_STATUS.md
	devStatusContent := "# Development Status\n\n- [ ] **DEN-817**: Decouple Git Commit Synthesis Engine: OxiLLaMa HTTP Adapter (pkg/devtool/commit.go)\n- [ ] **DEN-814**: Dynamic LiteLLM Routing Table Updater (pkg/gateway/updater.go)\n"
	_ = os.WriteFile(filepath.Join(tempDir, "DEV_STATUS.md"), []byte(devStatusContent), 0644)

	// 1. Resolve based on staged file correlation
	res, err := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/devtool/commit.go", "pkg/devtool/commit_test.go"})
	if err != nil {
		t.Fatalf("unexpected error resolving smart ticket: %v", err)
	}
	if res.ID != "DEN-817" {
		t.Errorf("expected ticket DEN-817, got %s", res.ID)
	}
	if res.Scope != "devtool" {
		t.Errorf("expected scope devtool, got %s", res.Scope)
	}

	// 2. Resolve gateway file correlation
	resGateway, err := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/gateway/updater.go"})
	if err != nil {
		t.Fatalf("unexpected error resolving gateway ticket: %v", err)
	}
	if resGateway.ID != "DEN-814" {
		t.Errorf("expected ticket DEN-814, got %s", resGateway.ID)
	}

	// 3. Fallback when no ticket matches
	resFallback, err := ResolveSmartTicket(context.Background(), tempDir, []string{"unknown/file.txt"})
	if err != nil {
		t.Fatalf("unexpected error in fallback: %v", err)
	}
	if resFallback.Scope != "repo" {
		t.Errorf("expected scope repo, got %s", resFallback.Scope)
	}
}

func TestExecuteCommit_SmartStrategyAndAmend(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()

	_ = exec.Command("git", "-C", tempDir, "init").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Test").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@example.com").Run()

	devStatusContent := "# Status\n- [ ] **DEN-817**: Decouple Git Commit Synthesis Engine (pkg/devtool/commit.go)\n"
	_ = os.WriteFile(filepath.Join(tempDir, "DEV_STATUS.md"), []byte(devStatusContent), 0644)

	// Create and commit a file
	filePath := filepath.Join(tempDir, "commit.go")
	_ = os.WriteFile(filePath, []byte("package devtool\n"), 0644)

	// 1. Execute commit with Smart Strategy (ticket_id omitted)
	res, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:       tempDir,
		StageAll:      true,
		UseLocalModel: false, // deterministic
		Smart:         true,
	})
	if err != nil {
		t.Fatalf("ExecuteCommit with Smart Strategy failed: %v", err)
	}
	if res.CommitSHA == "" {
		t.Errorf("expected valid commit sha")
	}

	// 2. Amend commit
	_ = os.WriteFile(filePath, []byte("package devtool\n// updated content\n"), 0644)
	resAmend, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:       tempDir,
		StageAll:      true,
		Amend:         true,
		Message:       "[DEN-817] feat(devtool): amended commit message",
		UseLocalModel: false,
	})
	if err != nil {
		t.Fatalf("ExecuteCommit amend failed: %v", err)
	}
	if !strings.Contains(resAmend.Message, "amended commit message") {
		t.Errorf("expected amended commit message, got: %s", resAmend.Message)
	}
}

func setupCommitRepoWithRemote(t *testing.T, branch string) (repoDir, bareDir string) {
	t.Helper()
	tmp := t.TempDir()
	bareDir = filepath.Join(tmp, "remote.git")
	repoDir = filepath.Join(tmp, "local")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(bareDir, 0755); err != nil {
		t.Fatal(err)
	}
	cmdBare := exec.Command("git", "init", "--bare", "-b", branch)
	cmdBare.Dir = bareDir
	if out, err := cmdBare.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v, out: %s", err, string(out))
	}

	cmdInit := exec.Command("git", "init", "-b", branch)
	cmdInit.Dir = repoDir
	if out, err := cmdInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v, out: %s", err, string(out))
	}

	_ = exec.Command("git", "-C", repoDir, "config", "user.name", "Test Agent").Run()
	_ = exec.Command("git", "-C", repoDir, "config", "user.email", "agent@dendrite.ai").Run()
	_ = exec.Command("git", "-C", repoDir, "remote", "add", "origin", bareDir).Run()

	return repoDir, bareDir
}

func setupCommitRepoWithNamedRemote(t *testing.T, branch, remoteName string) (repoDir, bareDir string) {
	t.Helper()
	tmp := t.TempDir()
	bareDir = filepath.Join(tmp, "remote.git")
	repoDir = filepath.Join(tmp, "local")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(bareDir, 0755); err != nil {
		t.Fatal(err)
	}
	cmdBare := exec.Command("git", "init", "--bare", "-b", branch)
	cmdBare.Dir = bareDir
	if out, err := cmdBare.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v, out: %s", err, string(out))
	}

	cmdInit := exec.Command("git", "init", "-b", branch)
	cmdInit.Dir = repoDir
	if out, err := cmdInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v, out: %s", err, string(out))
	}

	_ = exec.Command("git", "-C", repoDir, "config", "user.name", "Test Agent").Run()
	_ = exec.Command("git", "-C", repoDir, "config", "user.email", "agent@dendrite.ai").Run()
	_ = exec.Command("git", "-C", repoDir, "remote", "add", remoteName, bareDir).Run()

	return repoDir, bareDir
}

func TestExecuteCommit_Push_AC1_NoUpstreamAndExistingUpstream(t *testing.T) {
	ctx := context.Background()
	repoDir, _ := setupCommitRepoWithRemote(t, "develop")

	// 1. Initial commit (no upstream exists yet -> pushes with -u origin develop)
	f1 := filepath.Join(repoDir, "file1.txt")
	if err := os.WriteFile(f1, []byte("content 1"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := ExecuteCommit(ctx, CommitOptions{
		RepoDir:       repoDir,
		Message:       "feat: initial commit with push",
		StageAll:      true,
		UseLocalModel: false,
	})
	if err != nil {
		t.Fatalf("ExecuteCommit failed: %v", err)
	}
	if !res.Pushed {
		t.Errorf("expected res.Pushed to be true")
	}
	if res.PushError != nil {
		t.Errorf("unexpected PushError: %v", res.PushError)
	}

	// Verify AC-1: test with a bare repo in t.TempDir() as remote; git rev-parse <remote>/<branch> equals HEAD
	revRemote, err := exec.Command("git", "-C", repoDir, "rev-parse", "origin/develop").Output()
	if err != nil {
		t.Fatalf("git rev-parse origin/develop failed: %v", err)
	}
	revHead, err := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %v", err)
	}
	if strings.TrimSpace(string(revRemote)) != strings.TrimSpace(string(revHead)) {
		t.Errorf("origin/develop (%s) != HEAD (%s)", strings.TrimSpace(string(revRemote)), strings.TrimSpace(string(revHead)))
	}
	if strings.TrimSpace(string(revHead)) != res.CommitSHA {
		t.Errorf("HEAD (%s) != res.CommitSHA (%s)", strings.TrimSpace(string(revHead)), res.CommitSHA)
	}

	// 2. Second commit (upstream now exists -> pushes with git push)
	f2 := filepath.Join(repoDir, "file2.txt")
	if err := os.WriteFile(f2, []byte("content 2"), 0644); err != nil {
		t.Fatal(err)
	}

	res2, err := ExecuteCommit(ctx, CommitOptions{
		RepoDir:       repoDir,
		Message:       "feat: second commit with existing upstream",
		StageAll:      true,
		UseLocalModel: false,
	})
	if err != nil {
		t.Fatalf("second ExecuteCommit failed: %v", err)
	}
	if !res2.Pushed {
		t.Errorf("expected res2.Pushed to be true")
	}
	if res2.PushError != nil {
		t.Errorf("unexpected PushError: %v", res2.PushError)
	}

	revRemote2, _ := exec.Command("git", "-C", repoDir, "rev-parse", "origin/develop").Output()
	revHead2, _ := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output()
	if strings.TrimSpace(string(revRemote2)) != strings.TrimSpace(string(revHead2)) {
		t.Errorf("origin/develop (%s) != HEAD (%s)", strings.TrimSpace(string(revRemote2)), strings.TrimSpace(string(revHead2)))
	}
}

func TestExecuteCommit_Push_AC1_NonOriginRemote(t *testing.T) {
	ctx := context.Background()
	repoDir, _ := setupCommitRepoWithNamedRemote(t, "feat-custom", "backup")

	f := filepath.Join(repoDir, "file.txt")
	if err := os.WriteFile(f, []byte("custom content"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := ExecuteCommit(ctx, CommitOptions{
		RepoDir:       repoDir,
		Message:       "feat: commit to non-origin remote",
		StageAll:      true,
		UseLocalModel: false,
	})
	if err != nil {
		t.Fatalf("ExecuteCommit failed: %v", err)
	}
	if !res.Pushed {
		t.Errorf("expected res.Pushed to be true")
	}
	if res.PushError != nil {
		t.Errorf("unexpected PushError: %v", res.PushError)
	}

	revRemote, err := exec.Command("git", "-C", repoDir, "rev-parse", "backup/feat-custom").Output()
	if err != nil {
		t.Fatalf("git rev-parse backup/feat-custom failed: %v", err)
	}
	revHead, _ := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output()
	if strings.TrimSpace(string(revRemote)) != strings.TrimSpace(string(revHead)) {
		t.Errorf("backup/feat-custom != HEAD")
	}
}

func TestExecuteCommit_Push_AC2_NoRemote_DryRun_NoPush_EnvDisabled(t *testing.T) {
	ctx := context.Background()

	// Subtest 1: No remote configured
	t.Run("NoRemote", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Agent").Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.email", "agent@dendrite.ai").Run()
		_ = os.WriteFile(filepath.Join(tempDir, "f.txt"), []byte("data"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       tempDir,
			Message:       "feat: commit without remote",
			StageAll:      true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Pushed {
			t.Errorf("expected Pushed to be false")
		}
		if !res.PushSkipped {
			t.Errorf("expected PushSkipped to be true")
		}
		if res.PushReason != "no remote configured" {
			t.Errorf("expected PushReason 'no remote configured', got %q", res.PushReason)
		}
		if res.PushError != nil {
			t.Errorf("expected PushError to be nil, got: %v", res.PushError)
		}
	})

	// Subtest 2: DryRun
	t.Run("DryRun", func(t *testing.T) {
		repoDir, _ := setupCommitRepoWithRemote(t, "develop")
		_ = os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("dry run"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       repoDir,
			Message:       "feat: dry run commit",
			StageAll:      true,
			DryRun:        true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Pushed {
			t.Errorf("expected Pushed to be false")
		}
		if !res.PushSkipped {
			t.Errorf("expected PushSkipped to be true")
		}
		if res.PushReason != "dry-run mode" {
			t.Errorf("expected PushReason 'dry-run mode', got %q", res.PushReason)
		}
	})

	// Subtest 3: NoPush flag
	t.Run("NoPush", func(t *testing.T) {
		repoDir, _ := setupCommitRepoWithRemote(t, "develop")
		_ = os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("no push content"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       repoDir,
			Message:       "feat: commit with no-push",
			StageAll:      true,
			NoPush:        true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Pushed {
			t.Errorf("expected Pushed to be false")
		}
		if !res.PushSkipped {
			t.Errorf("expected PushSkipped to be true")
		}
		if res.PushReason != "--no-push flag specified" {
			t.Errorf("expected PushReason '--no-push flag specified', got %q", res.PushReason)
		}
		if res.PushError != nil {
			t.Errorf("expected PushError to be nil")
		}
		out, _ := exec.Command("git", "-C", repoDir, "rev-parse", "origin/develop").CombinedOutput()
		if !strings.Contains(string(out), "unknown revision") && !strings.Contains(string(out), "fatal:") {
			t.Errorf("remote tracking branch should not exist yet")
		}
	})

	// Subtest 4: DENDRITE_COMMIT_PUSH=false
	t.Run("EnvDisabled", func(t *testing.T) {
		t.Setenv("DENDRITE_COMMIT_PUSH", "false")
		repoDir, _ := setupCommitRepoWithRemote(t, "develop")
		_ = os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("env disabled content"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       repoDir,
			Message:       "feat: commit with env push disabled",
			StageAll:      true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Pushed {
			t.Errorf("expected Pushed to be false")
		}
		if !res.PushSkipped {
			t.Errorf("expected PushSkipped to be true")
		}
		if res.PushReason != "DENDRITE_COMMIT_PUSH=false" {
			t.Errorf("expected PushReason 'DENDRITE_COMMIT_PUSH=false', got %q", res.PushReason)
		}
		if res.PushError != nil {
			t.Errorf("expected PushError to be nil")
		}
	})
}

func TestExecuteCommit_Push_AC3_RejectedPushAfterAmend(t *testing.T) {
	ctx := context.Background()
	repoDir, _ := setupCommitRepoWithRemote(t, "develop")

	f := filepath.Join(repoDir, "f.txt")
	_ = os.WriteFile(f, []byte("initial"), 0644)

	res1, err := ExecuteCommit(ctx, CommitOptions{
		RepoDir:       repoDir,
		Message:       "feat: initial push",
		StageAll:      true,
		UseLocalModel: false,
	})
	if err != nil || !res1.Pushed {
		t.Fatalf("initial commit and push failed: %v", err)
	}

	// Amend the commit locally and attempt commit again without force
	_ = os.WriteFile(f, []byte("amended content"), 0644)
	resAmend, err := ExecuteCommit(ctx, CommitOptions{
		RepoDir:       repoDir,
		Message:       "feat: amended push without force",
		StageAll:      true,
		Amend:         true,
		UseLocalModel: false,
	})
	if err != nil {
		t.Fatalf("ExecuteCommit should return nil err even on push rejection, got: %v", err)
	}

	// Local commit kept
	if resAmend.CommitSHA == "" || resAmend.CommitSHA == "unknown-sha" {
		t.Errorf("expected valid local commit sha kept, got %s", resAmend.CommitSHA)
	}
	if resAmend.CommitSHA == res1.CommitSHA {
		t.Errorf("expected new commit sha after amend")
	}

	// Push rejected without force
	if resAmend.Pushed {
		t.Errorf("expected Pushed to be false on rejected non-fast-forward push")
	}
	if resAmend.PushError == nil {
		t.Fatalf("expected PushError to be set on rejected push")
	}
	errStr := resAmend.PushError.Error()
	if !strings.Contains(errStr, "rejected") && !strings.Contains(errStr, "non-fast-forward") && !strings.Contains(errStr, "failed") {
		t.Errorf("unexpected PushError message: %s", errStr)
	}
}

func TestExecuteCommit_Push_AC4_RefuseMainAndMaster(t *testing.T) {
	ctx := context.Background()

	t.Run("RefuseMain", func(t *testing.T) {
		repoDir, _ := setupCommitRepoWithRemote(t, "main")
		_ = os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("main content"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       repoDir,
			Message:       "feat: commit on main",
			StageAll:      true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("ExecuteCommit should succeed locally: %v", err)
		}
		if res.Pushed {
			t.Errorf("expected Pushed to be false on main")
		}
		if res.PushError == nil {
			t.Fatalf("expected PushError when pushing main is refused")
		}
		if !strings.Contains(res.PushError.Error(), "pushing main directly is refused") {
			t.Errorf("expected refusal error for main, got: %v", res.PushError)
		}
		if !res.PushSkipped {
			t.Errorf("expected PushSkipped to be true")
		}
	})

	t.Run("RefuseMaster", func(t *testing.T) {
		repoDir, _ := setupCommitRepoWithRemote(t, "master")
		_ = os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("master content"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       repoDir,
			Message:       "feat: commit on master",
			StageAll:      true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("ExecuteCommit should succeed locally: %v", err)
		}
		if res.Pushed {
			t.Errorf("expected Pushed to be false on master")
		}
		if res.PushError == nil {
			t.Fatalf("expected PushError when pushing master is refused")
		}
		if !strings.Contains(res.PushError.Error(), "pushing master directly is refused") {
			t.Errorf("expected refusal error for master, got: %v", res.PushError)
		}
		if !res.PushSkipped {
			t.Errorf("expected PushSkipped to be true")
		}
	})
}

func TestPushAfterCommit_RemoteCommandError(t *testing.T) {
	ctx := context.Background()
	res := &CommitResult{}
	opts := CommitOptions{RepoDir: t.TempDir()}

	runGit := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = opts.RepoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}

	err := pushAfterCommit(ctx, opts, res, runGit)
	if err != nil {
		t.Fatalf("expected nil err on remote error fallback: %v", err)
	}
	if !res.PushSkipped || res.PushReason != "no remote configured" {
		t.Errorf("expected push skipped with 'no remote configured', got %+v", res)
	}
}

func TestExtractTicketIDAndTitle(t *testing.T) {
	tests := []struct {
		name          string
		expectedID    string
		expectedTitle string
	}{
		{"[DEN-101] Fix bug in parser", "DEN-101", "Fix bug in parser"},
		{"[DEND-202] Add graph node", "DEND-202", "Add graph node"},
		{"DEN-303 Fix without brackets", "DEN-303", "Fix without brackets"},
		{"No ticket in this title", "", "No ticket in this title"},
	}

	for _, tt := range tests {
		id, title := extractTicketIDAndTitle(tt.name)
		if id != tt.expectedID || title != tt.expectedTitle {
			t.Errorf("extractTicketIDAndTitle(%q) = (%q, %q); want (%q, %q)", tt.name, id, title, tt.expectedID, tt.expectedTitle)
		}
	}
}

func TestInferScopeFromFiles(t *testing.T) {
	tests := []struct {
		files    []string
		expected string
	}{
		{[]string{"pkg/devtool/foo.go"}, "devtool"},
		{[]string{"pkg/devtool/commit.go"}, "devtool"},
		{[]string{"pkg/gateway/proxy.go"}, "gateway"},
		{[]string{"pkg/rag/search.go"}, "rag"},
		{[]string{"cmd/dendrite/ask/ask.go"}, "rag"},
		{[]string{"pkg/agent/triage/route.go"}, "triage"},
		{[]string{"pkg/inference/engine.go"}, "inference"},
		{[]string{"pkg/env/detector.go"}, "env"},
		{[]string{"cmd/dendrite/main.go"}, "cli"},
		{[]string{"docs/index.md"}, "docs"},
		{[]string{"scripts/setup.sh"}, "scripts"},
		{[]string{"unknown/file.go"}, "repo"},
		{nil, "repo"},
	}

	for _, tt := range tests {
		got := inferScopeFromFiles(tt.files)
		if got != tt.expected {
			t.Errorf("inferScopeFromFiles(%v) = %q; want %q", tt.files, got, tt.expected)
		}
	}
}

func TestInferTypeFromTitle(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"Fix login bug", "fix"},
		{"Fixing memory leak", "fix"},
		{"Doc update for readme", "docs"},
		{"Guide for setup", "docs"},
		{"Refactor auth middleware", "refactor"},
		{"Test suite additions", "test"},
		{"CI workflow update", "ci"},
		{"Pipeline configuration", "ci"},
		{"Implement new feature", "feat"},
	}

	for _, tt := range tests {
		got := inferTypeFromTitle(tt.title)
		if got != tt.expected {
			t.Errorf("inferTypeFromTitle(%q) = %q; want %q", tt.title, got, tt.expected)
		}
	}
}

func TestIsCannedOrInvalidCommitMessage_AdditionalCases(t *testing.T) {
	valid := []string{
		"style: format code",
		"build: update cargo",
		"perf: optimize query",
		"revert: rollback commit",
		"[DEN-101] style: format code",
		"[DEN-101] perf: improve speed",
	}
	for _, v := range valid {
		if isCannedOrInvalidCommitMessage(v) {
			t.Errorf("expected isCannedOrInvalidCommitMessage(%q) = false, got true", v)
		}
	}

	invalid := []string{
		"here's the commit message for you",
		"here is the commit message",
		"as an ai, here is the message",
	}
	for _, inv := range invalid {
		if !isCannedOrInvalidCommitMessage(inv) {
			t.Errorf("expected isCannedOrInvalidCommitMessage(%q) = true, got false", inv)
		}
	}
}

func TestSynthesizeDeterministicCommitMessage(t *testing.T) {
	// 1. smart != nil with title, scope, type, and > 8 staged files
	smart := &SmartTicketInfo{
		ID:    "DEN-100",
		Title: "[DEN-100] implement awesome feature",
		Scope: "gateway",
		Type:  "feat",
	}
	files10 := []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"}
	msg1 := synthesizeDeterministicCommitMessage("DEN-100", smart, "", files10)
	if !strings.HasPrefix(msg1, "feat(gateway): implement awesome feature") {
		t.Errorf("unexpected msg1: %s", msg1)
	}
	if !strings.Contains(msg1, "and 2 additional files") {
		t.Errorf("expected additional files line in msg1: %s", msg1)
	}

	// 2. smart != nil without ID in title and empty scope/type
	smartEmpty := &SmartTicketInfo{
		Title: "plain title",
	}
	msgEmpty := synthesizeDeterministicCommitMessage("", smartEmpty, "", []string{"f1"})
	if !strings.HasPrefix(msgEmpty, "feat(repo): plain title") {
		t.Errorf("unexpected msgEmpty: %s", msgEmpty)
	}

	// 3. smart == nil with diffStat
	msg2 := synthesizeDeterministicCommitMessage("", nil, "1 file changed, 10 insertions\n", nil)
	if !strings.HasPrefix(msg2, "chore: update files (1 file changed, 10 insertions)") {
		t.Errorf("unexpected msg2: %s", msg2)
	}

	// 4. smart == nil, no diffStat, but staged files present
	msg3 := synthesizeDeterministicCommitMessage("", nil, "", []string{"pkg/devtool/commit.go"})
	if !strings.HasPrefix(msg3, "feat(devtool): update devtool capabilities and synchronizations") {
		t.Errorf("unexpected msg3: %s", msg3)
	}

	// 5. smart == nil, no diffStat, no staged files
	msg4 := synthesizeDeterministicCommitMessage("", nil, "", nil)
	if msg4 != "feat(repo): autonomous repository update" {
		t.Errorf("unexpected msg4: %s", msg4)
	}
}

func TestResolveSmartTicket_GitBranch(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "heal/DEN-777-something", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

	initFile := filepath.Join(tempDir, "init.txt")
	_ = os.WriteFile(initFile, []byte("init"), 0644)
	_ = exec.Command("git", "-C", tempDir, "add", "init.txt").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "initial commit").Run()

	info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/gateway/test.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != "DEN-777" {
		t.Errorf("expected DEN-777 from branch, got %s", info.ID)
	}
	if !strings.Contains(info.Source, "Git Branch") {
		t.Errorf("expected Source to contain Git Branch, got %s", info.Source)
	}

	// Non-ticket branch falls through
	_ = exec.Command("git", "-C", tempDir, "checkout", "-b", "feature-no-id").Run()
	info2, err2 := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/gateway/test.go"})
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if info2.ID != "" {
		t.Errorf("expected empty ID for feature-no-id, got %s", info2.ID)
	}
}

func TestResolveSmartTicket_PlaneCases(t *testing.T) {
	isolateEnvironment(t)
	var handler http.HandlerFunc
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler != nil {
			handler(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	t.Setenv("PLANE_BASE_URL", ts.URL)
	t.Setenv("PLANE_API_KEY", "custom_plane_key")
	t.Setenv("PLANE_WORKSPACE_SLUG", "workspace")
	t.Setenv("PLANE_PROJECT_ID", "project")

	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()
	initFile := filepath.Join(tempDir, "init.txt")
	_ = os.WriteFile(initFile, []byte("init"), 0644)
	_ = exec.Command("git", "-C", tempDir, "add", "init.txt").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "init").Run()

	// Case 1: In Progress issue (includes item without ticket ID to cover continue)
	t.Run("InProgress", func(t *testing.T) {
		handler = func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"id":               "00000000-0000-0000-0000-000000000000",
						"name":             "Issue without ticket id",
						"state_slug":       "in-progress",
						"description_html": "<p>no id</p>",
					},
					{
						"id":               "11111111-1111-1111-1111-111111111111",
						"name":             "[DEN-301] Fix something in progress",
						"state":            "8e46611a-87ed-457f-8be0-3c238b37e6eb",
						"state_slug":       "in-progress",
						"description_html": "<p>some desc</p>",
					},
				},
			})
		}
		info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"file.txt"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != "DEN-301" {
			t.Errorf("expected DEN-301, got %s", info.ID)
		}
		if info.Source != "Plane v2 (In Progress)" {
			t.Errorf("expected Plane v2 (In Progress), got %s", info.Source)
		}
	})

	// Case 2: Matching staged files with score bonus for to-verify
	t.Run("DiffCorrelation", func(t *testing.T) {
		handler = func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"id":               "00000000-0000-0000-0000-000000000000",
						"name":             "Issue without ticket id",
						"state_slug":       "open",
						"description_html": "<p>touching pkg/auth/handler.go</p>",
					},
					{
						"id":               "22222222-2222-2222-2222-222222222222",
						"name":             "[DEN-302] Update auth service",
						"state":            "f36249b3-f67a-427a-ab8a-cf1693ebdada",
						"state_slug":       "to-verify",
						"description_html": "<p>touching pkg/auth/handler.go and models</p>",
					},
				},
			})
		}
		info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/auth/handler.go"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != "DEN-302" {
			t.Errorf("expected DEN-302, got %s", info.ID)
		}
		if !strings.Contains(info.Source, "Diff Correlation Score") {
			t.Errorf("expected Diff Correlation Score in source, got %s", info.Source)
		}
	})

	// Case 3: Recent To Verify when staged files don't correlate
	t.Run("RecentToVerify", func(t *testing.T) {
		handler = func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"id":               "33333333-3333-3333-3333-333333333333",
						"name":             "[DEN-303] Unrelated issue in to-verify",
						"state_slug":       "to-verify",
						"description_html": "<p>something else</p>",
					},
				},
			})
		}
		info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"xyz/unmatched.txt"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != "DEN-303" {
			t.Errorf("expected DEN-303, got %s", info.ID)
		}
		if !strings.Contains(info.Source, "Plane v2") {
			t.Errorf("expected Plane v2 in source, got %s", info.Source)
		}
	})
}

func TestFetchPlaneIssues_Errors(t *testing.T) {
	isolateEnvironment(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/500") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/badjson") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not valid json"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	ctx := context.Background()

	// 1. Status 500
	t.Setenv("PLANE_BASE_URL", ts.URL+"/500")
	pClient, _ := plane.NewClientFromEnv()
	_, err := fetchPlaneIssues(ctx, pClient)
	if err == nil || !strings.Contains(err.Error(), "plane api status 500") {
		t.Errorf("expected plane api status 500, got: %v", err)
	}

	// 2. Bad JSON
	t.Setenv("PLANE_BASE_URL", ts.URL+"/badjson")
	pClient2, _ := plane.NewClientFromEnv()
	_, err2 := fetchPlaneIssues(ctx, pClient2)
	if err2 == nil {
		t.Errorf("expected decode error on bad JSON")
	}

	// 3. Invalid URL (http.NewRequest error)
	t.Setenv("PLANE_BASE_URL", "://bad url")
	pClient3, _ := plane.NewClientFromEnv()
	_, err3 := fetchPlaneIssues(ctx, pClient3)
	if err3 == nil {
		t.Errorf("expected request creation error on bad URL")
	}

	// 4. Empty API Key fallback (line 260) & connection error (line 268)
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:59999")
	t.Setenv("PLANE_API_KEY", "")
	pClient4, _ := plane.NewClientFromEnv()
	_, err4 := fetchPlaneIssues(ctx, pClient4)
	if err4 == nil {
		t.Errorf("expected connection error on closed port")
	}
}

func TestResolveSmartTicket_DevStatusDirMatch(t *testing.T) {
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:59999") // dead plane endpoint
	tempDir := t.TempDir()

	devStatusContent := "# Development Status\n\n- [ ] **DEN-414**: Gateway component updates (pkg/gateway)\n"
	_ = os.WriteFile(filepath.Join(tempDir, "DEV_STATUS.md"), []byte(devStatusContent), 0644)

	// Staged file has dir "pkg/gateway" which matches the title via dirLower
	info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/gateway/client.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != "DEN-414" {
		t.Errorf("expected DEN-414, got %s", info.ID)
	}
}

func TestGenerateLocalCommitMessage_EdgeCases(t *testing.T) {
	ctx := context.Background()

	// 1. model == "" defaults to qwen3.8:27b, and ticket with title
	smart := &SmartTicketInfo{
		Title: "Test Smart Title",
	}
	msg, _, err := GenerateLocalCommitMessageWithContext(
		ctx,
		"diff stat",
		"full diff",
		"DEN-001",
		smart,
		"",                       // empty model -> defaults to qwen3.8:27b
		"http://127.0.0.1:59999", // dead ollama
		"http://127.0.0.1:59998", // dead litellm
		"",
		[]string{"pkg/devtool/devtool.go"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(msg, "[DEN-001]") {
		t.Errorf("expected [DEN-001] prefix, got: %s", msg)
	}

	// 2. oxModel without "qwen" (e.g. oxillama/deepseek) -> defaults to qwen3.8:27b
	tsOx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["model"] == "qwen3.8:27b" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"choices": []map[string]interface{}{
						{"message": map[string]string{"content": "feat(core): fallback model resolution"}},
					},
				})
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer tsOx.Close()

	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", tsOx.URL)
	t.Setenv("DENDRITE_COMMIT_PREFER_OXILLAMA", "true")

	msg2, _, err2 := GenerateLocalCommitMessageWithContext(
		ctx,
		"diff",
		"diff",
		"DEN-002",
		nil,
		"oxillama/deepseek",
		"",
		"",
		"",
		nil,
	)
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if !strings.Contains(msg2, "feat(core): fallback model resolution") {
		t.Errorf("expected synthesized message, got: %s", msg2)
	}

	// 3. OxiLLaMa HTTP canned response and 500 error fall through
	tsOxFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsOxFail.Close()

	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", tsOxFail.URL)

	// OxiLLaMa CLI script exits with error 1
	tempDir := t.TempDir()
	modelsDir := filepath.Join(tempDir, "models")
	modelDir := filepath.Join(modelsDir, "qwen_testfail")
	_ = os.MkdirAll(modelDir, 0755)

	scriptFail := filepath.Join(tempDir, "oxillama")
	_ = os.WriteFile(scriptFail, []byte("#!/bin/sh\nexit 1\n"), 0755)

	t.Setenv("DENDRITE_OXILLAMA_BIN", scriptFail)
	t.Setenv("DENDRITE_OXILLAMA_MODELS_DIR", modelsDir)

	// Ollama server returns 500
	tsOllamaFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsOllamaFail.Close()

	// LiteLLM server returns 500
	tsLiteLLMFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsLiteLLMFail.Close()

	msg3, mUsed3, err3 := GenerateLocalCommitMessageWithContext(
		ctx,
		"diffstat",
		"fulldiff",
		"DEN-003",
		nil,
		"oxillama/qwen:testfail",
		tsOllamaFail.URL,
		tsLiteLLMFail.URL,
		"",
		nil,
	)
	if err3 != nil {
		t.Fatalf("unexpected error on cascade fallback: %v", err3)
	}
	if mUsed3 != "deterministic-fallback" {
		t.Errorf("expected deterministic-fallback, got: %s", mUsed3)
	}
	if !strings.HasPrefix(msg3, "[DEN-003]") {
		t.Errorf("expected [DEN-003] prefix, got: %s", msg3)
	}
}

func TestExecuteCommit_StagingBranches(t *testing.T) {
	ctx := context.Background()

	// 1. Pre-staged file without StageAll and without Files
	t.Run("AlreadyStagedOnly", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

		f1 := filepath.Join(tempDir, "staged.txt")
		_ = os.WriteFile(f1, []byte("staged content"), 0644)
		f2 := filepath.Join(tempDir, "unstaged.txt")
		_ = os.WriteFile(f2, []byte("unstaged content"), 0644)

		_ = exec.Command("git", "-C", tempDir, "add", "staged.txt").Run()

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       tempDir,
			Message:       "feat: commit pre-staged only",
			StageAll:      false,
			NoPush:        true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Files) != 1 || res.Files[0] != "staged.txt" {
			t.Errorf("expected only staged.txt in commit, got %v", res.Files)
		}
	})

	// 2. Files slice with empty string entry
	t.Run("FilesSliceEmptyString", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

		f := filepath.Join(tempDir, "valid.txt")
		_ = os.WriteFile(f, []byte("hello"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       tempDir,
			Message:       "feat: commit with empty file path entry",
			Files:         []string{"", "   ", "valid.txt"},
			NoPush:        true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Files) != 1 || res.Files[0] != "valid.txt" {
			t.Errorf("expected only valid.txt, got %v", res.Files)
		}
	})

	// 3. StageAll failure (unreadable directory with chmod 0000)
	t.Run("StageAllFailure", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

		unreadableDir := filepath.Join(tempDir, "unreadable")
		_ = os.MkdirAll(unreadableDir, 0755)
		_ = os.WriteFile(filepath.Join(unreadableDir, "f.txt"), []byte("data"), 0644)
		_ = os.Chmod(unreadableDir, 0000)
		defer os.Chmod(unreadableDir, 0755)

		_, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:  tempDir,
			StageAll: true,
			NoPush:   true,
		})
		if err == nil {
			t.Fatalf("expected error from git add -A with unreadable directory")
		}
	})

	// 4. Default StageAll failure (when alreadyStaged == "" and !opts.Amend)
	t.Run("DefaultStageAllFailure", func(t *testing.T) {
		tempDir := t.TempDir()
		_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

		unreadableDir := filepath.Join(tempDir, "unreadable_default")
		_ = os.MkdirAll(unreadableDir, 0755)
		_ = os.WriteFile(filepath.Join(unreadableDir, "f.txt"), []byte("data"), 0644)
		_ = os.Chmod(unreadableDir, 0000)
		defer os.Chmod(unreadableDir, 0755)

		_, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:  tempDir,
			StageAll: false,
			NoPush:   true,
		})
		if err == nil {
			t.Fatalf("expected error from default git add -A with unreadable directory")
		}
	})

	// 5. Amend with no new staged changes
	t.Run("AmendNoNewChanges", func(t *testing.T) {
		isolateEnvironment(t)
		tempDir := t.TempDir()
		_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
		_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

		f := filepath.Join(tempDir, "initial.txt")
		_ = os.WriteFile(f, []byte("initial data"), 0644)
		_ = exec.Command("git", "-C", tempDir, "add", "-A").Run()
		_ = exec.Command("git", "-C", tempDir, "commit", "-m", "feat: initial commit").Run()

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       tempDir,
			Message:       "feat: amend message without staged changes",
			Amend:         true,
			NoPush:        true,
			NoSmart:       true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("unexpected error amending: %v", err)
		}
		if len(res.Files) == 0 {
			t.Errorf("expected files to be populated from diff-tree")
		}
		if res.Message != "feat: amend message without staged changes" {
			t.Errorf("unexpected message: %s", res.Message)
		}
	})
}

func TestExecuteCommit_ZeroMockPolicyGate(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

	mockFile := filepath.Join(tempDir, "service.go")
	bannedType := "type " + "Mock" + "Service struct{}"
	_ = os.WriteFile(mockFile, []byte("package main\n\n"+bannedType+"\n"), 0644)

	// 1. Attempt without AllowMocks -> rejected
	_, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:    tempDir,
		Message:    "feat: add mock service",
		StageAll:   true,
		AllowMocks: false,
		NoPush:     true,
		NoSmart:    true,
	})
	if err == nil || !strings.Contains(err.Error(), "zero-mock policy violation") {
		t.Fatalf("expected zero-mock policy violation error, got: %v", err)
	}

	// 2. Attempt with AllowMocks: true -> accepted
	res, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:    tempDir,
		Message:    "feat: emergency bypass mock gate",
		StageAll:   true,
		AllowMocks: true,
		NoPush:     true,
		NoSmart:    true,
	})
	if err != nil {
		t.Fatalf("expected commit with AllowMocks: true to succeed, got: %v", err)
	}
	if res.CommitSHA == "" {
		t.Errorf("expected valid commit sha")
	}
}

func TestExecuteCommit_SmartTicketOptions(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

	f := filepath.Join(tempDir, "f.txt")
	_ = os.WriteFile(f, []byte("data"), 0644)

	// 1. Explicit SmartTicket option
	smart := &SmartTicketInfo{
		ID:     "DEN-555",
		Title:  "Explicit smart ticket title",
		Scope:  "cli",
		Type:   "feat",
		Source: "User Supplied",
	}
	res, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:     tempDir,
		TicketID:    "DEN-555",
		StageAll:    true,
		SmartTicket: smart,
		NoSmart:     true,
		NoPush:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TicketID != "DEN-555" {
		t.Errorf("expected DEN-555, got %s", res.TicketID)
	}
	if res.SmartSource != "User Supplied" {
		t.Errorf("expected User Supplied, got %s", res.SmartSource)
	}

	// 2. NoSmart: true with TicketID == ""
	f2 := filepath.Join(tempDir, "f2.txt")
	_ = os.WriteFile(f2, []byte("data 2"), 0644)
	res2, err2 := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:  tempDir,
		StageAll: true,
		NoSmart:  true,
		NoPush:   true,
	})
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if res2.TicketID != "" {
		t.Errorf("expected empty ticket ID with NoSmart, got %s", res2.TicketID)
	}
}

func TestExecuteCommit_PreCommitHookFailure(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

	hooksDir := filepath.Join(tempDir, ".git", "hooks")
	_ = os.MkdirAll(hooksDir, 0755)
	hookPath := filepath.Join(hooksDir, "pre-commit")
	_ = os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0755)

	f := filepath.Join(tempDir, "sample.txt")
	_ = os.WriteFile(f, []byte("data"), 0644)

	_, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:  tempDir,
		Message:  "feat: failing pre-commit hook",
		StageAll: true,
		NoPush:   true,
		NoSmart:  true,
	})
	if err == nil || !strings.Contains(err.Error(), "git commit failed") {
		t.Fatalf("expected git commit failed error, got: %v", err)
	}
}

func TestExecuteCommit_RevParseHeadFailure(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

	hooksDir := filepath.Join(tempDir, ".git", "hooks")
	_ = os.MkdirAll(hooksDir, 0755)
	hookPath := filepath.Join(hooksDir, "post-commit")
	_ = os.WriteFile(hookPath, []byte("#!/bin/sh\nrm -f .git/refs/heads/*\n"), 0755)

	f := filepath.Join(tempDir, "sample.txt")
	_ = os.WriteFile(f, []byte("data"), 0644)

	res, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:  tempDir,
		Message:  "feat: commit with broken rev-parse",
		StageAll: true,
		NoPush:   true,
		NoSmart:  true,
	})
	if err != nil {
		t.Fatalf("commit itself should succeed: %v", err)
	}
	if res.CommitSHA != "unknown-sha" {
		t.Errorf("expected unknown-sha, got %s", res.CommitSHA)
	}
}

func TestExecuteCommit_LocalModelFallback(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

	f := filepath.Join(tempDir, "f.txt")
	_ = os.WriteFile(f, []byte("data for local model fallback"), 0644)

	// Local model endpoints return 500
	tsFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsFail.Close()

	res, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir:          tempDir,
		StageAll:         true,
		UseLocalModel:    true,
		OllamaEndpoint:   tsFail.URL,
		LiteLLMEndpoint:  tsFail.URL,
		OxillamaEndpoint: tsFail.URL,
		NoPush:           true,
		NoSmart:          true,
	})
	if err != nil {
		t.Fatalf("ExecuteCommit with failing local model endpoints should fallback deterministically, got: %v", err)
	}
	if res.CommitSHA == "" || res.CommitSHA == "unknown-sha" {
		t.Errorf("expected valid commit sha, got: %s", res.CommitSHA)
	}
	if !strings.HasPrefix(res.Message, "feat(repo):") && !strings.HasPrefix(res.Message, "chore:") {
		t.Errorf("expected synthesized deterministic message, got: %s", res.Message)
	}
}

func TestExecuteCommit_DiffCachedError(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()
	f := filepath.Join(tempDir, "sample.txt")
	_ = os.WriteFile(f, []byte("hello"), 0644)
	_ = exec.Command("git", "-C", tempDir, "add", "sample.txt").Run()

	// Corrupt index so git diff --cached --name-only fails
	_ = os.WriteFile(filepath.Join(tempDir, ".git", "index"), []byte("corrupt-index-data"), 0644)

	_, err := ExecuteCommit(context.Background(), CommitOptions{
		RepoDir: tempDir,
		NoPush:  true,
		// Amend with nothing staged skips 'git add -A', so the corrupt index surfaces at the staged-files check.
		Amend:   true,
		NoSmart: true,
	})
	if err == nil || !strings.Contains(err.Error(), "failed to check staged files") {
		t.Errorf("expected failed to check staged files error, got: %v", err)
	}
}

func TestResolveSmartTicket_EdgeCases(t *testing.T) {
	isolateEnvironment(t)
	var handler http.HandlerFunc
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler != nil {
			handler(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	t.Setenv("PLANE_BASE_URL", ts.URL)
	t.Setenv("PLANE_API_KEY", "custom_plane_key")
	t.Setenv("PLANE_WORKSPACE_SLUG", "workspace")
	t.Setenv("PLANE_PROJECT_ID", "project")

	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()
	initFile := filepath.Join(tempDir, "init.txt")
	_ = os.WriteFile(initFile, []byte("init"), 0644)
	_ = exec.Command("git", "-C", tempDir, "add", "init.txt").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "init").Run()

	// 1. Staged file base name match (base length > 4) and dir match
	handler = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"id":               "44444444-4444-4444-4444-444444444444",
					"name":             "[DEN-404] Update custom controller",
					"state_slug":       "open",
					"description_html": "<p>touching controller.go and pkg/custom</p>",
				},
			},
		})
	}
	info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/custom/controller.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != "DEN-404" {
		t.Errorf("expected DEN-404, got %s", info.ID)
	}

	// 2. Empty repoDir branch check (repoDir == "")
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:59999")
	infoEmptyRepo, errEmptyRepo := ResolveSmartTicket(context.Background(), "", []string{"file.txt"})
	if errEmptyRepo != nil {
		t.Fatalf("unexpected error with empty repoDir: %v", errEmptyRepo)
	}
	if infoEmptyRepo == nil {
		t.Errorf("expected non-nil infoEmptyRepo")
	}
}

func TestGenerateLocalCommitMessage_CannedResponses(t *testing.T) {
	isolateEnvironment(t)
	ctx := context.Background()

	// OxiLLaMa returns canned response
	tsOx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "processed your request safely with zero cloud tokens consumed"}},
			},
		})
	}))
	defer tsOx.Close()

	// Ollama returns canned response
	tsOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"response": "As an AI, here is the commit message: feat(repo): update",
		})
	}))
	defer tsOllama.Close()

	// LiteLLM returns empty choices
	tsLite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{},
		})
	}))
	defer tsLite.Close()

	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", tsOx.URL)
	t.Setenv("DENDRITE_COMMIT_PREFER_OXILLAMA", "true")

	msg, modelUsed, err := GenerateLocalCommitMessageWithContext(
		ctx,
		"",
		"",
		"", // empty ticketID
		nil,
		"oxillama/qwen3.8:27b",
		tsOllama.URL,
		tsLite.URL,
		"key",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if modelUsed != "deterministic-fallback" {
		t.Errorf("expected deterministic-fallback, got %s", modelUsed)
	}
	if !strings.HasPrefix(msg, "feat(repo): autonomous repository update") {
		t.Errorf("unexpected msg: %s", msg)
	}
}
