package hook

import (
	"Dendrite/pkg/engram"
	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubEngram puts a stand-in engram first on PATH (isolating its data) and returns the file it logs to.
func stubEngram(t *testing.T, body string) string {
	t.Helper()
	dir, log := t.TempDir(), filepath.Join(t.TempDir(), "calls.log")
	testkit.WriteFile(t, filepath.Join(dir, "engram"), "#!/bin/sh\n"+body+"\n", 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ENGRAM_DATA_DIR", t.TempDir())
	t.Setenv(engram.BinaryEnv, "")
	t.Setenv("LOG", log)
	return log
}

func pendingMemory(t *testing.T) []memory.Entry {
	t.Helper()
	p, _, err := memory.DefaultJournal().Pending()
	testkit.Check(t, err)
	return p
}

func TestDecisionHookJournalsTheAnswers(t *testing.T) {
	r := newHookRig(t, "To Do")
	var out bytes.Buffer
	testkit.Check(t, runDecision(context.Background(), r.askEvent(), &out))

	got := pendingMemory(t)
	if len(got) != 1 || got[0].Type != "decision" || !strings.Contains(got[0].Title, "Which model?") || !strings.Contains(got[0].Body, "A: ") {
		t.Fatalf("journal = %+v", got)
	}
}

func TestPlanHookJournalsThePlanWithItsTicket(t *testing.T) {
	r := newHookRig(t, "To Do")
	r.claim()
	var out bytes.Buffer
	testkit.Check(t, runPlanSaved(context.Background(), r.event("ExitPlanMode", "", "# Ship it\n- [ ] one step"), &out))

	got := pendingMemory(t)
	if len(got) != 1 || got[0].Type != "plan" || got[0].Ticket != "DEN-377" || got[0].Title != "Plan: Ship it" {
		t.Fatalf("journal = %+v", got)
	}
	testkit.Check(t, runPlanSaved(context.Background(), r.event("ExitPlanMode", "", ""), &out))
	testkit.Equal(t, "an empty plan journals nothing", len(pendingMemory(t)), 1)
}

func TestNothingIsJournaledWhenTheModeIsClaudeOnlyOrHomeIsUnknown(t *testing.T) {
	r := newHookRig(t, "To Do")
	var out bytes.Buffer
	testkit.Check(t, memory.SetOverride(os.Getenv("DENDRITE_HOME"), memory.ClaudeOnly))
	testkit.Check(t, runDecision(context.Background(), r.askEvent(), &out))
	testkit.Equal(t, "claude-only", len(pendingMemory(t)), 0)

	prev := memoryHome
	memoryHome = func() (string, error) { return "", errors.New("no home") }
	t.Cleanup(func() { memoryHome = prev })
	testkit.Check(t, runDecision(context.Background(), r.askEvent(), &out))
	testkit.Check(t, runSessionEnd(context.Background(), r.event("", "", ""), &out))
	testkit.Equal(t, "no home", len(pendingMemory(t)), 0)
}

func TestSessionEndFlushesTheJournalIntoEngramOnce(t *testing.T) {
	r := newHookRig(t, "To Do")
	log := stubEngram(t, `echo "$*" >> "$LOG"`)
	var out bytes.Buffer
	testkit.Check(t, runDecision(context.Background(), r.askEvent(), &out))

	out.Reset()
	testkit.Check(t, runSessionEnd(context.Background(), r.event("", "", ""), &out))
	if !strings.Contains(out.String(), "1 memory item(s) flushed to Engram") {
		t.Fatalf("output = %q", out.String())
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "save Decision: Which model?") {
		t.Fatalf("engram calls = %q", data)
	}

	out.Reset()
	testkit.Check(t, runSessionEnd(context.Background(), r.event("", "", ""), &out))
	testkit.Equal(t, "second session end is silent", out.Len(), 0)
}

func TestSessionEndReportsAnEngramFailureAndKeepsTheJournal(t *testing.T) {
	r := newHookRig(t, "To Do")
	stubEngram(t, "echo busy >&2; exit 1")
	var out bytes.Buffer
	testkit.Check(t, runDecision(context.Background(), r.askEvent(), &out))

	out.Reset()
	testkit.Check(t, runSessionEnd(context.Background(), r.event("", "", ""), &out))
	if !strings.Contains(out.String(), "[!]") || !strings.Contains(out.String(), "engram save") {
		t.Fatalf("output = %q", out.String())
	}
	testkit.Equal(t, "journal kept", len(pendingMemory(t)), 1)
}

func TestClipCutsOnACharacterBoundary(t *testing.T) {
	testkit.Equal(t, "short", clip("abc", 5), "abc")
	testkit.Equal(t, "cut", clip("abcdef", 3), "abc...")
	testkit.Equal(t, "multi-byte", clip("aé", 2), "a...") // é is 2 bytes: cutting at 2 would split it
}
