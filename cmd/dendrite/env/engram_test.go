package env

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/engram"
	"Dendrite/pkg/testkit"
)

const standInDoctor = `{"status":"ok","summary":{"total":10,"ok":10,"warnings":0,"blocked":0,"errors":0}}`

// engramStandIn writes an executable engram that logs its arguments to $LOG and answers version and doctor.
func engramStandIn(t *testing.T, doctor string) (dir, log string) {
	t.Helper()
	dir, log = t.TempDir(), filepath.Join(t.TempDir(), "calls.log")
	body := "#!/bin/sh\necho \"$*\" >> \"$LOG\"\ncase \"$1\" in\n  version) echo 'engram 3.7.2' ;;\n  doctor) echo '" + doctor + "' ;;\nesac\n"
	testkit.WriteFile(t, filepath.Join(dir, "engram"), body, 0o755)
	testkit.WriteFile(t, filepath.Join(dir, "claude"), "#!/bin/sh\nexit 0\n", 0o755)
	t.Setenv("LOG", log)
	return dir, log
}

func TestDiagnoseEngram(t *testing.T) {
	ctx := context.Background()
	dir, _ := engramStandIn(t, standInDoctor)
	t.Setenv(engram.BinaryEnv, filepath.Join(dir, "engram"))
	var buf bytes.Buffer
	diagnoseEngram(ctx, &buf)
	testkit.Contains(t, buf.String(), "[✓] Engram: engram 3.7.2", "doctor ok (10/10 checks ok)")

	t.Setenv(engram.BinaryEnv, "")
	t.Setenv("PATH", t.TempDir())
	buf.Reset()
	diagnoseEngram(ctx, &buf)
	testkit.Contains(t, buf.String(), "[-] Engram:", "dendrite soma install engram")
}

func TestDiagnoseEngramReportsBrokenBinariesAndFailingChecks(t *testing.T) {
	ctx := context.Background()
	bad := filepath.Join(t.TempDir(), "engram")
	testkit.WriteFile(t, bad, "#!/bin/sh\nexit 1\n", 0o755)
	t.Setenv(engram.BinaryEnv, bad)
	var buf bytes.Buffer
	diagnoseEngram(ctx, &buf)
	testkit.Contains(t, buf.String(), "does not run")

	noReport := filepath.Join(t.TempDir(), "engram")
	testkit.WriteFile(t, noReport, "#!/bin/sh\ncase \"$1\" in version) echo v;; doctor) echo nope;; esac\n", 0o755)
	t.Setenv(engram.BinaryEnv, noReport)
	buf.Reset()
	diagnoseEngram(ctx, &buf)
	testkit.Contains(t, buf.String(), "doctor failed")

	dir, _ := engramStandIn(t, `{"status":"error","summary":{"total":10,"ok":8,"warnings":0,"blocked":0,"errors":2}}`)
	t.Setenv(engram.BinaryEnv, filepath.Join(dir, "engram"))
	buf.Reset()
	diagnoseEngram(ctx, &buf)
	testkit.Contains(t, buf.String(), "[-] Engram: engram 3.7.2")
}

func TestSomaInstallEngram(t *testing.T) {
	dir, log := engramStandIn(t, standInDoctor)
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	t.Setenv(engram.BinaryEnv, "")
	var buf bytes.Buffer
	testkit.Check(t, runCmd(context.Background(), &buf, "soma", "install", "engram", "--yes"))
	data, _ := os.ReadFile(log)
	testkit.Contains(t, string(data), "setup claude-code --mcp-only")
	testkit.NotContains(t, string(data), "plugin")
	testkit.Contains(t, buf.String(), "Engram MCP registered for claude-code")
}
