package bootstrap

import (
	"Dendrite/pkg/testkit"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// preparedProject is a Go project with a hand-written CLAUDE.md after one Prepare run.
func preparedProject(t *testing.T) (root string, p *Preparer, report *PrepareReport) {
	t.Helper()
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A hand-written CLAUDE.md must survive; GEMINI.md is created.
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p = NewPreparer()
	report, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true})
	if err != nil {
		t.Fatal(err)
	}
	return root, p, report
}

func TestPrepare_AgentsAndRulesCarryTheDendriteFirstWorkflow(t *testing.T) {
	root, _, _ := preparedProject(t)
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
		"merge your branch into its target (`dendrite branch show`; `main` unless",
		"git push origin HEAD:<target>",
		"EGM artifact directory: docs/specs",
	} {
		if !strings.Contains(string(agents), want) {
			t.Errorf("AGENTS.md missing %q", want)
		}
	}
	rules, _ := os.ReadFile(filepath.Join(root, ".agents", "rules.md"))
	if !strings.Contains(string(rules), "dendrite work new <ticket>") || !strings.Contains(string(rules), "merge your branch into its target") {
		t.Error(".agents/rules.md is missing the working-copy or merge-to-target rule")
	}
}

func TestPrepare_EveryAgentFamilyGetsTheSameSkillAndThePointerFiles(t *testing.T) {
	root, _, _ := preparedProject(t)
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
	if !strings.Contains(string(first), "## Finishing a ticket") || !strings.Contains(string(first), "git merge origin/main") {
		t.Error("skill is missing the finish-and-merge steps")
	}
}

func TestPrepare_KeepsHandWrittenPointerFilesAndReportsTheSkills(t *testing.T) {
	root, _, report := preparedProject(t)
	if data, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md")); string(data) != "# mine\n" {
		t.Errorf("existing CLAUDE.md was overwritten: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "GEMINI.md")); string(data) != pointerFileContent {
		t.Errorf("GEMINI.md = %q", data)
	}
	if synced := strings.Join(report.SkillsSynced, ","); !strings.Contains(synced, DendriteWorkflowSkill) {
		t.Errorf("skills synced = %s", synced)
	}
}

func TestPrepare_RestoresDriftedSkillsAndIsOtherwiseIdempotent(t *testing.T) {
	root, p, _ := preparedProject(t)
	ctx := context.Background()
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

// projectOn is a git project whose directives choose integration as the integration branch ("" keeps detection).
func projectOn(t *testing.T, integration string) string {
	t.Helper()
	dir := t.TempDir()
	testkit.GitInit(t, dir, "develop")
	testkit.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	if integration != "" {
		path := filepath.Join(dir, ".github", "dendrite", "directives.toml")
		testkit.Check(t, os.MkdirAll(filepath.Dir(path), 0o755))
		testkit.WriteFile(t, path, "schema = 1\n[git]\nintegration_branch = \""+integration+"\"\nfeature_prefix = \"feat/\"\n", 0o644)
	}
	return dir
}

func TestTheRulesAndTheSkillNameTheProjectsIntegrationBranch(t *testing.T) {
	var rules strings.Builder
	writeNumberedRules(&rules, projectOn(t, "trunk"))
	skill := renderDendriteWorkflowSkill(projectOn(t, "trunk"))
	for name, text := range map[string]string{"rules": rules.String(), "skill": skill} {
		testkit.Contains(t, text, "`trunk`", "<ROOT>-<slug>/main")
		testkit.NotContains(t, text, "agent/", "feat/")
		testkit.NotContains(t, text, "{integration}", "{feature}")
		testkit.NotContains(t, text, "origin/develop", "HEAD:develop")
		_ = name
	}
	testkit.Contains(t, skill, "git merge origin/trunk", "git push origin HEAD:trunk")
}

func TestTheDefaultRulesStillSayDevelop(t *testing.T) {
	var rules strings.Builder
	writeNumberedRules(&rules, projectOn(t, ""))
	testkit.Contains(t, rules.String(), "never directly on `develop`, `main` or `master`", "<TICKET>-<slug>/main", "<ROOT>-<slug>/<TICKET>-<slug>", "dendrite branch show --ticket <ticket>")
	testkit.NotContains(t, rules.String(), "{integration}", "{feature}")
}
