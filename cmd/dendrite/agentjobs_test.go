package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/agentjobs"
)

func installTestExecutableScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	content := "#!/bin/sh\n" + body
	if err := os.WriteFile(p, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write script %s: %v", name, err)
	}
	return p
}

func TestAgentJobs_CLI_Submit_And_List(t *testing.T) {
	ctx := context.Background()
	binDir := t.TempDir()
	dummyRunner := installTestExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")
	agentjobs.DefaultDispatcher.Executable = dummyRunner

	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(promptFile, []byte("Prompt text for CLI submit"), 0644); err != nil {
		t.Fatalf("failed to write prompt file: %v", err)
	}

	args := []string{
		"submit",
		"--ticket", "DEN-CLI-1",
		"--agent", "agy",
		"--model", "gemini-3.8-flash-high",
		"--scope", "pkg/foo,cmd/x.go",
		"--prompt-file", promptFile,
		"--complexity", "2",
	}

	if err := handleAgentJobsCommand(ctx, args); err != nil {
		t.Fatalf("handleAgentJobsCommand submit failed: %v", err)
	}

	l := agentjobs.NewLedger("")
	jobs, err := l.List()
	if err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}

	var foundJob *agentjobs.Job
	for _, j := range jobs {
		if j.Ticket == "DEN-CLI-1" {
			foundJob = j
			break
		}
	}

	if foundJob == nil {
		t.Fatalf("submitted job not found in ledger")
	}

	if foundJob.Complexity != 2 {
		t.Errorf("job.Complexity = %d, want 2", foundJob.Complexity)
	}
	if len(foundJob.Scope) != 2 || foundJob.Scope[0] != "pkg/foo" {
		t.Errorf("job.Scope = %v, want [pkg/foo, cmd/x.go]", foundJob.Scope)
	}

	// Test list tabular
	if err := handleAgentJobsCommand(ctx, []string{"list"}); err != nil {
		t.Fatalf("handleAgentJobsCommand list failed: %v", err)
	}

	// Test list JSON
	if err := handleAgentJobsCommand(ctx, []string{"list", "--json"}); err != nil {
		t.Fatalf("handleAgentJobsCommand list --json failed: %v", err)
	}
}

func TestAgentJobs_CLI_Show_And_Tail(t *testing.T) {
	ctx := context.Background()
	binDir := t.TempDir()
	dummyRunner := installTestExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")
	agentjobs.DefaultDispatcher.Executable = dummyRunner

	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	_ = os.WriteFile(promptFile, []byte("Prompt text for show/tail"), 0644)

	submitArgs := []string{
		"submit",
		"--ticket", "DEN-CLI-2",
		"--prompt-file", promptFile,
	}
	if err := handleAgentJobsCommand(ctx, submitArgs); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	l := agentjobs.NewLedger("")
	jobs, _ := l.List()
	var jobID string
	for _, j := range jobs {
		if j.Ticket == "DEN-CLI-2" {
			jobID = j.ID
			break
		}
	}
	if jobID == "" {
		t.Fatalf("job ID not found")
	}

	// Append some events and log lines
	_ = l.Append(jobID, agentjobs.Event{Type: "dispatched", Data: map[string]any{"attempt": 1}})
	logPath := l.AgentLogPath(jobID)
	logLines := strings.Repeat("log output line\n", 60)
	_ = os.WriteFile(logPath, []byte(logLines), 0644)

	// Show
	if err := handleAgentJobsCommand(ctx, []string{"show", jobID}); err != nil {
		t.Fatalf("show failed: %v", err)
	}

	// Tail
	if err := handleAgentJobsCommand(ctx, []string{"tail", jobID}); err != nil {
		t.Fatalf("tail failed: %v", err)
	}

	// Tail with missing log file
	nonExistentJob := "DEN-NONEXISTENT-20261017T000000Z-0000"
	if err := handleAgentJobsCommand(ctx, []string{"tail", nonExistentJob}); err != nil {
		t.Fatalf("tail missing file should return nil: %v", err)
	}
}

func TestAgentJobs_CLI_Wait(t *testing.T) {
	ctx := context.Background()
	binDir := t.TempDir()
	dummyRunner := installTestExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")
	agentjobs.DefaultDispatcher.Executable = dummyRunner

	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	_ = os.WriteFile(promptFile, []byte("Wait prompt"), 0644)

	_ = handleAgentJobsCommand(ctx, []string{"submit", "--ticket", "DEN-WAIT", "--prompt-file", promptFile})

	l := agentjobs.NewLedger("")
	jobs, _ := l.List()
	var jobID string
	for _, j := range jobs {
		if j.Ticket == "DEN-WAIT" {
			jobID = j.ID
			break
		}
	}

	// Test wait timeout when not marked done
	err := handleAgentJobsCommand(ctx, []string{"wait", "--ticket", "DEN-WAIT", "--timeout", "50ms", "--interval", "10ms"})
	if err == nil {
		t.Fatalf("expected timeout error when job not done")
	}

	// Mark done
	_ = l.MarkDone(jobID)

	// Test wait succeeds when already marked done
	if err := handleAgentJobsCommand(ctx, []string{"wait", "--ticket", "DEN-WAIT", "--timeout", "1s", "--interval", "10ms"}); err != nil {
		t.Fatalf("wait failed for completed job: %v", err)
	}

	// Test wait with no matching jobs
	if err := handleAgentJobsCommand(ctx, []string{"wait", "--ticket", "DEN-NO-SUCH-TICKET"}); err != nil {
		t.Fatalf("wait on non-existent ticket should return nil: %v", err)
	}
}

func TestAgentJobs_CLI_Cancel_And_Retry(t *testing.T) {
	ctx := context.Background()
	binDir := t.TempDir()
	dummyRunner := installTestExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")
	agentjobs.DefaultDispatcher.Executable = dummyRunner

	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	_ = os.WriteFile(promptFile, []byte("Cancel/Retry prompt"), 0644)

	_ = handleAgentJobsCommand(ctx, []string{"submit", "--ticket", "DEN-CANCEL", "--prompt-file", promptFile})

	l := agentjobs.NewLedger("")
	jobs, _ := l.List()
	var jobID string
	for _, j := range jobs {
		if j.Ticket == "DEN-CANCEL" {
			jobID = j.ID
			break
		}
	}

	// Cancel
	if err := handleAgentJobsCommand(ctx, []string{"cancel", jobID}); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	job, _ := l.Load(jobID)
	if job.Status != agentjobs.StatusCancelled {
		t.Errorf("job.Status = %q, want %q", job.Status, agentjobs.StatusCancelled)
	}

	// Retry
	if err := handleAgentJobsCommand(ctx, []string{"retry", jobID}); err != nil {
		t.Fatalf("retry failed: %v", err)
	}

	job, _ = l.Load(jobID)
	if job.Status != agentjobs.StatusQueued {
		t.Errorf("job.Status after retry = %q, want %q", job.Status, agentjobs.StatusQueued)
	}
}

func TestAgentJobs_CLI_Run(t *testing.T) {
	ctx := context.Background()
	binDir := t.TempDir()
	dummyRunner := installTestExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")
	agentjobs.DefaultDispatcher.Executable = dummyRunner

	dummyAgy := installTestExecutableScript(t, binDir, "agy", `cat << 'EOF'
{"status": "SUCCESS", "conversation_id": "conv-run-123"}
EOF
exit 0
`)
	agentjobs.DefaultDispatcher.AgyPath = dummyAgy

	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	_ = os.WriteFile(promptFile, []byte("Run prompt"), 0644)

	_ = handleAgentJobsCommand(ctx, []string{"submit", "--ticket", "DEN-RUN", "--prompt-file", promptFile})

	l := agentjobs.NewLedger("")
	jobs, _ := l.List()
	var jobID string
	for _, j := range jobs {
		if j.Ticket == "DEN-RUN" {
			jobID = j.ID
			break
		}
	}

	// Test Run
	if err := handleAgentJobsCommand(ctx, []string{"run", jobID}); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	job, _ := l.Load(jobID)
	if job.Status == agentjobs.StatusQueued {
		t.Errorf("job should have executed and transitioned away from queued")
	}
}

func TestAgentJobs_CLI_Validation_And_Usage(t *testing.T) {
	ctx := context.Background()

	// Empty args -> usage
	if err := handleAgentJobsCommand(ctx, []string{}); err != nil {
		t.Errorf("empty args should print usage and return nil, got %v", err)
	}

	// Unknown action
	if err := handleAgentJobsCommand(ctx, []string{"unknown-action"}); err == nil {
		t.Errorf("expected error for unknown action")
	}

	// Missing args for commands that require an ID
	if err := handleAgentJobsCommand(ctx, []string{"show"}); err == nil {
		t.Errorf("show without ID should return error")
	}
	if err := handleAgentJobsCommand(ctx, []string{"tail"}); err == nil {
		t.Errorf("tail without ID should return error")
	}
	if err := handleAgentJobsCommand(ctx, []string{"cancel"}); err == nil {
		t.Errorf("cancel without ID should return error")
	}
	if err := handleAgentJobsCommand(ctx, []string{"retry"}); err == nil {
		t.Errorf("retry without ID should return error")
	}
	if err := handleAgentJobsCommand(ctx, []string{"run"}); err == nil {
		t.Errorf("run without ID should return error")
	}

	// Submit missing required flags
	if err := handleAgentJobsCommand(ctx, []string{"submit"}); err == nil {
		t.Errorf("submit without flags should error")
	}
	if err := handleAgentJobsCommand(ctx, []string{"submit", "--ticket", "DEN-X"}); err == nil {
		t.Errorf("submit without prompt should error")
	}
}

func TestAgentJobs_RoutedThroughBouton(t *testing.T) {
	for _, alias := range []string{"agent", "agt", "bouton"} {
		cmd, _, err := newRootCmd().Find([]string{alias, "jobs", "list"})
		if err != nil || cmd.CommandPath() != "dendrite bouton jobs list" {
			t.Errorf("%s jobs list resolved to %q (%v)", alias, cmd.CommandPath(), err)
		}
	}
}
