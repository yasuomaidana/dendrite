package vcs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupRealRepo creates a real local git repo paired with a real local bare remote origin.
func setupRealRepo(t *testing.T) (repoDir string, originDir string) {
	t.Helper()
	tmp := t.TempDir()
	originDir = filepath.Join(tmp, "origin.git")
	repoDir = filepath.Join(tmp, "repo")

	// 1. Create bare origin repository
	if err := os.MkdirAll(originDir, 0755); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, originDir, "init", "--bare", "-b", "develop")

	// 2. Clone repo from bare origin
	runGitCmd(t, tmp, "clone", originDir, repoDir)
	runGitCmd(t, repoDir, "config", "user.name", "Test Agent")
	runGitCmd(t, repoDir, "config", "user.email", "agent@dendrite.ai")

	// 3. Initial commit on develop
	readme := filepath.Join(repoDir, "README.md")
	_ = os.WriteFile(readme, []byte("# Real Repo\n"), 0644)
	runGitCmd(t, repoDir, "add", "README.md")
	runGitCmd(t, repoDir, "commit", "-m", "initial commit on develop")
	runGitCmd(t, repoDir, "push", "origin", "develop")

	return repoDir, originDir
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\nOutput: %s", args, dir, err, string(out))
	}
	return string(out)
}

func TestGitGuard_PreFlightBranchCheck_ForbiddenMain(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	// Switch to main locally
	runGitCmd(t, repoDir, "checkout", "-b", "main")

	guard := NewGitGuard(repoDir, ModeStandard)
	branch, err := guard.PreFlightBranchCheck(ctx)

	if !errors.Is(err, ErrForbiddenBranchMain) {
		t.Fatalf("expected ErrForbiddenBranchMain, got %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected evacuated branch to be 'develop', got '%s'", branch)
	}

	curr, _ := guard.CurrentBranch(ctx)
	if curr != "develop" {
		t.Fatalf("expected current branch on disk to be 'develop', got '%s'", curr)
	}
}

func TestGitGuard_EnforceBranchPolicy_EvacuatesCleanly(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	// Switch to master locally
	runGitCmd(t, repoDir, "checkout", "-b", "master")

	guard := NewGitGuard(repoDir, ModeStandard)
	branch, err := guard.EnforceBranchPolicy(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected 'develop', got '%s'", branch)
	}
}

func TestGitGuard_DegradedMode_BlocksMergeToDevelop(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	guard := NewGitGuard(repoDir, ModeDegradedBudget)
	err := guard.CommitAndPush(ctx, "feat: try commit to develop")
	if !errors.Is(err, ErrMergeLockedInDegradedMode) {
		t.Fatalf("expected ErrMergeLockedInDegradedMode, got %v", err)
	}
}

func TestGitGuard_CommitAndPush_NormalFlow(t *testing.T) {
	repoDir, originDir := setupRealRepo(t)
	ctx := context.Background()

	// Add file on develop
	newFile := filepath.Join(repoDir, "feature.txt")
	_ = os.WriteFile(newFile, []byte("feature content\n"), 0644)

	guard := NewGitGuard(repoDir, ModeStandard)
	err := guard.CommitAndPush(ctx, "feat(vcs): real commit and push")
	if err != nil {
		t.Fatalf("expected successful real commit and push, got: %v", err)
	}

	// Verify commit exists on remote origin
	logOut := runGitCmd(t, originDir, "log", "-n", "1", "--oneline", "develop")
	if !containsStr(logOut, "feat(vcs): real commit and push") {
		t.Fatalf("commit not found on bare origin: %s", logOut)
	}
}

func TestGitGuard_CommitAndPush_DivergenceRecovery(t *testing.T) {
	repoDir, originDir := setupRealRepo(t)
	ctx := context.Background()

	// 1. Create a second clone to push a divergent commit to origin/develop
	tmp := t.TempDir()
	secondRepo := filepath.Join(tmp, "second_clone")
	runGitCmd(t, tmp, "clone", originDir, secondRepo)
	runGitCmd(t, secondRepo, "config", "user.name", "Second Agent")
	runGitCmd(t, secondRepo, "config", "user.email", "second@dendrite.ai")

	remoteFile := filepath.Join(secondRepo, "remote_change.txt")
	_ = os.WriteFile(remoteFile, []byte("concurrent change\n"), 0644)
	runGitCmd(t, secondRepo, "add", "remote_change.txt")
	runGitCmd(t, secondRepo, "commit", "-m", "concurrent remote commit")
	runGitCmd(t, secondRepo, "push", "origin", "develop")

	// 2. In repoDir, create local commit without pulling (creating divergence)
	localFile := filepath.Join(repoDir, "local_change.txt")
	_ = os.WriteFile(localFile, []byte("local change\n"), 0644)

	guard := NewGitGuard(repoDir, ModeStandard)
	err := guard.CommitAndPush(ctx, "feat: local commit with auto divergence recovery")
	if err != nil {
		t.Fatalf("expected divergence recovery to succeed, got %v", err)
	}

	// Verify both commits are now on remote origin
	logOut := runGitCmd(t, originDir, "log", "-n", "3", "--oneline", "develop")
	if !containsStr(logOut, "concurrent remote commit") || !containsStr(logOut, "auto divergence recovery") {
		t.Fatalf("expected both commits reconciled on origin: %s", logOut)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || (len(needle) > 0 && len(haystack) > 0 && (haystack[:len(needle)] == needle || containsStr(haystack[1:], needle))))
}

func TestGitGuard_WithRunner_And_DetachedHead(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	guard := NewGitGuard(repoDir, ModeStandard).WithRunner(&DefaultGitRunner{})
	branch, err := guard.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("current branch failed: %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected develop, got %s", branch)
	}

	// Test EnsureAndCheckoutDevelop when already on develop
	if err := guard.EnsureAndCheckoutDevelop(ctx); err != nil {
		t.Fatalf("ensure develop failed: %v", err)
	}
}
