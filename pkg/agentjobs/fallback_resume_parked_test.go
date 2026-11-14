package agentjobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResumeParked(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")

	d := NewDispatcher(tempLedger)
	d.Executable = dummyRunner
	fakeNow := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fakeNow }

	// Mark provider "agy" as cooling down until +30 min
	if err := d.markProviderExhausted(providerAGY, fakeNow.Add(30*time.Minute)); err != nil {
		t.Fatalf("failed to mark provider exhausted: %v", err)
	}

	// Job 1: Parked, resume_after passed, provider "copilot" (aider) is NOT cooling -> should resume
	j1 := &Job{
		Ticket:      "DEN-1",
		Agent:       "aider",
		Model:       copilotModel,
		Status:      StatusParked,
		ResumeAfter: fakeNow.Add(-10 * time.Minute),
	}
	created1, err := tempLedger.Create(j1)
	if err != nil {
		t.Fatalf("failed to create j1: %v", err)
	}
	created1.Status = StatusParked
	created1.ResumeAfter = fakeNow.Add(-10 * time.Minute)
	_ = tempLedger.Save(created1)

	// Job 2: Parked, resume_after passed, but provider "agy" IS still cooling -> should NOT resume
	j2 := &Job{
		Ticket:      "DEN-2",
		Agent:       "agy",
		Model:       "gemini-3.8-flash-high",
		Status:      StatusParked,
		ResumeAfter: fakeNow.Add(-10 * time.Minute),
	}
	created2, err := tempLedger.Create(j2)
	if err != nil {
		t.Fatalf("failed to create j2: %v", err)
	}
	created2.Status = StatusParked
	created2.ResumeAfter = fakeNow.Add(-10 * time.Minute)
	_ = tempLedger.Save(created2)

	// Job 3: Parked, but resume_after in future -> should NOT resume
	j3 := &Job{
		Ticket:      "DEN-3",
		Agent:       "aider",
		Model:       copilotModel,
		Status:      StatusParked,
		ResumeAfter: fakeNow.Add(15 * time.Minute),
	}
	created3, err := tempLedger.Create(j3)
	if err != nil {
		t.Fatalf("failed to create j3: %v", err)
	}
	created3.Status = StatusParked
	created3.ResumeAfter = fakeNow.Add(15 * time.Minute)
	_ = tempLedger.Save(created3)

	// Job 4: Queued (not parked) -> should NOT resume
	j4 := &Job{
		Ticket: "DEN-4",
		Agent:  "aider",
		Model:  copilotModel,
		Status: StatusQueued,
	}
	created4, err := tempLedger.Create(j4)
	if err != nil {
		t.Fatalf("failed to create j4: %v", err)
	}

	resumed, err := d.ResumeParked()
	if err != nil {
		t.Fatalf("ResumeParked failed: %v", err)
	}

	if len(resumed) != 1 || resumed[0] != created1.ID {
		t.Fatalf("expected resumed=[%s], got %v", created1.ID, resumed)
	}

	loaded1, _ := tempLedger.Load(created1.ID)
	if loaded1.Status != StatusQueued {
		t.Errorf("j1 status = %q, want queued", loaded1.Status)
	}
	if loaded1.PGID <= 0 {
		t.Errorf("j1 PGID = %d, want > 0", loaded1.PGID)
	}

	loaded2, _ := tempLedger.Load(created2.ID)
	if loaded2.Status != StatusParked {
		t.Errorf("j2 status = %q, want parked", loaded2.Status)
	}

	loaded3, _ := tempLedger.Load(created3.ID)
	if loaded3.Status != StatusParked {
		t.Errorf("j3 status = %q, want parked", loaded3.Status)
	}

	loaded4, _ := tempLedger.Load(created4.ID)
	if loaded4.Status != StatusQueued {
		t.Errorf("j4 status = %q, want queued", loaded4.Status)
	}

	// Error branch: Ledger.List() fails
	// A missing root lists as empty by design; a root that is a regular file cannot be read.
	fileRoot := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileRoot, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	badLedger := NewLedger(fileRoot)
	dBad := NewDispatcher(badLedger)
	_, listErr := dBad.ResumeParked()
	if listErr == nil {
		t.Error("expected error when Ledger.List() fails")
	}

	// Error branch: d.Retry() fails
	// Needs a still-parked, eligible job: the earlier part of this test already resumed j4.
	retryLedger := NewLedger(t.TempDir())
	parked, err := retryLedger.Create(&Job{Ticket: "DEN-990", Agent: "agy", Status: StatusParked, ResumeAfter: fakeNow.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	parked.Status = StatusParked
	parked.ResumeAfter = fakeNow.Add(-time.Minute)
	if err := retryLedger.Save(parked); err != nil {
		t.Fatal(err)
	}
	dRetryFail := NewDispatcher(retryLedger)
	dRetryFail.Executable = "/nonexistent/path/cannot/run"
	dRetryFail.Now = func() time.Time { return fakeNow }
	_, retryErr := dRetryFail.ResumeParked()
	if retryErr == nil {
		t.Error("expected error when d.Retry fails")
	}
}
