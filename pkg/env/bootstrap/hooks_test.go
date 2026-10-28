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
	if len(entries) != 1 || entries[0]["matcher"] != preToolMatcher || hookCommandOf(entries[0]) != preToolCommand {
		t.Errorf("settings = %s", out)
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
	if len(entries) != 2 || entries[0]["matcher"] != "Edit" {
		t.Errorf("the user's own hook was disturbed: %s", out)
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
	if len(entries) != 1 || entries[0]["matcher"] != preToolMatcher || strings.Contains(string(out), "--old") {
		t.Errorf("the old hook was kept: %s", out)
	}
}

func TestMergeRefusesFilesItCannotUnderstand(t *testing.T) {
	for _, bad := range []string{`{not json`, `{"hooks": []}`, `{"hooks": "x"}`} {
		if _, changed, err := mergeClaudeHooks([]byte(bad)); err == nil || changed {
			t.Errorf("%q: err=%v changed=%v, want a refusal that leaves the file alone", bad, err, changed)
		}
	}
}

// runHook runs the generated hook command the way Claude Code does, with the given programs on PATH.
func runHook(t *testing.T, programs map[string]string) (exit int, stderr string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range programs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("/bin/sh", "-c", preToolCommand)
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
