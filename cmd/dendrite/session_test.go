package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/agentjobs"
)

func initTestGitRepo(t *testing.T, dir, branch string) {
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

func TestSessionCommand_DispatchAndHelp(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer

	// No arguments -> usage
	buf.Reset()
	if err := runSessionCommand(ctx, []string{}, &buf); err != nil {
		t.Fatalf("expected nil on empty args, got: %v", err)
	}
	if !strings.Contains(buf.String(), "dendrite session [command]") {
		t.Errorf("expected usage output, got: %s", buf.String())
	}

	// Help flags
	helpFlags := [][]string{{"--help"}, {"-h"}}
	for _, h := range helpFlags {
		buf.Reset()
		if err := runSessionCommand(ctx, h, &buf); err != nil {
			t.Fatalf("expected nil on %v, got: %v", h, err)
		}
		if !strings.Contains(buf.String(), "dendrite session [command]") {
			t.Errorf("expected usage for %v", h)
		}
	}

	// Unknown subcommand
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"bogus"}, &buf); err == nil {
		t.Errorf("expected error on unknown command, got nil")
	}

}

func TestSessionCommand_CheckpointAndResume_TextAndJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", tmpDir)

	worktreeDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	initTestGitRepo(t, worktreeDir, "feature-session-test")

	// Set up jobs ledger inside the state directory
	jobsRoot := filepath.Join(tmpDir, "jobs")
	ledger := agentjobs.NewLedger(jobsRoot)

	// Create a running job and a verified job
	_, _ = ledger.Create(&agentjobs.Job{Ticket: "DEN-992", Status: agentjobs.StatusRunning})
	_, _ = ledger.Create(&agentjobs.Job{Ticket: "DEN-991", Status: agentjobs.StatusVerified})

	ctx := context.Background()
	var buf bytes.Buffer

	// 1. Run Checkpoint with flags (text mode)
	checkpointArgs := []string{
		"checkpoint",
		"--worktree", worktreeDir,
		"--decision", "Use pure Go handoff capsule",
		"--decision", "Support multiple decisions",
		"--next", "DEN-993",
		"--next", "DEN-994",
		"--note", "Orchestrator ready to sleep",
	}

	buf.Reset()
	if err := runSessionCommand(ctx, checkpointArgs, &buf); err != nil {
		t.Fatalf("checkpoint command failed: %v", err)
	}
	outStr := buf.String()
	if !strings.Contains(outStr, "[✓] Session checkpoint written to") {
		t.Errorf("checkpoint missing success banner: %s", outStr)
	}
	if !strings.Contains(outStr, "feature-session-test") {
		t.Errorf("checkpoint missing branch name: %s", outStr)
	}
	if !strings.Contains(outStr, "In-Flight Jobs:       1") {
		t.Errorf("checkpoint missing in-flight count: %s", outStr)
	}
	if !strings.Contains(outStr, "Verified Uncommitted: 1") {
		t.Errorf("checkpoint missing verified count: %s", outStr)
	}
	if !strings.Contains(outStr, "Pending Decisions:    2") {
		t.Errorf("checkpoint missing pending decisions count: %s", outStr)
	}
	if !strings.Contains(outStr, "Next:                 DEN-993, DEN-994") {
		t.Errorf("checkpoint missing next list: %s", outStr)
	}

	// 2. Run Resume (text mode) immediately: no status changes yet
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume"}, &buf); err != nil {
		t.Fatalf("resume command failed: %v", err)
	}
	resumeStr := buf.String()
	if !strings.Contains(resumeStr, "# Session Handoff") {
		t.Errorf("resume missing handoff markdown: %s", resumeStr)
	}
	if !strings.Contains(resumeStr, "since checkpoint: no status changes") {
		t.Errorf("resume missing delta line: %s", resumeStr)
	}

	// 3. Mutate ledger: 1 job verified, 1 parked
	now := time.Now().UTC()
	parkedTime := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	jParked, _ := ledger.Create(&agentjobs.Job{Ticket: "DEN-995", Status: agentjobs.StatusParked, ResumeAfter: parkedTime})
	jParked.UpdatedAt = now
	_ = ledger.Save(jParked)

	jVerified2, _ := ledger.Create(&agentjobs.Job{Ticket: "DEN-996", Status: agentjobs.StatusVerified})
	jVerified2.UpdatedAt = now
	_ = ledger.Save(jVerified2)

	// 4. Run Resume again (text mode) -> shows delta
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume"}, &buf); err != nil {
		t.Fatalf("resume with delta failed: %v", err)
	}
	resumeWithDelta := buf.String()
	if !strings.Contains(resumeWithDelta, "since checkpoint: 1 verified, 1 parked until 18:00") {
		t.Errorf("resume missing expected delta, got: %s", resumeWithDelta)
	}

	// 5. Run Resume in JSON mode (--json)
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume", "--json"}, &buf); err != nil {
		t.Fatalf("resume --json failed: %v", err)
	}
	var rep agentjobs.ResumeReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("failed to unmarshal resume --json output: %v\nOutput: %s", err, buf.String())
	}
	if rep.Checkpoint.Branch != "feature-session-test" {
		t.Errorf("rep.Checkpoint.Branch = %q, want feature-session-test", rep.Checkpoint.Branch)
	}
	if !strings.Contains(rep.Delta, "1 verified") {
		t.Errorf("rep.Delta = %q, expected '1 verified'", rep.Delta)
	}

	// 6. Run Checkpoint with --json and --notes
	buf.Reset()
	cpJSONArgs := []string{
		"checkpoint",
		"--worktree", worktreeDir,
		"--note", "Updated notes via --note flag",
		"--json",
	}
	if err := runSessionCommand(ctx, cpJSONArgs, &buf); err != nil {
		t.Fatalf("checkpoint --json failed: %v", err)
	}
	var cp agentjobs.Checkpoint
	if err := json.Unmarshal(buf.Bytes(), &cp); err != nil {
		t.Fatalf("failed to unmarshal checkpoint --json output: %v\nOutput: %s", err, buf.String())
	}
	if cp.Notes != "Updated notes via --note flag" {
		t.Errorf("cp.Notes = %q, want 'Updated notes via --note flag'", cp.Notes)
	}

	// 6b. Repeated --note flags are all kept, as a bullet list
	buf.Reset()
	multiNoteArgs := []string{
		"checkpoint",
		"--worktree", worktreeDir,
		"--path", filepath.Join(tmpDir, "multi-note-handoff.md"),
		"--note", "first note",
		"--note", "second note",
		"--json",
	}
	if err := runSessionCommand(ctx, multiNoteArgs, &buf); err != nil {
		t.Fatalf("checkpoint with repeated --note failed: %v", err)
	}
	var multiCP agentjobs.Checkpoint
	if err := json.Unmarshal(buf.Bytes(), &multiCP); err != nil {
		t.Fatalf("failed to unmarshal checkpoint output: %v\nOutput: %s", err, buf.String())
	}
	if multiCP.Notes != "- first note\n- second note" {
		t.Errorf("multiCP.Notes = %q, want both notes as a bullet list", multiCP.Notes)
	}

	// 7. Test custom --path / --file for resume
	customFile := filepath.Join(tmpDir, "custom-handoff.md")
	_ = agentjobs.WriteCheckpoint(customFile, &cp)
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume", "--path", customFile}, &buf); err != nil {
		t.Fatalf("resume --file failed: %v", err)
	}
	if !strings.Contains(buf.String(), "# Session Handoff") {
		t.Errorf("resume --file output missing handoff: %s", buf.String())
	}
}

func TestSessionCommand_Errors(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer

	// Flag parse error in checkpoint
	if err := runSessionCommand(ctx, []string{"checkpoint", "--nonexistent-flag"}, &buf); err == nil {
		t.Errorf("expected flag error on checkpoint, got nil")
	}

	// Flag parse error in resume
	if err := runSessionCommand(ctx, []string{"resume", "--nonexistent-flag"}, &buf); err == nil {
		t.Errorf("expected flag error on resume, got nil")
	}

	// Resume without existing checkpoint
	tmpDir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", tmpDir)
	if err := runSessionCommand(ctx, []string{"resume"}, &buf); err == nil {
		t.Errorf("expected error on resume without checkpoint, got nil")
	}
	if err := runSessionCommand(ctx, []string{"resume", "--json"}, &buf); err == nil {
		t.Errorf("expected error on resume --json without checkpoint, got nil")
	}

	// Checkpoint failure: target path is a non-empty directory
	badPath := filepath.Join(tmpDir, "bad-dir")
	if err := os.MkdirAll(filepath.Join(badPath, "child"), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := runSessionCommand(ctx, []string{"checkpoint", "--path", badPath}, &buf); err == nil {
		t.Errorf("expected write error on checkpoint, got nil")
	}
}
