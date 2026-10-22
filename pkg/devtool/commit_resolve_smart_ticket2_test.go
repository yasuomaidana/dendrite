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
