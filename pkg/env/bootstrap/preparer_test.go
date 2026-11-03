package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/fsutil/fsutiltest"
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
