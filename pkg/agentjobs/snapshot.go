package agentjobs

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Snapshot is what can be seen of one job right now: enough to tell a busy agent from a stuck one.
type Snapshot struct {
	Job    string `json:"job"`
	Ticket string `json:"ticket"`
	Agent  string `json:"agent"`
	Model  string `json:"model"`
	Status Status `json:"status"`
	// ElapsedSeconds is the time since the job was created.
	ElapsedSeconds int64 `json:"elapsed_seconds"`
	// ProcessAlive is true while the job's process group exists.
	ProcessAlive bool `json:"process_alive"`
	// Requests is the number of model requests the agent has made (agy jobs).
	Requests int `json:"requests"`
	// LastRequestAgeSeconds is the age of the newest model request; -1 when there is none.
	LastRequestAgeSeconds int64 `json:"last_request_age_seconds"`
	// ChangedFiles counts the files changed in the job's working copy, excluding Dendrite's own state.
	ChangedFiles int `json:"changed_files"`
	// StallInSeconds is the time left before the watcher would call a running job stalled (0 when
	// it is not running or already past it).
	StallInSeconds int64 `json:"stall_in_seconds"`
	// WatcherAlive says whether a stall watcher is running over this ledger.
	WatcherAlive bool `json:"watcher_alive"`
}

// Final reports whether the job has reached a state it will not leave by itself.
func (s Snapshot) Final() bool {
	switch s.Status {
	case StatusQueued, StatusRunning, StatusParked, StatusFailedRetryable:
		return false
	}
	return true
}

// Line renders the snapshot as one line for a terminal.
func (s Snapshot) Line() string {
	secs := func(n int64) string { return (time.Duration(n) * time.Second).String() }
	parts := []string{string(s.Status) + " " + secs(s.ElapsedSeconds)}
	if s.ProcessAlive {
		parts = append(parts, "process alive")
	} else {
		parts = append(parts, "no process")
	}
	if s.Agent == "agy" {
		if s.Requests == 0 {
			parts = append(parts, "no model request yet")
		} else {
			parts = append(parts, fmt.Sprintf("%d model requests, last %s ago", s.Requests, secs(s.LastRequestAgeSeconds)))
		}
	}
	parts = append(parts, fmt.Sprintf("%d files changed", s.ChangedFiles))
	if s.Status == StatusRunning {
		parts = append(parts, "stall check in "+secs(s.StallInSeconds))
	}
	if s.WatcherAlive {
		parts = append(parts, "watcher up")
	} else {
		parts = append(parts, "WATCHER DOWN")
	}
	return strings.Join(parts, " | ")
}

// Snapshot looks at a job without changing anything.
func (w *Watcher) Snapshot(ctx context.Context, id string) (Snapshot, error) {
	job, err := w.Ledger.Load(id)
	if err != nil {
		return Snapshot{}, fmt.Errorf("agentjobs: failed to load job %q: %w", id, err)
	}
	now := w.now()
	s := Snapshot{
		Job: job.ID, Ticket: job.Ticket, Agent: job.Agent, Model: job.Model, Status: job.Status,
		ElapsedSeconds:        int64(now.Sub(job.CreatedAt).Seconds()),
		ProcessAlive:          job.PGID > 0 && !w.IsProcessDead(job.PGID),
		LastRequestAgeSeconds: -1,
	}
	if act, aErr := ReadAgyActivity(w.Ledger.AgyLogPath(id)); aErr == nil {
		s.Requests = act.Requests
		if !act.LastRequest.IsZero() {
			s.LastRequestAgeSeconds = int64(now.Sub(act.LastRequest).Seconds())
		}
	}
	s.ChangedFiles = w.changedFiles(ctx, job)
	if job.Status == StatusRunning {
		if left := w.stallTimeout() - now.Sub(w.lastActivity(job)); left > 0 {
			s.StallInSeconds = int64(left.Seconds())
		}
	}
	_, s.WatcherAlive = w.Ledger.WatcherAlive(now)
	return s, nil
}

func (w *Watcher) stallTimeout() time.Duration {
	if w.StallTimeout > 0 {
		return w.StallTimeout
	}
	return DefaultStallTimeout
}

// changedFiles counts the entries of `git status --porcelain` in the job's working copy that are not
// Dendrite's own state. An unreadable working copy counts as none.
func (w *Watcher) changedFiles(ctx context.Context, job *Job) int {
	git := "git"
	if w.Dispatcher != nil && w.Dispatcher.GitPath != "" {
		git = w.Dispatcher.GitPath
	}
	cmd := exec.CommandContext(ctx, git, "status", "--porcelain")
	cmd.Dir = job.Worktree
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range bytes.Split(out, []byte("\n")) {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(string(line[3:]))
		if i := strings.LastIndex(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		if w.Dispatcher != nil && w.Dispatcher.isStatePath(job.Worktree, strings.Trim(path, `"`)) {
			continue
		}
		n++
	}
	return n
}
