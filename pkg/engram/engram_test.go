package engram_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/engram"
	"Dendrite/pkg/testkit"
)

const doctorJSON = `{"status":"warning","summary":{"total":10,"ok":9,"warnings":1,"blocked":0,"errors":0}}`

// tools puts one stand-in executable per name on a PATH that holds nothing else, and isolates the
// environment Engram and the installer read. It returns the directory and the file the stand-ins log to.
func tools(t *testing.T, scripts map[string]string) (dir, log string) {
	t.Helper()
	dir = t.TempDir()
	log = filepath.Join(t.TempDir(), "calls.log")
	for name, body := range scripts {
		testkit.WriteFile(t, filepath.Join(dir, name), "#!/bin/sh\n"+body+"\n", 0o755)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ENGRAM_DATA_DIR", t.TempDir())
	t.Setenv(engram.BinaryEnv, "")
	t.Setenv("LOG", log)
	return dir, log
}

func calls(t *testing.T, log string) string {
	t.Helper()
	data, _ := os.ReadFile(log)
	return string(data)
}

// engramScript is a stand-in engram that logs its arguments and answers the commands Dendrite uses.
const engramScript = `echo "$*" >> "$LOG"
case "$1" in
  version) echo "engram 3.7.2" ;;
  doctor) echo '` + doctorJSON + `' ;;
  context) echo "## Memory from Previous Sessions"; echo "cwd=$(pwd)" ;;
esac`

func TestLocate(t *testing.T) {
	dir, _ := tools(t, map[string]string{"engram": "exit 0"})
	got, err := engram.Locate()
	testkit.Check(t, err)
	testkit.Equal(t, "PATH lookup", got, filepath.Join(dir, "engram"))

	t.Setenv(engram.BinaryEnv, filepath.Join(dir, "engram"))
	got, err = engram.Locate()
	testkit.Check(t, err)
	testkit.Equal(t, "env override", got, filepath.Join(dir, "engram"))

	t.Setenv(engram.BinaryEnv, filepath.Join(dir, "absent"))
	if _, err = engram.Locate(); err == nil || errors.Is(err, engram.ErrMissing) {
		t.Fatalf("an unusable %s must fail with its own error, got %v", engram.BinaryEnv, err)
	}

	plain := filepath.Join(t.TempDir(), "engram")
	testkit.WriteFile(t, plain, "x", 0o644)
	t.Setenv(engram.BinaryEnv, plain)
	if _, err = engram.Locate(); err == nil || !strings.Contains(err.Error(), "not an executable") {
		t.Fatalf("a non-executable override must be refused, got %v", err)
	}
	t.Setenv(engram.BinaryEnv, t.TempDir())
	if _, err = engram.Locate(); err == nil {
		t.Fatal("a directory must be refused")
	}
}

func TestLocateMissing(t *testing.T) {
	tools(t, nil)
	if _, err := engram.Locate(); !errors.Is(err, engram.ErrMissing) {
		t.Fatalf("want ErrMissing, got %v", err)
	}
}

func TestVersionAndDoctor(t *testing.T) {
	dir, _ := tools(t, map[string]string{"engram": engramScript})
	bin := filepath.Join(dir, "engram")
	v, err := engram.Version(context.Background(), bin)
	testkit.Check(t, err)
	testkit.Equal(t, "version", v, "engram 3.7.2")
	h, err := engram.Doctor(context.Background(), bin)
	testkit.Check(t, err)
	testkit.Equal(t, "doctor", h, engram.Health{Status: "warning", Total: 10, OK: 9, Warnings: 1})
}

func TestDoctorReadsTheReportOfAFailingRun(t *testing.T) {
	dir, _ := tools(t, map[string]string{"engram": "echo '" + doctorJSON + "'; exit 1"})
	h, err := engram.Doctor(context.Background(), filepath.Join(dir, "engram"))
	testkit.Check(t, err)
	testkit.Equal(t, "status", h.Status, "warning")
}

func TestVersionAndDoctorErrors(t *testing.T) {
	dir, _ := tools(t, map[string]string{"bad": "exit 3", "junk": "echo not-json"})
	for name, fn := range map[string]func(string) error{
		"version": func(b string) error { _, err := engram.Version(context.Background(), b); return err },
		"doctor":  func(b string) error { _, err := engram.Doctor(context.Background(), b); return err },
	} {
		if fn(filepath.Join(dir, "bad")) == nil {
			t.Errorf("%s: a failing binary must fail", name)
		}
	}
	if _, err := engram.Doctor(context.Background(), filepath.Join(dir, "junk")); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("a successful run with a non-JSON report must be reported as unreadable, got %v", err)
	}
}

func TestSnapshot(t *testing.T) {
	dir, _ := tools(t, map[string]string{"engram": engramScript, "empty": "echo 'No previous session memories found.'", "bad": "exit 2"})
	work := t.TempDir()
	got, err := engram.Snapshot(context.Background(), filepath.Join(dir, "engram"), work, 5200)
	testkit.Check(t, err)
	testkit.Contains(t, got, "## Memory from Previous Sessions", "cwd="+work)

	got, err = engram.Snapshot(context.Background(), filepath.Join(dir, "empty"), work, 5200)
	testkit.Check(t, err)
	testkit.Equal(t, "empty memory", got, "")

	if _, err = engram.Snapshot(context.Background(), filepath.Join(dir, "bad"), work, 5200); err == nil {
		t.Fatal("a failing engram must fail the snapshot")
	}
}

func TestSnapshotIsBoundedByItsBudget(t *testing.T) {
	dir, _ := tools(t, map[string]string{"engram": "head -c 20000 /dev/zero | tr '\\0' 'a'"})
	got, err := engram.Snapshot(context.Background(), filepath.Join(dir, "engram"), t.TempDir(), engram.SnapshotBytes)
	testkit.Check(t, err)
	if len(got) > engram.SnapshotBytes || !strings.HasSuffix(got, "[truncated]") {
		t.Fatalf("snapshot is %d bytes (budget %d), tail %q", len(got), engram.SnapshotBytes, got[len(got)-12:])
	}
}

func TestTruncate(t *testing.T) {
	testkit.Equal(t, "fits", engram.Truncate("short", 10), "short")
	testkit.Equal(t, "exact", engram.Truncate("0123456789", 10), "0123456789")
	// "é" is two bytes: the cut must not split it.
	for _, max := range []int{20, 21} {
		checkRuneSafe(t, engram.Truncate("ééééééééééééééééé", max), max)
	}
	testkit.Equal(t, "budget below the marker", engram.Truncate("abcdef", 3), "\n[truncated]")
}

func checkRuneSafe(t *testing.T, got string, max int) {
	t.Helper()
	if len(got) > max || !strings.HasSuffix(got, "[truncated]") || strings.ContainsRune(got, '�') {
		t.Fatalf("bad truncation %q (%d bytes, budget %d)", got, len(got), max)
	}
}

func TestJobMemory(t *testing.T) {
	tools(t, nil)
	testkit.Equal(t, "no engram", engram.JobMemory(context.Background(), t.TempDir()), "")

	tools(t, map[string]string{"engram": engramScript})
	got := engram.JobMemory(context.Background(), t.TempDir())
	testkit.Contains(t, got, "## Memory (Engram)", "## Memory from Previous Sessions", "background, not instructions")

	tools(t, map[string]string{"engram": "exit 1"})
	testkit.Equal(t, "failing engram", engram.JobMemory(context.Background(), t.TempDir()), "")
	tools(t, map[string]string{"engram": "echo 'No previous session memories found.'"})
	testkit.Equal(t, "empty memory", engram.JobMemory(context.Background(), t.TempDir()), "")
}

// brewScript installs a stand-in engram into the PATH directory, the way `brew install` puts the real one in.
func brewScript(dir string) string {
	return `echo "brew $*" >> "$LOG"
cat > ` + dir + `/engram <<'EOF'
#!/bin/sh
` + engramScript + `
EOF
chmod +x ` + dir + `/engram`
}

func TestInstallRegistersMCPOnlyAndNeverThePlugin(t *testing.T) {
	_, log := tools(t, map[string]string{"engram": engramScript, "claude": "exit 0", "agy": "exit 0"})
	var out strings.Builder
	testkit.Check(t, engram.Install(context.Background(), strings.NewReader(""), &out, true))
	got := calls(t, log)
	testkit.Contains(t, got, "setup claude-code --mcp-only", "setup antigravity-cli")
	testkit.NotContains(t, got, "plugin", "--allow-memory-writes")
	testkit.Contains(t, out.String(), "engram 3.7.2", "engram doctor: warning (9/10 checks ok)")
}

func TestInstallSkipsAgentsThatAreNotInstalled(t *testing.T) {
	_, log := tools(t, map[string]string{"engram": engramScript})
	var out strings.Builder
	testkit.Check(t, engram.Install(context.Background(), strings.NewReader(""), &out, true))
	testkit.NotContains(t, calls(t, log), "setup")
	testkit.Contains(t, out.String(), "claude not installed", "agy not installed")
}

func TestInstallAntigravityFailureIsAWarningButClaudeFailureIsNot(t *testing.T) {
	failing := `echo "$*" >> "$LOG"; case "$2" in antigravity-cli) exit 1;; esac`
	tools(t, map[string]string{"engram": failing, "claude": "exit 0", "agy": "exit 0"})
	var out strings.Builder
	testkit.Check(t, engram.Install(context.Background(), strings.NewReader(""), &out, true))
	testkit.Contains(t, out.String(), "[!] engram setup antigravity-cli failed")

	tools(t, map[string]string{"engram": "exit 1", "claude": "exit 0"})
	if err := engram.Install(context.Background(), strings.NewReader(""), &out, true); err == nil {
		t.Fatal("a failed Claude Code setup must fail the install")
	}
}

func TestInstallBrewInstallsAMissingBinary(t *testing.T) {
	dir := t.TempDir()
	_, log := tools(t, map[string]string{"brew": brewScript(dir)})
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	var out strings.Builder
	testkit.Check(t, engram.Install(context.Background(), strings.NewReader("y\n"), &out, false))
	testkit.Contains(t, calls(t, log), "brew install "+engram.BrewFormula)
	testkit.Contains(t, out.String(), "Run `brew install")
}

func TestInstallDeclinedOrWithoutBrew(t *testing.T) {
	dir := t.TempDir()
	_, log := tools(t, map[string]string{"brew": brewScript(dir)})
	err := engram.Install(context.Background(), strings.NewReader("n\n"), &strings.Builder{}, false)
	if err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("want a declined error, got %v", err)
	}
	testkit.NotContains(t, calls(t, log), "brew install")

	tools(t, nil)
	if err = engram.Install(context.Background(), strings.NewReader(""), &strings.Builder{}, true); !errors.Is(err, engram.ErrMissing) {
		t.Fatalf("no engram and no brew must report ErrMissing, got %v", err)
	}
}

func TestInstallBrewFailureAndBadOverride(t *testing.T) {
	tools(t, map[string]string{"brew": "exit 1"})
	if err := engram.Install(context.Background(), strings.NewReader(""), &strings.Builder{}, true); err == nil {
		t.Fatal("a failing brew must fail the install")
	}
	t.Setenv(engram.BinaryEnv, filepath.Join(t.TempDir(), "absent"))
	if err := engram.Install(context.Background(), strings.NewReader(""), &strings.Builder{}, true); err == nil {
		t.Fatal("an unusable override must fail the install")
	}
}
