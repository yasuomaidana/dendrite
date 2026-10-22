package agentjobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLedger_DefaultRoot(t *testing.T) {
	root := DefaultRoot()
	if root == "" {
		t.Fatalf("DefaultRoot() returned empty string")
	}
	if !strings.Contains(root, "jobs") {
		t.Errorf("DefaultRoot() = %q, expected to contain 'jobs'", root)
	}

	lDefault := NewLedger("")
	if lDefault.Root() != root {
		t.Errorf("NewLedger(\"\").Root() = %q, want %q", lDefault.Root(), root)
	}
}

func TestLedger_Paths(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	id := "DEN-100-20261017T180000Z-abcd"
	if got, want := l.Root(), tmpDir; got != want {
		t.Errorf("Root() = %q, want %q", got, want)
	}
	if got, want := l.JobDir(id), filepath.Join(tmpDir, id); got != want {
		t.Errorf("JobDir(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.JobPath(id), filepath.Join(tmpDir, id, "job.json"); got != want {
		t.Errorf("JobPath(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.EventsPath(id), filepath.Join(tmpDir, id, "events.jsonl"); got != want {
		t.Errorf("EventsPath(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.DoneMarkerPath(id), filepath.Join(tmpDir, id, "done"); got != want {
		t.Errorf("DoneMarkerPath(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.PromptPath(id), filepath.Join(tmpDir, id, "prompt.md"); got != want {
		t.Errorf("PromptPath(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.PromptPath(id), filepath.Join(tmpDir, id, "prompt.md"); got != want {
		t.Errorf("PromptPath(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.CapsulePath(id), filepath.Join(tmpDir, id, "capsule.md"); got != want {
		t.Errorf("CapsulePath(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.CapsulePath(id), filepath.Join(tmpDir, id, "capsule.md"); got != want {
		t.Errorf("CapsulePath(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.AgentLogPath(id), filepath.Join(tmpDir, id, "agent.log"); got != want {
		t.Errorf("AgentLogPath(%q) = %q, want %q", id, got, want)
	}
	if got, want := l.AgentLogPath(id), filepath.Join(tmpDir, id, "agent.log"); got != want {
		t.Errorf("AgentLogPath(%q) = %q, want %q", id, got, want)
	}
}

func TestLedger_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	jobInput := &Job{
		Ticket:         "DEN-988",
		Complexity:     3,
		Worktree:       "/tmp/worktree",
		Scope:          []string{"pkg/agentjobs/ledger.go"},
		PromptFile:     "/tmp/prompt.md",
		Agent:          "agy",
		Model:          "gemini-2.5-pro",
		Tier:           1,
		ConversationID: "conv-12345",
	}

	created, err := l.Create(jobInput)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !strings.HasPrefix(created.ID, "DEN-988-") {
		t.Errorf("Job ID %q does not start with DEN-988-", created.ID)
	}
	if created.Status != StatusQueued {
		t.Errorf("created.Status = %v, want %v", created.Status, StatusQueued)
	}
	if created.CreatedAt.IsZero() {
		t.Errorf("created.CreatedAt is zero")
	}
	if created.UpdatedAt.IsZero() {
		t.Errorf("created.UpdatedAt is zero")
	}
	if created.Attempts == nil {
		t.Errorf("created.Attempts is nil")
	}

	loaded, err := l.Load(created.ID)
	if err != nil {
		t.Fatalf("Load(%q) failed: %v", created.ID, err)
	}

	if loaded.ID != created.ID {
		t.Errorf("loaded.ID = %q, want %q", loaded.ID, created.ID)
	}
	if loaded.Ticket != "DEN-988" {
		t.Errorf("loaded.Ticket = %q, want DEN-988", loaded.Ticket)
	}
	if loaded.Complexity != 3 {
		t.Errorf("loaded.Complexity = %d, want 3", loaded.Complexity)
	}
	if loaded.Agent != "agy" {
		t.Errorf("loaded.Agent = %q, want agy", loaded.Agent)
	}
	if loaded.ConversationID != "conv-12345" {
		t.Errorf("loaded.ConversationID = %q, want conv-12345", loaded.ConversationID)
	}

	// Update job state and save atomically
	loaded.Status = StatusRunning
	loaded.Attempts["agy"] = 1
	loaded.ResumeAfter = time.Now().UTC().Add(10 * time.Minute)

	if err := l.Save(loaded); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded, err := l.Load(created.ID)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if reloaded.Status != StatusRunning {
		t.Errorf("reloaded.Status = %v, want %v", reloaded.Status, StatusRunning)
	}
	if reloaded.Attempts["agy"] != 1 {
		t.Errorf("reloaded.Attempts['agy'] = %d, want 1", reloaded.Attempts["agy"])
	}
	if reloaded.ResumeAfter.IsZero() {
		t.Errorf("reloaded.ResumeAfter is zero")
	}
}

func TestLedger_CreateDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	past := time.Now().UTC().Add(-1 * time.Hour)
	customAttempts := map[string]int{"agent1": 2}

	// Test with empty Ticket, preset CreatedAt, preset Status, preset Attempts
	job := &Job{
		CreatedAt: past,
		Status:    StatusRunning,
		Attempts:  customAttempts,
	}

	created, err := l.Create(job)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !strings.HasPrefix(created.ID, "JOB-") {
		t.Errorf("Job ID %q does not start with JOB-", created.ID)
	}
	if !created.CreatedAt.Equal(past) {
		t.Errorf("created.CreatedAt = %v, want %v", created.CreatedAt, past)
	}
	if created.Status != StatusRunning {
		t.Errorf("created.Status = %v, want %v", created.Status, StatusRunning)
	}
	if created.Attempts["agent1"] != 2 {
		t.Errorf("created.Attempts['agent1'] = %d, want 2", created.Attempts["agent1"])
	}
}

func TestLedger_CreateAndSaveErrors(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	if _, err := l.Create(nil); err == nil {
		t.Errorf("Create(nil) expected error, got nil")
	}

	if err := l.Save(nil); err == nil {
		t.Errorf("Save(nil) expected error, got nil")
	}

	if err := l.Save(&Job{}); err == nil {
		t.Errorf("Save without ID expected error, got nil")
	}

	// Root is a regular file: causes os.MkdirAll inside saveLocked to fail
	fileNotDir := filepath.Join(tmpDir, "file-not-dir")
	if err := os.WriteFile(fileNotDir, []byte("plain file"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	lFileRoot := NewLedger(fileNotDir)
	if _, err := lFileRoot.Create(&Job{Ticket: "ERR"}); err == nil {
		t.Errorf("Create with file root expected error, got nil")
	}
	if err := lFileRoot.Save(&Job{ID: "ERR-123"}); err == nil {
		t.Errorf("Save with file root expected error, got nil")
	}

	// Unwritable destination to test temp file creation failure in saveLocked
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0555); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	lBad := NewLedger(readOnlyDir)
	if _, err := lBad.Create(&Job{Ticket: "ERR"}); err == nil {
		t.Errorf("Create in read-only dir expected error, got nil")
	}

	// Make destPath in saveLocked an existing directory to force os.Rename error
	jobForRename, err := l.Create(&Job{Ticket: "RENAME"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	jobPath := l.JobPath(jobForRename.ID)
	if err := os.Remove(jobPath); err != nil {
		t.Fatalf("Remove job.json failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(jobPath, "nested"), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := l.Save(jobForRename); err == nil {
		t.Errorf("Save over non-empty directory expected rename error, got nil")
	}

	// Load non-existent and empty ID
	if _, err := l.Load(""); err == nil {
		t.Errorf("Load(\"\") expected error, got nil")
	}
	if _, err := l.Load("non-existent"); err == nil {
		t.Errorf("Load(non-existent) expected error, got nil")
	}

	// Load corrupted job.json
	corruptDir := filepath.Join(tmpDir, "corrupted-job")
	if err := os.MkdirAll(corruptDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, "job.json"), []byte("{invalid-json"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := l.Load("corrupted-job"); err == nil {
		t.Errorf("Load corrupted job expected error, got nil")
	}
}

func TestLedger_WALWithTornLastLine(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	job, err := l.Create(&Job{Ticket: "DEN-988"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	ev1 := Event{Type: "started", Data: map[string]any{"cmd": "run"}}
	if err := l.Append(job.ID, ev1); err != nil {
		t.Fatalf("Append 1 failed: %v", err)
	}

	ev2 := Event{Time: time.Now().UTC(), Type: "progress", Data: map[string]any{"step": "analyze"}}
	if err := l.Append(job.ID, ev2); err != nil {
		t.Fatalf("Append 2 failed: %v", err)
	}

	events, err := l.Events(job.ID)
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("Events returned %d events, want 2", len(events))
	}
	if events[0].Seq != 1 || events[1].Seq != 2 {
		t.Errorf("Unexpected sequence numbers: %d, %d", events[0].Seq, events[1].Seq)
	}

	// Simulate a crash leaving a torn/partial last line without terminating newline
	tornLine := []byte(`{"seq": 3, "time": "2026-10-17T18:05:00Z", "type": "interrup`)
	eventsPath := l.EventsPath(job.ID)
	f, err := os.OpenFile(eventsPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	if _, err := f.Write(tornLine); err != nil {
		t.Fatalf("Write torn line failed: %v", err)
	}
	_ = f.Close()

	// Reading events must tolerate and skip the torn last line
	eventsTolerated, err := l.Events(job.ID)
	if err != nil {
		t.Fatalf("Events failed to tolerate torn line: %v", err)
	}
	if len(eventsTolerated) != 2 {
		t.Fatalf("Events returned %d events after torn line, want 2", len(eventsTolerated))
	}
	if eventsTolerated[0].Type != "started" || eventsTolerated[1].Type != "progress" {
		t.Errorf("Corrupted event data read: %+v", eventsTolerated)
	}

	// Append a new event after the torn line; it should truncate the torn bytes and assign Seq 3
	ev3 := Event{Type: "resumed", Data: map[string]any{"reason": "retry"}}
	if err := l.Append(job.ID, ev3); err != nil {
		t.Fatalf("Append after torn line failed: %v", err)
	}

	recoveredEvents, err := l.Events(job.ID)
	if err != nil {
		t.Fatalf("Events after recovery failed: %v", err)
	}
	if len(recoveredEvents) != 3 {
		t.Fatalf("Events after recovery returned %d events, want 3", len(recoveredEvents))
	}
	if recoveredEvents[2].Seq != 3 {
		t.Errorf("recoveredEvents[2].Seq = %d, want 3", recoveredEvents[2].Seq)
	}
	if recoveredEvents[2].Type != "resumed" {
		t.Errorf("recoveredEvents[2].Type = %q, want resumed", recoveredEvents[2].Type)
	}
}

func TestLedger_WALErrorsAndCorruptedMiddle(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	if err := l.Append("", Event{}); err == nil {
		t.Errorf("Append with empty ID expected error, got nil")
	}
	if _, err := l.Events(""); err == nil {
		t.Errorf("Events with empty ID expected error, got nil")
	}

	// Non-existent job returns empty events, nil
	events, err := l.Events("no-such-job")
	if err != nil {
		t.Fatalf("Events(no-such-job) returned error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Events(no-such-job) returned %d items, want 0", len(events))
	}

	// Middle-of-file corruption (not the last line) must report an error
	job, err := l.Create(&Job{Ticket: "DEN-CORRUPT"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	eventsPath := l.EventsPath(job.ID)
	corruptContent := strings.Join([]string{
		`{"seq": 1, "time": "2026-10-17T18:00:00Z", "type": "valid1"}`,
		`{corrupt-middle-line`,
		`{"seq": 3, "time": "2026-10-17T18:02:00Z", "type": "valid3"}`,
	}, "\n") + "\n"

	if err := os.WriteFile(eventsPath, []byte(corruptContent), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if _, err := l.Events(job.ID); err == nil {
		t.Errorf("Events with middle corruption expected error, got nil")
	}

	// Append must also return error when events log has middle corruption
	if err := l.Append(job.ID, Event{Type: "next"}); err == nil {
		t.Errorf("Append on corrupted log expected error, got nil")
	}

	// Directory in place of eventsPath causes os.OpenFile and os.ReadFile to fail
	jobDirEvents, err := l.Create(&Job{Ticket: "DEN-DIR-EVT"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := os.Mkdir(l.EventsPath(jobDirEvents.ID), 0755); err != nil {
		t.Fatalf("Mkdir eventsPath failed: %v", err)
	}
	if err := l.Append(jobDirEvents.ID, Event{Type: "fail"}); err == nil {
		t.Errorf("Append with directory eventsPath expected error, got nil")
	}
	if _, err := l.Events(jobDirEvents.ID); err == nil {
		t.Errorf("Events with directory eventsPath expected error, got nil")
	}

	// File in place of root causes os.MkdirAll to fail in Append
	fileNotDir := filepath.Join(tmpDir, "file-not-dir-append")
	if err := os.WriteFile(fileNotDir, []byte("file"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	lBadRoot := NewLedger(fileNotDir)
	if err := lBadRoot.Append("JOB-1", Event{Type: "test"}); err == nil {
		t.Errorf("Append with file root expected error, got nil")
	}
}
