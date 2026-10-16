package vcs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestGitGuard_NewGitGuard_DefaultMode(t *testing.T) {
	guard := NewGitGuard(".", "")
	if guard.Mode != ModeStandard {
		t.Fatalf("expected default ModeStandard, got %s", guard.Mode)
	}
	guard.Runner = nil
	_, _ = guard.runGit(context.Background(), "version")
}

func TestGitGuard_CurrentBranch_Errors(t *testing.T) {
	runner := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 0 && (args[0] == "symbolic-ref" || args[0] == "rev-parse")
		},
	}
	guard := NewGitGuard(".", ModeStandard).WithRunner(runner)
	_, err := guard.CurrentBranch(context.Background())
	if err == nil {
		t.Fatalf("expected error from CurrentBranch when both git calls fail, got nil")
	}
}

func TestGitGuard_EnsureAndCheckoutDevelop_CheckoutFail(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	runner := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 1 && args[0] == "checkout" && args[1] == "develop"
		},
	}
	guard := NewGitGuard(repoDir, ModeStandard).WithRunner(runner)
	err := guard.EnsureAndCheckoutDevelop(context.Background())
	if err == nil {
		t.Fatalf("expected checkout develop error, got nil")
	}
}

func TestGitGuard_EnsureAndCheckoutDevelop_RemoteDevelop(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	// Create a branch feature, delete local develop
	runGitCmd(t, repoDir, "checkout", "-b", "feature")
	runGitCmd(t, repoDir, "branch", "-D", "develop")

	guard := NewGitGuard(repoDir, ModeStandard)
	err := guard.EnsureAndCheckoutDevelop(ctx)
	if err != nil {
		t.Fatalf("expected checkout from remote develop to succeed: %v", err)
	}
}

func TestGitGuard_EnsureAndCheckoutDevelop_NeitherExists(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "bare_repo")
	_ = os.MkdirAll(repoDir, 0755)
	runGitCmd(t, repoDir, "init", "-b", "main")
	runGitCmd(t, repoDir, "config", "user.name", "Agent")
	runGitCmd(t, repoDir, "config", "user.email", "agent@dendrite.ai")
	readme := filepath.Join(repoDir, "README.md")
	_ = os.WriteFile(readme, []byte("main\n"), 0644)
	runGitCmd(t, repoDir, "add", "README.md")
	runGitCmd(t, repoDir, "commit", "-m", "init")

	guard := NewGitGuard(repoDir, ModeStandard)
	err := guard.EnsureAndCheckoutDevelop(context.Background())
	if err != nil {
		t.Fatalf("expected create develop branch: %v", err)
	}
}

func TestGitGuard_PreFlightBranchCheck_EvacuateError(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	runGitCmd(t, repoDir, "checkout", "-b", "main")

	runner := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 1 && args[0] == "checkout" && args[1] == "develop"
		},
	}
	guard := NewGitGuard(repoDir, ModeStandard).WithRunner(runner)
	_, err := guard.PreFlightBranchCheck(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to evacuate") {
		t.Fatalf("expected failed to evacuate error, got: %v", err)
	}
}

func TestGitGuard_PreFlightBranchCheck_StandardNonDevelopBranch(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	runGitCmd(t, repoDir, "checkout", "-b", "feature-work")

	guard := NewGitGuard(repoDir, ModeStandard)
	branch, err := guard.PreFlightBranchCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected develop, got %s", branch)
	}
}

func TestGitGuard_CommitAndPush_NothingToCommit(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	guard := NewGitGuard(repoDir, ModeStandard)
	err := guard.CommitAndPush(context.Background(), "feat: no changes")
	if err != nil {
		t.Fatalf("expected success on empty status, got %v", err)
	}
}

func TestGitGuard_CommitAndPush_Failures(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	// Case 1: add fails
	runner1 := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 0 && args[0] == "add"
		},
	}
	guard1 := NewGitGuard(repoDir, ModeStandard).WithRunner(runner1)
	err := guard1.CommitAndPush(ctx, "msg")
	if err == nil || !strings.Contains(err.Error(), "git add failed") {
		t.Fatalf("expected git add failure, got: %v", err)
	}

	// Case 2: status fails
	runner2 := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 0 && args[0] == "status"
		},
	}
	guard2 := NewGitGuard(repoDir, ModeStandard).WithRunner(runner2)
	err = guard2.CommitAndPush(ctx, "msg")
	if err == nil || !strings.Contains(err.Error(), "git status failed") {
		t.Fatalf("expected git status failure, got: %v", err)
	}

	// Case 3: commit fails
	newFile := filepath.Join(repoDir, "change.txt")
	_ = os.WriteFile(newFile, []byte("data\n"), 0644)
	runner3 := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 0 && args[0] == "commit"
		},
	}
	guard3 := NewGitGuard(repoDir, ModeStandard).WithRunner(runner3)
	err = guard3.CommitAndPush(ctx, "msg")
	if err == nil || !strings.Contains(err.Error(), "git commit failed") {
		t.Fatalf("expected git commit failure, got: %v", err)
	}
}

func TestGitGuard_PushWithRebaseRetry_PullRebaseFail(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	runner := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			if len(args) > 0 && args[0] == "push" {
				return true
			}
			if len(args) > 0 && args[0] == "pull" {
				return true
			}
			return false
		},
		failError: errors.New("non-fast-forward rejected"),
	}
	guard := NewGitGuard(repoDir, ModeStandard).WithRunner(runner)
	err := guard.pushWithRebaseRetry(context.Background(), "develop", 2)
	if err == nil || !strings.Contains(err.Error(), "pull --rebase failed") {
		t.Fatalf("expected pull --rebase failure, got: %v", err)
	}
}

func TestGitGuard_PushWithRebaseRetry_OtherPushError(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	runner := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 0 && args[0] == "push"
		},
		failError: errors.New("fatal: could not resolve host"),
	}
	guard := NewGitGuard(repoDir, ModeStandard).WithRunner(runner)
	err := guard.pushWithRebaseRetry(context.Background(), "develop", 2)
	if err == nil || !strings.Contains(err.Error(), "could not resolve host") {
		t.Fatalf("expected host resolution error, got: %v", err)
	}
}

type interceptorRunner struct {
	base      GitRunner
	failMatch func(args []string) bool
	failError error
}

func (r *interceptorRunner) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	if r.failMatch != nil && r.failMatch(args) {
		if r.failError != nil {
			return "", r.failError
		}
		return "", errors.New("simulated git execution failure")
	}
	return r.base.RunGit(ctx, dir, args...)
}


func TestGitGuard_CurrentBranch_DetachedHead_Success(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	runGitCmd(t, repoDir, "checkout", "HEAD~0")

	guard := NewGitGuard(repoDir, ModeStandard)
	branch, err := guard.CurrentBranch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on detached HEAD: %v", err)
	}
	if branch == "" {
		t.Fatalf("expected non-empty rev for detached HEAD")
	}
}

func TestGitGuard_EnsureAndCheckoutDevelop_TrackingFail_And_CreateFail(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	// 1. origin/develop tracking checkout fails
	runGitCmd(t, repoDir, "checkout", "-b", "feature1")
	runGitCmd(t, repoDir, "branch", "-D", "develop")
	runner1 := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 1 && args[0] == "checkout" && args[1] == "-b"
		},
	}
	guard1 := NewGitGuard(repoDir, ModeStandard).WithRunner(runner1)
	err := guard1.EnsureAndCheckoutDevelop(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to track remote origin/develop") {
		t.Fatalf("expected failed to track error, got: %v", err)
	}

	// 2. Neither exists, checkout -b develop fails
	tmp := t.TempDir()
	r2 := filepath.Join(tmp, "repo2")
	_ = os.MkdirAll(r2, 0755)
	runGitCmd(t, r2, "init", "-b", "main")
	runGitCmd(t, r2, "config", "user.name", "Agent")
	runGitCmd(t, r2, "config", "user.email", "agent@dendrite.ai")
	_ = os.WriteFile(filepath.Join(r2, "f"), []byte("data"), 0644)
	runGitCmd(t, r2, "add", "f")
	runGitCmd(t, r2, "commit", "-m", "init")

	guard2 := NewGitGuard(r2, ModeStandard).WithRunner(runner1)
	err = guard2.EnsureAndCheckoutDevelop(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to create develop branch") {
		t.Fatalf("expected failed to create develop error, got: %v", err)
	}
}

func TestGitGuard_PreFlightBranchCheck_CurrentBranchErr_And_StdDevErr(t *testing.T) {
	// 1. CurrentBranch error in PreFlightBranchCheck
	runner1 := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 0 && (args[0] == "symbolic-ref" || args[0] == "rev-parse")
		},
	}
	guard1 := NewGitGuard(".", ModeStandard).WithRunner(runner1)
	_, err := guard1.PreFlightBranchCheck(context.Background())
	if err == nil {
		t.Fatalf("expected error from PreFlightBranchCheck, got nil")
	}

	// 2. Standard branch non-develop checkout error
	repoDir, _ := setupRealRepo(t)
	runGitCmd(t, repoDir, "checkout", "-b", "some-feature")
	runner2 := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 1 && args[0] == "checkout" && args[1] == "develop"
		},
	}
	guard2 := NewGitGuard(repoDir, ModeStandard).WithRunner(runner2)
	_, err = guard2.PreFlightBranchCheck(context.Background())
	if err == nil {
		t.Fatalf("expected error from non-develop branch evacuation, got nil")
	}
}

func TestGitGuard_CommitAndPush_PolicyErr_And_MainCommit(t *testing.T) {
	// 1. Policy error in CommitAndPush
	runner1 := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 0 && (args[0] == "symbolic-ref" || args[0] == "rev-parse")
		},
	}
	guard1 := NewGitGuard(".", ModeStandard).WithRunner(runner1)
	err := guard1.CommitAndPush(context.Background(), "msg")
	if err == nil {
		t.Fatalf("expected error from CommitAndPush on policy error, got nil")
	}

	// 2. Evacuation on main in CommitAndPush
	guard2 := NewGitGuard(".", ModeStandard).WithRunner(&branchIndicatorRunner{name: "main"})
	err = guard2.CommitAndPush(context.Background(), "msg")
	if err != nil {
		t.Fatalf("expected successful commit after evacuation, got: %v", err)
	}
}

func TestGitGuard_PushWithRebaseRetry_MaxRetriesExhausted(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	runner := &interceptorRunner{
		base: &DefaultGitRunner{},
		failMatch: func(args []string) bool {
			return len(args) > 0 && args[0] == "push"
		},
		failError: errors.New("non-fast-forward rejected"),
	}
	guard := NewGitGuard(repoDir, ModeStandard).WithRunner(runner)
	err := guard.pushWithRebaseRetry(context.Background(), "develop", 2)
	if !errors.Is(err, ErrDivergenceMaxRetries) {
		t.Fatalf("expected ErrDivergenceMaxRetries, got: %v", err)
	}
}

type branchIndicatorRunner struct {
	name string
}

func (r *branchIndicatorRunner) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "symbolic-ref" {
		return r.name, nil
	}
	return "", nil
}
