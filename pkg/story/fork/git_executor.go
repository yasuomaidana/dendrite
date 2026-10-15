package fork

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitExecutor defines the contract for Git operations needed by the speculative branching engine.
type GitExecutor interface {
	RunGit(ctx context.Context, dir string, args ...string) (string, error)
	WorktreeAdd(ctx context.Context, repoDir, branch, path, commitish string) error
	WorktreeRemove(ctx context.Context, repoDir, path string, force bool) error
	WorktreePrune(ctx context.Context, repoDir string) error
	WorktreeList(ctx context.Context, repoDir string) ([]string, error)
	BranchDelete(ctx context.Context, repoDir, branch string, force bool) error
	MergeBranch(ctx context.Context, repoDir, branch string) error
	GetCurrentBranch(ctx context.Context, repoDir string) (string, error)
	GetHeadCommit(ctx context.Context, repoDir string) (string, error)
	CleanLocks(ctx context.Context, repoDir string) error
}

// OSGitExecutor executes native git CLI commands on the host operating system.
type OSGitExecutor struct{}

// NewOSGitExecutor constructs a new OSGitExecutor.
func NewOSGitExecutor() *OSGitExecutor {
	return &OSGitExecutor{}
}

// RunGit executes a git sub-command in the given working directory.
func (e *OSGitExecutor) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// WorktreeAdd adds a new worktree checked out to a new or existing branch.
func (e *OSGitExecutor) WorktreeAdd(ctx context.Context, repoDir, branch, path, commitish string) error {
	args := []string{"worktree", "add"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, path)
	if commitish != "" {
		args = append(args, commitish)
	}
	_, err := e.RunGit(ctx, repoDir, args...)
	return err
}

// WorktreeRemove unlinks and deletes a worktree directory.
func (e *OSGitExecutor) WorktreeRemove(ctx context.Context, repoDir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := e.RunGit(ctx, repoDir, args...)
	return err
}

// WorktreePrune cleans up worktree metadata for deleted directories.
func (e *OSGitExecutor) WorktreePrune(ctx context.Context, repoDir string) error {
	_, err := e.RunGit(ctx, repoDir, "worktree", "prune")
	return err
}

// WorktreeList lists all registered worktrees.
func (e *OSGitExecutor) WorktreeList(ctx context.Context, repoDir string) ([]string, error) {
	out, err := e.RunGit(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var worktrees []string
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			worktrees = append(worktrees, strings.TrimPrefix(line, "worktree "))
		}
	}
	return worktrees, nil
}

// BranchDelete deletes a git branch.
func (e *OSGitExecutor) BranchDelete(ctx context.Context, repoDir, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := e.RunGit(ctx, repoDir, "branch", flag, branch)
	return err
}

// MergeBranch merges a specified branch into the currently checked-out branch.
func (e *OSGitExecutor) MergeBranch(ctx context.Context, repoDir, branch string) error {
	_, err := e.RunGit(ctx, repoDir, "merge", "--no-ff", "-m", fmt.Sprintf("Merge speculative variant %s", branch), branch)
	return err
}

// GetCurrentBranch returns the active branch name.
func (e *OSGitExecutor) GetCurrentBranch(ctx context.Context, repoDir string) (string, error) {
	return e.RunGit(ctx, repoDir, "rev-parse", "--abbrev-ref", "HEAD")
}

// GetHeadCommit returns the full SHA of the current HEAD commit.
func (e *OSGitExecutor) GetHeadCommit(ctx context.Context, repoDir string) (string, error) {
	return e.RunGit(ctx, repoDir, "rev-parse", "HEAD")
}

// CleanLocks inspects and safely purges stale git index lock files and worktree lock files.
func (e *OSGitExecutor) CleanLocks(ctx context.Context, repoDir string) error {
	gitDir := filepath.Join(repoDir, ".git")
	indexLock := filepath.Join(gitDir, "index.lock")
	if _, err := os.Stat(indexLock); err == nil {
		_ = os.Remove(indexLock)
	}

	worktreesDir := filepath.Join(gitDir, "worktrees")
	if entries, err := os.ReadDir(worktreesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				subLock := filepath.Join(worktreesDir, entry.Name(), "locked")
				_ = os.Remove(subLock)
				subIdxLock := filepath.Join(worktreesDir, entry.Name(), "index.lock")
				_ = os.Remove(subIdxLock)
			}
		}
	}

	return e.WorktreePrune(ctx, repoDir)
}
