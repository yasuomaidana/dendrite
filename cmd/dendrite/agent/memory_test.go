package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/engram"
	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
)

func engramThatSays(t *testing.T, say string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "engram")
	testkit.WriteFile(t, bin, "#!/bin/sh\necho '"+say+"'\n", 0o755)
	t.Setenv(engram.BinaryEnv, bin)
}

func TestWithMemoryPutsTheSnapshotBeforeThePrompt(t *testing.T) {
	engramThatSays(t, "- [manual] **Chose Engram**")
	file := filepath.Join(t.TempDir(), "prompt.md")
	testkit.WriteFile(t, file, "task from file", 0o644)
	for name, spec := range map[string]agentjobs.SubmitSpec{
		"prompt":      {Prompt: "task from text"},
		"prompt file": {PromptFile: file},
	} {
		testkit.Check(t, withMemory(context.Background(), &spec))
		testkit.Contains(t, spec.Prompt, "## Memory (Engram)", "Chose Engram", "task from")
		testkit.Equal(t, name+": the file is folded into the prompt", spec.PromptFile, "")
	}
}

func TestWithMemoryTellsTheJobHowToSaveEvenWhenThereAreNoMemoriesYet(t *testing.T) {
	engramThatSays(t, "No previous session memories found.")
	spec := agentjobs.SubmitSpec{Prompt: "task"}
	testkit.Check(t, withMemory(context.Background(), &spec))
	testkit.Contains(t, spec.Prompt, "(no memories yet)", "dendrite memory save", "task")
}

func TestWithMemoryLeavesThePromptAloneWithoutEngram(t *testing.T) {
	t.Setenv(engram.BinaryEnv, filepath.Join(t.TempDir(), "absent"))
	spec := agentjobs.SubmitSpec{Prompt: "task", PromptFile: "kept"}
	testkit.Check(t, withMemory(context.Background(), &spec))
	testkit.Equal(t, "prompt", spec.Prompt, "task")
	testkit.Equal(t, "prompt file", spec.PromptFile, "kept")
}

func TestWithMemoryReportsAnUnreadablePromptFile(t *testing.T) {
	engramThatSays(t, "something to remember")
	spec := agentjobs.SubmitSpec{PromptFile: filepath.Join(t.TempDir(), "absent.md")}
	if err := withMemory(context.Background(), &spec); err == nil {
		t.Fatal("an unreadable prompt file must be an error")
	}
}

func TestWithMemoryIgnoresAnEmptyPrompt(t *testing.T) {
	engramThatSays(t, "something to remember")
	spec := agentjobs.SubmitSpec{}
	testkit.Check(t, withMemory(context.Background(), &spec))
	testkit.Equal(t, "an empty prompt stays empty so Submit rejects it", spec.Prompt, "")
}

func isolateMemory(t *testing.T, home string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", home)
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
}

func journaled(t *testing.T) []memory.Entry {
	t.Helper()
	got, _, err := memory.DefaultJournal().Pending()
	testkit.Check(t, err)
	return got
}

func TestRememberOutcomeJournalsOnlyFinalStatuses(t *testing.T) {
	isolateMemory(t, t.TempDir())
	for _, st := range []agentjobs.Status{agentjobs.StatusRunning, agentjobs.StatusFailedRetryable, agentjobs.StatusQuotaExhausted, agentjobs.StatusQueued} {
		rememberOutcome(&agentjobs.Job{ID: "j", Status: st})
	}
	testkit.Equal(t, "a job that is not over is not remembered", len(journaled(t)), 0)

	for _, st := range []agentjobs.Status{agentjobs.StatusSucceeded, agentjobs.StatusEarlyExit, agentjobs.StatusNeedsReview, agentjobs.StatusFailed} {
		rememberOutcome(&agentjobs.Job{ID: "j-" + string(st), Ticket: "DEN-1", Status: st, Agent: "agy", Attempts: map[string]int{"a": 1, "b": 2}})
	}
	got := journaled(t)
	if len(got) != 4 || got[0].Type != "outcome" || got[0].Ticket != "DEN-1" || !strings.Contains(got[0].Body, "after 3 attempt(s)") {
		t.Fatalf("every final status is remembered: %+v", got)
	}
}

func TestRememberOutcomeIsSkippedWithoutAHome(t *testing.T) {
	isolateMemory(t, "")
	t.Setenv("HOME", "")
	rememberOutcome(&agentjobs.Job{ID: "j", Status: agentjobs.StatusFailed})
	testkit.Equal(t, "no home, no journal", len(journaled(t)), 0)
}

func TestRunJobAndRememberJournalsTheOutcomeAndKeepsTheJobsError(t *testing.T) {
	isolateMemory(t, t.TempDir())
	boom := errors.New("runner failed")
	prev := runJob
	runJob = func(context.Context, string) error { return boom }
	t.Cleanup(func() { runJob = prev })

	testkit.Check(t, agentjobs.NewLedger("").Save(&agentjobs.Job{ID: "job-1", Ticket: "DEN-2", Status: agentjobs.StatusSucceeded, Agent: "agy"}))
	if err := runJobAndRemember(context.Background(), "job-1"); !errors.Is(err, boom) {
		t.Fatalf("the job's own error must come back, got %v", err)
	}
	if got := journaled(t); len(got) != 1 || got[0].Ticket != "DEN-2" {
		t.Fatalf("journal = %+v", got)
	}

	if err := runJobAndRemember(context.Background(), "absent-job"); !errors.Is(err, boom) {
		t.Fatalf("an unknown job still returns the runner's error, got %v", err)
	}
	testkit.Equal(t, "an unknown job journals nothing", len(journaled(t)), 1)
}
