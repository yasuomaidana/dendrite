package agentjobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// agent.log must fill while agy runs, not when it ends, and agy must be told where to write its own log.
func TestRunAgyStreamsOutputToAgentLogWhileRunning(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	argsFile := filepath.Join(t.TempDir(), "args")
	d.AgyPath = installExecutableScript(t, t.TempDir(), "agy", `echo "$@" > `+argsFile+`
echo "first line"
echo "warning on stderr" >&2
sleep 1
echo '{"conversation_id":"conv-live-1","status":"SUCCESS"}'
`)
	job := &Job{ID: "DEN-LIVE-1", Ticket: "DEN-LIVE", Agent: "agy", Model: "m", Worktree: t.TempDir()}
	_ = os.MkdirAll(ledger.JobDir(job.ID), 0o755)

	type result struct {
		conv string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		_, _, _, conv, err := d.runAgy(context.Background(), job, "prompt", "")
		done <- result{conv, err}
	}()

	deadline := time.Now().Add(5 * time.Second)
	var seen string
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(ledger.AgentLogPath(job.ID))
		seen = string(data)
		if strings.Contains(seen, "first line") && strings.Contains(seen, "warning on stderr") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(seen, "first line") || !strings.Contains(seen, "warning on stderr") {
		t.Fatalf("agent.log did not fill while agy was running:\n%q", seen)
	}
	select {
	case <-done:
		t.Fatal("agy had already finished: the log was not read while it ran")
	default:
	}

	r := <-done
	if r.err != nil || r.conv != "conv-live-1" {
		t.Fatalf("conversation = %q, err = %v", r.conv, r.err)
	}
	final, _ := os.ReadFile(ledger.AgentLogPath(job.ID))
	if !strings.Contains(string(final), "conv-live-1") {
		t.Errorf("the result envelope must reach agent.log too:\n%s", final)
	}
	args, _ := os.ReadFile(argsFile)
	if want := "--log-file " + ledger.AgyLogPath(job.ID); !strings.Contains(string(args), want) {
		t.Errorf("agy was started with %q, want it to contain %q", strings.TrimSpace(string(args)), want)
	}
}

// When agent.log cannot be opened the run still works; the result just is not mirrored.
func TestRunAgyWorksWhenAgentLogCannotBeOpened(t *testing.T) {
	ledger := NewLedger(t.TempDir())
	d := NewDispatcher(ledger)
	d.AgyPath = installExecutableScript(t, t.TempDir(), "agy", `echo '{"conversation_id":"conv-nolog","status":"SUCCESS"}'`+"\n")
	job := &Job{ID: "DEN-LIVE-2", Ticket: "DEN-LIVE", Agent: "agy", Model: "m", Worktree: t.TempDir()}
	_ = os.MkdirAll(ledger.AgentLogPath(job.ID), 0o755) // a directory where the file should be

	_, logTail, _, conv, err := d.runAgy(context.Background(), job, "prompt", "")
	if err != nil || conv != "conv-nolog" || !strings.Contains(logTail, "conv-nolog") {
		t.Errorf("conv = %q, logTail = %q, err = %v", conv, logTail, err)
	}
}
