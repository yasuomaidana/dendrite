package agentjobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResume_And_ResumeState(t *testing.T) {
	tmpDir := t.TempDir()
	handoffPath := filepath.Join(tmpDir, "handoff.md")
	ledgerDir := filepath.Join(tmpDir, "jobs")
	ledger := NewLedger(ledgerDir)

	createdAt := time.Now().UTC().Add(-10 * time.Minute)
	cp := &Checkpoint{
		Branch:    "develop",
		Worktree:  "/work",
		CreatedAt: createdAt,
	}
	if err := WriteCheckpoint(handoffPath, cp); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}

	// 1. Initially no jobs updated after createdAt
	reportInitial, err := ResumeState(handoffPath, ledger)
	if err != nil {
		t.Fatalf("ResumeState initial failed: %v", err)
	}
	if reportInitial.Delta != "since checkpoint: no status changes" {
		t.Errorf("unexpected delta: %q", reportInitial.Delta)
	}

	textInitial, err := Resume(handoffPath, ledger)
	if err != nil {
		t.Fatalf("Resume initial failed: %v", err)
	}
	if !strings.Contains(textInitial, "since checkpoint: no status changes") {
		t.Errorf("expected text to contain no status changes, got: %s", textInitial)
	}

	// 2. Add updated jobs matching the spec example:
	// "since checkpoint: 3 verified, 1 parked until 18:00, 1 failed"
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		j, _ := ledger.Create(&Job{Ticket: "DEN-V", Status: StatusVerified})
		j.UpdatedAt = now
		_ = ledger.Save(j)
	}

	parkedTime := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	jParked, _ := ledger.Create(&Job{Ticket: "DEN-P", Status: StatusParked, ResumeAfter: parkedTime})
	jParked.UpdatedAt = now
	_ = ledger.Save(jParked)

	jFailed, _ := ledger.Create(&Job{Ticket: "DEN-F", Status: StatusFailed})
	jFailed.UpdatedAt = now
	_ = ledger.Save(jFailed)

	report, err := ResumeState(handoffPath, ledger)
	if err != nil {
		t.Fatalf("ResumeState failed: %v", err)
	}

	expectedDelta := "since checkpoint: 3 verified, 1 parked until 18:00, 1 failed"
	if report.Delta != expectedDelta {
		t.Errorf("report.Delta = %q, want %q", report.Delta, expectedDelta)
	}

	resText, err := Resume(handoffPath, ledger)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if !strings.Contains(resText, expectedDelta) {
		t.Errorf("Resume output missing delta. Got:\n%s", resText)
	}
	if !strings.Contains(resText, "# Session Handoff") {
		t.Errorf("Resume output missing handoff text. Got:\n%s", resText)
	}
}

func TestResume_MultipleParkedAndOtherStatuses(t *testing.T) {
	tmpDir := t.TempDir()
	handoffPath := filepath.Join(tmpDir, "handoff.md")
	ledgerDir := filepath.Join(tmpDir, "jobs")
	ledger := NewLedger(ledgerDir)

	createdAt := time.Now().UTC().Add(-1 * time.Hour)
	cp := &Checkpoint{
		Branch:    "main",
		CreatedAt: createdAt,
	}
	_ = WriteCheckpoint(handoffPath, cp)

	now := time.Now().UTC()
	// Add 2 parked jobs (exercises len(parkedTimes) != 1 branch: "2 parked")
	jP1, _ := ledger.Create(&Job{Status: StatusParked})
	jP1.UpdatedAt = now
	_ = ledger.Save(jP1)

	jP2, _ := ledger.Create(&Job{Status: StatusParked})
	jP2.UpdatedAt = now
	_ = ledger.Save(jP2)

	// Add a job with a custom status not in preferredOrder to exercise remaining sorted branch
	jCustom, _ := ledger.Create(&Job{Status: Status("custom_status")})
	jCustom.UpdatedAt = now
	_ = ledger.Save(jCustom)

	jRunning, _ := ledger.Create(&Job{Status: StatusRunning})
	jRunning.UpdatedAt = now
	_ = ledger.Save(jRunning)

	report, err := ResumeState(handoffPath, ledger)
	if err != nil {
		t.Fatalf("ResumeState failed: %v", err)
	}

	if !strings.Contains(report.Delta, "2 parked") {
		t.Errorf("expected '2 parked' in delta, got: %s", report.Delta)
	}
	if !strings.Contains(report.Delta, "1 custom_status") {
		t.Errorf("expected '1 custom_status' in delta, got: %s", report.Delta)
	}
	if !strings.Contains(report.Delta, "1 running") {
		t.Errorf("expected '1 running' in delta, got: %s", report.Delta)
	}
}

func TestResume_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	// Missing file
	if _, err := Resume(filepath.Join(tmpDir, "non-existent.md"), nil); err == nil {
		t.Errorf("Resume with non-existent file expected error, got nil")
	}

	// Corrupted file
	corruptPath := filepath.Join(tmpDir, "corrupted.md")
	_ = os.WriteFile(corruptPath, []byte("```json\n{bad\n```"), 0644)
	if _, err := Resume(corruptPath, nil); err == nil {
		t.Errorf("Resume with corrupted file expected error, got nil")
	}

	// Ledger List error
	validPath := filepath.Join(tmpDir, "valid.md")
	cp := &Checkpoint{Branch: "main", CreatedAt: time.Now().UTC()}
	_ = WriteCheckpoint(validPath, cp)

	fileRoot := filepath.Join(tmpDir, "file-not-dir")
	_ = os.WriteFile(fileRoot, []byte("plain"), 0644)
	badLedger := NewLedger(fileRoot)

	if _, err := Resume(validPath, badLedger); err == nil {
		t.Errorf("Resume with broken ledger expected error, got nil")
	}
}

func TestGitHelpers_RealRepo(t *testing.T) {
	repoDir := t.TempDir()
	initGitRepo(t, repoDir, "my-test-branch")

	branch := gitBranchIn(repoDir)
	if branch != "my-test-branch" {
		t.Errorf("gitBranchIn = %q, want my-test-branch", branch)
	}

	top, err := gitTopLevelIn(repoDir)
	if err != nil {
		t.Fatalf("gitTopLevelIn failed: %v", err)
	}
	// On macOS /var vs /private/var symlink normalization
	if filepath.Clean(top) != filepath.Clean(repoDir) && !strings.HasSuffix(filepath.Clean(repoDir), filepath.Clean(top)) {
		t.Logf("top = %q, repoDir = %q", top, repoDir)
	}

	// Non-git dir
	emptyDir := t.TempDir()
	if b := gitBranchIn(emptyDir); b != "unknown" {
		t.Errorf("gitBranchIn on non-git dir = %q, want unknown", b)
	}
	if _, err := gitTopLevelIn(emptyDir); err == nil {
		t.Errorf("gitTopLevelIn on non-git dir expected error, got nil")
	}
}
