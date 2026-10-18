package agentjobs

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initGitRepo(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-b", branch)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		cmdInit := exec.Command("git", "init")
		cmdInit.Dir = dir
		if err := cmdInit.Run(); err != nil {
			t.Fatalf("git init failed: %v", err)
		}
		cmdCheckout := exec.Command("git", "checkout", "-b", branch)
		cmdCheckout.Dir = dir
		_ = cmdCheckout.Run()
	}
}

func TestJobRef_MarshalJSON(t *testing.T) {
	refZero := JobRef{
		ID:     "J1",
		Status: StatusRunning,
	}
	b, err := json.Marshal(refZero)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if strings.Contains(string(b), "resume_after") {
		t.Errorf("expected resume_after to be omitted when zero, got: %s", string(b))
	}

	resumeTime := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	refWithTime := JobRef{
		ID:          "J2",
		Status:      StatusParked,
		ResumeAfter: resumeTime,
	}
	b2, err := json.Marshal(refWithTime)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(b2), "resume_after") {
		t.Errorf("expected resume_after to be present, got: %s", string(b2))
	}
}

func TestBuildCheckpoint_WithGitAndLedger(t *testing.T) {
	repoDir := t.TempDir()
	initGitRepo(t, repoDir, "feature-den-992")

	ledgerDir := filepath.Join(repoDir, ".dendrite", "jobs")
	ledger := NewLedger(ledgerDir)

	now := time.Now().UTC()

	// 1. InFlight: Running, Queued, Parked
	jRunning, _ := ledger.Create(&Job{Ticket: "DEN-101", Status: StatusRunning, Agent: "agy", Model: "gemini-2.5-pro"})
	jQueued, _ := ledger.Create(&Job{Ticket: "DEN-102", Status: StatusQueued})
	parkedTime := now.Add(2 * time.Hour)
	jParked, _ := ledger.Create(&Job{Ticket: "DEN-103", Status: StatusParked, ResumeAfter: parkedTime})

	// 2. VerifiedUncommitted: Verified
	jVerified, _ := ledger.Create(&Job{Ticket: "DEN-104", Status: StatusVerified})

	// 3. Other status (neither in-flight nor verified): Succeeded, Committed
	_, _ = ledger.Create(&Job{Ticket: "DEN-105", Status: StatusSucceeded})
	_, _ = ledger.Create(&Job{Ticket: "DEN-106", Status: StatusCommitted})

	opts := CheckpointOptions{
		Worktree:         repoDir,
		PendingDecisions: []string{"Decision 1", "  Decision 2  ", "Decision 1", "Decision 3", ""},
		Next:             []string{"DEN-107", "  ", "DEN-108"},
		Notes:            "Testing checkpoint creation",
	}

	cp, err := BuildCheckpoint(ledger, opts)
	if err != nil {
		t.Fatalf("BuildCheckpoint failed: %v", err)
	}

	if cp.Branch != "feature-den-992" {
		t.Errorf("cp.Branch = %q, want feature-den-992", cp.Branch)
	}
	if cp.Worktree != repoDir {
		t.Errorf("cp.Worktree = %q, want %q", cp.Worktree, repoDir)
	}
	if len(cp.InFlight) != 3 {
		t.Fatalf("cp.InFlight has %d items, want 3", len(cp.InFlight))
	}
	if len(cp.VerifiedUncommitted) != 1 {
		t.Fatalf("cp.VerifiedUncommitted has %d items, want 1", len(cp.VerifiedUncommitted))
	}
	if cp.VerifiedUncommitted[0].Ticket != "DEN-104" {
		t.Errorf("Verified ticket = %q, want DEN-104", cp.VerifiedUncommitted[0].Ticket)
	}

	if len(cp.PendingDecisions) != 3 {
		t.Errorf("PendingDecisions has %d items, want 3 (deduplicated)", len(cp.PendingDecisions))
	}
	if len(cp.Next) != 2 {
		t.Errorf("Next has %d items, want 2 (non-empty)", len(cp.Next))
	}
	if cp.Notes != "Testing checkpoint creation" {
		t.Errorf("cp.Notes = %q", cp.Notes)
	}

	_ = jRunning
	_ = jQueued
	_ = jParked
	_ = jVerified
}

func TestBuildCheckpoint_DefaultsAndFallbacks(t *testing.T) {
	nonGitDir := t.TempDir()

	// Empty worktree and nil ledger
	cp, err := BuildCheckpoint(nil, CheckpointOptions{
		Worktree: nonGitDir,
		Notes:    "Fallback note via Note field",
	})
	if err != nil {
		t.Fatalf("BuildCheckpoint failed: %v", err)
	}
	if cp.Branch != "unknown" {
		t.Errorf("cp.Branch in non-git dir = %q, want unknown", cp.Branch)
	}
	if cp.Notes != "Fallback note via Note field" {
		t.Errorf("cp.Notes = %q, want 'Fallback note via Note field'", cp.Notes)
	}
	if len(cp.InFlight) != 0 || len(cp.VerifiedUncommitted) != 0 {
		t.Errorf("expected empty slices for in-flight and verified")
	}

	// Automatic worktree detection when Worktree is empty
	cpAuto, err := BuildCheckpoint(nil, CheckpointOptions{})
	if err != nil {
		t.Fatalf("BuildCheckpoint with auto worktree failed: %v", err)
	}
	if cpAuto.Worktree == "" {
		t.Errorf("cpAuto.Worktree should not be empty")
	}

	// Ledger List error returns error
	fileRoot := filepath.Join(nonGitDir, "file-not-dir")
	if err := os.WriteFile(fileRoot, []byte("plain"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	badLedger := NewLedger(fileRoot)
	if _, err := BuildCheckpoint(badLedger, CheckpointOptions{Worktree: nonGitDir}); err == nil {
		t.Errorf("BuildCheckpoint with bad ledger expected error, got nil")
	}
}

func TestWriteCheckpoint_And_ReadCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	handoffPath := filepath.Join(tmpDir, "session", "handoff.md")

	cp := &Checkpoint{
		Branch:              "main",
		Worktree:            "/workspace/project",
		InFlight:            []JobRef{{ID: "JOB-1", Ticket: "DEN-901", Status: StatusRunning, Agent: "agy"}},
		VerifiedUncommitted: []JobRef{{ID: "JOB-2", Ticket: "DEN-902", Status: StatusVerified}},
		PendingDecisions:    []string{"Pick local model"},
		Next:                []string{"DEN-903"},
		Notes:               "Session handoff ready",
		CreatedAt:           time.Date(2026, 10, 17, 18, 30, 0, 0, time.UTC),
	}

	// Nil checkpoint error
	if err := WriteCheckpoint(handoffPath, nil); err == nil {
		t.Errorf("WriteCheckpoint(nil) expected error, got nil")
	}

	// Write successfully
	if err := WriteCheckpoint(handoffPath, cp); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}

	content, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(content), "# Session Handoff") {
		t.Errorf("missing markdown header in: %s", string(content))
	}
	if !strings.Contains(string(content), "```json") {
		t.Errorf("missing json fence in: %s", string(content))
	}

	// Read checkpoint back
	loaded, err := ReadCheckpoint(handoffPath)
	if err != nil {
		t.Fatalf("ReadCheckpoint failed: %v", err)
	}
	if loaded.Branch != "main" {
		t.Errorf("loaded.Branch = %q, want main", loaded.Branch)
	}
	if loaded.Worktree != "/workspace/project" {
		t.Errorf("loaded.Worktree = %q, want /workspace/project", loaded.Worktree)
	}
	if len(loaded.InFlight) != 1 || loaded.InFlight[0].Ticket != "DEN-901" {
		t.Errorf("unexpected loaded InFlight: %+v", loaded.InFlight)
	}
	if len(loaded.VerifiedUncommitted) != 1 || loaded.VerifiedUncommitted[0].Ticket != "DEN-902" {
		t.Errorf("unexpected loaded VerifiedUncommitted: %+v", loaded.VerifiedUncommitted)
	}
	if loaded.Notes != "Session handoff ready" {
		t.Errorf("loaded.Notes = %q, want 'Session handoff ready'", loaded.Notes)
	}

	// Default handoff path resolution
	t.Setenv("DENDRITE_STATE_DIR", tmpDir)
	if err := WriteCheckpoint("", cp); err != nil {
		t.Fatalf("WriteCheckpoint with empty path failed: %v", err)
	}
	defaultLoaded, err := ReadCheckpoint("")
	if err != nil {
		t.Fatalf("ReadCheckpoint with empty path failed: %v", err)
	}
	if defaultLoaded.Branch != "main" {
		t.Errorf("defaultLoaded.Branch = %q, want main", defaultLoaded.Branch)
	}
}

func TestWriteCheckpoint_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	cp := &Checkpoint{Branch: "main"}

	// Directory creation failure: file in place of parent directory
	filePath := filepath.Join(tmpDir, "file-not-dir")
	if err := os.WriteFile(filePath, []byte("file"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	badTarget := filepath.Join(filePath, "sub", "handoff.md")
	if err := WriteCheckpoint(badTarget, cp); err == nil {
		t.Errorf("WriteCheckpoint with bad directory expected error, got nil")
	}

	// Temp file creation failure: read-only directory
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0555); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	readOnlyTarget := filepath.Join(readOnlyDir, "handoff.md")
	if err := WriteCheckpoint(readOnlyTarget, cp); err == nil {
		t.Errorf("WriteCheckpoint in read-only dir expected error, got nil")
	}

	// Rename failure: target path is an existing non-empty directory
	dirTarget := filepath.Join(tmpDir, "target-dir")
	if err := os.MkdirAll(filepath.Join(dirTarget, "nested"), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := WriteCheckpoint(dirTarget, cp); err == nil {
		t.Errorf("WriteCheckpoint over non-empty directory expected error, got nil")
	}
}

func TestReadCheckpoint_Variations(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Non-existent file
	if _, err := ReadCheckpoint(filepath.Join(tmpDir, "does-not-exist.md")); err == nil {
		t.Errorf("ReadCheckpoint non-existent file expected error, got nil")
	}

	// 2. Raw JSON file (no markdown code fence)
	rawJSONPath := filepath.Join(tmpDir, "raw.json")
	cpRaw := Checkpoint{Branch: "branch-raw", Worktree: "/raw"}
	rawBytes, _ := json.Marshal(cpRaw)
	if err := os.WriteFile(rawJSONPath, rawBytes, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	loadedRaw, err := ReadCheckpoint(rawJSONPath)
	if err != nil {
		t.Fatalf("ReadCheckpoint raw JSON failed: %v", err)
	}
	if loadedRaw.Branch != "branch-raw" {
		t.Errorf("loadedRaw.Branch = %q, want branch-raw", loadedRaw.Branch)
	}

	// 3. Corrupted JSON inside markdown fence
	corruptFencePath := filepath.Join(tmpDir, "corrupt-fence.md")
	corruptContent := "# Title\n\n```json\n{bad-json\n```\n"
	if err := os.WriteFile(corruptFencePath, []byte(corruptContent), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := ReadCheckpoint(corruptFencePath); err == nil {
		t.Errorf("ReadCheckpoint corrupted fence expected error, got nil")
	}
}

func TestRenderCheckpoint_TruncationAndFormatting(t *testing.T) {
	// Nil cp
	if RenderCheckpoint(nil) != "" {
		t.Errorf("RenderCheckpoint(nil) should return empty string")
	}

	// Empty lists and empty notes
	cpEmpty := &Checkpoint{
		Branch:    "dev",
		Worktree:  "/app",
		CreatedAt: time.Now().UTC(),
	}
	renderedEmpty := RenderCheckpoint(cpEmpty)
	if !strings.Contains(renderedEmpty, "- (none)") {
		t.Errorf("expected - (none) for empty lists, got: %s", renderedEmpty)
	}
	if !strings.Contains(renderedEmpty, "(none)\n") {
		t.Errorf("expected (none) for empty notes, got: %s", renderedEmpty)
	}

	// Long lists (> 5 items) to verify "… and N more"
	var inflight []JobRef
	for i := 1; i <= 8; i++ {
		inflight = append(inflight, JobRef{
			ID:     filepath.Join("JOB", string(rune('0'+i))),
			Ticket: filepath.Join("DEN", string(rune('0'+i))),
			Status: StatusRunning,
		})
	}

	var verified []JobRef
	for i := 1; i <= 7; i++ {
		verified = append(verified, JobRef{
			ID:     filepath.Join("VJOB", string(rune('0'+i))),
			Status: StatusVerified, // no ticket, will format ID
		})
	}

	var decisions []string
	for i := 1; i <= 6; i++ {
		decisions = append(decisions, "Decision "+string(rune('0'+i)))
	}

	var next []string
	for i := 1; i <= 9; i++ {
		next = append(next, "Next "+string(rune('0'+i)))
	}

	// Long notes > 300 chars
	longNotes := strings.Repeat("A long note paragraph explaining state. ", 15)

	// Parked job with resume_after and agent
	parkedTime := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	inflight[0].Status = StatusParked
	inflight[0].ResumeAfter = parkedTime
	inflight[0].Agent = "agy"

	cpLong := &Checkpoint{
		Branch:              "main",
		Worktree:            "/wt",
		InFlight:            inflight,
		VerifiedUncommitted: verified,
		PendingDecisions:    decisions,
		Next:                next,
		Notes:               longNotes,
		CreatedAt:           time.Now().UTC(),
	}

	renderedLong := RenderCheckpoint(cpLong)

	if !strings.Contains(renderedLong, "… and 3 more") {
		t.Errorf("expected '… and 3 more' for 8 inflight items, got:\n%s", renderedLong)
	}
	if !strings.Contains(renderedLong, "… and 2 more") {
		t.Errorf("expected '… and 2 more' for 7 verified items, got:\n%s", renderedLong)
	}
	if !strings.Contains(renderedLong, "… and 1 more") {
		t.Errorf("expected '… and 1 more' for 6 decisions, got:\n%s", renderedLong)
	}
	if !strings.Contains(renderedLong, "… and 4 more") {
		t.Errorf("expected '… and 4 more' for 9 next items, got:\n%s", renderedLong)
	}
	if !strings.Contains(renderedLong, "parked until 18:00, agy") {
		t.Errorf("expected parked detail formatting, got:\n%s", renderedLong)
	}
	if !strings.Contains(renderedLong, "…\n") {
		t.Errorf("expected truncated notes with ellipsis")
	}
}

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
