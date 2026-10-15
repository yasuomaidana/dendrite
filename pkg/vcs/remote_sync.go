package vcs

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ticketRegex = regexp.MustCompile(`(?i)(?:fixes\s+#|#|\[)?(DEN-\d+|PROJ-[A-Z]-\d+|#\d+)\]?`)
)

// RemoteSyncStatus contains the divergence metrics and reconciliation outcome.
type RemoteSyncStatus struct {
	Branch            string    `json:"branch"`
	Ahead             int       `json:"ahead"`
	Behind            int       `json:"behind"`
	RemoteCommits     []string  `json:"remote_commits"`
	ReconciledTickets []string  `json:"reconciled_tickets"`
	Updated           bool      `json:"updated"`
	SyncedAt          time.Time `json:"synced_at"`
	Details           string    `json:"details,omitempty"`
}

// RemoteSyncWatcher handles pre-flight remote polling, divergence detection, and reconciliation.
type RemoteSyncWatcher struct {
	Guard        *GitGuard
	Orchestrator *StackedBranchOrchestrator
}

// NewRemoteSyncWatcher initializes a RemoteSyncWatcher.
func NewRemoteSyncWatcher(guard *GitGuard, orch *StackedBranchOrchestrator) *RemoteSyncWatcher {
	if orch == nil {
		orch = NewStackedBranchOrchestrator(guard, "")
	}
	return &RemoteSyncWatcher{
		Guard:        guard,
		Orchestrator: orch,
	}
}

// FetchAndDetect executes git fetch --all --prune and computes ahead/behind counts.
func (w *RemoteSyncWatcher) FetchAndDetect(ctx context.Context, branch string) (*RemoteSyncStatus, error) {
	if branch == "" {
		var err error
		branch, err = w.Guard.CurrentBranch(ctx)
		if err != nil {
			return nil, err
		}
	}

	// 1. Fetch remote tracking state
	_, _ = w.Guard.runGit(ctx, "fetch", "--all", "--prune")

	// 2. Count divergence: git rev-list --left-right --count HEAD...origin/<branch>
	remoteRef := "origin/" + branch
	out, err := w.Guard.runGit(ctx, "rev-list", "--left-right", "--count", branch+"..."+remoteRef)
	if err != nil {
		// If remoteRef doesn't exist yet, we're not behind
		return &RemoteSyncStatus{
			Branch:   branch,
			Ahead:    0,
			Behind:   0,
			SyncedAt: time.Now(),
			Details:  "No remote tracking branch configured",
		}, nil
	}

	// Output format: "<ahead>\t<behind>"
	parts := strings.Fields(out)
	ahead := 0
	behind := 0
	if len(parts) >= 2 {
		ahead, _ = strconv.Atoi(parts[0])
		behind, _ = strconv.Atoi(parts[1])
	}

	return &RemoteSyncStatus{
		Branch:   branch,
		Ahead:    ahead,
		Behind:   behind,
		SyncedAt: time.Now(),
	}, nil
}

// SyncRemoteFixes checks for remote commits, rebases local branch, and reconciles ticket states.
func (w *RemoteSyncWatcher) SyncRemoteFixes(ctx context.Context, branch string) (*RemoteSyncStatus, error) {
	status, err := w.FetchAndDetect(ctx, branch)
	if err != nil {
		return nil, err
	}

	if status.Behind == 0 {
		status.Details = "Local workspace is already up to date with remote"
		return status, nil
	}

	// Record pre-pull HEAD SHA
	oldHead, _ := w.Guard.runGit(ctx, "rev-parse", "HEAD")

	// Pull with rebase
	pullOut, pErr := w.Guard.runGit(ctx, "pull", "--rebase", "origin", status.Branch)
	if pErr != nil {
		return status, fmt.Errorf("remote fix rebase failed on %s: %w (output: %s)", status.Branch, pErr, pullOut)
	}
	status.Updated = true

	// Parse incoming commits: git log <oldHead>..HEAD --oneline
	logOut, _ := w.Guard.runGit(ctx, "log", oldHead+"..HEAD", "--oneline")
	commitLines := strings.Split(strings.TrimSpace(logOut), "\n")
	for _, l := range commitLines {
		if strings.TrimSpace(l) != "" {
			status.RemoteCommits = append(status.RemoteCommits, l)
		}
	}

	// Extract ticket IDs from commit log
	status.ReconciledTickets = ExtractTicketIDs(logOut)
	status.Details = fmt.Sprintf("Successfully applied %d remote commit(s) via rebase", status.Behind)

	return status, nil
}

// ExtractTicketIDs parses ticket identifiers from commit messages.
func ExtractTicketIDs(text string) []string {
	matches := ticketRegex.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var result []string

	for _, m := range matches {
		if len(m) > 1 {
			id := strings.ToUpper(strings.TrimSpace(m[1]))
			if id != "" && !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}
	return result
}
