package agentjobs

import (
	"path/filepath"
	"testing"
)

func TestBuildCheckpoint_EmptyWorktree_InGitRepo(t *testing.T) {
	repoDir := t.TempDir()
	initGitRepo(t, repoDir, "feature-empty-worktree")

	t.Chdir(repoDir)

	cp, err := BuildCheckpoint(nil, CheckpointOptions{})
	if err != nil {
		t.Fatalf("BuildCheckpoint failed: %v", err)
	}

	if filepath.Base(cp.Worktree) != filepath.Base(repoDir) {
		t.Errorf("cp.Worktree base = %q, want %q (full: %q vs %q)", filepath.Base(cp.Worktree), filepath.Base(repoDir), cp.Worktree, repoDir)
	}
	if cp.Branch != "feature-empty-worktree" {
		t.Errorf("cp.Branch = %q, want feature-empty-worktree", cp.Branch)
	}
}

func TestBuildCheckpoint_EmptyWorktree_NonGitDir(t *testing.T) {
	nonGitDir := t.TempDir()

	t.Chdir(nonGitDir)

	cp, err := BuildCheckpoint(nil, CheckpointOptions{})
	if err != nil {
		t.Fatalf("BuildCheckpoint failed: %v", err)
	}

	if filepath.Base(cp.Worktree) != filepath.Base(nonGitDir) {
		t.Errorf("cp.Worktree base = %q, want %q (full: %q vs %q)", filepath.Base(cp.Worktree), filepath.Base(nonGitDir), cp.Worktree, nonGitDir)
	}
}

func TestResumeState_DefaultLedger(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "handoff.md")

	cp := &Checkpoint{Branch: "x"}
	if err := WriteCheckpoint(path, cp); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}

	report, err := ResumeState(path, nil)
	if err != nil {
		t.Fatalf("ResumeState failed: %v", err)
	}
	if report == nil {
		t.Fatalf("ResumeState returned nil report")
	}
	if report.Checkpoint == nil || report.Checkpoint.Branch != "x" {
		t.Errorf("expected checkpoint branch %q, got %+v", "x", report.Checkpoint)
	}
}
