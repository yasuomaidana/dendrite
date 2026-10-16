package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectStack(t *testing.T) {
	t.Run("detect go project with go.mod", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "go-proj-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		goModContent := "module testproj\n\ngo 1.24\n"
		if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
			t.Fatalf("failed to write go.mod: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, ".golangci.yml"), []byte("linters:\n  enable:\n    - errcheck\n"), 0644); err != nil {
			t.Fatalf("failed to write .golangci.yml: %v", err)
		}

		p := NewPreparer()
		fp, err := p.DetectStack(tmpDir)
		if err != nil {
			t.Fatalf("DetectStack failed: %v", err)
		}

		if fp.PrimaryStack != StackGo {
			t.Errorf("expected StackGo, got %s", fp.PrimaryStack)
		}
		if fp.GoVersion != "1.24" {
			t.Errorf("expected GoVersion 1.24, got %s", fp.GoVersion)
		}
		if len(fp.Linters) == 0 || fp.Linters[0] != "golangci-lint" {
			t.Errorf("expected golangci-lint in linters, got %v", fp.Linters)
		}
		if len(fp.Toolchains) == 0 || fp.Toolchains[0] != "go" {
			t.Errorf("expected go in toolchains, got %v", fp.Toolchains)
		}
	})

	t.Run("detect rust and polyglot typescript project with cargo and bun", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "polyglot-proj-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		_ = os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0644)
		_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte("{\"name\": \"demo-web\"}\n"), 0644)
		_ = os.WriteFile(filepath.Join(tmpDir, "tsconfig.json"), []byte("{}\n"), 0644)

		p := NewPreparer()
		fp, err := p.DetectStack(tmpDir)
		if err != nil {
			t.Fatalf("DetectStack failed: %v", err)
		}

		if fp.PrimaryStack != StackRust {
			t.Errorf("expected StackRust primary, got %s", fp.PrimaryStack)
		}
		if !fp.HasCargo {
			t.Errorf("expected HasCargo true")
		}
		if !fp.HasBun {
			t.Errorf("expected HasBun true")
		}
		if len(fp.SecondaryStacks) == 0 || fp.SecondaryStacks[0] != StackTypeScript {
			t.Errorf("expected StackTypeScript secondary, got %v", fp.SecondaryStacks)
		}
		if tc, ok := fp.ToolchainDetails["cargo"]; !ok || tc.Binary != "cargo" {
			t.Errorf("expected cargo toolchain details")
		}
		if tc, ok := fp.ToolchainDetails["bun"]; !ok || tc.Binary != "bun" {
			t.Errorf("expected bun toolchain details")
		}
	})

	t.Run("detect python project with uv", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "py-proj-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		_ = os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[project]\nname = \"app\"\n"), 0644)

		p := NewPreparer()
		fp, err := p.DetectStack(tmpDir)
		if err != nil {
			t.Fatalf("DetectStack failed: %v", err)
		}

		if fp.PrimaryStack != StackPython {
			t.Errorf("expected StackPython, got %s", fp.PrimaryStack)
		}
		if !fp.HasUV {
			t.Errorf("expected HasUV true")
		}
		if tc, ok := fp.ToolchainDetails["uv"]; !ok || tc.Binary != "uv" {
			t.Errorf("expected uv toolchain details")
		}
	})
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

func TestPreparePipeline_EndToEnd(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prepare-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create fake polyglot project (Go + Cargo + Bun + uv)
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module github.com/org/app\n\ngo 1.25\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"rust-core\"\n"), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "website"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "website", "package.json"), []byte("{\"name\": \"web-portal\"}\n"), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "scripts"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "scripts", "pyproject.toml"), []byte("[project]\nname = \"scripts\"\n"), 0644)

	_ = os.MkdirAll(filepath.Join(tmpDir, ".gemini", "skills", "my-skill"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".gemini", "skills", "my-skill", "SKILL.md"), []byte("# My Skill\n"), 0644)

	p := NewPreparer()
	p.SetLookPath(func(cmd string) (string, error) {
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
	})

	ctx := context.Background()

	// 1. Dry run execution
	dryReport, err := p.Prepare(ctx, PrepareOptions{
		ProjectPath: tmpDir,
		EnforceCICD: true,
		SyncSkills:  true,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if !dryReport.Success {
		t.Errorf("expected dry run success")
	}
	if len(dryReport.FilesCreated) == 0 {
		t.Errorf("expected files planned for creation in dry run")
	}

	// Verify no files written to disk during dry run
	if fileExists(filepath.Join(tmpDir, ".github", "copilot-instructions.md")) {
		t.Errorf("dry run should not create files on disk")
	}

	// 2. Real execution
	report, err := p.Prepare(ctx, PrepareOptions{
		ProjectPath: tmpDir,
		EnforceCICD: true,
		SyncSkills:  true,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("real prepare failed: %v", err)
	}

	if !report.Success {
		t.Errorf("expected report success")
	}
	if report.CICD.DefaultTool != "copilot" {
		t.Errorf("expected default tool copilot, got %s", report.CICD.DefaultTool)
	}
	if len(report.CICD.Toolchains) != 4 {
		t.Errorf("expected 4 toolchains in CICD report, got %d", len(report.CICD.Toolchains))
	}

	// Verify files actually created on disk
	copilotMD := filepath.Join(tmpDir, ".github", "copilot-instructions.md")
	if !fileExists(copilotMD) {
		t.Errorf("expected %s to exist", copilotMD)
	}
	content, _ := os.ReadFile(copilotMD)
	strContent := string(content)
	if !strings.Contains(strContent, "Zero Code Without Plane Ticket Mandate") {
		t.Errorf("missing Plane mandate in copilot instructions")
	}
	if !strings.Contains(strContent, "Approved & Enforced Toolchains (Go, Cargo, Bun, uv)") {
		t.Errorf("missing approved toolchains section in copilot instructions")
	}
	if !strings.Contains(strContent, "bun") || !strings.Contains(strContent, "uv") || !strings.Contains(strContent, "cargo") {
		t.Errorf("missing bun/uv/cargo references in copilot instructions")
	}

	ciYML := filepath.Join(tmpDir, ".github", "workflows", "ci.yml")
	if !fileExists(ciYML) {
		t.Errorf("expected %s to exist", ciYML)
	}
	ciContent, _ := os.ReadFile(ciYML)
	strCI := string(ciContent)
	if !strings.Contains(strCI, "setup-go") {
		t.Errorf("expected go setup in CI workflow")
	}
	if !strings.Contains(strCI, "cargo test") {
		t.Errorf("expected cargo test in CI workflow")
	}
	if !strings.Contains(strCI, "setup-bun") {
		t.Errorf("expected bun setup in CI workflow")
	}
	if !strings.Contains(strCI, "setup-uv") {
		t.Errorf("expected uv setup in CI workflow")
	}

	rulesMD := filepath.Join(tmpDir, ".agents", "rules.md")
	if !fileExists(rulesMD) {
		t.Errorf("expected %s to exist", rulesMD)
	}
	rulesData, _ := os.ReadFile(rulesMD)
	if !strings.Contains(string(rulesData), "bun") || !strings.Contains(string(rulesData), "cargo") || !strings.Contains(string(rulesData), "uv") {
		t.Errorf("expected rules to state bun, cargo, uv toolsets")
	}

	agentsMD := filepath.Join(tmpDir, "AGENTS.md")
	if !fileExists(agentsMD) {
		t.Errorf("expected %s to exist", agentsMD)
	}

	// Verify default copilot and compliance skills were seeded
	copilotSkill := filepath.Join(tmpDir, ".gemini", "skills", "copilot", "SKILL.md")
	if !fileExists(copilotSkill) {
		t.Errorf("expected seeded copilot skill at %s", copilotSkill)
	}

	// 3. Second run (idempotency check: files should report 0 created)
	report2, err := p.Prepare(ctx, PrepareOptions{
		ProjectPath: tmpDir,
		EnforceCICD: true,
		SyncSkills:  true,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("second prepare run failed: %v", err)
	}
	if len(report2.FilesCreated) != 0 {
		t.Errorf("expected 0 files created on idempotent run, got %d", len(report2.FilesCreated))
	}

	// 4. Test Sync and Update aliases
	syncRep, err := p.Sync(ctx, PrepareOptions{
		ProjectPath: tmpDir,
		EnforceCICD: true,
		SyncSkills:  true,
	})
	if err != nil || !syncRep.Success {
		t.Fatalf("Sync failed: %v", err)
	}

	updateRep, err := p.Update(ctx, PrepareOptions{
		ProjectPath: tmpDir,
		EnforceCICD: true,
		SyncSkills:  true,
	})
	if err != nil || !updateRep.Success {
		t.Fatalf("Update failed: %v", err)
	}
}
