package memory

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) { clitest.Main(m) }

// rig isolates the Dendrite home, the state dir and the user's home of one test.
func rig(t *testing.T) (home string) {
	t.Helper()
	clitest.PlainOutput(t)
	home = t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	return home
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := clitest.RunWith(context.Background(), &out, NewMemoryCmd(), append([]string{"memory"}, args...)...)
	return out.String(), err
}

func TestSaveJournalsAndFlushSendsItToEngram(t *testing.T) {
	rig(t)
	log := testkit.StubEngram(t, `echo "$*" >> "$LOG"`)

	out, err := run(t, "save", "--type", "decision", "--ticket", "DEN-9", "Use X", "because Y")
	testkit.Check(t, err)
	if !strings.Contains(out, "Journaled") {
		t.Fatalf("save output = %q", out)
	}
	if p, _, _ := memory.DefaultJournal().Pending(); len(p) != 1 || p[0].Ticket != "DEN-9" || p[0].Type != "decision" {
		t.Fatalf("journal = %+v", p)
	}

	out, err = run(t, "flush")
	testkit.Check(t, err)
	if !strings.Contains(out, "1 memory item(s) flushed to Engram") {
		t.Fatalf("flush output = %q", out)
	}
	if data, _ := os.ReadFile(log); !strings.Contains(string(data), "save Use X because Y --type decision") {
		t.Fatalf("engram calls = %s", data)
	}
	out, _ = run(t, "flush")
	if !strings.Contains(out, "0 memory item(s)") {
		t.Fatalf("a second flush adds nothing: %q", out)
	}
}

func TestSaveRefusesAnEmptyMemoryAndSkipsWhenEngramIsOff(t *testing.T) {
	home := rig(t)
	if _, err := run(t, "save", "title", " "); err == nil {
		t.Fatal("an empty body must be refused")
	}
	testkit.Check(t, memory.SetOverride(home, memory.ClaudeOnly))
	out, err := run(t, "save", "t", "b")
	testkit.Check(t, err)
	if !strings.Contains(out, "nothing journaled") {
		t.Fatalf("output = %q", out)
	}
	if p, _, _ := memory.DefaultJournal().Pending(); len(p) != 0 {
		t.Fatalf("journal = %+v", p)
	}
}

func TestModeShowsSetsAndClearsTheOverride(t *testing.T) {
	rig(t)
	out, err := run(t, "mode")
	testkit.Check(t, err)
	if !strings.Contains(out, "Memory mode: both (default)") {
		t.Fatalf("mode = %q", out)
	}
	out, err = run(t, "mode", "set", "engram")
	testkit.Check(t, err)
	if !strings.Contains(out, "engram (override)") {
		t.Fatalf("set = %q", out)
	}
	out, _ = run(t, "mode", "set", "auto")
	if !strings.Contains(out, "both (default)") {
		t.Fatalf("auto = %q", out)
	}
	if _, err = run(t, "mode", "set", "loud"); cli.ExitCodeFor(err) != cli.ExitUsage {
		t.Fatalf("an unknown mode is a usage error, got %v", err)
	}
}

func TestModeReportsAComputedModeAndPrintsJSON(t *testing.T) {
	home := rig(t)
	testkit.Check(t, memory.SaveState(home, memory.State{Computed: memory.ClaudeOnly, Reason: "engram unhealthy"}))
	out, _ := run(t, "mode")
	if !strings.Contains(out, "claude (computed: engram unhealthy)") {
		t.Fatalf("mode = %q", out)
	}
	out, _ = run(t, "mode", "--json")
	var got map[string]any
	testkit.Check(t, json.Unmarshal([]byte(out), &got))
	testkit.Equal(t, "json mode", got["mode"], any("claude"))
}

func TestSyncMirrorsAGivenFolderAndTheDefaultOne(t *testing.T) {
	rig(t)
	log := testkit.StubEngram(t, `echo "$*" >> "$LOG"`)
	dir := t.TempDir()
	testkit.WriteFile(t, filepath.Join(dir, "a.md"), "---\nname: fact-a\ndescription: d\nmetadata:\n  type: project\n---\nbody\n", 0o644)

	out, err := run(t, "sync", "--dir", dir)
	testkit.Check(t, err)
	if !strings.Contains(out, "1 Claude memory file(s) mirrored") {
		t.Fatalf("sync = %q", out)
	}
	if data, _ := os.ReadFile(log); !strings.Contains(string(data), "save fact-a") {
		t.Fatalf("engram calls = %s", data)
	}

	repo := t.TempDir()
	prev := claudeRepo
	claudeRepo = func() string { return repo }
	t.Cleanup(func() { claudeRepo = prev })
	userHome, _ := os.UserHomeDir()
	testkit.Check(t, os.MkdirAll(memory.ClaudeDir(userHome, repo), 0o755))
	out, err = run(t, "sync")
	testkit.Check(t, err)
	if !strings.Contains(out, "0 Claude memory file(s) mirrored") {
		t.Fatalf("default folder = %q", out)
	}
	if _, err = run(t, "sync", "--dir", filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing folder must be reported")
	}
}

func TestClaudeRepoIsTheMainCheckoutOrTheWorkingDirectory(t *testing.T) {
	repo := t.TempDir()
	clitest.GitRun(t, repo, "init", "-q")
	t.Chdir(repo)
	got, _ := filepath.EvalSymlinks(claudeRepo())
	want, _ := filepath.EvalSymlinks(repo)
	testkit.Equal(t, "inside a repository", got, want)

	plain := t.TempDir()
	t.Chdir(plain)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(plain))
	got, _ = filepath.EvalSymlinks(claudeRepo())
	want, _ = filepath.EvalSymlinks(plain)
	testkit.Equal(t, "outside a repository", got, want)
}

func TestCommandsReportAnUnknownHome(t *testing.T) {
	rig(t)
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	for _, args := range [][]string{{"save", "t", "b"}, {"flush"}, {"mode"}, {"mode", "set", "off"}, {"sync"}} {
		if _, err := run(t, args...); err == nil {
			t.Errorf("memory %v must report an unknown home", args)
		}
	}
}

func TestFlushReportsEngramFailureAndModeSetReportsAnUnwritableHome(t *testing.T) {
	rig(t)
	testkit.StubEngram(t, "exit 1")
	_, err := memory.DefaultJournal().Append(memory.Entry{Title: "t", Body: "b"})
	testkit.Check(t, err)
	if _, err := run(t, "flush"); err == nil {
		t.Fatal("a failing engram must fail the flush")
	}

	file := filepath.Join(t.TempDir(), "file")
	testkit.WriteFile(t, file, "x", 0o644)
	t.Setenv("DENDRITE_HOME", file)
	if _, err := run(t, "mode", "set", "off"); err == nil {
		t.Fatal("an unwritable home must fail `mode set`")
	}
}

func TestSyncReportsAnUnknownUserHome(t *testing.T) {
	rig(t)
	t.Setenv("HOME", "")
	if _, err := run(t, "sync"); err == nil {
		t.Fatal("without a user home the default folder cannot be found")
	}
}
