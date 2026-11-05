package agentjobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"Dendrite/pkg/fsutil"
)

// WatcherMaxAge is how long a watcher may go without a heartbeat before it counts as gone: three
// missed passes at the default interval.
const WatcherMaxAge = 3 * DefaultDaemonInterval

// DefaultWatcherIdleExit is how long a watcher started by `jobs submit` waits with no active job
// before it exits on its own.
const DefaultWatcherIdleExit = 30 * time.Minute

type heartbeat struct {
	PID  int       `json:"pid"`
	Time time.Time `json:"time"`
}

// HeartbeatPath is the file the watcher rewrites on every pass, so anyone can tell whether one is alive.
func (l *Ledger) HeartbeatPath() string {
	return filepath.Join(l.Root(), "watcher.json")
}

// WatcherLogPath is where a watcher started by Dendrite writes its output.
func (l *Ledger) WatcherLogPath() string {
	return filepath.Join(l.Root(), "watcher.log")
}

func (l *Ledger) writeHeartbeat(now time.Time) {
	data, _ := json.Marshal(heartbeat{PID: os.Getpid(), Time: now.UTC()})
	_ = fsutil.WriteFileAtomic(l.HeartbeatPath(), data)
}

// processExists reports whether a process with this pid is running.
var processExists = func(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// WatcherAlive reports whether a watcher is running over this ledger: its heartbeat is recent and its
// process exists. pid is the watcher's process id when it is alive.
func (l *Ledger) WatcherAlive(now time.Time) (pid int, alive bool) {
	data, err := os.ReadFile(l.HeartbeatPath())
	if err != nil {
		return 0, false
	}
	var hb heartbeat
	if json.Unmarshal(data, &hb) != nil || hb.PID <= 0 {
		return 0, false
	}
	if now.Sub(hb.Time) > WatcherMaxAge || !processExists(hb.PID) {
		return 0, false
	}
	return hb.PID, true
}

// Busy reports whether any job still needs the watcher: one that is queued, running, parked or waiting for
// a retry, a succeeded job not yet verified, or a failed one not yet re-dispatched.
func (w *Watcher) Busy() bool {
	jobs, err := w.Ledger.List()
	if err != nil {
		return true
	}
	for _, j := range jobs {
		switch j.Status {
		case StatusQueued, StatusRunning, StatusParked, StatusFailedRetryable, StatusSucceeded, StatusFailed:
			return true
		}
	}
	return false
}

// EnsureWatcher starts a detached stall watcher over the dispatcher's ledger unless one is alive. It
// returns the pid of the watcher it started, or 0 when one was already running. The watcher exits on its
// own once no job has been active for DefaultWatcherIdleExit.
func (d *Dispatcher) EnsureWatcher() (started int, err error) {
	if _, alive := d.Ledger.WatcherAlive(d.now()); alive {
		return 0, nil
	}
	exe, err := d.executable()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(d.Ledger.Root(), 0o755); err != nil {
		return 0, fmt.Errorf("agentjobs: failed to create ledger root: %w", err)
	}
	logFile, err := os.OpenFile(d.Ledger.WatcherLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("agentjobs: failed to open watcher.log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "bouton", "jobs", "daemon", "--idle-exit", DefaultWatcherIdleExit.String())
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = sysProcAttrSetsid()
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("agentjobs: failed to start the stall watcher: %w", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}
