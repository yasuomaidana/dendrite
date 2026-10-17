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
)

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
