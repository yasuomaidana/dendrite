package agentjobs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatcherAliveFollowsTheHeartbeat(t *testing.T) {
	l := NewLedger(t.TempDir())
	now := time.Now()
	if _, alive := l.WatcherAlive(now); alive {
		t.Fatal("no heartbeat, no watcher")
	}

	l.writeHeartbeat(now)
	if pid, alive := l.WatcherAlive(now.Add(time.Second)); !alive || pid != os.Getpid() {
		t.Errorf("fresh heartbeat of this process: pid = %d, alive = %v", pid, alive)
	}
	if _, alive := l.WatcherAlive(now.Add(WatcherMaxAge + time.Second)); alive {
		t.Error("a heartbeat older than WatcherMaxAge is a dead watcher")
	}

	old := processExists
	t.Cleanup(func() { processExists = old })
	processExists = func(int) bool { return false }
	if _, alive := l.WatcherAlive(now); alive {
		t.Error("a fresh heartbeat of a process that no longer exists is a dead watcher")
	}
}

func TestWatcherAliveRejectsBrokenHeartbeats(t *testing.T) {
	l := NewLedger(t.TempDir())
	_ = os.MkdirAll(l.Root(), 0o755)
	for name, content := range map[string]string{
		"not json":   "{",
		"no pid":     `{"time":"2099-01-01T00:00:00Z"}`,
		"negative":   `{"pid":-4,"time":"2099-01-01T00:00:00Z"}`,
		"zero value": `{}`,
	} {
		_ = os.WriteFile(l.HeartbeatPath(), []byte(content), 0o644)
		if _, alive := l.WatcherAlive(time.Now()); alive {
			t.Errorf("%s: must not count as a live watcher", name)
		}
	}
}

func TestProcessExistsSeesThisProcessAndNotAnImpossibleOne(t *testing.T) {
	if !processExists(os.Getpid()) {
		t.Error("this process exists")
	}
	if processExists(1 << 30) {
		t.Error("pid 2^30 does not exist")
	}
}

func TestTickWritesTheHeartbeat(t *testing.T) {
	l := NewLedger(t.TempDir())
	w := NewWatcher(l, NewDispatcher(l))
	if err := w.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(l.HeartbeatPath())
	if err != nil {
		t.Fatalf("a pass must leave a heartbeat: %v", err)
	}
	var hb heartbeat
	if json.Unmarshal(data, &hb) != nil || hb.PID != os.Getpid() || time.Since(hb.Time) > time.Minute {
		t.Errorf("heartbeat = %s", data)
	}
}

func TestBusyCountsEveryStateTheWatcherActsOn(t *testing.T) {
	cases := map[Status]bool{
		StatusQueued: true, StatusRunning: true, StatusParked: true, StatusFailedRetryable: true,
		StatusSucceeded: true, StatusFailed: true,
		StatusVerified: false, StatusCommitted: false, StatusCancelled: false, StatusNeedsReview: false,
	}
	for status, want := range cases {
		l := NewLedger(t.TempDir())
		if _, err := l.Create(&Job{Ticket: "DEN-1", Status: status}); err != nil {
			t.Fatal(err)
		}
		if got := NewWatcher(l, NewDispatcher(l)).Busy(); got != want {
			t.Errorf("Busy with a %s job = %v, want %v", status, got, want)
		}
	}
	empty := NewLedger(t.TempDir())
	if NewWatcher(empty, NewDispatcher(empty)).Busy() {
		t.Error("an empty ledger is not busy")
	}
	broken := NewLedger(filepath.Join(writeRegularFile(t), "jobs"))
	if !NewWatcher(broken, NewDispatcher(broken)).Busy() {
		t.Error("an unreadable ledger must count as busy: do not stop watching what cannot be seen")
	}
}

func writeRegularFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func waitForFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return string(data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s was not written in time", path)
	return ""
}

func TestEnsureWatcherStartsADetachedDaemonOnlyWhenNoneIsAlive(t *testing.T) {
	l := NewLedger(filepath.Join(t.TempDir(), "jobs"))
	out := filepath.Join(t.TempDir(), "args")
	d := NewDispatcher(l)
	d.Executable = installExecutableScript(t, t.TempDir(), "dendrite", `echo "$@" > `+out+"\n")

	pid, err := d.EnsureWatcher()
	if err != nil || pid <= 0 {
		t.Fatalf("pid = %d, err = %v", pid, err)
	}
	if got := strings.TrimSpace(waitForFile(t, out)); got != "bouton jobs daemon --idle-exit 30m0s" {
		t.Errorf("the watcher was started as %q", got)
	}

	// With a live watcher nothing more is started.
	_ = os.Remove(out)
	l.writeHeartbeat(time.Now())
	if pid, err := d.EnsureWatcher(); err != nil || pid != 0 {
		t.Errorf("a live watcher must be left alone: pid = %d, err = %v", pid, err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(out); err == nil {
		t.Error("a second watcher was started")
	}
}

func TestEnsureWatcherReportsWhyItCouldNotStart(t *testing.T) {
	t.Run("test binary never re-executes itself", func(t *testing.T) {
		d := NewDispatcher(NewLedger(t.TempDir()))
		d.Executable = ""
		if _, err := d.EnsureWatcher(); !errors.Is(err, ErrSelfExecInTest) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("ledger root that cannot be created", func(t *testing.T) {
		d := NewDispatcher(NewLedger(filepath.Join(writeRegularFile(t), "jobs")))
		d.Executable = installExecutableScript(t, t.TempDir(), "dendrite", "exit 0\n")
		if _, err := d.EnsureWatcher(); err == nil || !strings.Contains(err.Error(), "ledger root") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("watcher.log that cannot be opened", func(t *testing.T) {
		l := NewLedger(t.TempDir())
		_ = os.MkdirAll(l.WatcherLogPath(), 0o755)
		d := NewDispatcher(l)
		d.Executable = installExecutableScript(t, t.TempDir(), "dendrite", "exit 0\n")
		if _, err := d.EnsureWatcher(); err == nil || !strings.Contains(err.Error(), "watcher.log") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("binary that cannot be started", func(t *testing.T) {
		d := NewDispatcher(NewLedger(t.TempDir()))
		d.Executable = filepath.Join(t.TempDir(), "missing")
		if _, err := d.EnsureWatcher(); err == nil || !strings.Contains(err.Error(), "stall watcher") {
			t.Errorf("err = %v", err)
		}
	})
}
