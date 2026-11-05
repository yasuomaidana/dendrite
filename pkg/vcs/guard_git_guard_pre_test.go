package vcs

import (
	"context"
	"errors"
	"testing"
)

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
