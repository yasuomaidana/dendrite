package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every agent family reads a different file, but they must all be taught the same discipline.
func TestPrepare_EveryAgentFamilyGetsTheSameSearchAndLifecycleRules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPreparer().Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err != nil {
		t.Fatal(err)
	}

	needles := []string{
		"dendrite rag search",                   // retrieval first
		"`rg`/`grep`/`find` only when",          // raw search last
		"`dendrite rag grep`",                   // the indexed exact-pattern fallback
		"`dendrite work new <ticket>`",          // start moves the ticket to In Progress
		"`to verify` when the work is finished", // never straight to done
		"--no-ticket-guard",                     // the deliberate bypass
	}
	files := []string{
		"AGENTS.md",
		filepath.Join(".agents", "rules.md"),
		filepath.Join(".agents", "skills", DendriteWorkflowSkill, "SKILL.md"), // AGY
		filepath.Join(".gemini", "skills", DendriteWorkflowSkill, "SKILL.md"), // Gemini CLI
		filepath.Join(".claude", "skills", DendriteWorkflowSkill, "SKILL.md"), // Claude Code
	}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		for _, n := range needles {
			if !strings.Contains(string(data), n) {
				t.Errorf("%s does not teach %q", f, n)
			}
		}
	}
}

func TestPrepare_InstallsTheClaudeHookWithoutClobberingSettings(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(root, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"permissions":{"allow":["Bash(go test:*)"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPreparer()
	if _, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(settings)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("settings.json is not JSON: %v\n%s", err, data)
	}
	hooks := doc["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)
	if doc["permissions"] == nil || len(pre) != 2 || hookCommandOf(pre[0].(map[string]any)) != preToolCommand || hookCommandOf(pre[1].(map[string]any)) != editLockCommand {
		t.Errorf("settings = %s", data)
	}
	if hooks["PostToolUse"] == nil || hooks["SessionEnd"] == nil {
		t.Errorf("the plan and session-end hooks are missing: %s", data)
	}

	// A second prepare leaves the file untouched.
	info1, _ := os.Stat(settings)
	if _, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(settings)
	info2, _ := os.Stat(settings)
	if string(again) != string(data) || !info2.ModTime().Equal(info1.ModTime()) {
		t.Error("a second prepare rewrote settings.json")
	}

	// A settings file that cannot be parsed is never overwritten.
	if err := os.WriteFile(settings, []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err != nil {
		t.Fatal(err)
	}
	if kept, _ := os.ReadFile(settings); string(kept) != `{broken` {
		t.Errorf("an unparsable settings.json was overwritten: %s", kept)
	}
}

func TestWorkflowSkillTeachesDelegationAndEndingThePlan(t *testing.T) {
	skill := renderDendriteWorkflowSkill()
	for _, want := range []string{
		"### What to delegate", "### How to delegate well", "`--scope`", "must not commit",
		"zero-mock gate", "build tag", "## Ending the plan", "\"to verify\"", "Agents never set `done`",
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("the workflow skill lacks %q", want)
		}
	}
}
