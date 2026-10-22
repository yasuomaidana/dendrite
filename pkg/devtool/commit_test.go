package devtool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
