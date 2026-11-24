package bootstrap

import (
	"Dendrite/pkg/directive"
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
		"A trunk reaches `main` only by pull request: `dendrite chronos finish`",
		"git push origin HEAD:<trunk>",
		"EGM artifact directory: docs/specs",
	} {
		if !strings.Contains(string(agents), want) {
			t.Errorf("AGENTS.md missing %q", want)
		}
	}
	rules, _ := os.ReadFile(filepath.Join(root, ".agents", "rules.md"))
	if !strings.Contains(string(rules), "dendrite work new <ticket>") || !strings.Contains(string(rules), "only by pull request") {
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
	if !strings.Contains(string(first), "## Finishing a ticket") || !strings.Contains(string(first), "git merge origin/<parent trunk>") {
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

func TestAProjectThatIntegratesOnMainFinishesByPullRequest(t *testing.T) {
	dir := t.TempDir()
	testkit.GitInit(t, dir, "main")
	testkit.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	var rules strings.Builder
	writeNumberedRules(&rules, dir)
	skill := renderDendriteWorkflowSkill(dir)
	for _, text := range []string{rules.String(), skill} {
		testkit.Contains(t, text, "pull request", "waits in To Verify", "a person merges it", "Never merge into or push `main`/`master`")
		testkit.NotContains(t, text, "HEAD:main", "git merge origin/main", "{finish}", "{integration}")
	}
	testkit.Contains(t, skill, "dendrite chronos finish DEN-123", "git push origin HEAD:<parent trunk>")
}

func TestWorkflowViolationsNameEveryStaleBranchModel(t *testing.T) {
	for _, c := range []struct {
		name, text, integration, want string
	}{
		{"agent ticket branch", "on branch `agent/<ticket>`", "main", "stale branch name agent/<"},
		{"agent branch of a ticket", "on branch agent/den-123", "develop", "stale branch name agent/d"},
		{"feature trunk", "the feature branch `feature/<PARENT>-<slug>`", "main", "stale branch name feature/<"},
		{"a branch listed twice", "never directly on `main`, `main` or `master`", "main", "branch listed twice: `main`"},
		{"develop on a main project", "git merge origin/develop", "main", "develop is not the integration branch of this project (main)"},
		{"develop on a master project", "push to develop", "master", "develop is not the integration branch of this project (master)"},
		{"develop on a develop project", "push to develop", "develop", ""},
		{"clean text", "never directly on `main` or `master`; branch <TICKET>-<slug>/main", "main", ""},
	} {
		got := strings.Join(workflowViolations(c.text, c.integration), "; ")
		testkit.Equal(t, c.name, got, c.want)
	}
}

func TestEveryIntegrationBranchRendersRulesThatPassTheBranchModelCheck(t *testing.T) {
	for _, integration := range []string{"", "trunk", "main", "master"} {
		dir := projectOn(t, integration)
		if integration == "main" || integration == "master" {
			dir = t.TempDir()
			testkit.GitInit(t, dir, integration)
			testkit.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
		}
		testkit.Check(t, verifyWorkflowText(dir))
	}
}

func TestTheProtectedBranchesAreListedOnce(t *testing.T) {
	for integration, want := range map[string]string{
		"develop": "`develop`, `main` or `master`",
		"main":    "`main` or `master`",
		"master":  "`master` or `main`",
	} {
		testkit.Equal(t, integration, workflowBranches{Integration: integration}.protected(), want)
	}
}

func writeReaDirectives(t *testing.T, root, toml string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(directive.Path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestREASkillDescribesTheDefaultOwnershipRule(t *testing.T) {
	text := renderDendriteREASkill(t.TempDir())
	for _, want := range []string{
		"name: dendrite-rea", "the label `rea`", "a title starting `[REA]`",
		"dendrite retro handoff <ticket> --refused", "dendrite_rea_handoff", "owner:agy",
		"AGY sessions", "dendrite chronos claim <ticket> --agent agy", "dendrite retro analyze <target>", "dendrite_rea_search", "dendrite retro purge <id> --force", "dendrite retro reuse <id>", "dendrite retro engines", "docs/features/rea-engines.md", "dendrite retro export", "--sandbox host --trusted", "dendrite_rea_engines", "dendrite_rea_adr", "--ticket DEN-123", "under 4,000 tokens", "dendrite soma diagnose",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the REA skill lacks %q:\n%s", want, text)
		}
	}
}

func TestREASkillFollowsTheProjectsDirective(t *testing.T) {
	root := t.TempDir()
	writeReaDirectives(t, root, "schema = 1\n[rea]\nlabels = [\"reverse\"]\ntitle_prefixes = []\nrefusal_owner = \"none\"\n")
	text := renderDendriteREASkill(root)
	if !strings.Contains(text, "the label `reverse`") || strings.Contains(text, "[REA]") ||
		!strings.Contains(text, "stays with the session that claimed it") || strings.Contains(text, "owner:agy") {
		t.Errorf("the skill must follow rea.* of the directives:\n%s", text)
	}

	writeReaDirectives(t, root, "schema = 1\n[rea]\nlabels = []\ntitle_prefixes = []\nrefusal_owner = \"none\"\n")
	if text := renderDendriteREASkill(root); !strings.Contains(text, "none (no label or title prefix is configured)") {
		t.Errorf("no markers configured:\n%s", text)
	}

	writeReaDirectives(t, root, "not = [valid")
	if text := renderDendriteREASkill(root); !strings.Contains(text, "the label `rea`") {
		t.Errorf("unreadable directives fall back to the defaults:\n%s", text)
	}
}

func TestPrepareWritesTheREASkillForEveryAgent(t *testing.T) {
	root := t.TempDir()
	created, _, err := syncWorkflowSkill(root, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range skillDirs {
		path := filepath.Join(root, filepath.FromSlash(dir), DendriteREASkill, "SKILL.md")
		if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "# Dendrite REA") {
			t.Errorf("%s: %v", path, err)
		}
	}
	if len(created) != 2*len(skillDirs) {
		t.Errorf("created %v: want the workflow and REA skill in each of %v", created, skillDirs)
	}
}
