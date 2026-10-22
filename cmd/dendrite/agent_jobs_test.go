package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/projectid"
)

func setupCrossProjectEnv(t *testing.T) (home, otherProjDir string, otherLedger *agentjobs.Ledger) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	t.Setenv("DENDRITE_STATE_DIR", filepath.Join(t.TempDir(), "current_state"))

	otherProjDir = t.TempDir()
	planeJSON := `{"identifier":"DRAG","project_id":"drag-proj-uuid","name":"Dragon"}`
	if err := os.MkdirAll(filepath.Join(otherProjDir, ".dendrite"), 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(otherProjDir, ".dendrite", "plane.json"), []byte(planeJSON), 0644); err != nil {
		t.Fatalf("Write plane.json: %v", err)
	}

	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		t.Fatalf("DefaultIndexPath: %v", err)
	}
	idx, err := projectid.LoadIndex(indexPath)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	idx.Upsert(projectid.Entry{
		ID:   "dpid_dragon",
		Path: otherProjDir,
		Name: "Dragon",
	})
	if err := idx.Save(); err != nil {
		t.Fatalf("Save index: %v", err)
	}

	otherLedger = agentjobs.NewLedger(filepath.Join(otherProjDir, ".dendrite", "jobs"))
	return home, otherProjDir, otherLedger
}

func TestBoutonJobsWait_CrossProjectBlocksUntilDone(t *testing.T) {
	_, _, otherLedger := setupCrossProjectEnv(t)

	// Create running job in Dragon's ledger
	job, err := otherLedger.Create(&agentjobs.Job{
		Ticket: "DRAG-30",
		Agent:  "agy",
		Status: agentjobs.StatusRunning,
	})
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}

	// Mark done after a short delay
	go func() {
		time.Sleep(60 * time.Millisecond)
		_ = otherLedger.MarkDone(job.ID)
	}()

	var out bytes.Buffer
	ctx := context.Background()
	err = runCLI(ctx, &out, "bouton", "jobs", "wait", "--ticket", "DRAG-30", "--interval", "10ms", "--timeout", "5s")
	if err != nil {
		t.Fatalf("wait failed: %v, output: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "[✓] Job finished: "+job.ID) {
		t.Errorf("expected output to contain '[✓] Job finished: %s', got:\n%s", job.ID, output)
	}
}

func TestBoutonJobsWait_AlreadyFinishedExitsZero(t *testing.T) {
	_, _, otherLedger := setupCrossProjectEnv(t)

	job, err := otherLedger.Create(&agentjobs.Job{
		Ticket: "DRAG-30",
		Agent:  "agy",
		Status: agentjobs.StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}
	if err := otherLedger.MarkDone(job.ID); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	var out bytes.Buffer
	ctx := context.Background()
	err = runCLI(ctx, &out, "bouton", "jobs", "wait", "--ticket", "DRAG-30", "--interval", "10ms", "--timeout", "5s")
	if err != nil {
		t.Fatalf("wait on already finished job failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "[✓] Job finished: "+job.ID) {
		t.Errorf("expected output to contain '[✓] Job finished: %s', got:\n%s", job.ID, output)
	}
}

func TestBoutonJobsWait_UnknownTicketExitsNonZero(t *testing.T) {
	_ = t.TempDir()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())

	var out bytes.Buffer
	ctx := context.Background()
	err := runCLI(ctx, &out, "bouton", "jobs", "wait", "--ticket", "UNKNOWN-999", "--interval", "10ms", "--timeout", "1s")
	if err == nil {
		t.Fatal("expected error waiting for unknown ticket, got nil")
	}
	if exitCodeFor(err) == 0 {
		t.Errorf("expected non-zero exit code, got %d", exitCodeFor(err))
	}
}

func TestBoutonJobsWait_KnownProjectNoJobsExitsNonZero(t *testing.T) {
	setupCrossProjectEnv(t)

	var out bytes.Buffer
	ctx := context.Background()
	err := runCLI(ctx, &out, "bouton", "jobs", "wait", "--ticket", "DRAG-404", "--interval", "10ms", "--timeout", "1s")
	if err == nil {
		t.Fatal("expected error waiting for ticket without jobs, got nil")
	}
	if exitCodeFor(err) == 0 {
		t.Errorf("expected non-zero exit code, got %d", exitCodeFor(err))
	}
}

func TestBoutonJobsWait_NoTicketNoJobsPrintsAndExitsZero(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())

	var out bytes.Buffer
	ctx := context.Background()
	err := runCLI(ctx, &out, "bouton", "jobs", "wait", "--interval", "10ms", "--timeout", "1s")
	if err != nil {
		t.Fatalf("expected nil error for wait with no ticket and no jobs, got %v", err)
	}
	if !strings.Contains(out.String(), "[-] No matching jobs found") {
		t.Errorf("expected '[-] No matching jobs found', got: %s", out.String())
	}
}

func TestBoutonJobsShow_LatestJobOfTicket(t *testing.T) {
	_, _, otherLedger := setupCrossProjectEnv(t)

	now := time.Now().UTC()

	// Initial attempt
	job1, err := otherLedger.Create(&agentjobs.Job{
		Ticket:    "DRAG-30",
		Agent:     "agy",
		Model:     "gemini-3.8-flash-low",
		Status:    agentjobs.StatusFailed,
		CreatedAt: now.Add(-5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create job1: %v", err)
	}

	// Retry attempt (latest)
	job2, err := otherLedger.Create(&agentjobs.Job{
		Ticket:    "DRAG-30",
		Agent:     "agy",
		Model:     "gemini-3.8-flash-high",
		Status:    agentjobs.StatusSucceeded,
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create job2: %v", err)
	}
	_ = job1

	var out bytes.Buffer
	ctx := context.Background()
	err = runCLI(ctx, &out, "bouton", "jobs", "show", "--ticket", "DRAG-30")
	if err != nil {
		t.Fatalf("show --ticket DRAG-30 failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "[+] Job Specification: "+job2.ID) {
		t.Errorf("expected output to show latest job %s, got:\n%s", job2.ID, output)
	}
	if !strings.Contains(output, "gemini-3.8-flash-high") {
		t.Errorf("expected output to contain model of latest job, got:\n%s", output)
	}

	// Test --json flag
	var jsonOut bytes.Buffer
	err = runCLI(ctx, &jsonOut, "bouton", "jobs", "show", "--ticket", "DRAG-30", "--json")
	if err != nil {
		t.Fatalf("show --ticket DRAG-30 --json failed: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v\nOutput: %s", err, jsonOut.String())
	}
	jobData, ok := res["job"].(map[string]any)
	if !ok || jobData["id"] != job2.ID {
		t.Errorf("json job id = %v, want %s", jobData["id"], job2.ID)
	}

	// Unknown ticket exits non-zero
	var errOut bytes.Buffer
	err = runCLI(ctx, &errOut, "bouton", "jobs", "show", "--ticket", "UNKNOWN-1")
	if err == nil {
		t.Fatal("expected error for show with unknown ticket, got nil")
	}
	if exitCodeFor(err) == 0 {
		t.Errorf("expected non-zero exit code for unknown ticket, got %d", exitCodeFor(err))
	}

	// Mutually exclusive: <job-id> and --ticket
	var mutexOut bytes.Buffer
	err = runCLI(ctx, &mutexOut, "bouton", "jobs", "show", "JOB-1", "--ticket", "DRAG-30")
	if err == nil {
		t.Fatal("expected error for both <job-id> and --ticket, got nil")
	}
	if !isUsageError(err) {
		t.Errorf("expected usage error, got %v", err)
	}

	// Neither given
	var noneOut bytes.Buffer
	err = runCLI(ctx, &noneOut, "bouton", "jobs", "show")
	if err == nil {
		t.Fatal("expected error when neither <job-id> nor --ticket is provided, got nil")
	}
	if !isUsageError(err) {
		t.Errorf("expected usage error, got %v", err)
	}
}

func TestBoutonJobsShow_ByJobIDCrossProject(t *testing.T) {
	_, _, otherLedger := setupCrossProjectEnv(t)

	job, err := otherLedger.Create(&agentjobs.Job{
		Ticket: "DRAG-30",
		Agent:  "agy",
		Status: agentjobs.StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}

	var out bytes.Buffer
	ctx := context.Background()
	err = runCLI(ctx, &out, "bouton", "jobs", "show", job.ID)
	if err != nil {
		t.Fatalf("show %s failed: %v", job.ID, err)
	}

	output := out.String()
	if !strings.Contains(output, "[+] Job Specification: "+job.ID) {
		t.Errorf("expected output to contain job specification for %s, got:\n%s", job.ID, output)
	}
}

func TestBoutonJobsList_CrossProject(t *testing.T) {
	_, _, otherLedger := setupCrossProjectEnv(t)

	job1, _ := otherLedger.Create(&agentjobs.Job{
		Ticket: "DRAG-30",
		Agent:  "agy",
		Status: agentjobs.StatusFailed,
	})
	job2, _ := otherLedger.Create(&agentjobs.Job{
		Ticket: "DRAG-30",
		Agent:  "agy",
		Status: agentjobs.StatusSucceeded,
	})

	var out bytes.Buffer
	ctx := context.Background()
	err := runCLI(ctx, &out, "bouton", "jobs", "list", "--ticket", "DRAG-30")
	if err != nil {
		t.Fatalf("list --ticket DRAG-30 failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, job1.ID) || !strings.Contains(output, job2.ID) {
		t.Errorf("expected list to contain both jobs %s and %s, got:\n%s", job1.ID, job2.ID, output)
	}
	if !strings.Contains(output, "[+] Agent Jobs (2):") {
		t.Errorf("expected header '[+] Agent Jobs (2):', got:\n%s", output)
	}

	// JSON listing
	var jsonOut bytes.Buffer
	err = runCLI(ctx, &jsonOut, "bouton", "jobs", "list", "--ticket", "DRAG-30", "--json")
	if err != nil {
		t.Fatalf("list --ticket DRAG-30 --json failed: %v", err)
	}
	var jobsList []map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &jobsList); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(jobsList) != 2 {
		t.Errorf("expected 2 jobs in JSON list, got %d", len(jobsList))
	}

	// Unknown ticket exits non-zero
	var errOut bytes.Buffer
	err = runCLI(ctx, &errOut, "bouton", "jobs", "list", "--ticket", "UNKNOWN-999")
	if err == nil {
		t.Fatal("expected error for list with unknown ticket, got nil")
	}
	if exitCodeFor(err) == 0 {
		t.Errorf("expected non-zero exit code, got %d", exitCodeFor(err))
	}
}
