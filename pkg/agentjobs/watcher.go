package agentjobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// DefaultStallTimeout is the maximum duration an in-progress agent job may remain
// without any modification to its agent.log or in-scope files.
const DefaultStallTimeout = 15 * time.Minute

// DefaultDaemonInterval is the default polling interval for the watcher daemon.
const DefaultDaemonInterval = 30 * time.Second

// DefaultLaunchAgentPath returns the user-level launchd agent plist path.
func DefaultLaunchAgentPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "Library/LaunchAgents/dev.dendrite.jobs.plist"
	}
	return filepath.Join(home, "Library", "LaunchAgents", "dev.dendrite.jobs.plist")
}

// Watcher monitors agent jobs, running verification on succeeded jobs, re-dispatching
// failed jobs once with failure capsules, terminating stalled jobs, and waking parked jobs.
type Watcher struct {
	Ledger        *Ledger
	Dispatcher    *Dispatcher
	Runner        CommandRunner
	StallTimeout  time.Duration
	Now           func() time.Time
	ResumeParked  func() ([]string, error)
	KillPGID      func(pgid int) error
	IsProcessDead func(pgid int) bool
}

// NewWatcher creates an initialized Watcher.
func NewWatcher(l *Ledger, d *Dispatcher) *Watcher {
	if l == nil {
		l = NewLedger("")
	}
	if d == nil {
		d = NewDispatcher(l)
	}
	return &Watcher{
		Ledger:       l,
		Dispatcher:   d,
		Runner:       &ExecRunner{},
		StallTimeout: DefaultStallTimeout,
		Now:          time.Now,
		ResumeParked: d.ResumeParked,
		KillPGID: func(pgid int) error {
			if pgid > 0 {
				return syscall.Kill(-pgid, syscall.SIGKILL)
			}
			return nil
		},
		IsProcessDead: func(pgid int) bool {
			if pgid <= 0 {
				return true
			}
			// Sending signal 0 checks whether the process exists.
			err := syscall.Kill(-pgid, 0)
			if err != nil && errors.Is(err, syscall.ESRCH) {
				return true
			}
			// Fallback check on positive PID as well
			err2 := syscall.Kill(pgid, 0)
			return err2 != nil && errors.Is(err2, syscall.ESRCH)
		},
	}
}

func (w *Watcher) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// Tick performs one cycle of the watcher daemon logic:
// (1) verify every job with status succeeded
// (2) re-dispatch a failed job once with failure output appended as a capsule, or mark needs_review if failed again
// (3) stall detection: running jobs whose agent.log and in-scope files haven't changed for StallTimeout
// (4) resume parked jobs whose resume_after passed
func (w *Watcher) Tick(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	jobs, err := w.Ledger.List()
	if err != nil {
		return fmt.Errorf("watcher tick: failed to list jobs: %w", err)
	}

	now := w.now()
	stallTimeout := w.StallTimeout
	if stallTimeout <= 0 {
		stallTimeout = DefaultStallTimeout
	}

	for _, job := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		switch job.Status {
		case StatusSucceeded:
			_ = w.verifyJob(ctx, job)

		case StatusFailed:
			_ = w.handleFailedJob(ctx, job)

		case StatusRunning:
			_ = w.checkStall(ctx, job, now, stallTimeout)
		}
	}

	// (4) Resume parked jobs
	if err := ctx.Err(); err != nil {
		return err
	}
	if w.ResumeParked != nil {
		_, _ = w.ResumeParked()
	}

	return nil
}

func (w *Watcher) verifyJob(ctx context.Context, job *Job) error {
	return Verify(ctx, w.Ledger, job, w.Runner)
}

func (w *Watcher) handleFailedJob(ctx context.Context, job *Job) error {
	events, err := w.Ledger.Events(job.ID)
	if err != nil {
		return err
	}

	retriedCount := 0
	var lastFailedOutput string
	for _, ev := range events {
		if ev.Type == "retried" {
			retriedCount++
		}
		if ev.Type == "failed" {
			if out, ok := ev.Data["output"].(string); ok {
				lastFailedOutput = out
			}
		}
	}

	if retriedCount >= 1 {
		// Already retried once and failed again -> needs_review
		job.Status = StatusNeedsReview
		if err := w.Ledger.Save(job); err != nil {
			return err
		}
		return w.Ledger.Append(job.ID, Event{
			Type: "needs_review",
			Data: map[string]any{
				"reason": "job failed after retry",
			},
		})
	}

	// Re-dispatch once with the failure output appended to prompt as a capsule
	promptPath := w.Ledger.PromptPath(job.ID)
	originalPrompt, _ := os.ReadFile(promptPath)

	capsule := fmt.Sprintf("\n\n## Verification Failure Capsule\n```\n%s\n```\nFix the above failures; do not repeat the same mistakes.\n", tailString(lastFailedOutput, 4000))
	updatedPrompt := string(originalPrompt) + capsule
	if err := os.WriteFile(promptPath, []byte(updatedPrompt), 0644); err != nil {
		return fmt.Errorf("watcher: failed to append failure capsule to prompt: %w", err)
	}

	return w.Dispatcher.Retry(job.ID)
}

func (w *Watcher) checkStall(ctx context.Context, job *Job, now time.Time, stallTimeout time.Duration) error {
	lastActivity := job.UpdatedAt
	if job.CreatedAt.After(lastActivity) {
		lastActivity = job.CreatedAt
	}

	// Check agent.log mtime
	logPath := w.Ledger.AgentLogPath(job.ID)
	if fi, err := os.Stat(logPath); err == nil {
		if fi.ModTime().After(lastActivity) {
			lastActivity = fi.ModTime()
		}
	}

	// Check in-scope files mtime
	worktree := job.Worktree
	if worktree == "" {
		worktree, _ = os.Getwd()
	}

	for _, s := range job.Scope {
		target := filepath.Join(worktree, s)
		fi, err := os.Stat(target)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			_ = filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
				if err == nil && info.ModTime().After(lastActivity) {
					lastActivity = info.ModTime()
				}
				return nil
			})
		} else {
			if fi.ModTime().After(lastActivity) {
				lastActivity = fi.ModTime()
			}
		}
	}

	if now.Sub(lastActivity) <= stallTimeout {
		return nil
	}

	// Job is stalled!
	events, _ := w.Ledger.Events(job.ID)
	stalledCount := 0
	for _, ev := range events {
		if ev.Type == "stalled" {
			stalledCount++
		}
	}

	// Kill process group
	if w.KillPGID != nil && job.PGID > 0 {
		_ = w.KillPGID(job.PGID)
	}

	if stalledCount >= 1 {
		// Already stalled and retried once -> mark needs_review
		job.Status = StatusNeedsReview
		if err := w.Ledger.Save(job); err != nil {
			return err
		}
		return w.Ledger.Append(job.ID, Event{
			Type: "needs_review",
			Data: map[string]any{
				"reason": "job stalled repeatedly",
			},
		})
	}

	// Record stalled event
	_ = w.Ledger.Append(job.ID, Event{
		Type: "stalled",
		Data: map[string]any{
			"last_activity": lastActivity.UTC().Format(time.RFC3339),
			"timeout":       stallTimeout.String(),
		},
	})

	// Retry once (preserving/resuming conversation ID if present)
	return w.Dispatcher.Retry(job.ID)
}

// GenerateLaunchdPlist returns the launchd plist XML content to run `dendrite agent jobs daemon`.
func GenerateLaunchdPlist(binaryPath string) string {
	if binaryPath == "" {
		exe, err := os.Executable()
		if err == nil {
			binaryPath = exe
		} else {
			binaryPath = "dendrite"
		}
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>dev.dendrite.jobs</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>agent</string>
        <string>jobs</string>
        <string>daemon</string>
    </array>
    <key>KeepAlive</key>
    <true/>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/dendrite-jobs-daemon.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/dendrite-jobs-daemon.err</string>
</dict>
</plist>
`, binaryPath)
}

// InstallLaunchAgent writes the launchd plist to the specified plistPath.
// If plistPath is empty, DefaultLaunchAgentPath() is used.
func InstallLaunchAgent(plistPath, binaryPath string) (string, error) {
	if plistPath == "" {
		plistPath = DefaultLaunchAgentPath()
	}
	dir := filepath.Dir(plistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	content := GenerateLaunchdPlist(binaryPath)
	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write plist to %q: %w", plistPath, err)
	}
	return plistPath, nil
}
