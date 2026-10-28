package bootstrap

import (
	"encoding/json"
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
	if len(entries) != 1 || entries[0]["matcher"] != preToolMatcher || !strings.Contains(string(out), preToolCommand) {
		t.Errorf("settings = %s", out)
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
