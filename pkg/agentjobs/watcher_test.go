package agentjobs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWatcher_VerifySucceededJobs(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	worktree := t.TempDir()
	initGitRepo(t, worktree, "main")

	cMod := exec.Command("go", "mod", "init", "tinymod")
	cMod.Dir = worktree
	_ = cMod.Run()

	cAdd := exec.Command("git", "add", "go.mod")
	cAdd.Dir = worktree
	_ = cAdd.Run()
	cCommit := exec.Command("git", "commit", "-m", "init mod")
	cCommit.Dir = worktree
	_ = cCommit.Run()

	pkgDir := filepath.Join(worktree, "pkg", "okpkg")
	_ = os.MkdirAll(pkgDir, 0755)
	_ = os.WriteFile(filepath.Join(pkgDir, "ok.go"), []byte("package okpkg\n\nfunc Ok() bool {\n\treturn true\n}\n"), 0644)
	_ = os.WriteFile(filepath.Join(pkgDir, "ok_test.go"), []byte("package okpkg\n\nimport \"testing\"\n\nfunc TestOk(t *testing.T) {\n\tif !Ok() {\n\t\tt.Fail()\n\t}\n}\n"), 0644)

	job, err := ledger.Create(&Job{
		Ticket:   "DEN-991",
		Worktree: worktree,
		Scope:    []string{"pkg/okpkg"},
		Status:   StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("ledger.Create failed: %v", err)
	}

	dispatcher := NewDispatcher(ledger)
	watcher := NewWatcher(ledger, dispatcher)

	if err := watcher.Tick(context.Background()); err != nil {
		t.Fatalf("watcher.Tick failed: %v", err)
	}

	updated, err := ledger.Load(job.ID)
	if err != nil {
		t.Fatalf("ledger.Load failed: %v", err)
	}
	if updated.Status != StatusVerified {
		t.Errorf("job.Status = %q, want %q", updated.Status, StatusVerified)
	}
}

func TestWatcher_SingleRedispatchThenNeedsReview(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")

	dispatcher := NewDispatcher(ledger)
	dispatcher.Executable = dummyRunner

	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	_ = os.WriteFile(promptFile, []byte("Original task prompt"), 0644)

	job, err := ledger.Create(&Job{
		Ticket:   "DEN-991",
		Status:   StatusFailed,
		Scope:    []string{"pkg/foo"},
		Attempts: make(map[string]int),
	})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}
	_ = os.WriteFile(ledger.PromptPath(job.ID), []byte("Original task prompt"), 0644)

	// Simulate first failure event
	_ = ledger.Append(job.ID, Event{
		Type: "failed",
		Data: map[string]any{
			"output": "go test failed: syntax error in foo.go",
		},
	})

	watcher := NewWatcher(ledger, dispatcher)

	// Tick 1: Should re-dispatch once with failure capsule appended to prompt
	if err := watcher.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1 failed: %v", err)
	}

	updated1, err := ledger.Load(job.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if updated1.Status != StatusQueued {
		t.Errorf("job.Status after 1st retry = %q, want %q", updated1.Status, StatusQueued)
	}

	// Verify capsule was appended to prompt.md
	newPrompt, _ := os.ReadFile(ledger.PromptPath(job.ID))
	if !strings.Contains(string(newPrompt), "Verification Failure Capsule") {
		t.Errorf("prompt.md missing failure capsule: %s", string(newPrompt))
	}
	if !strings.Contains(string(newPrompt), "syntax error in foo.go") {
		t.Errorf("prompt.md missing failing output: %s", string(newPrompt))
	}

	// Now simulate that the re-dispatched run failed again
	updated1.Status = StatusFailed
	_ = ledger.Save(updated1)
	_ = ledger.Append(job.ID, Event{
		Type: "failed",
		Data: map[string]any{
			"output": "go test failed again",
		},
	})

	// Tick 2: Since it was already retried, it must be marked needs_review
	if err := watcher.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2 failed: %v", err)
	}

	updated2, err := ledger.Load(job.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if updated2.Status != StatusNeedsReview {
		t.Errorf("job.Status after 2nd failure = %q, want %q", updated2.Status, StatusNeedsReview)
	}

	events, _ := ledger.Events(job.ID)
	var foundNeedsReview bool
	for _, ev := range events {
		if ev.Type == "needs_review" {
			foundNeedsReview = true
		}
	}
	if !foundNeedsReview {
		t.Errorf("needs_review event was not recorded")
	}
}

func TestWatcher_StallDetection_WithFakeClockAndLiveProcess(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")

	dispatcher := NewDispatcher(ledger)
	dispatcher.Executable = dummyRunner

	// Start a real live long-running process (e.g. sleep 60)
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start live sleep process: %v", err)
	}
	pgid := cmd.Process.Pid
	defer func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		_ = cmd.Wait()
	}()

	// Ledger.Create/Save stamp UpdatedAt with the real clock, so the fake clock
	// must sit past real time for the stall window to elapse.
	now := time.Now().UTC().Add(time.Hour)
	oldTime := now.Add(-30 * time.Minute)

	worktree := t.TempDir()
	scopeFile := filepath.Join(worktree, "tracked.go")
	_ = os.WriteFile(scopeFile, []byte("package main\n"), 0644)
	_ = os.Chtimes(scopeFile, oldTime, oldTime)

	job, err := ledger.Create(&Job{
		Ticket:         "DEN-991",
		Status:         StatusRunning,
		Worktree:       worktree,
		Scope:          []string{"tracked.go"},
		ConversationID: "conv-agy-resume-123",
		PGID:           pgid,
		CreatedAt:      oldTime,
		UpdatedAt:      oldTime,
	})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	logPath := ledger.AgentLogPath(job.ID)
	_ = os.WriteFile(logPath, []byte("initial logs\n"), 0644)
	_ = os.Chtimes(logPath, oldTime, oldTime)

	var killedPGIDs []int
	watcher := NewWatcher(ledger, dispatcher)
	watcher.Now = func() time.Time { return now }
	watcher.StallTimeout = 15 * time.Minute
	watcher.KillPGID = func(id int) error {
		killedPGIDs = append(killedPGIDs, id)
		_ = syscall.Kill(-id, syscall.SIGKILL)
		return nil
	}

	// Tick 1: Detect stall, kill PGID, record "stalled", and retry once
	if err := watcher.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1 failed: %v", err)
	}

	if len(killedPGIDs) != 1 || killedPGIDs[0] != pgid {
		t.Errorf("expected killedPGIDs [%d], got %v", pgid, killedPGIDs)
	}

	updated1, _ := ledger.Load(job.ID)
	if updated1.Status != StatusQueued {
		t.Errorf("job.Status after stall retry = %q, want %q", updated1.Status, StatusQueued)
	}
	if updated1.ConversationID != "conv-agy-resume-123" {
		t.Errorf("job.ConversationID = %q, want conv-agy-resume-123 preserved", updated1.ConversationID)
	}

	events, _ := ledger.Events(job.ID)
	var foundStalled bool
	for _, ev := range events {
		if ev.Type == "stalled" {
			foundStalled = true
		}
	}
	if !foundStalled {
		t.Errorf("stalled event was not recorded")
	}

	// Now simulate that the resumed job is running and stalls again
	updated1.Status = StatusRunning
	updated1.UpdatedAt = oldTime
	_ = ledger.Save(updated1)
	_ = os.Chtimes(logPath, oldTime, oldTime)

	// Tick 2: Second stall -> marks needs_review
	if err := watcher.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2 failed: %v", err)
	}

	updated2, _ := ledger.Load(job.ID)
	if updated2.Status != StatusNeedsReview {
		t.Errorf("job.Status after 2nd stall = %q, want %q", updated2.Status, StatusNeedsReview)
	}
}

func TestWatcher_ResumeParkedHookInvoked(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	dispatcher := NewDispatcher(ledger)

	var resumeParkedCalled bool
	watcher := NewWatcher(ledger, dispatcher)
	watcher.ResumeParked = func() ([]string, error) {
		resumeParkedCalled = true
		return []string{"resumed-job-1"}, nil
	}

	if err := watcher.Tick(context.Background()); err != nil {
		t.Fatalf("watcher.Tick failed: %v", err)
	}

	if !resumeParkedCalled {
		t.Errorf("expected ResumeParked hook to be called on tick")
	}
}

func TestInstallLaunchAgent_PlistContent(t *testing.T) {
	tmpDir := t.TempDir()
	plistPath := filepath.Join(tmpDir, "dev.dendrite.jobs.plist")
	binaryPath := "/usr/local/bin/dendrite"

	outPath, err := InstallLaunchAgent(plistPath, binaryPath)
	if err != nil {
		t.Fatalf("InstallLaunchAgent failed: %v", err)
	}
	if outPath != plistPath {
		t.Errorf("outPath = %q, want %q", outPath, plistPath)
	}

	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("failed to read written plist: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "<string>dev.dendrite.jobs</string>") {
		t.Errorf("plist missing label dev.dendrite.jobs")
	}
	if !strings.Contains(content, "<string>/usr/local/bin/dendrite</string>") {
		t.Errorf("plist missing binary path")
	}
	if !strings.Contains(content, "<string>daemon</string>") {
		t.Errorf("plist missing daemon argument")
	}
	if !strings.Contains(content, "<key>KeepAlive</key>") {
		t.Errorf("plist missing KeepAlive")
	}
	if !strings.Contains(content, "<key>RunAtLoad</key>") {
		t.Errorf("plist missing RunAtLoad")
	}

	// Test default launch agent path and default binary path
	defPlist := DefaultLaunchAgentPath()
	if !strings.Contains(defPlist, "dev.dendrite.jobs.plist") {
		t.Errorf("DefaultLaunchAgentPath = %q, want to contain dev.dendrite.jobs.plist", defPlist)
	}

	plistContentDefault := GenerateLaunchdPlist("")
	if !strings.Contains(plistContentDefault, "agent") || !strings.Contains(plistContentDefault, "daemon") {
		t.Errorf("GenerateLaunchdPlist(\"\") generated unexpected content: %s", plistContentDefault)
	}
}

func TestWatcher_ContextCancellation(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	watcher := NewWatcher(ledger, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := watcher.Tick(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
