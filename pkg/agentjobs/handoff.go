package agentjobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/statedir"
)

// DefaultHandoffPath is the canonical relative path to the session handoff file.
const DefaultHandoffPath = ".dendrite/session/handoff.md"

// JobRef summarizes a job's identity and status within a session checkpoint.
type JobRef struct {
	ID          string    `json:"id"`
	Ticket      string    `json:"ticket,omitempty"`
	Status      Status    `json:"status"`
	Agent       string    `json:"agent,omitempty"`
	Model       string    `json:"model,omitempty"`
	ResumeAfter time.Time `json:"resume_after,omitempty"`
}

type jobRefJSON JobRef

// MarshalJSON customizes JSON serialization to omit zero-valued ResumeAfter.
func (r JobRef) MarshalJSON() ([]byte, error) {
	if r.ResumeAfter.IsZero() {
		type jobRefWithoutResumeAfter struct {
			jobRefJSON
			ResumeAfter *time.Time `json:"resume_after,omitempty"`
		}
		return json.Marshal(jobRefWithoutResumeAfter{
			jobRefJSON:  jobRefJSON(r),
			ResumeAfter: nil,
		})
	}
	return json.Marshal(jobRefJSON(r))
}

// Checkpoint captures the orchestration session state for handoff and resumption.
type Checkpoint struct {
	Branch              string    `json:"branch"`
	Worktree            string    `json:"worktree"`
	Ticket              string    `json:"ticket,omitempty"`
	InFlight            []JobRef  `json:"in_flight"`
	VerifiedUncommitted []JobRef  `json:"verified_uncommitted"`
	PendingDecisions    []string  `json:"pending_decisions,omitempty"`
	Next                []string  `json:"next,omitempty"`
	Notes               string    `json:"notes,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// CheckpointOptions specifies configuration when constructing a Checkpoint.
type CheckpointOptions struct {
	Worktree         string   `json:"worktree,omitempty"`
	Ticket           string   `json:"ticket,omitempty"`
	PendingDecisions []string `json:"pending_decisions,omitempty"`
	Next             []string `json:"next,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

// ResumeReport contains structured resume outcome information.
type ResumeReport struct {
	Checkpoint  *Checkpoint    `json:"checkpoint"`
	HandoffText string         `json:"handoff_text"`
	Delta       string         `json:"delta"`
	Counts      map[Status]int `json:"counts"`
	UpdatedJobs []JobRef       `json:"updated_jobs,omitempty"`
}

// BuildCheckpoint inspects the jobs ledger, determines the git branch in the worktree,
// and merges options into a new Checkpoint instance.
func BuildCheckpoint(ledger *Ledger, opts CheckpointOptions) (*Checkpoint, error) {
	if ledger == nil {
		ledger = NewLedger("")
	}

	jobs, err := ledger.List()
	if err != nil {
		return nil, fmt.Errorf("build checkpoint: failed to list jobs: %w", err)
	}

	inFlight := make([]JobRef, 0)
	verifiedUncommitted := make([]JobRef, 0)

	for _, job := range jobs {
		ref := JobRef{
			ID:          job.ID,
			Ticket:      job.Ticket,
			Status:      job.Status,
			Agent:       job.Agent,
			Model:       job.Model,
			ResumeAfter: job.ResumeAfter,
		}
		switch job.Status {
		case StatusRunning, StatusQueued, StatusParked:
			inFlight = append(inFlight, ref)
		case StatusVerified:
			verifiedUncommitted = append(verifiedUncommitted, ref)
		}
	}

	worktree := opts.Worktree
	if worktree == "" {
		if top, err := gitTopLevelIn(""); err == nil && top != "" {
			worktree = top
		} else if cwd, err := os.Getwd(); err == nil {
			worktree = cwd
		} else {
			worktree = "."
		}
	}

	branch := gitBranchIn(worktree)

	decisions := make([]string, 0)
	seen := make(map[string]bool)
	for _, d := range opts.PendingDecisions {
		trimmed := strings.TrimSpace(d)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			decisions = append(decisions, trimmed)
		}
	}

	nextList := make([]string, 0)
	for _, n := range opts.Next {
		trimmed := strings.TrimSpace(n)
		if trimmed != "" {
			nextList = append(nextList, trimmed)
		}
	}

	notes := strings.TrimSpace(opts.Notes)

	cp := &Checkpoint{
		Branch:              branch,
		Worktree:            worktree,
		Ticket:              strings.TrimSpace(opts.Ticket),
		InFlight:            inFlight,
		VerifiedUncommitted: verifiedUncommitted,
		PendingDecisions:    decisions,
		Next:                nextList,
		Notes:               notes,
		CreatedAt:           time.Now().UTC(),
	}

	return cp, nil
}

func gitBranchIn(worktree string) string {
	// symbolic-ref also works on an unborn branch (no commits yet); rev-parse covers a detached HEAD.
	for _, args := range [][]string{{"symbolic-ref", "--short", "HEAD"}, {"rev-parse", "--short", "HEAD"}} {
		cmd := exec.Command("git", args...)
		if worktree != "" {
			cmd.Dir = worktree
		}
		if out, err := cmd.Output(); err == nil {
			if branch := strings.TrimSpace(string(out)); branch != "" {
				return branch
			}
		}
	}
	return "unknown"
}

func gitTopLevelIn(worktree string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if worktree != "" {
		cmd.Dir = worktree
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// WriteCheckpoint renders the checkpoint into compact Markdown under 500 tokens
// plus a machine-readable JSON block at the end, saving it to path (resolved via statedir.Resolve).
func WriteCheckpoint(path string, cp *Checkpoint) error {
	if cp == nil {
		return errors.New("agentjobs: cannot write nil checkpoint")
	}

	target := resolveHandoffPath(path)
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("write checkpoint: failed to create directory: %w", err)
	}

	content := RenderCheckpoint(cp)

	if err := writeFileAtomic(target, []byte(content)); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}

	return nil
}

func resolveHandoffPath(path string) string {
	if path == "" {
		path = DefaultHandoffPath
	}
	return statedir.Resolve(path)
}

const maxDisplayItems = 5

// RenderCheckpoint formats a Checkpoint as a compact markdown document under ~2000 characters
// with trailing machine-readable JSON block.
func RenderCheckpoint(cp *Checkpoint) string {
	if cp == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Session Handoff\n\n")
	sb.WriteString(fmt.Sprintf("- **Branch**: %s\n", cp.Branch))
	sb.WriteString(fmt.Sprintf("- **Worktree**: %s\n", cp.Worktree))
	if cp.Ticket != "" {
		sb.WriteString(fmt.Sprintf("- **Ticket**: %s\n", cp.Ticket))
	}
	sb.WriteString(fmt.Sprintf("- **Created**: %s\n\n", cp.CreatedAt.UTC().Format(time.RFC3339)))

	sb.WriteString("## In-Flight Jobs\n")
	sb.WriteString(renderJobList(cp.InFlight))
	sb.WriteString("\n")

	sb.WriteString("## Verified Uncommitted\n")
	sb.WriteString(renderJobList(cp.VerifiedUncommitted))
	sb.WriteString("\n")

	sb.WriteString("## Pending Decisions\n")
	sb.WriteString(renderStringList(cp.PendingDecisions))
	sb.WriteString("\n")

	sb.WriteString("## Next\n")
	sb.WriteString(renderStringList(cp.Next))
	sb.WriteString("\n")

	sb.WriteString("## Notes\n")
	if strings.TrimSpace(cp.Notes) == "" {
		sb.WriteString("(none)\n")
	} else {
		n := strings.TrimSpace(cp.Notes)
		if len(n) > 300 {
			n = n[:300] + "…"
		}
		sb.WriteString(n)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	jsonBytes, err := json.MarshalIndent(cp, "", "  ")
	if err == nil {
		sb.WriteString("```json\n")
		sb.Write(jsonBytes)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

func renderJobList(jobs []JobRef) string {
	if len(jobs) == 0 {
		return "- (none)\n"
	}
	var sb strings.Builder
	limit := len(jobs)
	if limit > maxDisplayItems {
		limit = maxDisplayItems
	}
	for i := 0; i < limit; i++ {
		j := jobs[i]
		label := j.Ticket
		if label == "" {
			label = j.ID
		}
		statusText := string(j.Status)
		if j.Status == StatusParked && !j.ResumeAfter.IsZero() {
			statusText = fmt.Sprintf("parked until %s", j.ResumeAfter.UTC().Format("15:04"))
		}
		if j.Agent != "" {
			statusText = fmt.Sprintf("%s, %s", statusText, j.Agent)
		}
		sb.WriteString(fmt.Sprintf("- %s (%s)\n", label, statusText))
	}
	if len(jobs) > maxDisplayItems {
		sb.WriteString(fmt.Sprintf("- … and %d more\n", len(jobs)-maxDisplayItems))
	}
	return sb.String()
}

func renderStringList(items []string) string {
	if len(items) == 0 {
		return "- (none)\n"
	}
	var sb strings.Builder
	limit := len(items)
	if limit > maxDisplayItems {
		limit = maxDisplayItems
	}
	for i := 0; i < limit; i++ {
		sb.WriteString(fmt.Sprintf("- %s\n", items[i]))
	}
	if len(items) > maxDisplayItems {
		sb.WriteString(fmt.Sprintf("- … and %d more\n", len(items)-maxDisplayItems))
	}
	return sb.String()
}

// ReadCheckpoint parses a Checkpoint from handoff.md, extracting its trailing JSON block.
func ReadCheckpoint(path string) (*Checkpoint, error) {
	target := resolveHandoffPath(path)
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}

	content := string(data)
	jsonBlock := extractJSONBlock(content)

	var cp Checkpoint
	if err := json.Unmarshal([]byte(jsonBlock), &cp); err != nil {
		return nil, fmt.Errorf("read checkpoint: failed to parse JSON: %w", err)
	}

	return &cp, nil
}

func extractJSONBlock(content string) string {
	const fence = "```json"
	idx := strings.LastIndex(content, fence)
	if idx != -1 {
		rest := content[idx+len(fence):]
		end := strings.Index(rest, "```")
		if end != -1 {
			return strings.TrimSpace(rest[:end])
		}
	}
	return strings.TrimSpace(content)
}

// ResumeState loads the handoff checkpoint and calculates status deltas for any ledger jobs
// whose UpdatedAt is after the checkpoint's CreatedAt.
func ResumeState(path string, ledger *Ledger) (*ResumeReport, error) {
	target := resolveHandoffPath(path)
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("resume: failed to read handoff: %w", err)
	}

	cp, err := ReadCheckpoint(target)
	if err != nil {
		return nil, fmt.Errorf("resume: failed to read checkpoint: %w", err)
	}

	if ledger == nil {
		ledger = NewLedger("")
	}

	jobs, err := ledger.List()
	if err != nil {
		return nil, fmt.Errorf("resume: failed to list ledger jobs: %w", err)
	}

	counts := make(map[Status]int)
	var parkedResumeTimes []time.Time
	var updatedJobs []JobRef

	for _, job := range jobs {
		if job.UpdatedAt.After(cp.CreatedAt) {
			counts[job.Status]++
			ref := JobRef{
				ID:          job.ID,
				Ticket:      job.Ticket,
				Status:      job.Status,
				Agent:       job.Agent,
				Model:       job.Model,
				ResumeAfter: job.ResumeAfter,
			}
			updatedJobs = append(updatedJobs, ref)
			if job.Status == StatusParked && !job.ResumeAfter.IsZero() {
				parkedResumeTimes = append(parkedResumeTimes, job.ResumeAfter)
			}
		}
	}

	delta := formatDelta(counts, parkedResumeTimes)

	report := &ResumeReport{
		Checkpoint:  cp,
		HandoffText: strings.TrimRight(string(data), "\n"),
		Delta:       delta,
		Counts:      counts,
		UpdatedJobs: updatedJobs,
	}

	return report, nil
}

func formatDelta(counts map[Status]int, parkedTimes []time.Time) string {
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return "since checkpoint: no status changes"
	}

	preferredOrder := []Status{
		StatusVerified,
		StatusParked,
		StatusFailed,
		StatusFailedRetryable,
		StatusRunning,
		StatusQueued,
		StatusSucceeded,
		StatusCommitted,
		StatusNeedsReview,
		StatusEarlyExit,
		StatusQuotaExhausted,
		StatusEscalated,
	}

	var parts []string
	handled := make(map[Status]bool)

	for _, s := range preferredOrder {
		if c := counts[s]; c > 0 {
			handled[s] = true
			if s == StatusParked && len(parkedTimes) == 1 {
				parts = append(parts, fmt.Sprintf("%d parked until %s", c, parkedTimes[0].UTC().Format("15:04")))
			} else {
				parts = append(parts, fmt.Sprintf("%d %s", c, s))
			}
		}
	}

	var remaining []string
	for s, c := range counts {
		if !handled[s] && c > 0 {
			remaining = append(remaining, fmt.Sprintf("%d %s", c, s))
		}
	}
	sort.Strings(remaining)
	parts = append(parts, remaining...)

	return "since checkpoint: " + strings.Join(parts, ", ")
}

// Resume returns the handoff text plus a delta of job status changes since the checkpoint's CreatedAt.
func Resume(path string, ledger *Ledger) (string, error) {
	report, err := ResumeState(path, ledger)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s\n\n%s\n", report.HandoffText, report.Delta), nil
}
