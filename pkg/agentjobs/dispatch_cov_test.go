package agentjobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoverage_PackageLevelSubmitAndRunJob(t *testing.T) {
	origLedger := DefaultDispatcher.Ledger
	origExe := DefaultDispatcher.Executable
	origAgy := DefaultDispatcher.AgyPath
	defer func() {
		DefaultDispatcher.Ledger = origLedger
		DefaultDispatcher.Executable = origExe
		DefaultDispatcher.AgyPath = origAgy
	}()

	tempLedger := NewLedger(t.TempDir())
	binDir := t.TempDir()
	dummyRunner := installExecutableScript(t, binDir, "dummy_dendrite", "exit 0\n")
	agyScript := installExecutableScript(t, binDir, "agy", `
echo '{"status": "SUCCESS"}'
exit 0
`)

	DefaultDispatcher.Ledger = tempLedger
	DefaultDispatcher.Executable = dummyRunner
	DefaultDispatcher.AgyPath = agyScript

	spec := SubmitSpec{
		Ticket: "DEN-989",
		Prompt: "Package level submit test",
	}

	id, err := Submit(spec)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if id == "" {
		t.Fatalf("expected non-empty job ID")
	}

	ctx := context.Background()
	if err := RunJob(ctx, id); err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}
}

func TestCoverage_NewDispatcher_EnvEndpoint(t *testing.T) {
	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", "http://custom-host:9999")
	d := NewDispatcher(nil)
	if d.OxillamaEndpoint != "http://custom-host:9999" {
		t.Errorf("expected custom endpoint, got %q", d.OxillamaEndpoint)
	}
}

func TestCoverage_Submit_EdgeCases(t *testing.T) {
	t.Run("whitespace prompt returns error", func(t *testing.T) {
		d := NewDispatcher(NewLedger(t.TempDir()))
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		_, err := d.Submit(SubmitSpec{Prompt: "   \t\n  "})
		if err == nil {
			t.Fatal("expected error for whitespace prompt")
		}
	})

	t.Run("ledger Create failure", func(t *testing.T) {
		tempFile := filepath.Join(t.TempDir(), "regular_file")
		if err := os.WriteFile(tempFile, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		badLedger := NewLedger(filepath.Join(tempFile, "subledger"))
		d := NewDispatcher(badLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		_, err := d.Submit(SubmitSpec{Prompt: "valid prompt"})
		if err == nil {
			t.Fatal("expected error from ledger Create")
		}
	})

	t.Run("prompt file write failure", func(t *testing.T) {
		origRand := randReader
		defer func() { randReader = origRand }()
		randReader = bytes.NewReader(bytes.Repeat([]byte{0x77}, 200))

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")

		for time.Now().UTC().Nanosecond() > 700_000_000 {
			time.Sleep(10 * time.Millisecond)
		}
		now := time.Now().UTC()
		idNow := fmt.Sprintf("DEN-PROMPT-%s-777777", now.Format("20060102T150405Z"))
		idNext := fmt.Sprintf("DEN-PROMPT-%s-777777", now.Add(time.Second).Format("20060102T150405Z"))

		if err := os.MkdirAll(filepath.Join(tempLedger.Root(), idNow, "prompt.md"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(tempLedger.Root(), idNext, "prompt.md"), 0755); err != nil {
			t.Fatal(err)
		}

		_, err := d.Submit(SubmitSpec{
			Ticket: "DEN-PROMPT",
			Prompt: "some prompt",
		})
		if err == nil {
			t.Fatal("expected error when writing prompt.md over directory")
		}
		if !strings.Contains(err.Error(), "failed to write prompt.md") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("ledger Save failure on submit", func(t *testing.T) {
		origWriteFile := writeFile
		defer func() { writeFile = origWriteFile }()

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")

		writeCount := 0
		writeFile = func(name string, data []byte, perm os.FileMode) error {
			writeCount++
			if writeCount > 1 {
				return errors.New("simulated save error")
			}
			return os.WriteFile(name, data, perm)
		}

		_, err := d.Submit(SubmitSpec{Prompt: "valid prompt"})
		if err == nil {
			t.Fatal("expected error from ledger Save")
		}
		if !strings.Contains(err.Error(), "failed to save job metadata") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("worktree Getwd failure", func(t *testing.T) {
		origWd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(origWd) }()

		tmpDir := t.TempDir()
		delDir := filepath.Join(tmpDir, "deleted_dir")
		if err := os.Mkdir(delDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(delDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(delDir); err != nil {
			t.Fatal(err)
		}

		d := NewDispatcher(NewLedger(tmpDir))
		d.Executable = installExecutableScript(t, tmpDir, "dummy", "exit 0\n")
		_, err = d.Submit(SubmitSpec{Worktree: "", Prompt: "test"})
		_ = os.Chdir(origWd)
		if err != nil && !strings.Contains(err.Error(), "failed to resolve worktree") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCoverage_RunJob_EdgeCases(t *testing.T) {
	t.Run("load nonexistent job fails", func(t *testing.T) {
		d := NewDispatcher(NewLedger(t.TempDir()))
		err := d.RunJob(context.Background(), "nonexistent-job")
		if err == nil {
			t.Fatal("expected error loading nonexistent job")
		}
	})

	t.Run("save failure at start of run", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")

		id, err := d.Submit(SubmitSpec{Prompt: "test"})
		if err != nil {
			t.Fatal(err)
		}

		origWriteFile := writeFile
		defer func() { writeFile = origWriteFile }()
		writeFile = func(name string, data []byte, perm os.FileMode) error {
			return errors.New("simulated save error")
		}

		err = d.RunJob(context.Background(), id)
		if err == nil {
			t.Fatal("expected save failure error")
		}
		if !strings.Contains(err.Error(), "failed to update job status") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing prompt file fails", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")

		id, err := d.Submit(SubmitSpec{Prompt: "test"})
		if err != nil {
			t.Fatal(err)
		}

		_ = os.Remove(tempLedger.PromptPath(id))

		err = d.RunJob(context.Background(), id)
		if err == nil {
			t.Fatal("expected error reading prompt")
		}
		job, _ := tempLedger.Load(id)
		if job.Status != StatusFailed {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusFailed)
		}
	})

	t.Run("nil attempts map is initialized", func(t *testing.T) {
		worktree := createGitRepo(t)
		binDir := t.TempDir()
		agyScript := installExecutableScript(t, binDir, "agy", `
echo done > out.txt
echo '{"status": "SUCCESS"}'
exit 0
`)
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		d.AgyPath = agyScript

		id, err := d.Submit(SubmitSpec{Prompt: "test", Worktree: worktree, Scope: []string{"."}})
		if err != nil {
			t.Fatal(err)
		}

		job, _ := tempLedger.Load(id)
		job.Attempts = nil
		_ = tempLedger.Save(job)

		_ = d.RunJob(context.Background(), id)

		job, _ = tempLedger.Load(id)
		if job.Attempts == nil || job.Attempts["agy"] != 1 {
			t.Errorf("attempts not initialized properly: %+v", job.Attempts)
		}
	})

	t.Run("scope check error when worktree is not git repo", func(t *testing.T) {
		nonGitDir := t.TempDir()
		binDir := t.TempDir()
		agyScript := installExecutableScript(t, binDir, "agy", `
echo '{"status": "SUCCESS"}'
exit 0
`)
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		d.AgyPath = agyScript

		id, err := d.Submit(SubmitSpec{
			Worktree: nonGitDir,
			Scope:    []string{"pkg/foo"},
			Prompt:   "test",
		})
		if err != nil {
			t.Fatal(err)
		}

		_ = d.RunJob(context.Background(), id)
		events, _ := tempLedger.Events(id)
		if len(events) == 0 {
			t.Fatal("expected events")
		}
	})

	t.Run("scope violation with early exit", func(t *testing.T) {
		worktree := createGitRepo(t)
		binDir := t.TempDir()
		agyScript := installExecutableScript(t, binDir, "agy", fmt.Sprintf(`
mkdir -p "%s/cmd"
echo "sneaky" > "%s/cmd/sneaky.txt"
echo "launched a subagent"
exit 0
`, worktree, worktree))

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		d.AgyPath = agyScript

		id, err := d.Submit(SubmitSpec{
			Worktree: worktree,
			Scope:    []string{"pkg/allowed"},
			Prompt:   "test",
		})
		if err != nil {
			t.Fatal(err)
		}

		_ = d.RunJob(context.Background(), id)

		job, _ := tempLedger.Load(id)
		if job.Status != StatusNeedsReview {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusNeedsReview)
		}
	})

	t.Run("ClassDone with no file changes results in StatusEarlyExit", func(t *testing.T) {
		worktree := createGitRepo(t)
		binDir := t.TempDir()
		agyScript := installExecutableScript(t, binDir, "agy", `
cat << 'EOF'
{"conversation_id": "conv-noop", "status": "SUCCESS", "response": "Nothing to do"}
EOF
exit 0
`)

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		d.AgyPath = agyScript

		id, err := d.Submit(SubmitSpec{
			Worktree: worktree,
			Scope:    []string{"pkg/foo"},
			Prompt:   "No-op job",
		})
		if err != nil {
			t.Fatal(err)
		}

		_ = d.RunJob(context.Background(), id)

		job, _ := tempLedger.Load(id)
		if job.Status != StatusEarlyExit {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusEarlyExit)
		}
	})

	t.Run("ClassPermission retried and exhausted results in StatusFailed", func(t *testing.T) {
		worktree := createGitRepo(t)
		binDir := t.TempDir()
		agyScript := installExecutableScript(t, binDir, "agy", `
echo 'jetski: auto-denied command permission'
exit 0
`)

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		d.AgyPath = agyScript

		id, err := d.Submit(SubmitSpec{
			Worktree: worktree,
			Scope:    []string{"pkg/foo"},
			Prompt:   "Permanent permission deny",
		})
		if err != nil {
			t.Fatal(err)
		}

		_ = d.RunJob(context.Background(), id)

		job, _ := tempLedger.Load(id)
		if job.Status != StatusFailed {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusFailed)
		}
		if job.Attempts["agy"] != 2 {
			t.Errorf("job.Attempts['agy'] = %d, want 2", job.Attempts["agy"])
		}
	})

	t.Run("unclassified class goes to default StatusFailed", func(t *testing.T) {
		worktree := createGitRepo(t)
		binDir := t.TempDir()
		agyScript := installExecutableScript(t, binDir, "agy", `
echo 'completely unhandled internal panic in agent binary'
exit 17
`)

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		d.AgyPath = agyScript

		id, err := d.Submit(SubmitSpec{
			Worktree: worktree,
			Scope:    []string{"pkg/foo"},
			Prompt:   "Crash",
		})
		if err != nil {
			t.Fatal(err)
		}

		_ = d.RunJob(context.Background(), id)

		job, _ := tempLedger.Load(id)
		if job.Status != StatusFailed {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusFailed)
		}
	})
}
