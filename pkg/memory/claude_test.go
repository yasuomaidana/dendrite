package memory_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
)

const userMemory = `---
name: user-prefers-short-answers
description: terse replies, no recap
metadata:
  type: user
---

Keep replies short.
`

func claudeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testkit.WriteFile(t, filepath.Join(dir, "MEMORY.md"), "- [x](x.md) — hook\n", 0o644)
	testkit.WriteFile(t, filepath.Join(dir, "user_short.md"), userMemory, 0o644)
	testkit.WriteFile(t, filepath.Join(dir, "plain.md"), "no frontmatter, just text\n", 0o644)
	testkit.WriteFile(t, filepath.Join(dir, "notes.txt"), "ignored", 0o644)
	return dir
}

func TestClaudeDirIsTheProjectsMemoryFolderOfTheRepo(t *testing.T) {
	got := memory.ClaudeDir("/Users/me", "/Users/me/GoLandProjects/My.Repo")
	testkit.Equal(t, "ClaudeDir", got, "/Users/me/.claude/projects/-Users-me-GoLandProjects-My-Repo/memory")
}

func TestReadClaudeParsesFrontmatterAndSkipsTheIndex(t *testing.T) {
	got, err := memory.ReadClaude(claudeDir(t))
	testkit.Check(t, err)
	if len(got) != 2 {
		t.Fatalf("want 2 memories (not MEMORY.md, not .txt), got %+v", got)
	}
	byName := map[string]memory.ClaudeMemory{}
	for _, m := range got {
		byName[m.Name] = m
	}
	u := byName["user-prefers-short-answers"]
	if u.Description != "terse replies, no recap" || u.Type != "user" || strings.TrimSpace(u.Body) != "Keep replies short." {
		t.Fatalf("parsed = %+v", u)
	}
	p := byName["plain"]
	if p.Type != "note" || !strings.Contains(p.Body, "no frontmatter") {
		t.Fatalf("a file without frontmatter is a note named after the file: %+v", p)
	}
}

func TestReadClaudeReportsAMissingFolder(t *testing.T) {
	if _, err := memory.ReadClaude(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestMirrorClaudeSavesEachMemoryIntoEngram(t *testing.T) {
	log := stubEngram(t, `echo "$*" >> "$LOG"`)
	n, err := memory.MirrorClaude(context.Background(), t.TempDir(), claudeDir(t))
	testkit.Check(t, err)
	testkit.Equal(t, "mirrored", n, 2)
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "save user-prefers-short-answers") || !strings.Contains(string(data), "--type user") {
		t.Fatalf("engram calls = %s", data)
	}
	if strings.Contains(string(data), "MEMORY") {
		t.Fatalf("the index must not be mirrored: %s", data)
	}
}

func TestMirrorClaudeDoesNothingWhenEngramIsOffOrMissing(t *testing.T) {
	home := t.TempDir()
	stubEngram(t, "exit 0")
	testkit.Check(t, memory.SetOverride(home, memory.ClaudeOnly))
	n, err := memory.MirrorClaude(context.Background(), home, claudeDir(t))
	testkit.Check(t, err)
	testkit.Equal(t, "mode claude", n, 0)

	t.Setenv("PATH", t.TempDir())
	n, err = memory.MirrorClaude(context.Background(), t.TempDir(), claudeDir(t))
	testkit.Check(t, err)
	testkit.Equal(t, "no engram", n, 0)
}

func TestMirrorClaudeStopsAtTheFirstEngramFailure(t *testing.T) {
	stubEngram(t, "exit 1")
	if _, err := memory.MirrorClaude(context.Background(), t.TempDir(), claudeDir(t)); err == nil {
		t.Fatal("a failing engram must be reported")
	}
	if _, err := memory.MirrorClaude(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing Claude memory folder must be reported")
	}
}
