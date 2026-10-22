package agentjobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunJob_PERMISSION_And_EARLY_EXIT(t *testing.T) {
	t.Run("PERMISSION retry succeeds", func(t *testing.T) {
		worktree := createGitRepo(t)
		binDir := t.TempDir()
		counterFile := filepath.Join(binDir, "cnt_perm.txt")
		_ = os.WriteFile(counterFile, []byte("0"), 0644)

		agyScript := installExecutableScript(t, binDir, "agy", fmt.Sprintf(`
CNT=$(cat "%s")
echo $((CNT + 1)) > "%s"
if [ "$CNT" -eq 0 ]; then
  echo 'jetski: auto-denied command permission'
  exit 0
fi
mkdir -p "%s/pkg/a"
echo "package a" > "%s/pkg/a/a.go"
echo '{"status": "SUCCESS"}'
exit 0
`, counterFile, counterFile, worktree, worktree))

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
		d.AgyPath = agyScript

		id, _ := d.Submit(SubmitSpec{
			Ticket:   "DEN-989",
			Worktree: worktree,
			Scope:    []string{"pkg/a"},
			Prompt:   "Permission test",
		})

		if err := d.RunJob(context.Background(), id); err != nil {
			t.Fatalf("RunJob failed: %v", err)
		}

		job, _ := tempLedger.Load(id)
		if job.Status != StatusSucceeded {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusSucceeded)
		}
		if job.Attempts["agy"] != 2 {
			t.Errorf("expected 2 attempts, got %d", job.Attempts["agy"])
		}
	})

	t.Run("EARLY_EXIT retried once then marked early_exit", func(t *testing.T) {
		worktree := createGitRepo(t)
		binDir := t.TempDir()

		agyScript := installExecutableScript(t, binDir, "agy", `
echo 'launched a subagent'
exit 0
`)

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
		d.AgyPath = agyScript

		id, _ := d.Submit(SubmitSpec{
			Ticket:   "DEN-989",
			Worktree: worktree,
			Prompt:   "Subagent exit",
		})

		if err := d.RunJob(context.Background(), id); err != nil {
			t.Fatalf("RunJob failed: %v", err)
		}

		job, _ := tempLedger.Load(id)
		if job.Status != StatusEarlyExit {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusEarlyExit)
		}
		if job.Attempts["agy"] != 2 {
			t.Errorf("expected 2 attempts (1 initial + 1 retry), got %d", job.Attempts["agy"])
		}
	})
}

func TestRunJob_ScopeViolation(t *testing.T) {
	worktree := createGitRepo(t)
	binDir := t.TempDir()

	agyScript := installExecutableScript(t, binDir, "agy", fmt.Sprintf(`
mkdir -p "%s/cmd"
echo "package main\nfunc Sneaky() {}" > "%s/cmd/sneaky.go"
echo '{"status": "SUCCESS"}'
exit 0
`, worktree, worktree))

	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
	d.AgyPath = agyScript

	id, err := d.Submit(SubmitSpec{
		Ticket:   "DEN-989",
		Worktree: worktree,
		Scope:    []string{"pkg/allowed"},
		Prompt:   "Edit out of scope",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}

	job, err := tempLedger.Load(id)
	if err != nil {
		t.Fatalf("failed to load job: %v", err)
	}

	if job.Status != StatusNeedsReview {
		t.Errorf("job.Status = %q, want %q", job.Status, StatusNeedsReview)
	}

	events, _ := tempLedger.Events(id)
	var foundScopeViolation bool
	for _, ev := range events {
		if ev.Type == "scope_violation" {
			foundScopeViolation = true
			paths, ok := ev.Data["offending_paths"].([]any)
			if !ok || len(paths) == 0 {
				t.Errorf("offending_paths missing or empty: %+v", ev.Data)
			}
		}
	}
	if !foundScopeViolation {
		t.Errorf("expected scope_violation event in events log")
	}
}

func TestRunJob_Oxillama_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"content": "func OxillamaGenerated() string { return \"ok\" }",
					},
				},
			},
			"usage": map[string]any{
				"total_tokens": 42,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tempLedger := NewLedger(t.TempDir())
	d := NewDispatcher(tempLedger)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
	d.OxillamaEndpoint = server.URL

	id, err := d.Submit(SubmitSpec{
		Ticket: "DEN-989",
		Agent:  "oxillama",
		Prompt: "Generate Go code",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if err := d.RunJob(context.Background(), id); err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}

	job, err := tempLedger.Load(id)
	if err != nil {
		t.Fatalf("failed to load job: %v", err)
	}

	if job.Status != StatusSucceeded {
		t.Errorf("job.Status = %q, want %q", job.Status, StatusSucceeded)
	}

	outputPath := filepath.Join(tempLedger.JobDir(id), "output.md")
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output.md: %v", err)
	}
	if !strings.Contains(string(content), "OxillamaGenerated") {
		t.Errorf("output.md content = %q, expected OxillamaGenerated", string(content))
	}
}

func TestRunJob_Oxillama_EmptyResponseAndErrors(t *testing.T) {
	t.Run("Empty response classified as EARLY_EXIT", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"content": ""}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
		d.OxillamaEndpoint = server.URL

		id, _ := d.Submit(SubmitSpec{
			Ticket: "DEN-989",
			Agent:  "oxillama",
			Prompt: "Generate empty",
		})

		_ = d.RunJob(context.Background(), id)
		job, _ := tempLedger.Load(id)
		if job.Status != StatusEarlyExit {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusEarlyExit)
		}
	})

	t.Run("HTTP 429 classified as QUOTA", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "rate limit exceeded: 429", http.StatusTooManyRequests)
		}))
		defer server.Close()

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below
		d.OxillamaEndpoint = server.URL

		id, _ := d.Submit(SubmitSpec{
			Ticket: "DEN-989",
			Agent:  "oxillama",
			Prompt: "Rate limit",
		})

		_ = d.RunJob(context.Background(), id)
		job, _ := tempLedger.Load(id)
		// oxillama is the last rung of the ladder, so a 429 parks the job until the provider resets.
		if job.Status != StatusParked {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusParked)
		}
	})
}

func TestCheckScope_Variations(t *testing.T) {
	worktree := createGitRepo(t)
	d := NewDispatcher(nil)
	d.Executable = installExecutableScript(t, t.TempDir(), "dummy_runner", "exit 0\n") // RunJob is driven synchronously below

	// Clean worktree
	inScope, offending, err := d.CheckScope(context.Background(), worktree, []string{"pkg/foo"})
	if err != nil {
		t.Fatalf("CheckScope failed: %v", err)
	}
	if inScope || len(offending) > 0 {
		t.Errorf("expected clean repo, got inScope=%v, offending=%v", inScope, offending)
	}

	// File in scope
	_ = os.MkdirAll(filepath.Join(worktree, "pkg", "foo"), 0755)
	_ = os.WriteFile(filepath.Join(worktree, "pkg", "foo", "test.go"), []byte("package foo"), 0644)
	inScope, offending, err = d.CheckScope(context.Background(), worktree, []string{"pkg/foo"})
	if err != nil {
		t.Fatalf("CheckScope failed: %v", err)
	}
	if !inScope || len(offending) > 0 {
		t.Errorf("expected file in scope, got inScope=%v, offending=%v", inScope, offending)
	}

	// File out of scope
	_ = os.WriteFile(filepath.Join(worktree, "outside.txt"), []byte("bad"), 0644)
	inScope, offending, err = d.CheckScope(context.Background(), worktree, []string{"pkg/foo"})
	if err != nil {
		t.Fatalf("CheckScope failed: %v", err)
	}
	if !inScope {
		t.Errorf("expected inScope=true because pkg/foo/test.go is present")
	}
	if len(offending) != 1 || offending[0] != "outside.txt" {
		t.Errorf("expected offending [outside.txt], got %v", offending)
	}
}

func TestCancelAndRetry(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")

	d := NewDispatcher(tempLedger)
	d.Executable = dummyRunner

	id, err := d.Submit(SubmitSpec{
		Ticket: "DEN-989",
		Prompt: "Test Cancel and Retry",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Cancel
	if err := d.Cancel(id); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	job, _ := tempLedger.Load(id)
	if job.Status != StatusCancelled {
		t.Errorf("job.Status = %q, want %q", job.Status, StatusCancelled)
	}

	// Retry
	if err := d.Retry(id); err != nil {
		t.Fatalf("Retry failed: %v", err)
	}

	job, _ = tempLedger.Load(id)
	if job.Status != StatusQueued {
		t.Errorf("job.Status after retry = %q, want %q", job.Status, StatusQueued)
	}
}

func TestRunJob_ContextCancelled(t *testing.T) {
	tempLedger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")

	d := NewDispatcher(tempLedger)
	d.Executable = dummyRunner

	id, _ := d.Submit(SubmitSpec{Ticket: "DEN-989", Prompt: "Cancel ctx"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := d.RunJob(ctx, id)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	job, _ := tempLedger.Load(id)
	if job.Status != StatusFailed {
		t.Errorf("job.Status = %q, want %q", job.Status, StatusFailed)
	}
}
