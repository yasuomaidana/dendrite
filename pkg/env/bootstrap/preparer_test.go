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
	})

	t.Run("detect rust and polyglot typescript project", func(t *testing.T) {
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
		if len(fp.SecondaryStacks) == 0 || fp.SecondaryStacks[0] != StackTypeScript {
			t.Errorf("expected StackTypeScript secondary, got %v", fp.SecondaryStacks)
		}
	})

	t.Run("detect python project", func(t *testing.T) {
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

func TestPreparePipeline_EndToEnd(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prepare-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create fake go project
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module github.com/org/app\n\ngo 1.24\n"), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, ".gemini", "skills", "my-skill"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, ".gemini", "skills", "my-skill", "SKILL.md"), []byte("# My Skill\n"), 0644)

	p := NewPreparer()
	p.SetLookPath(func(cmd string) (string, error) {
		if cmd == "copilot" {
			return "/usr/local/bin/copilot", nil
		}
		return "", errors.New("not found")
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
	if report.CICD.CoverageThreshold != 85.0 {
		t.Errorf("expected coverage threshold 85.0, got %f", report.CICD.CoverageThreshold)
	}

	// Verify files actually created on disk
	copilotMD := filepath.Join(tmpDir, ".github", "copilot-instructions.md")
	if !fileExists(copilotMD) {
		t.Errorf("expected %s to exist", copilotMD)
	}
	content, _ := os.ReadFile(copilotMD)
	if !strings.Contains(string(content), "Zero Code Without Plane Ticket Mandate") {
		t.Errorf("missing Plane mandate in copilot instructions")
	}
	if !strings.Contains(string(content), ">= 85.0%") {
		t.Errorf("missing coverage gate in copilot instructions")
	}

	ciYML := filepath.Join(tmpDir, ".github", "workflows", "ci.yml")
	if !fileExists(ciYML) {
		t.Errorf("expected %s to exist", ciYML)
	}
	ciContent, _ := os.ReadFile(ciYML)
	if !strings.Contains(string(ciContent), "setup-go") {
		t.Errorf("expected go setup in CI workflow")
	}

	rulesMD := filepath.Join(tmpDir, ".agents", "rules.md")
	if !fileExists(rulesMD) {
		t.Errorf("expected %s to exist", rulesMD)
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
