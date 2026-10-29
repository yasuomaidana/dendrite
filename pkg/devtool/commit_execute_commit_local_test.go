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

func TestExecuteCommit_SpecificFilesLeaveOtherStagedWorkAlone(t *testing.T) {
	t.Setenv("DENDRITE_COMMIT_PUSH", "false")
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base"), 0644)
	_ = exec.Command("git", "-C", dir, "add", "base.txt").Run()
	_ = exec.Command("git", "-C", dir, "commit", "-q", "-m", "base").Run()

	// The developer has work in progress staged; the tool commits only its own file.
	_ = os.WriteFile(filepath.Join(dir, "wip.go"), []byte("package wip"), 0644)
	_ = exec.Command("git", "-C", dir, "add", "wip.go").Run()
	_ = os.WriteFile(filepath.Join(dir, "link.json"), []byte("{}"), 0644)

	res, err := ExecuteCommit(context.Background(), CommitOptions{
		TicketID: "DEN-1036", Files: []string{"link.json"}, Message: "chore: link", RepoDir: dir,
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(res.Files) != 1 || res.Files[0] != "link.json" {
		t.Errorf("reported files = %v; want only link.json", res.Files)
	}
	committed, _ := exec.Command("git", "-C", dir, "show", "--name-only", "--format=", "HEAD").Output()
	if strings.TrimSpace(string(committed)) != "link.json" {
		t.Errorf("HEAD contains %q; want only link.json", committed)
	}
	staged, _ := exec.Command("git", "-C", dir, "diff", "--cached", "--name-only").Output()
	if strings.TrimSpace(string(staged)) != "wip.go" {
		t.Errorf("staged after commit = %q; the developer's wip.go must stay staged", staged)
	}
}

// The git pre-commit hook honours --no-ticket-guard through the environment ExecuteCommit gives git.
func TestExecuteCommit_NoTicketGuardReachesTheGitHook(t *testing.T) {
	isolateEnvironment(t)
	t.Setenv(TicketGuardEnv, "")
	repo := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", repo).Run()
	_ = exec.Command("git", "-C", repo, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", repo, "config", "user.email", "test@test.ai").Run()
	seen := filepath.Join(t.TempDir(), "seen")
	hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho \"guard=$DENDRITE_TICKET_GUARD\" > "+seen+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		noGuard bool
		want    string
	}{{true, "guard=off"}, {false, "guard="}} {
		if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(tc.want), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ExecuteCommit(context.Background(), CommitOptions{RepoDir: repo, StageAll: true, NoSmart: true, NoPush: true, Message: "chore: x", NoTicketGuard: tc.noGuard})
		if err != nil && !tc.noGuard {
			// without the bypass the ticket guard itself may refuse; the hook then never ran for this case
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(seen); strings.TrimSpace(string(got)) != tc.want {
			t.Errorf("noGuard=%v: hook saw %q, want %q", tc.noGuard, strings.TrimSpace(string(got)), tc.want)
		}
	}
}
