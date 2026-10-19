package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepare_EmitsDendriteFirstRulesForEveryAgentFamily(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A hand-written CLAUDE.md must survive; GEMINI.md is created.
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewPreparer()
	ctx := context.Background()
	report, err := p.Prepare(ctx, PrepareOptions{ProjectPath: root, SyncSkills: true})
	if err != nil {
		t.Fatal(err)
	}

	agents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	for _, want := range []string{
		"## Dendrite-First Workflow",
		"dendrite work new <ticket>",
		"dendrite-work/",
		"LiteLLM gateway",
		"--isolate",
		"## Commit & VCS Guidance",
		"## Session Start & Handoff",
		"dendrite session resume",
		"dendrite session checkpoint",
		"merge your branch into `develop` and push",
		"git push origin HEAD:develop",
	} {
		if !strings.Contains(string(agents), want) {
			t.Errorf("AGENTS.md missing %q", want)
		}
	}

	rules, _ := os.ReadFile(filepath.Join(root, ".agents", "rules.md"))
	if !strings.Contains(string(rules), "dendrite work new <ticket>") {
		t.Error(".agents/rules.md missing the working-copy rule")
	}
	if !strings.Contains(string(rules), "merge your branch into `develop`") {
		t.Error(".agents/rules.md missing the merge-to-develop rule")
	}

	var first []byte
	for _, dir := range skillDirs {
		data, err := os.ReadFile(filepath.Join(root, dir, DendriteWorkflowSkill, "SKILL.md"))
		if err != nil {
			t.Fatalf("skill missing in %s: %v", dir, err)
		}
		if first == nil {
			first = data
		} else if string(first) != string(data) {
			t.Errorf("skill in %s differs from %s", dir, skillDirs[0])
		}
	}
	if !strings.HasPrefix(string(first), "---\nname: dendrite-workflow\n") {
		t.Errorf("skill frontmatter missing: %q", string(first[:40]))
	}
	if !strings.Contains(string(first), "## Finishing a ticket") || !strings.Contains(string(first), "git merge origin/develop") {
		t.Error("skill is missing the finish-and-merge-to-develop steps")
	}

	if data, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md")); string(data) != "# mine\n" {
		t.Errorf("existing CLAUDE.md was overwritten: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "GEMINI.md")); string(data) != pointerFileContent {
		t.Errorf("GEMINI.md = %q", data)
	}

	synced := strings.Join(report.SkillsSynced, ",")
	if !strings.Contains(synced, DendriteWorkflowSkill) {
		t.Errorf("skills synced = %s", synced)
	}

	// Drifted copies are restored; a second run is otherwise a no-op.
	drifted := filepath.Join(root, ".claude", "skills", DendriteWorkflowSkill, "SKILL.md")
	if err := os.WriteFile(drifted, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	report2, err := p.Prepare(ctx, PrepareOptions{ProjectPath: root, SyncSkills: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report2.FilesCreated) != 0 || len(report2.FilesUpdated) != 1 {
		t.Errorf("second run created=%v updated=%v", report2.FilesCreated, report2.FilesUpdated)
	}
	report3, _ := p.Prepare(ctx, PrepareOptions{ProjectPath: root, SyncSkills: true})
	if len(report3.FilesCreated)+len(report3.FilesUpdated) != 0 {
		t.Errorf("third run not idempotent: created=%v updated=%v", report3.FilesCreated, report3.FilesUpdated)
	}
}

func TestPrepare_DryRunWritesNoWorkflowFiles(t *testing.T) {
	root := t.TempDir()
	p := NewPreparer()
	report, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(report.FilesCreated, ","), "CLAUDE.md") {
		t.Errorf("dry run should report CLAUDE.md: %v", report.FilesCreated)
	}
	if fileExists(filepath.Join(root, "CLAUDE.md")) || fileExists(filepath.Join(root, ".claude")) {
		t.Error("dry run must not write files")
	}
}

func TestPrepare_WorkflowSkillWriteError(t *testing.T) {
	root := t.TempDir()
	// A file where the .claude directory should be makes the skill write fail.
	if err := os.WriteFile(filepath.Join(root, ".claude"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPreparer().Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err == nil {
		t.Fatal("expected an error when the skill directory cannot be created")
	}
}

func TestPrepare_PointerFileWriteError(t *testing.T) {
	root := t.TempDir()
	// A dangling symlink: fileExists is false but writing through it fails.
	if err := os.Symlink(filepath.Join(root, "missing", "x"), filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPreparer().Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err == nil {
		t.Fatal("expected an error when CLAUDE.md cannot be written")
	}
}
