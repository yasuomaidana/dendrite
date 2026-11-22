package session

import (
	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// isolate points the journal and the memory mode of one test at their own directories.
func isolate(t *testing.T, home string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", home)
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
}

func TestCheckpointJournalsDecisionsAndNotesAndFlushesThemToEngram(t *testing.T) {
	isolate(t, t.TempDir())
	log := testkit.StubEngram(t, `echo "$*" >> "$LOG"`)
	var out bytes.Buffer

	rememberCheckpoint(context.Background(), SessionCheckpointOptions{Decisions: []string{"keep X"}, Notes: []string{"blocked on CI"}}, "DEN-9", &out)

	if !strings.Contains(out.String(), "2 memory item(s) flushed to Engram") {
		t.Fatalf("output = %q", out.String())
	}
	data, _ := os.ReadFile(log)
	for _, want := range []string{"save Decision: keep X keep X --type decision", "save Note: blocked on CI blocked on CI --type note"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("engram calls missing %q in:\n%s", want, data)
		}
	}
	if p, _, _ := memory.DefaultJournal().Pending(); len(p) != 0 {
		t.Errorf("flushed entries must not stay pending: %+v", p)
	}
}

func TestCheckpointSaysNothingWithoutMemoryAndReportsAnEngramFailure(t *testing.T) {
	isolate(t, t.TempDir())
	var out bytes.Buffer
	rememberCheckpoint(context.Background(), SessionCheckpointOptions{}, "", &out)
	testkit.Equal(t, "nothing to remember", out.Len(), 0)

	testkit.StubEngram(t, "echo busy >&2; exit 1")
	rememberCheckpoint(context.Background(), SessionCheckpointOptions{Notes: []string{"n"}}, "", &out)
	if !strings.Contains(out.String(), "[!] memory not flushed to Engram") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCheckpointMemoryIsSkippedWhenTheHomeIsUnknown(t *testing.T) {
	isolate(t, "")
	t.Setenv("HOME", "")
	var out bytes.Buffer
	rememberCheckpoint(context.Background(), SessionCheckpointOptions{Notes: []string{"n"}}, "", &out)
	testkit.Equal(t, "output", out.Len(), 0)
	if p, _, _ := memory.DefaultJournal().Pending(); len(p) != 0 {
		t.Errorf("nothing may be journaled without a home: %+v", p)
	}
}

func TestResumeReportsAModeTheWeeklyCheckSwitched(t *testing.T) {
	home := t.TempDir()
	isolate(t, home)
	var out bytes.Buffer
	printMemoryMode(&out)
	testkit.Equal(t, "nothing switched", out.Len(), 0)

	testkit.Check(t, memory.SaveState(home, memory.State{Computed: memory.ClaudeOnly, Reason: "engram unhealthy", Since: time.Now()}))
	printMemoryMode(&out)
	testkit.Contains(t, out.String(), "Memory mode is claude", "engram unhealthy")

	out.Reset()
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	printMemoryMode(&out)
	testkit.Equal(t, "no home", out.Len(), 0)
}
