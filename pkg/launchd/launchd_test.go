package launchd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var weekly = Job{Label: "dev.test.job", Binary: "/bin/dendrite", Args: []string{"memory", "check"}, LogName: "check", Weekday: 1, Hour: 9, Minute: 30}

func TestPlistRunsTheBinaryWithItsArgsAtTheScheduledTime(t *testing.T) {
	p := weekly.Plist("/tmp/logs")
	for _, want := range []string{
		"<string>dev.test.job</string>", "<string>/bin/dendrite</string>", "<string>memory</string>", "<string>check</string>",
		"<key>Weekday</key>\n        <integer>1</integer>", "<key>Minute</key>\n        <integer>30</integer>",
		"/tmp/logs/check.log", "/tmp/logs/check.err",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist lacks %q:\n%s", want, p)
		}
	}
}

func TestAgentsDirDefaultsToTheUsersLaunchAgents(t *testing.T) {
	got, err := AgentsDir("", func() (string, error) { return "/Users/me", nil })
	if err != nil || got != "/Users/me/Library/LaunchAgents" {
		t.Fatalf("AgentsDir = %q, %v", got, err)
	}
	if got, _ := AgentsDir("/x", nil); got != "/x" {
		t.Fatalf("an explicit directory wins, got %q", got)
	}
	if _, err := AgentsDir("", func() (string, error) { return "", errors.New("no home") }); err == nil {
		t.Fatal("a missing home must be reported")
	}
}

func TestInstallWritesThePlistAndRemoveDeletesItTwiceOver(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agents")
	path, err := weekly.Install(dir, t.TempDir())
	if err != nil || path != filepath.Join(dir, "dev.test.job.plist") {
		t.Fatalf("Install = %q, %v", path, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := weekly.Remove(dir); err != nil {
			t.Fatalf("Remove #%d: %v", i, err)
		}
	}
}

func TestInstallAndRemoveReportAnUnwritableDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := weekly.Install(filepath.Join(file, "agents"), t.TempDir()); err == nil {
		t.Fatal("Install must fail when the directory cannot be created")
	}
	if _, err := weekly.Remove(filepath.Join(file, "agents")); err == nil {
		t.Fatal("Remove must report an error other than not-exist")
	}
}

func TestLoadBootsOutThenBootstrapsAndReportsLaunchctlsMessage(t *testing.T) {
	var calls []string
	run := func(args ...string) ([]byte, error) { calls = append(calls, args[0]); return nil, nil }
	if err := Load(run, "/p.plist"); err != nil || strings.Join(calls, ",") != "bootout,bootstrap" {
		t.Fatalf("calls = %v, err = %v", calls, err)
	}
	failing := func(args ...string) ([]byte, error) {
		if args[0] == "bootstrap" {
			return []byte("Bootstrap failed: 5\n"), errors.New("exit 5")
		}
		return nil, nil
	}
	if err := Load(failing, "/p.plist"); err == nil || !strings.Contains(err.Error(), "Bootstrap failed: 5") {
		t.Fatalf("err = %v, want launchctl's message", err)
	}
	calls = nil
	Unload(run, "/p.plist")
	if len(calls) != 1 || calls[0] != "bootout" {
		t.Fatalf("Unload calls = %v", calls)
	}
}

func TestExecAndDomainAreTheRealLaunchctlAndTheUsersDomain(t *testing.T) {
	if !strings.HasPrefix(Domain(), "gui/") {
		t.Fatalf("Domain = %q", Domain())
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := Exec("print"); err == nil {
		t.Fatal("Exec must run launchctl from PATH (and fail when it is absent)")
	}
}

func TestLogDirIsUnderTheDendriteHomeOrTheTempDir(t *testing.T) {
	home := t.TempDir()
	got := LogDir(func() (string, error) { return home, nil })
	if got != filepath.Join(home, "logs") {
		t.Fatalf("LogDir = %q", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("the log directory must exist: %v", err)
	}
	if got := LogDir(func() (string, error) { return "", errors.New("no home") }); got != os.TempDir() {
		t.Fatalf("without a home LogDir = %q, want the temp dir", got)
	}
}
