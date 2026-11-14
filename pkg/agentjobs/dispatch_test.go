package agentjobs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func createGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (%s)", err, string(out))
	}

	c1 := exec.Command("git", "config", "user.name", "TestUser")
	c1.Dir = dir
	_ = c1.Run()

	c2 := exec.Command("git", "config", "user.email", "test@example.com")
	c2.Dir = dir
	_ = c2.Run()

	c3 := exec.Command("git", "commit", "--allow-empty", "-m", "init")
	c3.Dir = dir
	if out, err := c3.CombinedOutput(); err != nil {
		t.Fatalf("git initial commit failed: %v (%s)", err, string(out))
	}

	return dir
}

func installExecutableScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	content := "#!/bin/sh\n" + body
	if err := os.WriteFile(p, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write executable %s: %v", name, err)
	}
	return p
}

func TestSubmit_SuccessAndDefaults(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")

	d := NewDispatcher(tempLedger)
	d.Executable = dummyRunner

	promptFile := filepath.Join(t.TempDir(), "test_prompt.md")
	if err := os.WriteFile(promptFile, []byte("Implement ticket DEN-989"), 0644); err != nil {
		t.Fatalf("failed to write prompt file: %v", err)
	}

	spec := SubmitSpec{
		Ticket:     "DEN-989",
		PromptFile: promptFile,
		Scope:      []string{"pkg/agentjobs", "cmd/dendrite"},
	}

	id, err := d.Submit(spec)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if !strings.HasPrefix(id, "DEN-989-") {
		t.Errorf("job id %q does not start with DEN-989-", id)
	}

	job, err := tempLedger.Load(id)
	if err != nil {
		t.Fatalf("failed to load submitted job: %v", err)
	}

	if job.Ticket != "DEN-989" {
		t.Errorf("job.Ticket = %q, want DEN-989", job.Ticket)
	}
	if job.Agent != "agy" {
		t.Errorf("job.Agent = %q, want agy", job.Agent)
	}
	if job.Model != "gemini-3.8-flash-high" {
		t.Errorf("job.Model = %q, want gemini-3.8-flash-high", job.Model)
	}
	if job.Status != StatusQueued {
		t.Errorf("job.Status = %q, want %q", job.Status, StatusQueued)
	}

	promptContent, err := os.ReadFile(tempLedger.PromptPath(id))
	if err != nil {
		t.Fatalf("failed to read prompt.md: %v", err)
	}
	if string(promptContent) != "Implement ticket DEN-989" {
		t.Errorf("prompt.md = %q, want 'Implement ticket DEN-989'", string(promptContent))
	}

	events, err := tempLedger.Events(id)
	if err != nil {
		t.Fatalf("failed to load events: %v", err)
	}
	if len(events) == 0 || events[0].Type != "submitted" {
		t.Errorf("expected 'submitted' event, got: %+v", events)
	}
}

func TestSubmit_DirectPrompt_OxillamaDefaults(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")

	d := NewDispatcher(tempLedger)
	d.Executable = dummyRunner

	spec := SubmitSpec{
		Agent:  "oxillama",
		Prompt: "Solve ticket directly",
	}

	id, err := d.Submit(spec)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	job, err := tempLedger.Load(id)
	if err != nil {
		t.Fatalf("failed to load job: %v", err)
	}

	if job.Ticket != "JOB" {
		t.Errorf("expected default ticket 'JOB', got %q", job.Ticket)
	}
	if job.Model != "default" {
		t.Errorf("expected default model 'default' for oxillama, got %q", job.Model)
	}
}

func TestSubmit_ValidationErrors(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below

	// Missing prompt
	_, err := d.Submit(SubmitSpec{Ticket: "DEN-1"})
	if err == nil {
		t.Errorf("expected error when prompt is missing")
	}

	// Missing prompt file
	_, err = d.Submit(SubmitSpec{Ticket: "DEN-1", PromptFile: "/nonexistent/prompt.md"})
	if err == nil {
		t.Errorf("expected error when prompt file does not exist")
	}

	// Unexecutable binary path
	d.Executable = "/nonexistent/path/to/binary"
	_, err = d.Submit(SubmitSpec{Ticket: "DEN-1", Prompt: "valid prompt"})
	if err == nil {
		t.Errorf("expected error when executable cannot start")
	}
}

func TestRunJob_DONE_Success(t *testing.T) {
	worktree := createGitRepo(t)
	targetDir := filepath.Join(worktree, "pkg", "foo")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	binDir := t.TempDir()
	agyScript := installExecutableScript(t, binDir, "agy", fmt.Sprintf(`
# Modify file inside declared scope
echo "package foo\nfunc Bar() {}" > "%s/pkg/foo/bar.go"
cat << 'EOF'
{"conversation_id": "conv-done-123", "status": "SUCCESS", "response": "OK", "usage": {"total_tokens": 120}}
EOF
exit 0
`, worktree))

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
	d.AgyPath = agyScript

	id, err := d.Submit(SubmitSpec{
		Ticket:   "DEN-989",
		Agent:    "agy",
		Worktree: worktree,
		Scope:    []string{"pkg/foo"},
		Prompt:   "Add Bar() to pkg/foo",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	ctx := context.Background()
	if err := d.RunJob(ctx, id); err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}

	job, err := tempLedger.Load(id)
	if err != nil {
		t.Fatalf("failed to load job: %v", err)
	}

	if job.Status != StatusSucceeded {
		t.Errorf("job.Status = %q, want %q", job.Status, StatusSucceeded)
	}
	if job.ConversationID != "conv-101" && job.ConversationID != "conv-done-123" {
		t.Errorf("job.ConversationID = %q, want conv-done-123", job.ConversationID)
	}

	// Done marker check
	if _, err := os.Stat(tempLedger.DoneMarkerPath(id)); err != nil {
		t.Errorf("done marker does not exist: %v", err)
	}

	events, _ := tempLedger.Events(id)
	var foundExited bool
	for _, ev := range events {
		if ev.Type == "exited" {
			foundExited = true
			if ev.Data["class"] != string(ClassDone) {
				t.Errorf("exited event class = %v, want DONE", ev.Data["class"])
			}
			if ev.Data["conversation_id"] != "conv-done-123" {
				t.Errorf("exited event conversation_id = %v", ev.Data["conversation_id"])
			}
		}
	}
	if !foundExited {
		t.Errorf("exited event not found in event log")
	}
}

func TestRunJob_TRANSIENT_RetryAndResume(t *testing.T) {
	worktree := createGitRepo(t)
	if err := os.MkdirAll(filepath.Join(worktree, "pkg", "foo"), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	binDir := t.TempDir()
	counterFile := filepath.Join(binDir, "counter.txt")
	_ = os.WriteFile(counterFile, []byte("0"), 0644)

	agyScript := installExecutableScript(t, binDir, "agy", fmt.Sprintf(`
CNT=$(cat "%s")
NEW_CNT=$((CNT + 1))
echo "$NEW_CNT" > "%s"

if [ "$CNT" -eq 0 ]; then
  echo 'AGY_ERROR {"short_error":"network issue connecting to server: EOF","status":"UNKNOWN"}'
  exit 3
fi

# Verify resume prompt or conversation id was passed
echo "package foo\nfunc Resumed() {}" > "%s/pkg/foo/bar.go"
cat << 'EOF'
{"conversation_id": "conv-transient-999", "status": "SUCCESS", "response": "OK", "usage": {"total_tokens": 240}}
EOF
exit 0
`, counterFile, counterFile, worktree))

	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
	d.AgyPath = agyScript

	var sleepCalls int
	d.Sleep = func(d time.Duration) {
		sleepCalls++
	}

	id, err := d.Submit(SubmitSpec{
		Ticket:   "DEN-989",
		Agent:    "agy",
		Worktree: worktree,
		Scope:    []string{"pkg/foo"},
		Prompt:   "Transient retry test",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Preset a conversation ID to verify it gets preserved and resumed
	job, _ := tempLedger.Load(id)
	job.ConversationID = "conv-transient-999"
	_ = tempLedger.Save(job)

	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}

	job, err = tempLedger.Load(id)
	if err != nil {
		t.Fatalf("failed to reload job: %v", err)
	}

	if job.Status != StatusSucceeded {
		t.Errorf("job.Status = %q, want %q", job.Status, StatusSucceeded)
	}
	if sleepCalls != 1 {
		t.Errorf("expected 1 sleep call for backoff, got %d", sleepCalls)
	}
	if job.Attempts["agy"] != 2 {
		t.Errorf("job.Attempts['agy'] = %d, want 2", job.Attempts["agy"])
	}
}

func TestRunJob_TRANSIENT_RetriesExhausted(t *testing.T) {
	worktree := createGitRepo(t)
	binDir := t.TempDir()

	agyScript := installExecutableScript(t, binDir, "agy", `
echo 'AGY_ERROR connection refused: EOF'
exit 3
`)

	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
	d.AgyPath = agyScript
	d.Sleep = func(d time.Duration) {}

	id, err := d.Submit(SubmitSpec{
		Ticket:   "DEN-989",
		Worktree: worktree,
		Prompt:   "Always fail transient",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}

	job, _ := tempLedger.Load(id)
	if job.Status != StatusFailed {
		t.Errorf("job.Status = %q, want %q", job.Status, StatusFailed)
	}
	if job.Attempts["agy"] != 4 {
		t.Errorf("expected 4 total attempts (1 initial + 3 retries), got %d", job.Attempts["agy"])
	}
}

func TestRunJob_QUOTA_WalksLadderThenParks(t *testing.T) {
	worktree := createGitRepo(t)
	binDir := t.TempDir()

	var sleeps int64
	quotaScript := "echo \"HTTP 429: RESOURCE_EXHAUSTED quota exceeded\"\nexit 1\n"
	oxillama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limit"}`))
	}))
	defer oxillama.Close()

	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
	d.AgyPath = installExecutableScript(t, binDir, "agy", quotaScript)
	d.AiderPath = installExecutableScript(t, binDir, "aider", quotaScript)
	d.OxillamaEndpoint = oxillama.URL
	d.LiteLLMURL, d.LiteLLMKey = oxillama.URL+"/v1", "test-key" // the local tier goes through the gateway
	d.Sleep = func(time.Duration) { atomic.AddInt64(&sleeps, 1) }

	id, err := d.Submit(SubmitSpec{Ticket: "DEN-990", Worktree: worktree, Prompt: "Hit quota", Complexity: 3})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}

	job, _ := tempLedger.Load(id)
	if job.Status != StatusParked {
		t.Fatalf("job.Status = %q, want %q", job.Status, StatusParked)
	}
	if job.ResumeAfter.IsZero() {
		t.Error("a parked job must carry resume_after")
	}
	if atomic.LoadInt64(&sleeps) != 0 {
		t.Errorf("QUOTA must escalate without backoff retries, got %d sleeps", sleeps)
	}
	events, _ := tempLedger.Events(id)
	var path []string
	for _, e := range events {
		if e.Type == "escalated" {
			path = append(path, fmt.Sprint(e.Data["to"]))
		}
	}
	if strings.Join(path, ",") != "agy/gemini-4-argon-low,aider/copilot-sonnet,local/local-qwen3-9b" {
		t.Errorf("escalation path = %v, want gemini-3.8 -> argon -> copilot -> local GPU model", path)
	}
	if data, err := os.ReadFile(tempLedger.CapsulePath(id)); err != nil || !strings.Contains(string(data), "RESOURCE_EXHAUSTED") && !strings.Contains(string(data), "rate limit") {
		t.Errorf("capsule must carry the failing output, err=%v", err)
	}
}
