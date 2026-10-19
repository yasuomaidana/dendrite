package agentjobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

func TestCoverage_RunAgy_Branches(t *testing.T) {
	t.Run("nonexistent agy binary returns exit code 1", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
		d.AgyPath = "/nonexistent/path/to/agy"

		job := &Job{
			ID:       "DEN-AGY-1",
			Ticket:   "DEN-AGY",
			Agent:    "agy",
			Worktree: t.TempDir(),
		}
		_ = os.MkdirAll(tempLedger.JobDir(job.ID), 0755)

		code, logTail, _, _, err := d.runAgy(context.Background(), job, "prompt", "")
		if err == nil {
			t.Fatal("expected error from nonexistent binary")
		}
		if code != 1 {
			t.Errorf("expected exitCode 1, got %d", code)
		}
		if logTail == "" {
			t.Errorf("expected non-empty logTail")
		}
	})

	t.Run("extract conversation ID from wrapped json and bad braces", func(t *testing.T) {
		tests := []struct {
			name     string
			script   string
			wantConv string
		}{
			{
				name: "wrapped json",
				script: `
echo "Some diagnostic prefix"
echo '{"conversation_id": "conv-embedded-99", "status": "SUCCESS"}'
echo "Some diagnostic suffix"
exit 0
`,
				wantConv: "conv-embedded-99",
			},
			{
				name: "reversed braces",
				script: `
echo "reversed: } {"
exit 0
`,
				wantConv: "",
			},
			{
				name: "no braces",
				script: `
echo "plain text output"
exit 0
`,
				wantConv: "",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				binDir := t.TempDir()
				agyScript := installExecutableScript(t, binDir, "agy", tc.script)
				tempLedger := NewLedger(t.TempDir())
				d := NewDispatcher(tempLedger)
				d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")
				d.AgyPath = agyScript

				job := &Job{
					ID:       "DEN-AGY-2",
					Ticket:   "DEN-AGY",
					Agent:    "agy",
					Worktree: t.TempDir(),
				}
				_ = os.MkdirAll(tempLedger.JobDir(job.ID), 0755)

				_, _, _, convID, _ := d.runAgy(context.Background(), job, "prompt", "")
				if convID != tc.wantConv {
					t.Errorf("convID = %q, want %q", convID, tc.wantConv)
				}
			})
		}
	})
}

func TestCoverage_RunOxillama_Branches(t *testing.T) {
	t.Run("default endpoint and default client when nil", func(t *testing.T) {
		// Never reach the real local OxiLLaMa: the default endpoint comes from the environment.
		t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", "http://127.0.0.1:1")
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.OxillamaEndpoint = ""
		d.HTTPClient = nil

		job := &Job{
			ID:       "DEN-OXI-1",
			Ticket:   "DEN-OXI",
			Agent:    "oxillama",
			Worktree: t.TempDir(),
		}
		_ = os.MkdirAll(tempLedger.JobDir(job.ID), 0755)

		code, _, _, err := d.runOxillama(context.Background(), job, "prompt")
		if err == nil {
			t.Fatal("expected network error")
		}
		if code != 1 {
			t.Errorf("expected code 1, got %d", code)
		}
	})

	t.Run("invalid url fails http.NewRequestWithContext", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.OxillamaEndpoint = "http://invalid-url\x7f"

		job := &Job{
			ID:       "DEN-OXI-2",
			Ticket:   "DEN-OXI",
			Agent:    "oxillama",
			Worktree: t.TempDir(),
		}
		_ = os.MkdirAll(tempLedger.JobDir(job.ID), 0755)

		code, _, _, err := d.runOxillama(context.Background(), job, "prompt")
		if err == nil {
			t.Fatal("expected error from invalid URL")
		}
		if code != 1 {
			t.Errorf("expected code 1, got %d", code)
		}
	})

	t.Run("body read error via hijacked connection", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("short"))
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, _ := hijacker.Hijack()
				_ = conn.Close()
			}
		}))
		defer server.Close()

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.OxillamaEndpoint = server.URL

		job := &Job{
			ID:       "DEN-OXI-3",
			Ticket:   "DEN-OXI",
			Agent:    "oxillama",
			Worktree: t.TempDir(),
		}
		_ = os.MkdirAll(tempLedger.JobDir(job.ID), 0755)

		code, _, _, err := d.runOxillama(context.Background(), job, "prompt")
		if err == nil {
			t.Fatal("expected body read error")
		}
		if code != http.StatusOK {
			t.Errorf("expected status 200, got %d", code)
		}
	})

	t.Run("choices array is empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"choices": []any{},
				"usage":   map[string]any{"total_tokens": 10},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.OxillamaEndpoint = server.URL

		job := &Job{
			ID:       "DEN-OXI-4",
			Ticket:   "DEN-OXI",
			Agent:    "oxillama",
			Worktree: t.TempDir(),
		}
		_ = os.MkdirAll(tempLedger.JobDir(job.ID), 0755)

		code, _, usage, err := d.runOxillama(context.Background(), job, "prompt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != 0 {
			t.Errorf("expected code 0, got %d", code)
		}
		if usage == nil {
			t.Errorf("expected usage map")
		}
		outputPath := filepath.Join(tempLedger.JobDir(job.ID), "output.md")
		data, _ := os.ReadFile(outputPath)
		if len(data) != 0 {
			t.Errorf("expected empty output.md, got %q", string(data))
		}
	})
}

func TestCoverage_CheckScope_Branches(t *testing.T) {
	t.Run("empty worktree resolves to current dir and empty gitPath defaults to git", func(t *testing.T) {
		d := NewDispatcher(nil)
		d.GitPath = ""
		_, _, err := d.CheckScope(context.Background(), "", []string{"."})
		if err != nil {
			t.Fatalf("CheckScope failed: %v", err)
		}
	})

	t.Run("git status fails on non-git directory", func(t *testing.T) {
		d := NewDispatcher(nil)
		_, _, err := d.CheckScope(context.Background(), t.TempDir(), []string{"."})
		if err == nil {
			t.Fatal("expected git status error in non-git directory")
		}
	})

	t.Run("git rename and quoted filenames", func(t *testing.T) {
		repo := createGitRepo(t)
		oldFile := filepath.Join(repo, "old.txt")
		if err := os.WriteFile(oldFile, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", "old.txt")
		cmd.Dir = repo
		_ = cmd.Run()
		cmd = exec.Command("git", "commit", "-m", "add old")
		cmd.Dir = repo
		_ = cmd.Run()

		cmd = exec.Command("git", "mv", "old.txt", "new.txt")
		cmd.Dir = repo
		_ = cmd.Run()

		spaceFile := filepath.Join(repo, "spaced name.txt")
		if err := os.WriteFile(spaceFile, []byte("spaced"), 0644); err != nil {
			t.Fatal(err)
		}

		d := NewDispatcher(nil)
		inScope, offending, err := d.CheckScope(context.Background(), repo, []string{"", "  ", "new.txt", "spaced name.txt"})
		if err != nil {
			t.Fatalf("CheckScope failed: %v", err)
		}
		if !inScope {
			t.Errorf("expected inScope=true for renamed and spaced files")
		}
		if len(offending) > 0 {
			t.Errorf("unexpected offending files: %v", offending)
		}
	})

	t.Run("scope variations dot prefix and subpath prefix", func(t *testing.T) {
		repo := createGitRepo(t)
		subDir := filepath.Join(repo, "sub", "dir")
		_ = os.MkdirAll(subDir, 0755)
		_ = os.WriteFile(filepath.Join(subDir, "file.go"), []byte("package sub"), 0644)

		d := NewDispatcher(nil)

		inScope, offending, err := d.CheckScope(context.Background(), repo, []string{"."})
		if err != nil || !inScope || len(offending) > 0 {
			t.Errorf("scope '.' failed: inScope=%v, offending=%v, err=%v", inScope, offending, err)
		}

		inScope, offending, err = d.CheckScope(context.Background(), repo, []string{"sub"})
		if err != nil || !inScope || len(offending) > 0 {
			t.Errorf("scope 'sub' failed: inScope=%v, offending=%v, err=%v", inScope, offending, err)
		}

		inScope, offending, err = d.CheckScope(context.Background(), repo, []string{"sub/dir/file.go"})
		if err != nil || !inScope || len(offending) > 0 {
			t.Errorf("exact match failed: inScope=%v, offending=%v, err=%v", inScope, offending, err)
		}
	})
}

func TestCoverage_Retry_Branches(t *testing.T) {
	t.Run("nonexistent job ID returns error", func(t *testing.T) {
		d := NewDispatcher(NewLedger(t.TempDir()))
		err := d.Retry("nonexistent-job")
		if err == nil {
			t.Fatal("expected error retrying nonexistent job")
		}
	})

	t.Run("save failure returns error", func(t *testing.T) {
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

		err = d.Retry(id)
		if err == nil {
			t.Fatal("expected save error on Retry")
		}
	})

	t.Run("spawnRunner failure returns error", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")

		id, err := d.Submit(SubmitSpec{Prompt: "test"})
		if err != nil {
			t.Fatal(err)
		}

		d.Executable = ""
		err = d.Retry(id)
		if err != ErrSelfExecInTest {
			t.Fatalf("expected ErrSelfExecInTest, got %v", err)
		}
	})
}

func TestCoverage_Cancel_Branches(t *testing.T) {
	t.Run("nonexistent job ID returns error", func(t *testing.T) {
		d := NewDispatcher(NewLedger(t.TempDir()))
		err := d.Cancel("nonexistent-job")
		if err == nil {
			t.Fatal("expected error cancelling nonexistent job")
		}
	})

	t.Run("job with PGID 0 skips kill", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = installExecutableScript(t, t.TempDir(), "dummy", "exit 0\n")

		id, err := d.Submit(SubmitSpec{Prompt: "test"})
		if err != nil {
			t.Fatal(err)
		}

		job, _ := tempLedger.Load(id)
		job.PGID = 0
		_ = tempLedger.Save(job)

		err = d.Cancel(id)
		if err != nil {
			t.Fatalf("unexpected cancel error: %v", err)
		}
		job, _ = tempLedger.Load(id)
		if job.Status != StatusCancelled {
			t.Errorf("job.Status = %q, want %q", job.Status, StatusCancelled)
		}
	})

	t.Run("save failure returns error", func(t *testing.T) {
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

		err = d.Cancel(id)
		if err == nil {
			t.Fatal("expected save error on Cancel")
		}
	})
}

func TestCoverage_BackoffDuration_Capping(t *testing.T) {
	d := NewDispatcher(nil)

	d1 := d.backoffDuration(1)
	if d1 < 2*time.Second || d1 > 3*time.Second {
		t.Errorf("backoffDuration(1) = %v, want ~2s", d1)
	}

	d10 := d.backoffDuration(10)
	if d10 < 120*time.Second || d10 > 121*time.Second {
		t.Errorf("backoffDuration(10) = %v, want ~120s (capped)", d10)
	}
}

func TestCoverage_SpawnRunner_Branches(t *testing.T) {
	t.Run("empty executable in test triggers ErrSelfExecInTest", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = ""

		_, err := d.spawnRunner("job-123")
		if err != ErrSelfExecInTest {
			t.Fatalf("expected ErrSelfExecInTest, got %v", err)
		}
	})

	t.Run("log file open failure", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		binDir := t.TempDir()
		d.Executable = installExecutableScript(t, binDir, "dummy", "exit 0\n")

		jobID := "job-log-fail"
		logDir := tempLedger.AgentLogPath(jobID)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			t.Fatal(err)
		}

		_, err := d.spawnRunner(jobID)
		if err == nil {
			t.Fatal("expected error opening agent.log directory")
		}
		if !strings.Contains(err.Error(), "failed to open agent.log") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("cmd.Start failure with invalid binary", func(t *testing.T) {
		tempLedger := NewLedger(t.TempDir())
		d := NewDispatcher(tempLedger)
		d.Executable = "/nonexistent/binary/path"

		jobID := "job-start-fail"
		_ = os.MkdirAll(tempLedger.JobDir(jobID), 0755)

		_, err := d.spawnRunner(jobID)
		if err == nil {
			t.Fatal("expected error starting invalid binary")
		}
		if !strings.Contains(err.Error(), "failed to start job") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})
}
