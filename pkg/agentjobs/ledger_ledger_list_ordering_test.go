package agentjobs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestLedger_ListOrdering(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	// Empty list on fresh root
	emptyList, err := l.List()
	if err != nil {
		t.Fatalf("List on empty root failed: %v", err)
	}
	if len(emptyList) != 0 {
		t.Errorf("List returned %d items, want 0", len(emptyList))
	}

	now := time.Now().UTC()

	// Create 3 jobs out of order
	job2, err := l.Create(&Job{Ticket: "DEN-002", CreatedAt: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("Create job2 failed: %v", err)
	}
	job1, err := l.Create(&Job{Ticket: "DEN-001", CreatedAt: now.Add(1 * time.Minute)})
	if err != nil {
		t.Fatalf("Create job1 failed: %v", err)
	}
	job3, err := l.Create(&Job{Ticket: "DEN-003", CreatedAt: now.Add(3 * time.Minute)})
	if err != nil {
		t.Fatalf("Create job3 failed: %v", err)
	}

	// Also add two jobs with identical CreatedAt to exercise the ID tie-break branch
	sameTime := now.Add(4 * time.Minute)
	job4, err := l.Create(&Job{Ticket: "DEN-004", CreatedAt: sameTime})
	if err != nil {
		t.Fatalf("Create job4 failed: %v", err)
	}
	job5, err := l.Create(&Job{Ticket: "DEN-005", CreatedAt: sameTime})
	if err != nil {
		t.Fatalf("Create job5 failed: %v", err)
	}

	// Add an ignored regular file in root
	if err := os.WriteFile(filepath.Join(tmpDir, "ignored.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Add an ignored directory without job.json
	if err := os.Mkdir(filepath.Join(tmpDir, "empty-dir"), 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	// Add an ignored directory with invalid job.json
	invalidDir := filepath.Join(tmpDir, "invalid-dir")
	if err := os.Mkdir(invalidDir, 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "job.json"), []byte("{bad-json"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	jobs, err := l.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(jobs) != 5 {
		t.Fatalf("List returned %d jobs, want 5", len(jobs))
	}

	if jobs[0].ID != job1.ID {
		t.Errorf("jobs[0].ID = %q, want %q", jobs[0].ID, job1.ID)
	}
	if jobs[1].ID != job2.ID {
		t.Errorf("jobs[1].ID = %q, want %q", jobs[1].ID, job2.ID)
	}
	if jobs[2].ID != job3.ID {
		t.Errorf("jobs[2].ID = %q, want %q", jobs[2].ID, job3.ID)
	}

	// Tie-break check for identical timestamps
	if jobs[3].CreatedAt.Equal(jobs[4].CreatedAt) {
		if jobs[3].ID > jobs[4].ID {
			t.Errorf("Tie break failed: %q should be before %q", jobs[3].ID, jobs[4].ID)
		}
	} else {
		t.Errorf("Expected jobs[3] and jobs[4] to have identical CreatedAt")
	}
	_ = job4
	_ = job5

	// Non-existent root directory returns empty list, nil
	lNonExistent := NewLedger(filepath.Join(tmpDir, "does-not-exist"))
	listNonExistent, err := lNonExistent.List()
	if err != nil {
		t.Fatalf("List on non-existent root failed: %v", err)
	}
	if len(listNonExistent) != 0 {
		t.Errorf("List on non-existent returned %d jobs, want 0", len(listNonExistent))
	}

	// Regular file as root directory returns read error (not IsNotExist)
	fileRoot := filepath.Join(tmpDir, "root-file")
	if err := os.WriteFile(fileRoot, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	lFileRoot := NewLedger(fileRoot)
	if _, err := lFileRoot.List(); err == nil {
		t.Errorf("List with file as root expected error, got nil")
	}
}

func TestLedger_MarkDone(t *testing.T) {
	tmpDir := t.TempDir()
	l := NewLedger(tmpDir)

	if err := l.MarkDone(""); err == nil {
		t.Errorf("MarkDone with empty ID expected error, got nil")
	}

	if err := l.MarkDone("non-existent-job"); err == nil {
		t.Errorf("MarkDone for non-existent job expected error, got nil")
	}

	job, err := l.Create(&Job{Ticket: "DEN-988"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	donePath := l.DoneMarkerPath(job.ID)
	if _, err := os.Stat(donePath); !os.IsNotExist(err) {
		t.Fatalf("Done marker file must not exist before MarkDone")
	}

	beforeMark := time.Now().UTC().Add(-1 * time.Second)
	if err := l.MarkDone(job.ID); err != nil {
		t.Fatalf("MarkDone failed: %v", err)
	}

	info, err := os.Stat(donePath)
	if err != nil {
		t.Fatalf("os.Stat(done) failed: %v", err)
	}
	if info.ModTime().Before(beforeMark) {
		t.Errorf("done marker mtime %v is older than beforeMark %v", info.ModTime(), beforeMark)
	}

	content, err := os.ReadFile(donePath)
	if err != nil {
		t.Fatalf("ReadFile(done) failed: %v", err)
	}
	parsedTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(content)))
	if err != nil {
		t.Fatalf("Failed to parse done marker timestamp %q: %v", string(content), err)
	}
	if parsedTime.Before(beforeMark) {
		t.Errorf("Parsed done marker time %v is older than beforeMark %v", parsedTime, beforeMark)
	}

	// Update mark done again
	if err := l.MarkDone(job.ID); err != nil {
		t.Fatalf("Second MarkDone failed: %v", err)
	}

	// Force os.WriteFile error by creating a directory where the done marker should be
	jobDoneDir, err := l.Create(&Job{Ticket: "DONE-ERR"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := os.Mkdir(l.DoneMarkerPath(jobDoneDir.ID), 0755); err != nil {
		t.Fatalf("Mkdir doneMarker failed: %v", err)
	}
	if err := l.MarkDone(jobDoneDir.ID); err == nil {
		t.Errorf("MarkDone over directory expected error, got nil")
	}
}

func TestRandomSuffix(t *testing.T) {
	s1 := randomSuffix(3)
	s2 := randomSuffix(3)
	if len(s1) != 6 {
		t.Errorf("len(s1) = %d, want 6", len(s1))
	}
	if s1 == s2 {
		t.Errorf("randomSuffix returned identical values %q, %q", s1, s2)
	}

	// Test fallback when entropy reader errors
	orig := randReader
	t.Cleanup(func() { randReader = orig })
	randReader = iotest.ErrReader(errors.New("simulated entropy failure"))
	fallback := randomSuffix(2)
	if len(fallback) == 0 {
		t.Errorf("randomSuffix fallback returned empty string")
	}
}

func TestJobJSONSerialization(t *testing.T) {
	j := Job{
		ID:          "DEN-1",
		Status:      StatusQueued,
		ResumeAfter: time.Time{},
	}
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	// Zero time ResumeAfter should not appear in serialized JSON
	if strings.Contains(string(b), "resume_after") {
		t.Errorf("resume_after should be omitted when zero, got %s", string(b))
	}

	// Non-zero ResumeAfter should appear
	now := time.Now().UTC()
	j2 := Job{
		ID:          "DEN-2",
		Status:      StatusParked,
		ResumeAfter: now,
	}
	b2, err := json.Marshal(j2)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !strings.Contains(string(b2), "resume_after") {
		t.Errorf("resume_after should be present when non-zero, got %s", string(b2))
	}
}

func TestStatusAndClassConstants(t *testing.T) {
	statuses := []struct {
		val  Status
		want string
	}{
		{StatusQueued, "queued"},
		{StatusRunning, "running"},
		{StatusSucceeded, "succeeded"},
		{StatusFailedRetryable, "failed_retryable"},
		{StatusQuotaExhausted, "quota_exhausted"},
		{StatusEarlyExit, "early_exit"},
		{StatusNeedsReview, "needs_review"},
		{StatusVerified, "verified"},
		{StatusFailed, "failed"},
		{StatusParked, "parked"},
		{StatusEscalated, "escalated"},
		{StatusCommitted, "committed"},
	}

	for _, s := range statuses {
		if string(s.val) != s.want {
			t.Errorf("Status %v != %q", s.val, s.want)
		}
	}

	classes := []struct {
		val  Class
		want string
	}{
		{QUOTA, "QUOTA"},
		{ClassQuota, "QUOTA"},
		{TRANSIENT, "TRANSIENT"},
		{ClassTransient, "TRANSIENT"},
		{PERMISSION, "PERMISSION"},
		{ClassPermission, "PERMISSION"},
		{EARLY_EXIT, "EARLY_EXIT"},
		{ClassEarlyExit, "EARLY_EXIT"},
		{DONE, "DONE"},
		{ClassDone, "DONE"},
	}

	for _, c := range classes {
		if string(c.val) != c.want {
			t.Errorf("Class %v != %q", c.val, c.want)
		}
	}
}

func TestNewLedger_DefaultRootResolvedLazily(t *testing.T) {
	l := NewLedger("")
	dir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", dir)
	if got, want := l.Root(), filepath.Join(dir, "jobs"); got != want {
		t.Fatalf("Root() = %q; want %q (state dir set after construction)", got, want)
	}
}
