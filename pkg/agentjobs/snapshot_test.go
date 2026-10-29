package agentjobs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// livePGID starts a real process in its own session and returns its process group id.
func livePGID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	return pgid
}

func newSnapshotWatcher(t *testing.T) (*Watcher, *Ledger, string) {
	t.Helper()
	l := NewLedger(t.TempDir())
	return NewWatcher(l, NewDispatcher(l)), l, createGitRepo(t)
}

func TestSnapshotOfARunningAgyJob(t *testing.T) {
	w, l, repo := newSnapshotWatcher(t)
	job, err := l.Create(&Job{Ticket: "DPLANE-2", Agent: "agy", Model: "m", Status: StatusRunning, Worktree: repo, PGID: livePGID(t)})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	w.Now = func() time.Time { return now.Add(5 * time.Minute) }

	_ = os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a"), 0o644)
	_ = os.MkdirAll(filepath.Join(repo, ".dendrite", "jobs"), 0o755)
	_ = os.WriteFile(filepath.Join(repo, ".dendrite", "jobs", "x"), []byte("state"), 0o644)
	log := sprintfReq(1, 2) + sprintfReq(3, 4)
	if err := os.WriteFile(l.AgyLogPath(job.ID), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	l.writeHeartbeat(now.Add(5 * time.Minute))

	s, err := w.Snapshot(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Job != job.ID || s.Ticket != "DPLANE-2" || s.Status != StatusRunning || s.Final() {
		t.Errorf("identity: %+v", s)
	}
	if !s.ProcessAlive || s.Requests != 2 || s.ChangedFiles != 1 || !s.WatcherAlive {
		t.Errorf("liveness: %+v (Dendrite's own state must not count as a changed file)", s)
	}
	if s.ElapsedSeconds < 290 || s.ElapsedSeconds > 400 {
		t.Errorf("ElapsedSeconds = %d, want about 300", s.ElapsedSeconds)
	}
	if s.LastRequestAgeSeconds < 0 {
		t.Errorf("LastRequestAgeSeconds = %d, the log has requests", s.LastRequestAgeSeconds)
	}
	if s.StallInSeconds <= 0 || s.StallInSeconds > int64(DefaultStallTimeout.Seconds()) {
		t.Errorf("StallInSeconds = %d", s.StallInSeconds)
	}
	line := s.Line()
	for _, want := range []string{"running 5m", "process alive", "2 model requests", "1 files changed", "stall check in", "watcher up"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q lacks %q", line, want)
		}
	}
}

func TestSnapshotOfJobsThatAreNotRunning(t *testing.T) {
	w, l, repo := newSnapshotWatcher(t)
	for _, c := range []struct {
		status Status
		agent  string
		final  bool
		want   []string
		not    []string
	}{
		{StatusSucceeded, "agy", true, []string{"succeeded", "no process", "no model request yet", "WATCHER DOWN"}, []string{"stall check"}},
		{StatusQueued, "aider", false, []string{"queued", "no process"}, []string{"model request", "stall check"}},
		{StatusParked, "local", false, nil, nil},
		{StatusFailedRetryable, "agy", false, nil, nil},
		{StatusCancelled, "agy", true, nil, nil},
	} {
		job, err := l.Create(&Job{Ticket: "T-1", Agent: c.agent, Status: c.status, Worktree: repo})
		if err != nil {
			t.Fatal(err)
		}
		s, err := w.Snapshot(context.Background(), job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if s.Final() != c.final || s.StallInSeconds != 0 || s.LastRequestAgeSeconds != -1 {
			t.Errorf("%s: %+v", c.status, s)
		}
		for _, want := range c.want {
			if !strings.Contains(s.Line(), want) {
				t.Errorf("%s: line %q lacks %q", c.status, s.Line(), want)
			}
		}
		for _, not := range c.not {
			if strings.Contains(s.Line(), not) {
				t.Errorf("%s: line %q must not contain %q", c.status, s.Line(), not)
			}
		}
	}
}

func TestSnapshotOfUnknownJobAndBrokenWorktrees(t *testing.T) {
	w, l, _ := newSnapshotWatcher(t)
	if _, err := w.Snapshot(context.Background(), "nope"); err == nil {
		t.Error("an unknown job is an error")
	}
	job, _ := l.Create(&Job{Ticket: "T-2", Agent: "agy", Status: StatusRunning, Worktree: t.TempDir()})
	s, err := w.Snapshot(context.Background(), job.ID)
	if err != nil || s.ChangedFiles != 0 {
		t.Errorf("a working copy that is not a git repository counts no changes: %+v, %v", s, err)
	}
	// A log that cannot be read leaves the request count at zero instead of failing the snapshot.
	_ = os.MkdirAll(l.AgyLogPath(job.ID), 0o755)
	if s, err := w.Snapshot(context.Background(), job.ID); err != nil || s.Requests != 0 {
		t.Errorf("unreadable agy.log: %+v, %v", s, err)
	}
}

func TestChangedFilesHandlesRenamesQuotesAndAMissingDispatcher(t *testing.T) {
	w, _, repo := newSnapshotWatcher(t)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	_ = os.WriteFile(filepath.Join(repo, "old.txt"), []byte("x"), 0o644)
	git("add", "old.txt")
	git("commit", "-m", "add old")
	git("mv", "old.txt", "new.txt")
	_ = os.WriteFile(filepath.Join(repo, "spaced name.txt"), []byte("y"), 0o644)

	job := &Job{Worktree: repo}
	if got := w.changedFiles(context.Background(), job); got != 2 {
		t.Errorf("a rename and a file with a space = %d changes, want 2", got)
	}

	w.Dispatcher = nil
	if got := w.changedFiles(context.Background(), job); got != 2 {
		t.Errorf("without a dispatcher every entry counts: %d, want 2", got)
	}
	w.Dispatcher = &Dispatcher{GitPath: filepath.Join(t.TempDir(), "no-git")}
	if got := w.changedFiles(context.Background(), job); got != 0 {
		t.Errorf("git that cannot run counts none: %d", got)
	}
}

func TestLastActivityIsTheNewestSignOfLife(t *testing.T) {
	l := NewLedger(t.TempDir())
	w := NewWatcher(l, NewDispatcher(l))
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	worktree := t.TempDir()
	_ = os.MkdirAll(filepath.Join(worktree, "src", "deep"), 0o755)
	deepFile := filepath.Join(worktree, "src", "deep", "f.go")
	_ = os.WriteFile(deepFile, []byte("x"), 0o644)
	_ = os.Chtimes(deepFile, at(40), at(40))
	// The walk counts directories too, so they get a known time as well.
	for _, dir := range []string{filepath.Join(worktree, "src", "deep"), filepath.Join(worktree, "src")} {
		_ = os.Chtimes(dir, at(20), at(20))
	}

	job := &Job{ID: "J-LA", CreatedAt: at(0), UpdatedAt: at(10), Worktree: worktree, Scope: []string{"missing", "src"}}
	_ = os.MkdirAll(l.JobDir(job.ID), 0o755)
	if got := w.lastActivity(job); !got.Equal(at(40)) {
		t.Errorf("a file deep inside a scope directory is the newest sign: %v", got)
	}

	_ = os.WriteFile(l.AgyLogPath(job.ID), []byte("x"), 0o644)
	_ = os.Chtimes(l.AgyLogPath(job.ID), at(55), at(55))
	if got := w.lastActivity(job); !got.Equal(at(55)) {
		t.Errorf("agy's own log counts: %v", got)
	}

	job.CreatedAt = at(90)
	if got := w.lastActivity(job); !got.Equal(at(90)) {
		t.Errorf("a job created after everything else is as old as its creation: %v", got)
	}

	// With no worktree the current directory is used; only the ledger entry and logs remain here.
	cwdJob := &Job{ID: "J-CWD", CreatedAt: at(5), UpdatedAt: at(7)}
	if got := w.lastActivity(cwdJob); !got.Equal(at(7)) {
		t.Errorf("lastActivity without a scope = %v", got)
	}
}

func TestStallTimeoutDefaultsWhenUnset(t *testing.T) {
	w := &Watcher{}
	if w.stallTimeout() != DefaultStallTimeout {
		t.Errorf("stallTimeout = %v", w.stallTimeout())
	}
	w.StallTimeout = time.Minute
	if w.stallTimeout() != time.Minute {
		t.Errorf("stallTimeout = %v", w.stallTimeout())
	}
}

// A busy agent reads and thinks for minutes without touching a file. Its agy log keeps growing, so the
// watcher must leave it alone, and must act as soon as that log goes quiet too.
func TestWatcherDoesNotKillAJobWhoseAgyLogIsFresh(t *testing.T) {
	l := NewLedger(t.TempDir())
	d := NewDispatcher(l)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
	pgid := livePGID(t)

	worktree := t.TempDir()
	scopeFile := filepath.Join(worktree, "tracked.go")
	_ = os.WriteFile(scopeFile, []byte("package main\n"), 0o644)
	now := time.Now().UTC().Add(time.Hour)
	old := now.Add(-30 * time.Minute)
	_ = os.Chtimes(scopeFile, old, old)

	job, err := l.Create(&Job{Ticket: "DEN-9", Status: StatusRunning, Worktree: worktree, Scope: []string{"tracked.go"}, PGID: pgid, CreatedAt: old, UpdatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{l.AgentLogPath(job.ID), l.AgyLogPath(job.ID)} {
		_ = os.WriteFile(p, []byte("log\n"), 0o644)
		_ = os.Chtimes(p, old, old)
	}

	var killed []int
	w := NewWatcher(l, d)
	w.Now = func() time.Time { return now }
	w.StallTimeout = 15 * time.Minute
	w.KillPGID = func(id int) error { killed = append(killed, id); return nil }

	_ = os.Chtimes(l.AgyLogPath(job.ID), now.Add(-time.Minute), now.Add(-time.Minute))
	if err := w.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(killed) != 0 {
		t.Fatalf("a job making model requests was killed: %v", killed)
	}
	if got, _ := l.Load(job.ID); got.Status != StatusRunning {
		t.Errorf("status = %s, want running", got.Status)
	}

	_ = os.Chtimes(l.AgyLogPath(job.ID), old, old)
	if err := w.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(killed) != 1 || killed[0] != pgid {
		t.Errorf("a job with no sign of life for 30 minutes must be killed: %v", killed)
	}
}
