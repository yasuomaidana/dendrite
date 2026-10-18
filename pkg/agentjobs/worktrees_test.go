package agentjobs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/workcopy"
)

func TestWorktreeInUse(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	busy := t.TempDir()
	done := t.TempDir()
	_, _ = ledger.Create(&Job{Ticket: "A", Status: StatusRunning, Worktree: busy})
	_, _ = ledger.Create(&Job{Ticket: "B", Status: StatusCommitted, Worktree: done})
	_, _ = ledger.Create(&Job{Ticket: "C", Status: StatusQueued})

	inUse := ledger.WorktreeInUse()
	if !inUse(busy) {
		t.Error("running job's worktree must be in use")
	}
	if inUse(done) {
		t.Error("committed job's worktree must not be in use")
	}
	if inUse(t.TempDir()) {
		t.Error("unrelated path must not be in use")
	}
}

func TestWorktreeInUse_UnreadableLedgerIsConservative(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !NewLedger(file).WorktreeInUse()("/anything") {
		t.Error("an unreadable ledger must report every path as in use")
	}
}

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestSubmit_IsolateCreatesAndReusesWorkingCopy(t *testing.T) {
	base := workcopy.Canon(t.TempDir())
	repo := filepath.Join(base, "Proj")
	root := filepath.Join(base, "work")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(workcopy.EnvWorkDir, root)
	gitT(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-q", "-m", "init")

	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_dendrite", "exit 0\n")
	d.RepoDir = repo

	spec := SubmitSpec{Ticket: "DEN-5", Prompt: "do it", Isolate: true}
	id, err := d.Submit(spec)
	if err != nil {
		t.Fatal(err)
	}
	job, err := ledger.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Proj-DEN-5")
	if workcopy.Canon(job.Worktree) != want {
		t.Fatalf("job worktree = %s, want %s", job.Worktree, want)
	}

	// A second job on the same ticket reuses the copy.
	id2, err := d.Submit(spec)
	if err != nil {
		t.Fatal(err)
	}
	job2, _ := ledger.Load(id2)
	if workcopy.Canon(job2.Worktree) != want {
		t.Fatalf("second job worktree = %s, want %s", job2.Worktree, want)
	}

	// An explicit worktree wins over --isolate.
	explicit := t.TempDir()
	id3, err := d.Submit(SubmitSpec{Ticket: "DEN-6", Prompt: "x", Isolate: true, Worktree: explicit})
	if err != nil {
		t.Fatal(err)
	}
	job3, _ := ledger.Load(id3)
	if workcopy.Canon(job3.Worktree) != workcopy.Canon(explicit) {
		t.Fatalf("explicit worktree ignored: %s", job3.Worktree)
	}
}

func TestSubmit_IsolateOutsideRepoFails(t *testing.T) {
	t.Setenv(workcopy.EnvWorkDir, "")
	d := NewDispatcher(NewLedger(t.TempDir()))
	d.RepoDir = t.TempDir()
	_, err := d.Submit(SubmitSpec{Ticket: "DEN-7", Prompt: "x", Isolate: true})
	if err == nil || !strings.Contains(err.Error(), "isolated working copy") {
		t.Fatalf("expected isolation error, got %v", err)
	}
}

func TestStandardPreambleForbidsSelfMadeCopies(t *testing.T) {
	if !strings.Contains(StandardPreamble, "Never create clones or worktrees yourself") {
		t.Error("preamble must forbid agents from creating their own clones")
	}
}
