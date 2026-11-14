package bootstrap

import (
	"Dendrite/pkg/directive"
	"Dendrite/pkg/fsutil/fsutiltest"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectStack(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		assert func(t *testing.T, fp ProjectFingerprint)
	}{
		{
			name: "detect go project with go.mod",
			files: map[string]string{
				"go.mod":        "module testproj\n\ngo 1.24\n",
				".golangci.yml": "linters:\n  enable:\n    - errcheck\n",
			},
			assert: func(t *testing.T, fp ProjectFingerprint) {
				assertPrimaryStack(t, fp.PrimaryStack, StackGo, "")
				assertGoVersion(t, fp.GoVersion, "1.24")
				assertFirst(t, fp.Linters, "golangci-lint", "linters")
				assertFirst(t, fp.Toolchains, "go", "toolchains")
			},
		},
		{
			name: "detect rust and polyglot typescript project with cargo and bun",
			files: map[string]string{
				"Cargo.toml":    "[package]\nname = \"demo\"\n",
				"package.json":  "{\"name\": \"demo-web\", \"scripts\": {\"test\": \"bun test\"}}\n",
				"bun.lock":      "{}\n",
				"tsconfig.json": "{}\n",
			},
			assert: func(t *testing.T, fp ProjectFingerprint) {
				assertPrimaryStack(t, fp.PrimaryStack, StackRust, "primary")
				assertTrue(t, fp.HasCargo, "HasCargo")
				assertTrue(t, fp.HasBun, "HasBun")
				assertFirst(t, fp.SecondaryStacks, StackTypeScript, "secondary stacks")
				assertToolchainBinary(t, fp.ToolchainDetails, "cargo", "cargo")
				assertToolchainBinary(t, fp.ToolchainDetails, "bun", "bun")
			},
		},
		{
			name: "detect python project with uv",
			files: map[string]string{
				"pyproject.toml": "[project]\nname = \"app\"\n\n[tool.pytest.ini_options]\n",
			},
			assert: func(t *testing.T, fp ProjectFingerprint) {
				assertPrimaryStack(t, fp.PrimaryStack, StackPython, "")
				assertTrue(t, fp.HasUV, "HasUV")
				assertToolchainBinary(t, fp.ToolchainDetails, "uv", "uv")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { runDetectStackCase(t, tc.files, tc.assert) })
	}
}

func runDetectStackCase(t *testing.T, files map[string]string, check func(*testing.T, ProjectFingerprint)) {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		fsutiltest.WriteFile(t, dir, rel, content)
	}
	fp, err := NewPreparer().DetectStack(dir)
	if err != nil {
		t.Fatalf("DetectStack failed: %v", err)
	}
	check(t, fp)
}

func assertPrimaryStack(t *testing.T, got, want LanguageStack, label string) {
	t.Helper()
	if got != want {
		if label != "" {
			t.Errorf("expected %s %s, got %s", want, label, got)
		} else {
			t.Errorf("expected %s, got %s", want, got)
		}
	}
}

func assertGoVersion(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("expected GoVersion %s, got %s", want, got)
	}
}

func assertFirst[T comparable](t *testing.T, list []T, want T, what string) {
	t.Helper()
	if len(list) == 0 || list[0] != want {
		t.Errorf("expected %v in %s, got %v", want, what, list)
	}
}

func assertTrue(t *testing.T, condition bool, name string) {
	t.Helper()
	if !condition {
		t.Errorf("expected %s true", name)
	}
}

func assertToolchainBinary(t *testing.T, details map[string]ToolchainInfo, name, wantBinary string) {
	t.Helper()
	tc, ok := details[name]
	if !ok || tc.Binary != wantBinary {
		t.Errorf("expected %s toolchain details", name)
	}
}

func TestProbeGitHubCopilot(t *testing.T) {
	p := NewPreparer()

	t.Run("copilot binary directly found", func(t *testing.T) {
		p.SetLookPath(func(cmd string) (string, error) {
			if cmd == "copilot" {
				return "/usr/local/bin/copilot", nil
			}
			return "", errors.New("not found")
		})

		installed, bin, cmd := p.ProbeGitHubCopilot()
		if !installed {
			t.Errorf("expected installed true")
		}
		if bin != "/usr/local/bin/copilot" {
			t.Errorf("expected /usr/local/bin/copilot, got %s", bin)
		}
		if cmd != "" {
			t.Errorf("expected empty install command when installed")
		}
	})

	t.Run("gh cli with copilot extension found", func(t *testing.T) {
		p.SetLookPath(func(cmd string) (string, error) {
			if cmd == "copilot" {
				return "", errors.New("not found")
			}
			if cmd == "gh" {
				return "/opt/homebrew/bin/gh", nil
			}
			return "", errors.New("not found")
		})

		installed, bin, _ := p.ProbeGitHubCopilot()
		if !installed {
			t.Errorf("expected installed true via gh")
		}
		if !strings.Contains(bin, "gh copilot") {
			t.Errorf("expected gh copilot in binary string, got %s", bin)
		}
	})

	t.Run("copilot completely missing provides installation guide", func(t *testing.T) {
		p.SetLookPath(func(cmd string) (string, error) {
			return "", errors.New("not found")
		})

		installed, bin, cmd := p.ProbeGitHubCopilot()
		if installed {
			t.Errorf("expected installed false")
		}
		if bin != "" {
			t.Errorf("expected empty binary path")
		}
		if !strings.Contains(cmd, "npm install -g @github/copilot-cli") {
			t.Errorf("expected npm install recommendation, got %s", cmd)
		}
	})
}

func TestProbeToolchains(t *testing.T) {
	p := NewPreparer()
	p.SetLookPath(func(cmd string) (string, error) {
		switch cmd {
		case "go":
			return "/usr/local/go/bin/go", nil
		case "cargo":
			return "/Users/test/.cargo/bin/cargo", nil
		case "bun":
			return "/Users/test/.bun/bin/bun", nil
		case "uv":
			return "/opt/homebrew/bin/uv", nil
		default:
			return "", errors.New("not found")
		}
	})

	tools := p.ProbeToolchains()
	if len(tools) != 4 {
		t.Fatalf("expected 4 toolchains, got %d", len(tools))
	}

	found := make(map[string]bool)
	for _, tc := range tools {
		if !tc.Installed {
			t.Errorf("expected toolchain %s to be installed", tc.Name)
		}
		found[tc.Name] = true
	}

	for _, expected := range []string{"go", "cargo", "bun", "uv"} {
		if !found[expected] {
			t.Errorf("missing toolchain %s in probe results", expected)
		}
	}
}

func e2eOptions(dir string, dryRun bool) PrepareOptions {
	return PrepareOptions{ProjectPath: dir, EnforceCICD: true, SyncSkills: true, DryRun: dryRun}
}

func TestPreparePipeline_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	setupPolyglotTestProject(t, dir)
	p := NewPreparer()
	p.SetLookPath(testToolchainLookup)

	steps := []struct {
		name string
		run  func(t *testing.T, p *Preparer, dir string)
	}{
		{"dry run execution", e2eDryRun},
		{"real execution", e2eReal},
		{"idempotency check", e2eIdempotent},
		{"sync and update aliases", e2eAliases},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) { step.run(t, p, dir) })
	}
}

func e2eDryRun(t *testing.T, p *Preparer, dir string) {
	report, err := p.Prepare(context.Background(), e2eOptions(dir, true))
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if !report.Success {
		t.Errorf("expected dry run success")
	}
	if len(report.FilesCreated) == 0 {
		t.Errorf("expected files planned for creation in dry run")
	}
	if fileExists(filepath.Join(dir, ".github", "copilot-instructions.md")) {
		t.Errorf("dry run should not create files on disk")
	}
}

func e2eReal(t *testing.T, p *Preparer, dir string) {
	report, err := p.Prepare(context.Background(), e2eOptions(dir, false))
	if err != nil {
		t.Fatalf("real prepare failed: %v", err)
	}
	assertPrepareReport(t, report)
	assertCreatedFilesContent(t, dir)
}

func e2eIdempotent(t *testing.T, p *Preparer, dir string) {
	report, err := p.Prepare(context.Background(), e2eOptions(dir, false))
	if err != nil {
		t.Fatalf("second prepare run failed: %v", err)
	}
	if len(report.FilesCreated) != 0 {
		t.Errorf("expected 0 files created on idempotent run, got %d", len(report.FilesCreated))
	}
}

func e2eAliases(t *testing.T, p *Preparer, dir string) {
	ctx := context.Background()
	for name, run := range map[string]func(context.Context, PrepareOptions) (*PrepareReport, error){"Sync": p.Sync, "Update": p.Update} {
		report, err := run(ctx, e2eOptions(dir, false))
		if err != nil || !report.Success {
			t.Fatalf("%s failed: %v", name, err)
		}
	}
}

func setupPolyglotTestProject(t *testing.T, dir string) {
	t.Helper()
	for rel, content := range map[string]string{
		"go.mod":                           "module github.com/org/app\n\ngo 1.25\n",
		"Cargo.toml":                       "[package]\nname = \"rust-core\"\n",
		"website/package.json":             "{\"name\": \"web-portal\", \"scripts\": {\"build\": \"vite build\"}}\n",
		"website/bun.lock":                 "{}\n",
		"scripts/pyproject.toml":           "[project]\nname = \"scripts\"\n",
		"scripts/tests/test_scripts.py":    "",
		".gemini/skills/my-skill/SKILL.md": "# My Skill\n",
	} {
		fsutiltest.WriteFile(t, dir, rel, content)
	}
}

func testToolchainLookup(cmd string) (string, error) {
	switch cmd {
	case "copilot":
		return "/usr/local/bin/copilot", nil
	case "go":
		return "/usr/local/go/bin/go", nil
	case "cargo":
		return "/Users/test/.cargo/bin/cargo", nil
	case "bun":
		return "/Users/test/.bun/bin/bun", nil
	case "uv":
		return "/opt/homebrew/bin/uv", nil
	default:
		return "", errors.New("not found")
	}
}

func assertPrepareReport(t *testing.T, report *PrepareReport) {
	t.Helper()
	if !report.Success {
		t.Errorf("expected report success")
	}
	if report.CICD.DefaultTool != "copilot" {
		t.Errorf("expected default tool copilot, got %s", report.CICD.DefaultTool)
	}
	if len(report.CICD.Toolchains) != 4 {
		t.Errorf("expected 4 toolchains in CICD report, got %d", len(report.CICD.Toolchains))
	}
}

func assertCreatedFilesContent(t *testing.T, tmpDir string) {
	t.Helper()
	assertCopilotInstructionsContent(t, tmpDir)
	assertCIWorkflowContent(t, tmpDir)
	assertAgentRulesAndSkillContent(t, tmpDir)
}

func assertCopilotInstructionsContent(t *testing.T, tmpDir string) {
	t.Helper()
	copilotMD := filepath.Join(tmpDir, ".github", "copilot-instructions.md")
	assertTargetFileExists(t, copilotMD)
	content, _ := os.ReadFile(copilotMD)
	strContent := string(content)
	if !strings.Contains(strContent, "Zero Code Without Plane Ticket Mandate") {
		t.Errorf("missing Plane mandate in copilot instructions")
	}
	if !strings.Contains(strContent, "Approved & Enforced Toolchains") {
		t.Errorf("missing approved toolchains section in copilot instructions")
	}
	if !strings.Contains(strContent, "bun") || !strings.Contains(strContent, "uv") || !strings.Contains(strContent, "cargo") {
		t.Errorf("missing bun/uv/cargo references in copilot instructions")
	}
}

func assertCIWorkflowContent(t *testing.T, tmpDir string) {
	t.Helper()
	ciYML := filepath.Join(tmpDir, ".github", "workflows", "ci.yml")
	assertTargetFileExists(t, ciYML)
	ciContent, _ := os.ReadFile(ciYML)
	strCI := string(ciContent)
	if !strings.Contains(strCI, "setup-go") {
		t.Errorf("expected go setup in CI workflow")
	}
	if !strings.Contains(strCI, "cargo llvm-cov") {
		t.Errorf("expected cargo coverage run in CI workflow")
	}
	if !strings.Contains(strCI, "setup-bun") {
		t.Errorf("expected bun setup in CI workflow")
	}
	if !strings.Contains(strCI, "setup-uv") {
		t.Errorf("expected uv setup in CI workflow")
	}
}

func assertAgentRulesAndSkillContent(t *testing.T, tmpDir string) {
	t.Helper()
	rulesMD := filepath.Join(tmpDir, ".agents", "rules.md")
	assertTargetFileExists(t, rulesMD)
	rulesData, _ := os.ReadFile(rulesMD)
	if !strings.Contains(string(rulesData), "bun") || !strings.Contains(string(rulesData), "cargo") || !strings.Contains(string(rulesData), "uv") {
		t.Errorf("expected rules to state bun, cargo, uv toolsets")
	}

	agentsMD := filepath.Join(tmpDir, "AGENTS.md")
	assertTargetFileExists(t, agentsMD)

	copilotSkill := filepath.Join(tmpDir, ".gemini", "skills", "copilot", "SKILL.md")
	if !fileExists(copilotSkill) {
		t.Errorf("expected seeded copilot skill at %s", copilotSkill)
	}
}

func assertTargetFileExists(t *testing.T, path string) {
	t.Helper()
	if !fileExists(path) {
		t.Errorf("expected %s to exist", path)
	}
}

func TestEnsureRagEnvAddsTheURLOnceAndKeepsEdits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	t.Setenv("POSTGRES_PORT", "")
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("POSTGRES_PORT=6543\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, updated, err := ensureRagEnv(project, false)
	if err != nil || !created || updated {
		t.Fatalf("first run: created=%v updated=%v err=%v", created, updated, err)
	}
	path := filepath.Join(home, RagEnvFile)
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "@localhost:6543/dendrite_rag") {
		t.Fatalf("port from the project's .env not used: %s", data)
	}
	custom := "DENDRITE_RAG_PG_URL=postgresql://me@db.internal/rag\n"
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if created, updated, err := ensureRagEnv(project, false); err != nil || created || updated {
		t.Fatalf("an existing URL must be kept: created=%v updated=%v err=%v", created, updated, err)
	}
	if data, _ := os.ReadFile(path); string(data) != custom {
		t.Fatalf("file changed: %s", data)
	}
}

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
		"dendrite rag search",                           // retrieval first
		"`rg`/`grep`/`find` only when",                  // raw search last
		"`dendrite rag grep`",                           // the indexed exact-pattern fallback
		"`dendrite work new <ticket>`",                  // start moves the ticket to In Progress
		"`dendrite chronos finish <ticket>`",            // the gate decides done, to verify or blocked
		"`dendrite chronos decision <ticket>",           // questions and answers go on the ticket
		"`dendrite directive \"<rule>\"`",               // quality limits are changed through the directive command
		"`dendrite allostasis gate`",                    // the one command that checks the directives
		"`blocked` with a handoff",                      // a failed finish leaves the diagnosis
		"`dendrite hook pre-edit` refuses file changes", // the write lock
		"--no-ticket-guard",                             // the deliberate bypass
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

// settingsProject is a Go project whose .claude/settings.json holds a permission, prepared once.
func settingsProject(t *testing.T) (root, settings string, p *Preparer) {
	t.Helper()
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings = filepath.Join(root, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"permissions":{"allow":["Bash(go test:*)"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p = NewPreparer()
	if _, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err != nil {
		t.Fatal(err)
	}
	return root, settings, p
}

func TestPrepare_InstallsTheClaudeHooksKeepingThePermissions(t *testing.T) {
	_, settings, _ := settingsProject(t)
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
}

func TestPrepare_ASecondRunLeavesTheClaudeSettingsUntouched(t *testing.T) {
	root, settings, p := settingsProject(t)
	data, _ := os.ReadFile(settings)
	info1, _ := os.Stat(settings)
	if _, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(settings)
	info2, _ := os.Stat(settings)
	if string(again) != string(data) || !info2.ModTime().Equal(info1.ModTime()) {
		t.Error("a second prepare rewrote settings.json")
	}
}

func TestPrepare_NeverOverwritesClaudeSettingsItCannotParse(t *testing.T) {
	root, settings, p := settingsProject(t)
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
	skill := renderDendriteWorkflowSkill(t.TempDir())
	for _, want := range []string{
		"### What to delegate", "### How to delegate well", "`--scope`", "must not commit",
		"zero-mock gate", "build tag", "## Ending the plan", "\"to verify\"", "Agents never set `done`",
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("the workflow skill lacks %q", want)
		}
	}
}

// prepareCI runs Prepare with the CI/CD step only, in a Go project at root.
func prepareCI(t *testing.T, root string) *PrepareReport {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewPreparer()
	p.SetLookPath(func(string) (string, error) { return "", os.ErrNotExist })
	report, err := p.Prepare(context.Background(), PrepareOptions{ProjectPath: root, EnforceCICD: true})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func readCI(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeCI(t *testing.T, root, content string) {
	t.Helper()
	path := filepath.Join(root, ".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareWritesAManagedCIWorkflowAndLeavesItAloneWhenCurrent(t *testing.T) {
	root := t.TempDir()
	first := prepareCI(t, root)
	if got := readCI(t, root); !strings.HasPrefix(got, ManagedHeader+"\n") {
		t.Fatalf("a generated workflow starts with the managed header: %.80q", got)
	}
	if !contains(first.FilesCreated, filepath.Join(".github", "workflows", "ci.yml")) || len(first.Notes) != 0 {
		t.Errorf("first run: created %v, notes %v", first.FilesCreated, first.Notes)
	}
	second := prepareCI(t, root)
	if contains(second.FilesUpdated, filepath.Join(".github", "workflows", "ci.yml")) || contains(second.FilesCreated, filepath.Join(".github", "workflows", "ci.yml")) || len(second.Notes) != 0 {
		t.Errorf("second run changes nothing: %+v", second)
	}
}

func TestPrepareRewritesAStaleManagedWorkflow(t *testing.T) {
	root := t.TempDir()
	writeCI(t, root, ManagedHeader+"\nname: old template\n")
	report := prepareCI(t, root)
	if got := readCI(t, root); strings.Contains(got, "old template") || !strings.HasPrefix(got, ManagedHeader) {
		t.Errorf("a managed workflow follows the template: %.120q", got)
	}
	if !contains(report.FilesUpdated, filepath.Join(".github", "workflows", "ci.yml")) {
		t.Errorf("updated = %v", report.FilesUpdated)
	}
}

func TestPrepareKeepsAHandWrittenWorkflowAndSaysSo(t *testing.T) {
	root := t.TempDir()
	const mine = "name: CI\non: [push]\njobs:\n  verify:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go run ./cmd/dendrite allostasis coverage --ratchet\n"
	writeCI(t, root, mine)
	report := prepareCI(t, root)
	if got := readCI(t, root); got != mine {
		t.Fatalf("a hand-written workflow must stay byte for byte: %q", got)
	}
	if contains(report.FilesUpdated, filepath.Join(".github", "workflows", "ci.yml")) || len(report.Notes) != 1 || !strings.Contains(report.Notes[0], "hand-written") {
		t.Errorf("updated %v, notes %v", report.FilesUpdated, report.Notes)
	}
}

func TestWriteManagedFileDryRunWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ci.yml")
	created, updated, kept, err := writeManagedFile(path, ManagedHeader+"\nx\n", true)
	if err != nil || !created || updated || kept {
		t.Fatalf("created %v updated %v kept %v err %v", created, updated, kept, err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("a dry run must not create the file")
	}
}

func TestPrepareGivesAProjectItsDefaultDirectivesOnce(t *testing.T) {
	root := t.TempDir()
	first := prepareCI(t, root)
	for _, want := range []string{filepath.FromSlash(directive.Path), "codecov.yml", ".jscpd.json"} {
		if !contains(first.FilesCreated, want) {
			t.Errorf("first run did not create %s: %v", want, first.FilesCreated)
		}
	}
	set, err := directive.Load(root)
	if err != nil || set.Loc.MaxProductionLines != 500 {
		t.Fatalf("%+v, %v", set, err)
	}
	if again := prepareCI(t, root); contains(again.FilesCreated, filepath.FromSlash(directive.Path)) || contains(again.FilesCreated, "codecov.yml") {
		t.Errorf("a project that has directives keeps them: %v", again.FilesCreated)
	}
}

func TestEnsureDirectivesDryRunOnlyPlans(t *testing.T) {
	root := t.TempDir()
	created, _, notes, err := ensureDirectives(root, true)
	if err != nil || len(created) == 0 || len(notes) != 0 || directive.Exists(root) {
		t.Fatalf("a dry run only plans: %v %v %v", created, notes, err)
	}
}

func TestEnsureDirectivesKeepsHandWrittenFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "codecov.yml"), []byte("coverage: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, _, notes, err := ensureDirectives(root, false)
	if err != nil || contains(created, "codecov.yml") || len(notes) != 1 || !strings.Contains(notes[0], "hand-written codecov.yml") {
		t.Fatalf("%v %v %v", created, notes, err)
	}
	if body, _ := os.ReadFile(filepath.Join(root, "codecov.yml")); string(body) != "coverage: {}\n" {
		t.Error("the hand-written codecov.yml must stay")
	}
}

func TestEnsureDirectivesReportsDirectivesItCannotWrite(t *testing.T) {
	blocked := t.TempDir()
	if err := os.MkdirAll(filepath.Join(blocked, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, ".github", "dendrite"), []byte("a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ensureDirectives(blocked, false); err == nil {
		t.Error("directives that cannot be written are an error")
	}
}

func TestEnsureDirectivesRewritesALintConfigAfterALimitChange(t *testing.T) {
	root := t.TempDir()
	prepareCI(t, root)
	set, err := directive.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	set.Complexity.MaxCyclomatic = 5
	if err := directive.Save(root, set); err != nil {
		t.Fatal(err)
	}
	created, updated, _, err := ensureDirectives(root, false)
	if err != nil || len(created) != 0 || !contains(updated, ".golangci.yml") {
		t.Errorf("a changed limit rewrites the generated config: created %v updated %v err %v", created, updated, err)
	}
}

func TestIsDendriteHookEntryOnlyAcceptsEntriesRunningOurHooks(t *testing.T) {
	ours := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": editLockCommand}}}
	for name, tc := range map[string]struct {
		entry any
		want  bool
	}{
		"not an object":  {"just a string", false},
		"no hooks":       {map[string]any{"matcher": "Edit"}, false},
		"empty hooks":    {map[string]any{"hooks": []any{}}, false},
		"someone else's": {map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "./lint.sh"}}}, false},
		"ours":           {ours, true},
	} {
		if got := isDendriteHookEntry(tc.entry); got != tc.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestAGoOnlyProjectGetsNoBunOrUvAnywhere(t *testing.T) {
	root := t.TempDir()
	report := prepareCI(t, root)
	ci := readCI(t, root)
	for _, banned := range []string{"setup-bun", "setup-uv", "bun ", "uv "} {
		if strings.Contains(ci, banned) {
			t.Errorf("a Go project's workflow mentions %q:\n%s", banned, ci)
		}
	}
	if !strings.Contains(ci, "loom-gate complexity") || !strings.Contains(ci, "golangci-lint") {
		t.Errorf("the workflow gates complexity and lints Go:\n%s", ci)
	}
	copilot, err := os.ReadFile(filepath.Join(root, ".github", "copilot-instructions.md"))
	if err != nil || strings.Contains(string(copilot), "Bun") || strings.Contains(string(copilot), "uv`") {
		t.Errorf("the instructions list only the project's toolchains: %v\n%s", err, copilot)
	}
	if report.CICD.CoverageThreshold != 100 {
		t.Errorf("the coverage threshold is the directive's, got %v", report.CICD.CoverageThreshold)
	}
}

func TestComplexityLimitsComeFromTheDirectivesInEveryFile(t *testing.T) {
	root := t.TempDir()
	prepareCI(t, root)
	set, err := directive.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	set.Complexity.MaxCyclomatic, set.Complexity.MaxCognitive = 7, 11
	if err := directive.Save(root, set); err != nil {
		t.Fatal(err)
	}
	prepareCI(t, root)
	golangci, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil || !strings.Contains(string(golangci), "min-complexity: 7") || !strings.Contains(string(golangci), "min-complexity: 11") {
		t.Errorf("golangci limits follow the directives: %v\n%s", err, golangci)
	}
	copilot, _ := os.ReadFile(filepath.Join(root, ".github", "copilot-instructions.md"))
	if !strings.Contains(string(copilot), "cyclomatic complexity <= 7 and cognitive complexity <= 11") {
		t.Errorf("the instructions follow the directives:\n%s", copilot)
	}
}

func TestAdoptCITakesOverAHandWrittenWorkflowAndKeepsItsJobsReachable(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n\ngo 1.25\n")
	fsutiltest.WriteFile(t, root, ".github/workflows/ci.yml", "name: CI\njobs:\n  sidecar-build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make sidecar\n")
	fsutiltest.WriteFile(t, root, ".github/workflows/quality.yml", "name: quality\njobs:\n  loc:\n    runs-on: ubuntu-latest\n    steps:\n      - run: bash scripts/check-max-lines.sh\n")
	t.Setenv("DENDRITE_HOME", t.TempDir())
	p := NewPreparer()
	report, err := p.Prepare(t.Context(), PrepareOptions{ProjectPath: root, EnforceCICD: true, AdoptCI: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(readCI(t, root), ManagedHeader) {
		t.Error("the workflow is now generated")
	}
	prev, err := os.ReadFile(filepath.Join(root, ".github", "dendrite", "ci.previous.yml"))
	if err != nil || !strings.Contains(string(prev), "sidecar-build") {
		t.Errorf("the old workflow is archived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "quality.yml")); err == nil {
		t.Error("the hand-written quality workflow is replaced by the quality job")
	}
	joined := strings.Join(report.Notes, "\n")
	if !strings.Contains(joined, "sidecar-build") || !strings.Contains(joined, "pipeline.yml") {
		t.Errorf("the notes name the jobs to move into the extension: %v", report.Notes)
	}
}

func TestTheProjectExtensionAddsEnvPreTestStepsAndJobs(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, ".github/dendrite/pipeline.yml", "env:\n  FOO: bar\npre_test:\n  - name: Build the sidecar\n    run: make sidecar\njobs:\n  docs:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make docs\nskip_coverage_floor: true\n")
	prepareCI(t, root)
	ci := readCI(t, root)
	for _, want := range []string{"FOO: bar", "Build the sidecar", "make sidecar", "  docs:", "make docs"} {
		if !strings.Contains(ci, want) {
			t.Errorf("the workflow lacks %q:\n%s", want, ci)
		}
	}
	if strings.Contains(ci, "Coverage floor") {
		t.Error("skip_coverage_floor leaves the threshold to the project")
	}
}

func TestAMalformedExtensionIsAnErrorThatNamesTheFile(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n\ngo 1.25\n")
	fsutiltest.WriteFile(t, root, ".github/dendrite/pipeline.yml", "unknown_key: 1\n")
	t.Setenv("DENDRITE_HOME", t.TempDir())
	_, err := NewPreparer().Prepare(t.Context(), PrepareOptions{ProjectPath: root, EnforceCICD: true})
	if err == nil || !strings.Contains(err.Error(), "pipeline.yml") {
		t.Errorf("want an error naming pipeline.yml, got %v", err)
	}
}
