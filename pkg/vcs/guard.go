package vcs

import (
	"Dendrite/pkg/branchpolicy"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

var (
	// ErrForbiddenBranchMain is returned when an operation attempts to mutate or commit to 'main' or 'master'.
	ErrForbiddenBranchMain = errors.New("vcs: direct work on 'main' is strictly prohibited; switched to the integration branch")
	// ErrMergeLockedInDegradedMode is returned when merging to the integration branch is attempted during DegradedBudgetMode.
	ErrMergeLockedInDegradedMode = errors.New("vcs: merging into the integration branch is forbidden in DegradedBudgetMode")
	// ErrDivergenceMaxRetries is returned when push retries after rebase are exhausted.
	ErrDivergenceMaxRetries = errors.New("vcs: remote divergence could not be resolved after rebase retries")
)

// ExecutionMode defines the operating paradigm for VCS operations.
type ExecutionMode string

const (
	// ModeStandard represents normal operation: works on the integration branch, commits and pushes immediately.
	ModeStandard ExecutionMode = "STANDARD"
	// ModeDegradedBudget represents low-token operation: works on 'local/<module>', pushes branch, never merges to the integration branch.
	ModeDegradedBudget ExecutionMode = "DEGRADED_BUDGET"
	// ModeAuditChanges represents vendor rebase/audit operation: reviews 'local/<module>', fixes, and merges to the integration branch.
	ModeAuditChanges ExecutionMode = "AUDIT_CHANGES"
)

// GitRunner executes git commands.
type GitRunner interface {
	RunGit(ctx context.Context, dir string, args ...string) (string, error)
}

// DefaultGitRunner implements GitRunner using os/exec.
type DefaultGitRunner struct{}

// RunGit executes a git command in the specified directory.
func (r *DefaultGitRunner) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s failed: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// GitGuard coordinates branch policy enforcement, main evacuation, and atomic commit-push transactions.
type GitGuard struct {
	RepoDir string
	Mode    ExecutionMode
	Runner  GitRunner
	mu      sync.Mutex
}

// NewGitGuard initializes a GitGuard for a repository directory.
func NewGitGuard(repoDir string, mode ExecutionMode) *GitGuard {
	if mode == "" {
		mode = ModeStandard
	}
	return &GitGuard{
		RepoDir: repoDir,
		Mode:    mode,
		Runner:  &DefaultGitRunner{},
	}
}

// WithRunner allows injecting a custom GitRunner for testing or sandboxing.
func (g *GitGuard) WithRunner(runner GitRunner) *GitGuard {
	g.Runner = runner
	return g
}

func (g *GitGuard) runGit(ctx context.Context, args ...string) (string, error) {
	runner := g.Runner
	if runner == nil {
		runner = &DefaultGitRunner{}
	}
	return runner.RunGit(ctx, g.RepoDir, args...)
}

// CurrentBranch returns the name of the currently checked out branch.
func (g *GitGuard) CurrentBranch(ctx context.Context) (string, error) {
	out, err := g.runGit(ctx, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		// Fallback for detached HEAD or shallow checkouts
		rev, revErr := g.runGit(ctx, "rev-parse", "--abbrev-ref", "HEAD")
		if revErr != nil {
			return "", fmt.Errorf("unable to determine current branch: %w", err)
		}
		return rev, nil
	}
	return strings.TrimSpace(out), nil
}

// target is where this repository's work integrates (see pkg/branchpolicy). A trunk (main or master) that was
// only detected, not chosen, is never an integration branch here: the guard keeps its rule that agents do not
// work on main and moves to (creating when missing) develop. Choosing main in the directives lifts that rule.
func (g *GitGuard) target() branchpolicy.Target {
	t := branchpolicy.For(g.RepoDir)
	t.Branch = t.Integration // the guard keeps agents on the integration branch, not on a ticket's feature branch
	if !t.Explicit() && (t.Branch == "main" || t.Branch == "master") {
		t.Branch = "develop"
	}
	return t
}

// IntegrationBranch is the branch this repository's work merges into.
func (g *GitGuard) IntegrationBranch() string { return g.target().Branch }

// PushRemote is the remote work is pushed to.
func (g *GitGuard) PushRemote() string { return g.target().PushRemote }

// forbidden reports whether branch is a trunk (main or master) that is not the repository's integration branch.
func forbidden(t branchpolicy.Target, branch string) bool {
	return (branch == "main" || branch == "master") && branch != t.Branch
}

// EnsureAndCheckoutIntegration guarantees the integration branch exists locally and is checked out.
func (g *GitGuard) EnsureAndCheckoutIntegration(ctx context.Context) error {
	t := g.target()
	b, remote := t.Branch, t.PushRemote
	if _, err := g.runGit(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+b); err == nil {
		if _, chkErr := g.runGit(ctx, "checkout", b); chkErr != nil {
			return fmt.Errorf("failed to checkout existing %s branch: %w", b, chkErr)
		}
		_, _ = g.runGit(ctx, "pull", "--ff-only", remote, b)
		return nil
	}
	if _, err := g.runGit(ctx, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote+"/"+b); err == nil {
		if _, chkErr := g.runGit(ctx, "checkout", "-b", b, "--track", remote+"/"+b); chkErr != nil {
			return fmt.Errorf("failed to track remote %s/%s: %w", remote, b, chkErr)
		}
		return nil
	}
	if _, chkErr := g.runGit(ctx, "checkout", "-b", b); chkErr != nil {
		return fmt.Errorf("failed to create %s branch: %w", b, chkErr)
	}
	_, _ = g.runGit(ctx, "push", "-u", remote, b)
	return nil
}

// PreFlightBranchCheck validates the current branch before any mutation or commit.
// If the active branch is 'main' or 'master', it evacuates to the integration branch and returns ErrForbiddenBranchMain.
func (g *GitGuard) PreFlightBranchCheck(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	branch, err := g.CurrentBranch(ctx)
	if err != nil {
		return "", err
	}

	t := g.target()
	if forbidden(t, branch) {
		if devErr := g.EnsureAndCheckoutIntegration(ctx); devErr != nil {
			return "", fmt.Errorf("failed to evacuate %s to %s: %w", branch, t.Branch, devErr)
		}
		return t.Branch, ErrForbiddenBranchMain
	}

	if g.Mode == ModeStandard && branch != t.Branch && !strings.HasPrefix(branch, "local/") {
		if devErr := g.EnsureAndCheckoutIntegration(ctx); devErr != nil {
			return "", devErr
		}
		return t.Branch, nil
	}

	return branch, nil
}

// EnforceBranchPolicy ensures the agent never operates on 'main' and switches to the integration branch.
func (g *GitGuard) EnforceBranchPolicy(ctx context.Context) (string, error) {
	branch, err := g.PreFlightBranchCheck(ctx)
	if errors.Is(err, ErrForbiddenBranchMain) {
		// Evacuated successfully; return the active integration branch without blocking if called as policy
		return g.target().Branch, nil
	}
	return branch, err
}

// CommitAndPush executes an atomic commit followed immediately by a remote push.
// If the remote diverges, it pulls with rebase and retries up to 3 times.
func (g *GitGuard) CommitAndPush(ctx context.Context, message string) error {
	branch, err := g.EnforceBranchPolicy(ctx)
	if err != nil {
		return err
	}

	// Guard against commits on main
	t := g.target()
	if forbidden(t, branch) {
		return ErrForbiddenBranchMain
	}

	// DegradedBudgetMode cannot commit directly to the integration branch
	if g.Mode == ModeDegradedBudget && branch == t.Branch {
		return ErrMergeLockedInDegradedMode
	}

	// 1. Stage all changes
	if _, err := g.runGit(ctx, "add", "-A"); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Check if there is anything to commit
	status, err := g.runGit(ctx, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}
	if strings.TrimSpace(status) == "" {
		// Nothing to commit, but verify push
		return g.pushWithRebaseRetry(ctx, branch, 3)
	}

	// 2. Commit
	if _, err := g.runGit(ctx, "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// 3. Immediately push to origin with auto-rebase on divergence
	return g.pushWithRebaseRetry(ctx, branch, 3)
}

func (g *GitGuard) pushWithRebaseRetry(ctx context.Context, branch string, maxRetries int) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, pushErr := g.runGit(ctx, "push", "--set-upstream", "origin", branch)
		if pushErr == nil {
			return nil
		}

		// Check if failure is divergence / rejection
		errStr := pushErr.Error()
		if strings.Contains(errStr, "rejected") || strings.Contains(errStr, "non-fast-forward") || strings.Contains(errStr, "fetch first") {
			// Pull with rebase
			if _, pullErr := g.runGit(ctx, "pull", "--rebase", "origin", branch); pullErr != nil {
				return fmt.Errorf("pull --rebase failed during divergence recovery on attempt %d: %w", attempt, pullErr)
			}
			continue
		}

		// Other push error (e.g. no remote configured or network issue)
		return pushErr
	}
	return ErrDivergenceMaxRetries
}
