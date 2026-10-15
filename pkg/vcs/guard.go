package vcs

import (
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
	ErrForbiddenBranchMain = errors.New("vcs: direct work on 'main' is strictly prohibited; switched to 'develop'")
	// ErrMergeLockedInDegradedMode is returned when merging to 'develop' is attempted during DegradedBudgetMode.
	ErrMergeLockedInDegradedMode = errors.New("vcs: merging into 'develop' is forbidden in DegradedBudgetMode")
	// ErrDivergenceMaxRetries is returned when push retries after rebase are exhausted.
	ErrDivergenceMaxRetries = errors.New("vcs: remote divergence could not be resolved after rebase retries")
)

// ExecutionMode defines the operating paradigm for VCS operations.
type ExecutionMode string

const (
	// ModeStandard represents normal operation: works on 'develop', commits and pushes immediately.
	ModeStandard ExecutionMode = "STANDARD"
	// ModeDegradedBudget represents low-token operation: works on 'local/<module>', pushes branch, never merges to 'develop'.
	ModeDegradedBudget ExecutionMode = "DEGRADED_BUDGET"
	// ModeAuditChanges represents vendor rebase/audit operation: reviews 'local/<module>', fixes, and merges to 'develop'.
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

// EnsureAndCheckoutDevelop guarantees the 'develop' branch exists locally and is checked out.
func (g *GitGuard) EnsureAndCheckoutDevelop(ctx context.Context) error {
	// Check if develop branch exists locally
	_, err := g.runGit(ctx, "show-ref", "--verify", "--quiet", "refs/heads/develop")
	if err == nil {
		// develop exists locally
		if _, chkErr := g.runGit(ctx, "checkout", "develop"); chkErr != nil {
			return fmt.Errorf("failed to checkout existing develop branch: %w", chkErr)
		}
		// Pull --ff-only if remote origin/develop exists
		_, _ = g.runGit(ctx, "pull", "--ff-only", "origin", "develop")
		return nil
	}

	// Check if remote origin/develop exists
	_, rErr := g.runGit(ctx, "show-ref", "--verify", "--quiet", "refs/remotes/origin/develop")
	if rErr == nil {
		if _, chkErr := g.runGit(ctx, "checkout", "-b", "develop", "--track", "origin/develop"); chkErr != nil {
			return fmt.Errorf("failed to track remote origin/develop: %w", chkErr)
		}
		return nil
	}

	// Create new develop branch from current HEAD and push upstream
	if _, chkErr := g.runGit(ctx, "checkout", "-b", "develop"); chkErr != nil {
		return fmt.Errorf("failed to create develop branch: %w", chkErr)
	}
	_, _ = g.runGit(ctx, "push", "-u", "origin", "develop")
	return nil
}

// PreFlightBranchCheck validates the current branch before any mutation or commit.
// If the active branch is 'main' or 'master', it evacuates to 'develop' and returns ErrForbiddenBranchMain.
func (g *GitGuard) PreFlightBranchCheck(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	branch, err := g.CurrentBranch(ctx)
	if err != nil {
		return "", err
	}

	if branch == "main" || branch == "master" {
		if devErr := g.EnsureAndCheckoutDevelop(ctx); devErr != nil {
			return "", fmt.Errorf("failed to evacuate %s to develop: %w", branch, devErr)
		}
		return "develop", ErrForbiddenBranchMain
	}

	if g.Mode == ModeStandard && branch != "develop" && !strings.HasPrefix(branch, "local/") {
		if devErr := g.EnsureAndCheckoutDevelop(ctx); devErr != nil {
			return "", devErr
		}
		return "develop", nil
	}

	return branch, nil
}

// EnforceBranchPolicy ensures the agent never operates on 'main' and switches to 'develop'.
func (g *GitGuard) EnforceBranchPolicy(ctx context.Context) (string, error) {
	branch, err := g.PreFlightBranchCheck(ctx)
	if errors.Is(err, ErrForbiddenBranchMain) {
		// Evacuated successfully; return active develop branch without blocking if called as policy
		return "develop", nil
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
	if branch == "main" || branch == "master" {
		return ErrForbiddenBranchMain
	}

	// DegradedBudgetMode cannot commit directly to develop
	if g.Mode == ModeDegradedBudget && branch == "develop" {
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
