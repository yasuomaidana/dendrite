package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/agentjobs"
)

func watchFixture(t *testing.T, status agentjobs.Status) (*agentjobs.Watcher, *agentjobs.Ledger, *agentjobs.Job) {
	t.Helper()
	l := agentjobs.NewLedger(t.TempDir())
	job, err := l.Create(&agentjobs.Job{Ticket: "DEN-77", Agent: "agy", Model: "m", Status: status, Worktree: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return agentjobs.NewWatcher(l, agentjobs.NewDispatcher(l)), l, job
}

func jsonOutput(t *testing.T, on bool) {
	t.Helper()
	old := globals.JSON
	globals.JSON = on
	t.Cleanup(func() { globals.JSON = old })
}

func TestRunJobsWatchStopsAtAFinalState(t *testing.T) {
	jsonOutput(t, false)
	w, _, job := watchFixture(t, agentjobs.StatusSucceeded)
	var buf bytes.Buffer
	if err := runJobsWatch(context.Background(), w, job.ID, false, time.Millisecond, &buf); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(buf.String()), "\n"); len(lines) != 1 || !strings.Contains(lines[0], "succeeded") {
		t.Errorf("a finished job prints one line and stops:\n%s", buf.String())
	}
}

func TestRunJobsWatchOnceAndJSON(t *testing.T) {
	w, _, job := watchFixture(t, agentjobs.StatusRunning)
	var buf bytes.Buffer

	jsonOutput(t, false)
	if err := runJobsWatch(context.Background(), w, job.ID, true, time.Hour, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "running") || !strings.Contains(buf.String(), "WATCHER DOWN") {
		t.Errorf("text line:\n%s", buf.String())
	}

	jsonOutput(t, true)
	buf.Reset()
	if err := runJobsWatch(context.Background(), w, job.ID, true, time.Hour, &buf); err != nil {
		t.Fatal(err)
	}
	var s agentjobs.Snapshot
	if err := json.Unmarshal(buf.Bytes(), &s); err != nil {
		t.Fatalf("JSON output: %v\n%s", err, buf.String())
	}
	if s.Job != job.ID || s.Status != agentjobs.StatusRunning || s.WatcherAlive || s.LastRequestAgeSeconds != -1 {
		t.Errorf("snapshot = %+v", s)
	}
}

func TestRunJobsWatchKeepsPrintingUntilCancelled(t *testing.T) {
	jsonOutput(t, false)
	w, _, job := watchFixture(t, agentjobs.StatusRunning)
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	var buf bytes.Buffer
	if err := runJobsWatch(ctx, w, job.ID, false, 20*time.Millisecond, &buf); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "running"); n < 2 {
		t.Errorf("a running job is reported on every interval, got %d lines:\n%s", n, buf.String())
	}
}

func TestRunJobsWatchReportsUnknownJobsAndWriteErrors(t *testing.T) {
	jsonOutput(t, false)
	w, _, job := watchFixture(t, agentjobs.StatusRunning)
	if err := runJobsWatch(context.Background(), w, "nope", true, time.Hour, &bytes.Buffer{}); err == nil {
		t.Error("an unknown job is an error")
	}
	jsonOutput(t, true)
	if err := runJobsWatch(context.Background(), w, job.ID, true, time.Hour, failingWriter{}); err == nil {
		t.Error("a JSON write failure is an error")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }

func TestResolveJob(t *testing.T) {
	l := agentjobs.NewLedger(t.TempDir())
	older, _ := l.Create(&agentjobs.Job{Ticket: "DEN-88", Status: agentjobs.StatusSucceeded, CreatedAt: time.Now().Add(-time.Hour)})
	newer, _ := l.Create(&agentjobs.Job{Ticket: "DEN-88", Status: agentjobs.StatusRunning, CreatedAt: time.Now()})

	if rl, id, err := resolveJob(l, older.ID, ""); err != nil || rl != l || id != older.ID {
		t.Errorf("by id: %v %q %v", rl, id, err)
	}
	if _, id, err := resolveJob(l, "", "DEN-88"); err != nil || id != newer.ID {
		t.Errorf("by ticket must pick the newest job: %q, %v", id, err)
	}
	if _, _, err := resolveJob(l, "DEN-88-20990101T000000Z-zzzzzz", ""); err == nil {
		t.Error("an id that is not in any ledger is an error")
	}
	if _, _, err := resolveJob(l, "", "DEN-12345"); err == nil || !strings.Contains(err.Error(), "no matching jobs") {
		t.Errorf("a ticket with no jobs: %v", err)
	}
}

func TestBoutonJobsWatchCommand(t *testing.T) {
	jsonOutput(t, false)
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	l := agentjobs.NewLedger("")
	if _, err := l.Create(&agentjobs.Job{Ticket: "DEN-99", Agent: "agy", Status: agentjobs.StatusRunning, Worktree: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	var buf bytes.Buffer
	if err := runCLI(ctx, &buf, "bouton", "jobs", "watch", "--ticket", "DEN-99", "--once"); err != nil {
		t.Fatalf("watch: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "running") {
		t.Errorf("output:\n%s", buf.String())
	}
	for name, args := range map[string][]string{
		"no job and no ticket":   {"bouton", "jobs", "watch"},
		"both a job and ticket":  {"bouton", "jobs", "watch", "J-1", "--ticket", "DEN-99"},
		"non-positive interval":  {"bouton", "jobs", "watch", "--ticket", "DEN-99", "--interval", "0s"},
		"ticket without any job": {"bouton", "jobs", "watch", "--ticket", "DEN-4040"},
	} {
		if err := runCLI(ctx, &bytes.Buffer{}, args...); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func scriptIn(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "dendrite")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEnsureJobWatcherSaysWhatItDid(t *testing.T) {
	t.Run("starts one when none is alive", func(t *testing.T) {
		d := agentjobs.NewDispatcher(agentjobs.NewLedger(filepath.Join(t.TempDir(), "jobs")))
		d.Executable = scriptIn(t, "exit 0\n")
		var buf bytes.Buffer
		ensureJobWatcher(d, &buf)
		if !strings.Contains(buf.String(), "Stall watcher started (pid ") || !strings.Contains(buf.String(), "watcher.log") {
			t.Errorf("output:\n%s", buf.String())
		}
	})
	t.Run("silent when one is alive", func(t *testing.T) {
		l := agentjobs.NewLedger(t.TempDir())
		writeHeartbeat(t, l, os.Getppid())
		d := agentjobs.NewDispatcher(l)
		d.Executable = scriptIn(t, "exit 0\n")
		var buf bytes.Buffer
		ensureJobWatcher(d, &buf)
		if buf.Len() != 0 {
			t.Errorf("a live watcher needs no message:\n%s", buf.String())
		}
	})
	t.Run("warns when it cannot start one", func(t *testing.T) {
		d := agentjobs.NewDispatcher(agentjobs.NewLedger(t.TempDir()))
		d.Executable = ""
		var buf bytes.Buffer
		ensureJobWatcher(d, &buf)
		if !strings.Contains(buf.String(), "[!] No stall watcher is running") || !strings.Contains(buf.String(), "dendrite bouton jobs daemon") {
			t.Errorf("output:\n%s", buf.String())
		}
	})
}

// writeHeartbeat records a heartbeat of pid, as a running watcher would.
func writeHeartbeat(t *testing.T, l *agentjobs.Ledger, pid int) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"pid": pid, "time": time.Now().UTC()})
	if err := os.MkdirAll(l.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.HeartbeatPath(), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunJobsDaemonExitsWhenIdle(t *testing.T) {
	l := agentjobs.NewLedger(t.TempDir())
	w := agentjobs.NewWatcher(l, agentjobs.NewDispatcher(l))
	var errOut bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runJobsDaemon(context.Background(), w, 10*time.Millisecond, 40*time.Millisecond, &errOut)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the watcher did not exit although no job needed it")
	}
	if !strings.Contains(errOut.String(), "exiting") {
		t.Errorf("output:\n%s", errOut.String())
	}
	if _, alive := l.WatcherAlive(time.Now()); !alive {
		t.Error("the watcher must have left a heartbeat")
	}
}

func TestRunJobsDaemonExitsAtOnceWhenIdleOnTheFirstPass(t *testing.T) {
	l := agentjobs.NewLedger(t.TempDir())
	w := agentjobs.NewWatcher(l, agentjobs.NewDispatcher(l))
	if err := runJobsDaemon(context.Background(), w, time.Hour, time.Nanosecond, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func TestRunJobsDaemonStaysWhileJobsNeedIt(t *testing.T) {
	l := agentjobs.NewLedger(t.TempDir())
	if _, err := l.Create(&agentjobs.Job{Ticket: "DEN-1", Status: agentjobs.StatusRunning, Worktree: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	w := agentjobs.NewWatcher(l, agentjobs.NewDispatcher(l))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := runJobsDaemon(ctx, w, 10*time.Millisecond, 30*time.Millisecond, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 180*time.Millisecond {
		t.Errorf("the watcher left after %v although a job was running", time.Since(start))
	}
}

func TestRunJobsDaemonDoesNotStartASecondWatcher(t *testing.T) {
	l := agentjobs.NewLedger(t.TempDir())
	writeHeartbeat(t, l, os.Getppid())
	w := agentjobs.NewWatcher(l, agentjobs.NewDispatcher(l))
	var errOut bytes.Buffer
	if err := runJobsDaemon(context.Background(), w, time.Hour, 0, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "already running") {
		t.Errorf("output:\n%s", errOut.String())
	}
}

func TestWarnWatcherDown(t *testing.T) {
	now := time.Now()
	idle := agentjobs.NewLedger(t.TempDir())
	var buf bytes.Buffer
	warnWatcherDown(idle, now, &buf)
	if buf.Len() != 0 {
		t.Errorf("no jobs in flight, no warning:\n%s", buf.String())
	}

	busy := agentjobs.NewLedger(t.TempDir())
	_, _ = busy.Create(&agentjobs.Job{Ticket: "DEN-1", Status: agentjobs.StatusRunning})
	_, _ = busy.Create(&agentjobs.Job{Ticket: "DEN-2", Status: agentjobs.StatusQueued})
	_, _ = busy.Create(&agentjobs.Job{Ticket: "DEN-3", Status: agentjobs.StatusVerified})
	warnWatcherDown(busy, now, &buf)
	if !strings.Contains(buf.String(), "2 job(s) in flight and no stall watcher is running") {
		t.Errorf("output:\n%s", buf.String())
	}

	buf.Reset()
	writeHeartbeat(t, busy, os.Getppid())
	warnWatcherDown(busy, now, &buf)
	if buf.Len() != 0 {
		t.Errorf("a live watcher needs no warning:\n%s", buf.String())
	}

	unreadable := agentjobs.NewLedger(filepath.Join(writeRegular(t), "jobs"))
	warnWatcherDown(unreadable, now, &buf)
	if buf.Len() != 0 {
		t.Errorf("an unreadable ledger is not reported as a dead watcher:\n%s", buf.String())
	}
}

func writeRegular(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
