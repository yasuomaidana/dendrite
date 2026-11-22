package memory

import (
	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const healthyEngram = `case "$1" in
  doctor) echo '{"status":"ok","summary":{"total":10,"ok":10,"warnings":0,"blocked":0,"errors":0}}' ;;
  export) printf '{"observations":[]}' > "$2" ;;
  context) echo "## Memory" ;;
  save) echo saved ;;
esac`

// claudeFolderOf writes one memory into a folder, points the default repository at a temp dir, and returns the folder.
func claudeFolderOf(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testkit.WriteFile(t, filepath.Join(dir, "fact.md"), "---\nname: fact\ndescription: a fact\n---\nbody\n", 0o644)
	testkit.WriteFile(t, filepath.Join(dir, "MEMORY.md"), "- [fact](fact.md) — a fact\n", 0o644)
	repo := t.TempDir()
	prev := claudeRepo
	claudeRepo = func() string { return repo }
	t.Cleanup(func() { claudeRepo = prev })
	return dir
}

func TestCheckScoresBothStoresRecordsItAndPrintsTheMode(t *testing.T) {
	home := rig(t)
	testkit.StubEngram(t, healthyEngram)
	dir := claudeFolderOf(t)

	out, err := run(t, "check", "--dir", dir)
	testkit.Check(t, err)
	testkit.Contains(t, out, "Memory check", "Claude", "Engram", "The scores point to:", "Mode in force: both")
	if _, ok := memory.LastCheck(home); !ok {
		t.Fatal("the check must be recorded")
	}

	out, err = run(t, "check", "--dir", dir, "--quiet")
	testkit.Check(t, err)
	testkit.Equal(t, "quiet", out, "")
}

func TestCheckWithoutEngramSwitchesToClaudeAtOnce(t *testing.T) {
	home := rig(t)
	t.Setenv("PATH", t.TempDir())
	out, err := run(t, "check", "--dir", claudeFolderOf(t))
	testkit.Check(t, err)
	testkit.Contains(t, out, "engram is not installed", "Mode in force: claude")
	testkit.Equal(t, "computed", memory.LoadState(home).Computed, memory.ClaudeOnly)
}

func TestCheckShowsAHandSetOverrideAndPrintsJSON(t *testing.T) {
	home := rig(t)
	t.Setenv("PATH", t.TempDir())
	testkit.Check(t, memory.SetOverride(home, memory.Off))
	out, err := run(t, "check", "--dir", claudeFolderOf(t))
	testkit.Check(t, err)
	testkit.Contains(t, out, "Mode in force: off (override)")

	out, err = run(t, "check", "--dir", claudeFolderOf(t), "--json")
	testkit.Check(t, err)
	var got struct{ Mode string }
	testkit.Check(t, json.Unmarshal([]byte(out), &got))
	testkit.Equal(t, "json mode", got.Mode, "off")
}

func TestCheckReportsWhatCannotBeDone(t *testing.T) {
	rig(t)
	if _, err := run(t, "check", "--dir", filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing Claude memory folder must be reported")
	}

	testkit.StubEngram(t, `case "$1" in save) exit 1 ;; *) exit 0 ;; esac`)
	if _, err := run(t, "check", "--dir", claudeFolderOf(t)); err == nil {
		t.Fatal("a failing mirror must be reported")
	}

	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", "")
	if _, err := run(t, "check"); err == nil {
		t.Fatal("without a user home the default folder cannot be found")
	}
	t.Setenv("DENDRITE_HOME", "")
	if _, err := run(t, "check"); err == nil {
		t.Fatal("without a Dendrite home the check cannot run")
	}
}

func TestCheckReportsAnUnwritableRecordOrState(t *testing.T) {
	for name, block := range map[string]func(home string){
		"record": func(home string) { testkit.WriteFile(t, filepath.Join(home, "memory"), "x", 0o644) },
		"state":  func(home string) { _ = os.MkdirAll(memory.StatePath(home), 0o755) },
	} {
		t.Run(name, func(t *testing.T) {
			home := rig(t)
			t.Setenv("PATH", t.TempDir())
			block(home)
			if _, err := run(t, "check", "--dir", claudeFolderOf(t)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// launchctlLog replaces launchctl with a recorder (and the platform with macOS) for one test.
func launchctlLog(t *testing.T, fail bool) *[]string {
	t.Helper()
	var calls []string
	prevCtl, prevOS := launchctl, goos
	launchctl = func(args ...string) ([]byte, error) {
		calls = append(calls, args[0])
		if fail && args[0] == "bootstrap" {
			return []byte("Bootstrap failed"), errors.New("exit 5")
		}
		return nil, nil
	}
	goos = "darwin"
	t.Cleanup(func() { launchctl, goos = prevCtl, prevOS })
	return &calls
}

func TestScheduleInstallsLoadsAndRemovesTheWeeklyJob(t *testing.T) {
	rig(t)
	calls := launchctlLog(t, false)
	dir := t.TempDir()

	out, err := run(t, "schedule", "--agents-dir", dir)
	testkit.Check(t, err)
	testkit.Contains(t, out, "dev.dendrite.memory.plist", "Mondays 09:30", "Loaded into launchd")
	plist, _ := os.ReadFile(filepath.Join(dir, "dev.dendrite.memory.plist"))
	testkit.Contains(t, string(plist), "<string>memory</string>", "<string>check</string>", "<string>--quiet</string>")
	testkit.Equal(t, "launchctl calls", strings.Join(*calls, ","), "bootout,bootstrap")

	*calls = nil
	out, err = run(t, "schedule", "--remove", "--agents-dir", dir)
	testkit.Check(t, err)
	testkit.Contains(t, out, "removed")
	testkit.Equal(t, "remove calls", strings.Join(*calls, ","), "bootout")
	if _, err := os.Stat(filepath.Join(dir, "dev.dendrite.memory.plist")); !os.IsNotExist(err) {
		t.Fatalf("the plist must be gone: %v", err)
	}

	*calls = nil
	_, err = run(t, "schedule", "--no-load", "--agents-dir", dir)
	testkit.Check(t, err)
	testkit.Equal(t, "no-load touches no launchctl", len(*calls), 0)
}

func TestScheduleOffMacPrintsTheCronLine(t *testing.T) {
	rig(t)
	prev := goos
	goos = "linux"
	t.Cleanup(func() { goos = prev })
	out, err := run(t, "schedule")
	testkit.Check(t, err)
	testkit.Contains(t, out, "launchd is not available", "30 9 * * 1", "memory check --quiet")
}

func TestScheduleReportsWhatGoesWrong(t *testing.T) {
	rig(t)
	launchctlLog(t, true)
	if _, err := run(t, "schedule", "--agents-dir", t.TempDir()); err == nil || !strings.Contains(err.Error(), "Bootstrap failed") {
		t.Fatalf("a launchctl failure must carry its message, got %v", err)
	}

	file := filepath.Join(t.TempDir(), "file")
	testkit.WriteFile(t, file, "x", 0o644)
	if _, err := run(t, "schedule", "--agents-dir", filepath.Join(file, "agents")); err == nil {
		t.Fatal("an unwritable agents directory must be reported")
	}

	dir := t.TempDir()
	plist := filepath.Join(dir, "dev.dendrite.memory.plist")
	testkit.WriteFile(t, filepath.Join(plist, "child"), "x", 0o644) // a non-empty directory cannot be removed
	if _, err := run(t, "schedule", "--remove", "--agents-dir", dir); err == nil {
		t.Fatal("a plist that cannot be removed must be reported")
	}

	t.Setenv("HOME", "")
	if _, err := run(t, "schedule"); err == nil {
		t.Fatal("without a home there is no LaunchAgents directory")
	}
}

func TestScheduleFallsBackToThePlainCommandNameWhenTheBinaryPathIsUnknown(t *testing.T) {
	rig(t)
	launchctlLog(t, false)
	prev := executable
	executable = func() (string, error) { return "", errors.New("unknown") }
	t.Cleanup(func() { executable = prev })
	dir := t.TempDir()
	_, err := run(t, "schedule", "--no-load", "--agents-dir", dir)
	testkit.Check(t, err)
	plist, _ := os.ReadFile(filepath.Join(dir, "dev.dendrite.memory.plist"))
	testkit.Contains(t, string(plist), "<string>dendrite</string>")
}
