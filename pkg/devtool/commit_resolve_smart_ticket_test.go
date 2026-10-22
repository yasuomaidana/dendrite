package devtool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
