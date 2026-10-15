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
