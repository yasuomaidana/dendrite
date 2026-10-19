package agentjobs

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"Dendrite/pkg/statedir"
)

var randReader io.Reader = rand.Reader

// Ledger manages persistent agent jobs and their write-ahead event logs.
type Ledger struct {
	root string
	mu   sync.RWMutex
}

// DefaultRoot returns the canonical agent jobs root directory resolved via statedir.
func DefaultRoot() string {
	return statedir.Resolve(".dendrite/jobs")
}

// NewLedger returns a new Ledger rooted at the specified directory.
// If root is empty, DefaultRoot() is resolved on each use, so a state directory
// set after construction (the CLI's --state-dir) is honoured.
func NewLedger(root string) *Ledger {
	return &Ledger{root: root}
}

// Root returns the root directory of this ledger.
func (l *Ledger) Root() string {
	if l.root == "" {
		return DefaultRoot()
	}
	return l.root
}

// JobDir returns the directory path for the specified job ID.
func (l *Ledger) JobDir(id string) string {
	return filepath.Join(l.Root(), id)
}

// JobPath returns the path to the job.json file for the specified job ID.
func (l *Ledger) JobPath(id string) string {
	return filepath.Join(l.Root(), id, "job.json")
}

// EventsPath returns the path to the events.jsonl file for the specified job ID.
func (l *Ledger) EventsPath(id string) string {
	return filepath.Join(l.Root(), id, "events.jsonl")
}

// DoneMarkerPath returns the path to the done marker file for the specified job ID.
func (l *Ledger) DoneMarkerPath(id string) string {
	return filepath.Join(l.Root(), id, "done")
}

// PromptPath returns the path to prompt.md for the specified job ID.
func (l *Ledger) PromptPath(id string) string {
	return filepath.Join(l.Root(), id, "prompt.md")
}

// CapsulePath returns the path to capsule.md for the specified job ID.
func (l *Ledger) CapsulePath(id string) string {
	return filepath.Join(l.Root(), id, "capsule.md")
}

// AgentLogPath returns the path to agent.log for the specified job ID.
func (l *Ledger) AgentLogPath(id string) string {
	return filepath.Join(l.Root(), id, "agent.log")
}

// randomSuffix returns a random hex string of 2*n characters.
func randomSuffix(n int) string {
	b := make([]byte, n)
	if _, err := randReader.Read(b); err != nil {
		return fmt.Sprintf("%04x", time.Now().UnixNano()&0xffff)
	}
	return hex.EncodeToString(b)
}

// Create generates a new unique job ID (ticket + "-" + UTC timestamp + short random suffix),
// initializes timestamps and default fields, and saves the job atomically.
func (l *Ledger) Create(job *Job) (*Job, error) {
	if job == nil {
		return nil, errors.New("agentjobs: cannot create nil job")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ticket := job.Ticket
	if ticket == "" {
		ticket = "JOB"
	}

	now := time.Now().UTC()
	job.ID = fmt.Sprintf("%s-%s-%s", ticket, now.Format("20060102T150405Z"), randomSuffix(3))

	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now

	if job.Status == "" {
		job.Status = StatusQueued
	}
	if job.Attempts == nil {
		job.Attempts = make(map[string]int)
	}

	if err := l.saveLocked(job); err != nil {
		return nil, err
	}
	return job, nil
}

// Save atomically writes the job specification to job.json using a temp file and rename.
func (l *Ledger) Save(job *Job) error {
	if job == nil {
		return errors.New("agentjobs: cannot save nil job")
	}
	if job.ID == "" {
		return errors.New("agentjobs: cannot save job without ID")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	job.UpdatedAt = time.Now().UTC()
	return l.saveLocked(job)
}

func (l *Ledger) saveLocked(job *Job) error {
	jobDir := l.JobDir(job.ID)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return fmt.Errorf("agentjobs: failed to create job directory: %w", err)
	}

	data, _ := json.MarshalIndent(job, "", "  ")
	data = append(data, '\n')

	if err := writeFileAtomic(l.JobPath(job.ID), data); err != nil {
		return fmt.Errorf("agentjobs: failed to save job: %w", err)
	}
	return nil
}

// Load reads and parses job.json for the specified job ID.
func (l *Ledger) Load(id string) (*Job, error) {
	if id == "" {
		return nil, errors.New("agentjobs: job id cannot be empty")
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	data, err := os.ReadFile(l.JobPath(id))
	if err != nil {
		return nil, fmt.Errorf("agentjobs: failed to read job %q: %w", id, err)
	}

	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("agentjobs: failed to parse job %q: %w", id, err)
	}
	return &job, nil
}

// Append writes an event to events.jsonl, automatically assigning an increasing Seq and fsyncing.
// If the file ended with a torn/partial line, it truncates the torn bytes before appending.
func (l *Ledger) Append(id string, event Event) error {
	if id == "" {
		return errors.New("agentjobs: job id cannot be empty")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	jobDir := l.JobDir(id)
	if err := os.MkdirAll(jobDir, 0755); err != nil {
		return fmt.Errorf("agentjobs: failed to create job directory: %w", err)
	}

	eventsPath := l.EventsPath(id)
	events, validOffset, err := l.readEventsWithOffset(id)
	if err != nil {
		return err
	}

	var maxSeq int64 = 0
	for _, ev := range events {
		if ev.Seq > maxSeq {
			maxSeq = ev.Seq
		}
	}
	event.Seq = maxSeq + 1

	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}

	if info, err := os.Stat(eventsPath); err == nil && info.Size() > validOffset {
		_ = os.Truncate(eventsPath, validOffset)
	}

	f, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("agentjobs: failed to open events log: %w", err)
	}
	defer f.Close()

	line, _ := json.Marshal(event)
	line = append(line, '\n')

	_, _ = f.Write(line)
	_ = f.Sync()
	return nil
}

// Events reads all events from events.jsonl. If the last line is torn or partial, it is safely skipped.
func (l *Ledger) Events(id string) ([]Event, error) {
	if id == "" {
		return nil, errors.New("agentjobs: job id cannot be empty")
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	events, _, err := l.readEventsWithOffset(id)
	return events, err
}

func (l *Ledger) readEventsWithOffset(id string) ([]Event, int64, error) {
	eventsPath := l.EventsPath(id)
	content, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, 0, nil
		}
		return nil, 0, fmt.Errorf("agentjobs: failed to read events log: %w", err)
	}

	rawLines := bytes.Split(content, []byte("\n"))
	var nonBlank [][]byte
	for _, line := range rawLines {
		if len(bytes.TrimSpace(line)) > 0 {
			nonBlank = append(nonBlank, line)
		}
	}

	var events []Event
	var validOffset int64
	offset := int64(0)
	n := len(nonBlank)

	for _, rawLine := range rawLines {
		lineWithNL := int64(len(rawLine))
		if offset+lineWithNL < int64(len(content)) && content[offset+lineWithNL] == '\n' {
			lineWithNL++
		}

		trimmed := bytes.TrimSpace(rawLine)
		if len(trimmed) > 0 {
			var ev Event
			if err := json.Unmarshal(trimmed, &ev); err != nil {
				if len(events) == n-1 && bytes.Equal(trimmed, nonBlank[n-1]) {
					offset += lineWithNL
					continue
				}
				return nil, 0, fmt.Errorf("agentjobs: corrupted event: %w", err)
			}
			events = append(events, ev)
			validOffset = offset + lineWithNL
		}
		offset += lineWithNL
	}

	if events == nil {
		events = []Event{}
	}
	return events, validOffset, nil
}

// List returns all jobs in the ledger sorted chronologically by CreatedAt.
func (l *Ledger) List() ([]*Job, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entries, err := os.ReadDir(l.Root())
	if err != nil {
		if os.IsNotExist(err) {
			return []*Job{}, nil
		}
		return nil, fmt.Errorf("agentjobs: failed to read jobs root directory: %w", err)
	}

	var jobs []*Job
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jobPath := filepath.Join(l.Root(), entry.Name(), "job.json")
		data, err := os.ReadFile(jobPath)
		if err != nil {
			// Subdirectory without a readable job.json is skipped.
			continue
		}
		var j Job
		if err := json.Unmarshal(data, &j); err != nil {
			// Corrupted job.json is skipped.
			continue
		}
		jobs = append(jobs, &j)
	}

	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})

	if jobs == nil {
		jobs = []*Job{}
	}
	return jobs, nil
}

// MarkDone writes/updates the "done" marker file with the current UTC timestamp
// and updates the file modification time so watchers can poll its mtime.
func (l *Ledger) MarkDone(id string) error {
	if id == "" {
		return errors.New("agentjobs: job id cannot be empty")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	jobDir := l.JobDir(id)
	if _, err := os.Stat(jobDir); err != nil {
		return fmt.Errorf("agentjobs: job directory not found: %w", err)
	}

	donePath := l.DoneMarkerPath(id)
	now := time.Now().UTC()
	content := []byte(now.Format(time.RFC3339Nano) + "\n")

	if err := os.WriteFile(donePath, content, 0644); err != nil {
		return fmt.Errorf("agentjobs: failed to write done marker: %w", err)
	}
	_ = os.Chtimes(donePath, now, now)
	return nil
}

// writeFile is the file writer used by writeFileAtomic; tests replace it to exercise write failures.
var writeFile = os.WriteFile

// writeFileAtomic writes data next to dest and renames it into place, so readers never see a
// partially written file. Write and close errors are surfaced (a truncated temp file is never renamed).
func writeFileAtomic(dest string, data []byte) error {
	tmp := dest + ".tmp-" + randomSuffix(6)
	if err := writeFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
