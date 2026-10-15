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

// mockRunner allows simulating git commands and responses.
type mockRunner struct {
	responses map[string]string
	errors    map[string]error
	history   []string
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		responses: make(map[string]string),
		errors:    make(map[string]error),
		history:   make([]string, 0),
	}
}

func (m *mockRunner) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	m.history = append(m.history, key)

	if err, ok := m.errors[key]; ok {
		return m.responses[key], err
	}
	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}
	return "", nil
}

func TestGitGuard_PreFlightBranchCheck_ForbiddenMain(t *testing.T) {
	ctx := context.Background()
	mock := newMockRunner()
	// Current branch is main
	mock.responses["symbolic-ref --short HEAD"] = "main"
	// show-ref for develop succeeds
	mock.responses["show-ref --verify --quiet refs/heads/develop"] = ""

	guard := NewGitGuard("/test/repo", ModeStandard).WithRunner(mock)
	branch, err := guard.PreFlightBranchCheck(ctx)

	if !errors.Is(err, ErrForbiddenBranchMain) {
		t.Fatalf("expected ErrForbiddenBranchMain, got %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected evacuated branch to be 'develop', got '%s'", branch)
	}
}

func TestGitGuard_EnforceBranchPolicy_EvacuatesCleanly(t *testing.T) {
	ctx := context.Background()
	mock := newMockRunner()
	mock.responses["symbolic-ref --short HEAD"] = "master"
	mock.responses["show-ref --verify --quiet refs/heads/develop"] = ""

	guard := NewGitGuard("/test/repo", ModeStandard).WithRunner(mock)
	branch, err := guard.EnforceBranchPolicy(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected 'develop', got '%s'", branch)
	}
}

func TestGitGuard_DegradedMode_BlocksMergeToDevelop(t *testing.T) {
	ctx := context.Background()
	mock := newMockRunner()
	mock.responses["symbolic-ref --short HEAD"] = "develop"

	guard := NewGitGuard("/test/repo", ModeDegradedBudget).WithRunner(mock)
	err := guard.CommitAndPush(ctx, "feat: try commit to develop")
	if !errors.Is(err, ErrMergeLockedInDegradedMode) {
		t.Fatalf("expected ErrMergeLockedInDegradedMode, got %v", err)
	}
}

func TestGitGuard_CommitAndPush_NormalFlow(t *testing.T) {
	ctx := context.Background()
	mock := newMockRunner()
	mock.responses["symbolic-ref --short HEAD"] = "develop"
	mock.responses["status --porcelain"] = "M pkg/vcs/guard.go"
	mock.responses["add -A"] = ""
	mock.responses["commit -m feat(vcs): add branch guard"] = ""
	mock.responses["push --set-upstream origin develop"] = ""

	guard := NewGitGuard("/test/repo", ModeStandard).WithRunner(mock)
	err := guard.CommitAndPush(ctx, "feat(vcs): add branch guard")
	if err != nil {
		t.Fatalf("expected successful commit and push, got: %v", err)
	}
}

func TestGitGuard_CommitAndPush_DivergenceRecovery(t *testing.T) {
	ctx := context.Background()
	mock := newMockRunner()
	mock.responses["symbolic-ref --short HEAD"] = "develop"
	mock.responses["status --porcelain"] = "M file.txt"

	// First push fails due to rejection/divergence
	pushCount := 0
	divergenceErr := errors.New("git push failed: exit status 1 (stderr: ! [rejected] develop -> develop (fetch first))")

	mockDynamic := &dynamicMockRunner{
		onRun: func(args []string) (string, error) {
			cmd := strings.Join(args, " ")
			if strings.HasPrefix(cmd, "push") {
				pushCount++
				if pushCount == 1 {
					return "", divergenceErr
				}
				return "", nil
			}
			if strings.HasPrefix(cmd, "symbolic-ref") {
				return "develop", nil
			}
			if strings.HasPrefix(cmd, "status") {
				return "M file.txt", nil
			}
			return "", nil
		},
	}

	guard := NewGitGuard("/test/repo", ModeStandard).WithRunner(mockDynamic)
	err := guard.CommitAndPush(ctx, "feat: auto rebase on divergence")
	if err != nil {
		t.Fatalf("expected divergence recovery to succeed, got %v", err)
	}
	if pushCount < 2 {
		t.Fatalf("expected push to retry after rebase, but push count was %d", pushCount)
	}
}

type dynamicMockRunner struct {
	onRun func(args []string) (string, error)
}

func (d *dynamicMockRunner) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	return d.onRun(args)
}

func TestGitGuard_RealGitRepo(t *testing.T) {
	// Verify behavior with real local git commands in temporary dir
	tmpDir, err := os.MkdirTemp("", "dendrite-vcs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cmd git %v failed: %v, out: %s", args, err, string(out))
		}
	}

	runCmd("init", "-b", "main")
	runCmd("config", "user.name", "Test Agent")
	runCmd("config", "user.email", "test@dendrite.ai")

	// Commit initial file
	f := filepath.Join(tmpDir, "README.md")
	_ = os.WriteFile(f, []byte("# Test Repo"), 0644)
	runCmd("add", "README.md")
	runCmd("commit", "-m", "initial commit")

	ctx := context.Background()
	guard := NewGitGuard(tmpDir, ModeStandard)

	// PreFlight check on main should switch to develop and return ErrForbiddenBranchMain
	branch, pErr := guard.PreFlightBranchCheck(ctx)
	if !errors.Is(pErr, ErrForbiddenBranchMain) {
		t.Fatalf("expected ErrForbiddenBranchMain, got %v", pErr)
	}
	if branch != "develop" {
		t.Fatalf("expected develop branch, got %s", branch)
	}

	curr, _ := guard.CurrentBranch(ctx)
	if curr != "develop" {
		t.Fatalf("expected current branch in repo to be develop, got %s", curr)
	}
}
