package bootstrap

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func preToolEntries(t *testing.T, doc []byte) []map[string]any {
	t.Helper()
	var m struct {
		Hooks struct {
			PreToolUse []map[string]any `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(doc, &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, doc)
	}
	return m.Hooks.PreToolUse
}

func TestMergeCreatesTheHookInAnEmptySettingsFile(t *testing.T) {
	out, changed, err := mergeClaudeHooks(nil)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	entries := preToolEntries(t, out)
	if len(entries) != 2 || entries[0]["matcher"] != preToolMatcher || hookCommandOf(entries[0]) != preToolCommand ||
		entries[1]["matcher"] != editLockMatcher || hookCommandOf(entries[1]) != editLockCommand {
		t.Errorf("settings = %s", out)
	}
	var doc struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	_ = json.Unmarshal(out, &doc)
	post, end := doc.Hooks["PostToolUse"], doc.Hooks["SessionEnd"]
	if len(post) != 2 || post[0]["matcher"] != planSavedMatcher || hookCommandOf(post[0]) != planSavedCommand ||
		post[1]["matcher"] != decisionMatcher || hookCommandOf(post[1]) != decisionCommand {
		t.Errorf("PostToolUse = %v", post)
	}
	if len(end) != 1 || end[0]["matcher"] != nil || hookCommandOf(end[0]) != sessionEndCommand {
		t.Errorf("SessionEnd = %v", end)
	}
	if strings.Contains(string(out), `\u00`) {
		t.Errorf("the command was HTML-escaped in the file: %s", out)
	}
}

func TestMergeKeepsEverythingElseAndIsIdempotent(t *testing.T) {
	existing := []byte(`{
  "permissions": {"allow": ["Bash(go test:*)"]},
  "env": {"FOO": "1"},
  "hooks": {
    "PreToolUse": [{"matcher": "Edit", "hooks": [{"type": "command", "command": "./scripts/lint.sh"}]}],
    "PostToolUse": [{"matcher": "Write", "hooks": [{"type": "command", "command": "echo done"}]}]
  }
}`)
	out, changed, err := mergeClaudeHooks(existing)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	var doc map[string]any
	_ = json.Unmarshal(out, &doc)
	if doc["permissions"] == nil || doc["env"] == nil || doc["hooks"].(map[string]any)["PostToolUse"] == nil {
		t.Errorf("unrelated settings were lost: %s", out)
	}
	entries := preToolEntries(t, out)
	if len(entries) != 3 || entries[0]["matcher"] != "Edit" || hookCommandOf(entries[0]) != "./scripts/lint.sh" {
		t.Errorf("the user's own hook was disturbed: %s", out)
	}
	post := doc["hooks"].(map[string]any)["PostToolUse"].([]any)
	if len(post) != 3 || post[0].(map[string]any)["matcher"] != "Write" {
		t.Errorf("the user's own PostToolUse hook was disturbed: %s", out)
	}

	again, changed, err := mergeClaudeHooks(out)
	if err != nil || changed || string(again) != string(out) {
		t.Errorf("a second merge changed the file (changed=%v, err=%v)", changed, err)
	}
}

func TestMergeReplacesAnOlderDendriteHookInsteadOfDuplicatingIt(t *testing.T) {
	old := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"dendrite hook pre-tool --old"}]}]}}`)
	out, changed, err := mergeClaudeHooks(old)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	entries := preToolEntries(t, out)
	if len(entries) != 2 || entries[0]["matcher"] != preToolMatcher || strings.Contains(string(out), "--old") {
		t.Errorf("the old hook was kept: %s", out)
	}
}

func TestMergeRefusesFilesItCannotUnderstand(t *testing.T) {
	for _, bad := range []string{`{not json`, `{"hooks": []}`, `{"hooks": "x"}`, `{"hooks": {"PostToolUse": "x"}}`, `{"hooks": {"SessionEnd": {"a": 1}}}`} {
		if _, changed, err := mergeClaudeHooks([]byte(bad)); err == nil || changed {
			t.Errorf("%q: err=%v changed=%v, want a refusal that leaves the file alone", bad, err, changed)
		}
	}
}

// runHook runs the generated search-guard command the way Claude Code does, with the given programs on PATH.
func runHook(t *testing.T, programs map[string]string) (exit int, stderr string) {
	t.Helper()
	return runHookCommand(t, preToolCommand, programs)
}

// runHookCommand runs a generated hook command with the given programs on PATH.
func runHookCommand(t *testing.T, command string, programs map[string]string) (exit int, stderr string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range programs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = []string{"PATH=" + dir}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	cmd.Stdin = strings.NewReader(`{"tool_name":"Bash"}`)
	err := cmd.Run()
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) {
		return ee.ExitCode(), errBuf.String()
	} else if err != nil {
		t.Fatal(err)
	}
	return 0, errBuf.String()
}

func TestGeneratedHookFailsOpenAndStillBlocks(t *testing.T) {
	// No dendrite at all.
	if exit, _ := runHook(t, nil); exit != 0 {
		t.Errorf("no dendrite on PATH: exit %d, the agent must keep working", exit)
	}
	// A dendrite that predates `hook`: unknown command is exit 2, which must NOT read as a block.
	old := `echo 'unknown command "hook" for "dendrite"' >&2; exit 2`
	if exit, _ := runHook(t, map[string]string{"dendrite": old}); exit != 0 {
		t.Errorf("old dendrite: exit %d, want 0", exit)
	}
	// A dendrite that crashes.
	if exit, _ := runHook(t, map[string]string{"dendrite": "echo boom >&2; exit 1"}); exit != 0 {
		t.Errorf("crashing dendrite: exit %d, want 0", exit)
	}
	// The real guard blocking: its message reaches the agent with exit 2.
	guard := `echo "[-] dendrite-first: use dendrite rag search" >&2; exit 2`
	exit, stderr := runHook(t, map[string]string{"dendrite": guard})
	if exit != 2 || !strings.Contains(stderr, "dendrite-first: use dendrite rag search") {
		t.Errorf("guard block: exit %d stderr %q, want exit 2 with the message", exit, stderr)
	}
	// The guard allowing.
	if exit, _ := runHook(t, map[string]string{"dendrite": "exit 0"}); exit != 0 {
		t.Errorf("guard allow: exit %d", exit)
	}
}

// hookCommandOf returns the command of the first hook in a PreToolUse entry.
func hookCommandOf(entry map[string]any) string {
	inner, _ := entry["hooks"].([]any)
	if len(inner) == 0 {
		return ""
	}
	h, _ := inner[0].(map[string]any)
	cmd, _ := h["command"].(string)
	return cmd
}

func TestEditLockHookFailsClosed(t *testing.T) {
	// No dendrite at all: the write is refused and the reason is shown.
	exit, stderr := runHookCommand(t, editLockCommand, nil)
	if exit != 2 || !strings.Contains(stderr, "blocked: dendrite is not on PATH") {
		t.Errorf("no dendrite: exit %d, stderr %q", exit, stderr)
	}
	// The lock refusing: its message reaches the agent.
	lock := `echo "blocked: no Dendrite session/ticket claim" >&2; exit 2`
	if exit, stderr := runHookCommand(t, editLockCommand, map[string]string{"dendrite": lock}); exit != 2 || !strings.Contains(stderr, "no Dendrite session/ticket claim") {
		t.Errorf("lock: exit %d, stderr %q", exit, stderr)
	}
	// A dendrite that crashes, or predates `hook pre-edit`, is not a licence to write.
	if exit, _ := runHookCommand(t, editLockCommand, map[string]string{"dendrite": "echo boom >&2; exit 1"}); exit != 2 {
		t.Errorf("crash: exit %d, want 2", exit)
	}
	old := `echo 'unknown command "pre-edit" for "dendrite hook"' >&2; exit 2`
	if exit, stderr := runHookCommand(t, editLockCommand, map[string]string{"dendrite": old}); exit != 2 || !strings.Contains(stderr, "unknown command") {
		t.Errorf("old dendrite: exit %d, stderr %q", exit, stderr)
	}
	// The lock allowing.
	if exit, _ := runHookCommand(t, editLockCommand, map[string]string{"dendrite": "exit 0"}); exit != 0 {
		t.Errorf("allow: exit %d", exit)
	}
}

func TestPlanAndSessionEndHooksNeverFailTheAgent(t *testing.T) {
	for name, command := range map[string]string{"plan-saved": planSavedCommand, "session-end": sessionEndCommand} {
		if exit, _ := runHookCommand(t, command, nil); exit != 0 {
			t.Errorf("%s with no dendrite: exit %d", name, exit)
		}
		for _, body := range []string{"exit 0", "echo boom >&2; exit 1", "exit 2"} {
			if exit, _ := runHookCommand(t, command, map[string]string{"dendrite": body}); exit != 0 {
				t.Errorf("%s with dendrite %q: exit %d, want 0", name, body, exit)
			}
		}
	}
}

func TestEveryOwnedHookCommandCarriesTheMarker(t *testing.T) {
	for _, h := range dendriteHooks {
		if !strings.Contains(h.command, dendriteHookMarker) {
			t.Errorf("%s hook %q would not be recognised as Dendrite's and would be duplicated on every prepare", h.event, h.command)
		}
	}
}
