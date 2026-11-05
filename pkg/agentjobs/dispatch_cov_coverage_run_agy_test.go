package agentjobs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

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
