package agent

import (
	"context"
	"path/filepath"
	"testing"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/engram"
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
