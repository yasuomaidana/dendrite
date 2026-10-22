package agentjobs

import (
	"os"
	"strings"
	"testing"
)

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
