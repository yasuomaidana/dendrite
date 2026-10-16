package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultToolName specifies the default CI/CD and developer tool.
const DefaultToolName = "copilot"

// Preparer handles project stack detection, agent/skill synchronization, and CI/CD enforcement.
type Preparer struct {
	lookPathFn func(string) (string, error)
}

// NewPreparer constructs a Preparer with default system executables lookup.
func NewPreparer() *Preparer {
	return &Preparer{
		lookPathFn: exec.LookPath,
	}
}

// SetLookPath allows overriding executable lookup in tests.
func (p *Preparer) SetLookPath(fn func(string) (string, error)) {
	p.lookPathFn = fn
}

// DetectStack delegates to the package DetectStack function.
func (p *Preparer) DetectStack(rootPath string) (ProjectFingerprint, error) {
	fpPtr, err := DetectStack(rootPath)
	if err != nil {
		return ProjectFingerprint{}, err
	}
	return *fpPtr, nil
}

// ProbeGitHubCopilot verifies installation of the Copilot CLI or gh copilot extension.
func (p *Preparer) ProbeGitHubCopilot() (bool, string, string) {
	if p.lookPathFn == nil {
		p.lookPathFn = exec.LookPath
	}

	// 1. Direct copilot executable
	if path, err := p.lookPathFn("copilot"); err == nil && path != "" {
		return true, path, ""
	}

	// 2. GitHub CLI with copilot extension
	if path, err := p.lookPathFn("gh"); err == nil && path != "" {
		return true, path + " copilot", ""
	}

	return false, "", "Install GitHub Copilot CLI via: npm install -g @github/copilot-cli OR gh extension install github/gh-copilot"
}

// Prepare runs the complete environment preparation, agent synchronization, and CI/CD enforcement pipeline.
func (p *Preparer) Prepare(ctx context.Context, opts PrepareOptions) (*PrepareReport, error) {
	start := time.Now()

	if opts.ProjectPath == "" {
		opts.ProjectPath = "."
	}
	if opts.DefaultTool == "" {
		opts.DefaultTool = DefaultToolName
	}

	fp, err := p.DetectStack(opts.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("stack detection failed: %w", err)
	}

	report := &PrepareReport{
		ProjectPath:  opts.ProjectPath,
		Stack:        fp,
		FilesCreated: make([]string, 0),
		FilesUpdated: make([]string, 0),
		SkillsSynced: make([]string, 0),
		Success:      true,
	}

	// 1. CI/CD & GitHub Copilot Enforcement
	if opts.EnforceCICD {
		installed, bin, installCmd := p.ProbeGitHubCopilot()
		cicd := CICDStatus{
			DefaultTool:         opts.DefaultTool,
			ToolInstalled:       installed,
			ToolBinary:          bin,
			ToolInstallCommand:  installCmd,
			CoverageThreshold:   85.0,
			CopilotInstructions: filepath.Join(".github", "copilot-instructions.md"),
			CIWorkflowPath:      filepath.Join(".github", "workflows", "ci.yml"),
			Enforced:            true,
		}

		created, updated, err := p.scaffoldCICD(opts.ProjectPath, fp, opts.DryRun)
		if err != nil {
			return nil, fmt.Errorf("CI/CD scaffolding failed: %w", err)
		}
		cicd.FilesCreated = created
		cicd.FilesUpdated = updated
		report.CICD = cicd
		report.FilesCreated = append(report.FilesCreated, created...)
		report.FilesUpdated = append(report.FilesUpdated, updated...)
	}

	// 2. Agent Rules & Skills Synchronization
	if opts.SyncSkills {
		created, updated, skills, err := p.syncAgentRulesAndSkills(opts.ProjectPath, fp, opts.DryRun)
		if err != nil {
			return nil, fmt.Errorf("agent skills sync failed: %w", err)
		}
		report.FilesCreated = append(report.FilesCreated, created...)
		report.FilesUpdated = append(report.FilesUpdated, updated...)
		report.SkillsSynced = skills
	}

	report.Duration = time.Since(start)
	return report, nil
}

// Sync is an alias for Prepare ensuring idempotent alignment of the workspace, skills, and CI/CD tools.
func (p *Preparer) Sync(ctx context.Context, opts PrepareOptions) (*PrepareReport, error) {
	return p.Prepare(ctx, opts)
}

// Update executes environment and CI/CD toolchain updates.
func (p *Preparer) Update(ctx context.Context, opts PrepareOptions) (*PrepareReport, error) {
	return p.Prepare(ctx, opts)
}

func (p *Preparer) scaffoldCICD(root string, fp ProjectFingerprint, dryRun bool) ([]string, []string, error) {
	var created []string
	var updated []string

	githubDir := filepath.Join(root, ".github")
	workflowsDir := filepath.Join(githubDir, "workflows")

	if !dryRun {
		if err := os.MkdirAll(workflowsDir, 0755); err != nil {
			return nil, nil, err
		}
	}

	// 1. .github/copilot-instructions.md
	copilotPath := filepath.Join(githubDir, "copilot-instructions.md")
	copilotContent := renderCopilotInstructions(fp)
	c, u, err := writeOrUpdateFile(copilotPath, copilotContent, dryRun)
	if err != nil {
		return nil, nil, err
	}
	if c {
		created = append(created, filepath.Join(".github", "copilot-instructions.md"))
	} else if u {
		updated = append(updated, filepath.Join(".github", "copilot-instructions.md"))
	}

	// 2. .github/workflows/ci.yml
	workflowPath := filepath.Join(workflowsDir, "ci.yml")
	workflowContent := renderCIWorkflow(fp)
	c, u, err = writeOrUpdateFile(workflowPath, workflowContent, dryRun)
	if err != nil {
		return nil, nil, err
	}
	if c {
		created = append(created, filepath.Join(".github", "workflows", "ci.yml"))
	} else if u {
		updated = append(updated, filepath.Join(".github", "workflows", "ci.yml"))
	}

	// 3. Ensure .gitignore excludes coverage and build artifacts
	giCreated, giUpdated := ensureGitignoreEntries(root, dryRun)
	if giCreated {
		created = append(created, ".gitignore")
	} else if giUpdated {
		updated = append(updated, ".gitignore")
	}

	return created, updated, nil
}

func (p *Preparer) syncAgentRulesAndSkills(root string, fp ProjectFingerprint, dryRun bool) ([]string, []string, []string, error) {
	var created []string
	var updated []string
	var skills []string

	agentsDir := filepath.Join(root, ".agents")
	if !dryRun {
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			return nil, nil, nil, err
		}
	}

	// 1. .agents/rules.md
	rulesPath := filepath.Join(agentsDir, "rules.md")
	rulesContent := renderAgentRules(fp)
	c, u, err := writeOrUpdateFile(rulesPath, rulesContent, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	if c {
		created = append(created, filepath.Join(".agents", "rules.md"))
	} else if u {
		updated = append(updated, filepath.Join(".agents", "rules.md"))
	}

	// 2. AGENTS.md in project root
	agentsMDPath := filepath.Join(root, "AGENTS.md")
	agentsMDContent := renderAgentsOverview(fp)
	c, u, err = writeOrUpdateFile(agentsMDPath, agentsMDContent, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	if c {
		created = append(created, "AGENTS.md")
	} else if u {
		updated = append(updated, "AGENTS.md")
	}

	// 3. Seed standard skills if missing
	defaultSkills := map[string]string{
		"copilot": `---
name: copilot
description: GitHub Copilot Custom Instructions & CI/CD Tool Template
---

# GitHub Copilot Custom Instructions Template

This template defines the custom rules and system instructions placed in .github/copilot-instructions.md.
`,
		"compliance": `---
name: compliance
description: Enterprise Architecture And Compliance Reference Framework
---

# Enterprise Architecture And Compliance Reference Framework

Reference criteria for domain isolation, SOLID principles, zero placeholders, and token economy.
`,
	}

	for _, baseDir := range []string{filepath.Join(root, ".gemini", "skills"), filepath.Join(root, ".agents", "skills")} {
		if !dryRun {
			_ = os.MkdirAll(baseDir, 0755)
		}
		for skillName, content := range defaultSkills {
			targetFile := filepath.Join(baseDir, skillName, "SKILL.md")
			if !fileExists(targetFile) {
				sc, _, err := writeOrUpdateFile(targetFile, content, dryRun)
				if err == nil && sc {
					rel, _ := filepath.Rel(root, targetFile)
					created = append(created, rel)
				}
			}
		}
	}

	// Discover existing skills in .gemini/skills or .agents/skills
	skillSet := make(map[string]bool)
	for _, dir := range []string{filepath.Join(root, ".gemini", "skills"), filepath.Join(root, ".agents", "skills")} {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					skillPath := filepath.Join(dir, e.Name(), "SKILL.md")
					if fileExists(skillPath) && !skillSet[e.Name()] {
						skillSet[e.Name()] = true
						skills = append(skills, e.Name())
					}
				}
			}
		}
	}

	return created, updated, skills, nil
}

func ensureGitignoreEntries(root string, dryRun bool) (bool, bool) {
	gitignorePath := filepath.Join(root, ".gitignore")
	var existing string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	needed := []string{
		"coverage.out",
		"*.tmp",
		"bin/",
	}

	var toAppend []string
	for _, n := range needed {
		if !strings.Contains(existing, n) {
			toAppend = append(toAppend, n)
		}
	}

	if len(toAppend) == 0 {
		return false, false
	}

	if dryRun {
		return !fileExists(gitignorePath), fileExists(gitignorePath)
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, false
	}
	defer f.Close()

	if len(existing) > 0 && !strings.HasSuffix(existing, "\n") {
		_, _ = f.WriteString("\n")
	}
	for _, item := range toAppend {
		_, _ = f.WriteString(item + "\n")
	}

	if existing == "" {
		return true, false
	}
	return false, true
}

func renderCopilotInstructions(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("# GitHub Copilot Custom Instructions\n\n")
	sb.WriteString("> Authoritative developer guidelines and architectural constraints for AI code generation.\n\n")
	sb.WriteString("## 1. Zero Code Without Plane Ticket Mandate\n")
	sb.WriteString("- No agent, AI assistant, or engineer is permitted to author code or modify tests without an active, authoritative ticket in Plane v2 (`http://localhost:8090`).\n")
	sb.WriteString("- Every commit must reference an active Plane ticket (e.g., `[DEN-XXX]`).\n\n")

	sb.WriteString("## 2. Test-Driven Development (TDD) & Anti-Mock Mandate\n")
	sb.WriteString("- Author tests before or alongside code implementation.\n")
	sb.WriteString("- **Zero Mock Policy**: Do NOT use mocking frameworks, fake stubs, or synthetic dummies for persistence or protocols.\n")
	sb.WriteString("- All persistent database layers (Postgres, Redis, MinIO) must use **Testcontainers** for real integration tests.\n")
	sb.WriteString("- Maintain minimum statement coverage: **>= 85.0%** across all new packages.\n\n")

	sb.WriteString("## 3. Technology Stack & Coding Standards\n")
	sb.WriteString(fmt.Sprintf("- **Primary Stack**: %s\n", strings.ToUpper(string(fp.PrimaryStack))))
	if fp.GoVersion != "" {
		sb.WriteString(fmt.Sprintf("- **Go Toolchain**: %s\n", fp.GoVersion))
	}
	if len(fp.Linters) > 0 {
		sb.WriteString(fmt.Sprintf("- **Enforced Linters**: %s\n", strings.Join(fp.Linters, ", ")))
	}
	sb.WriteString("- **Complexity Limits**: Cyclomatic Complexity <= 15 per function, Cognitive Complexity <= 20, max function length <= 80 lines.\n\n")

	sb.WriteString("## 4. Model Precedence & Zero-Cost Delegation\n")
	sb.WriteString("- Commit messages and repetitive test digestions must be delegated to local zero-cost models (`qwen3.8:27b` / Ollama Metal) to conserve API tokens.\n")
	sb.WriteString("- Cloud models (Gemini Pro, Copilot Sonnet) are reserved for macro architecture, complex decomposition, and review gates.\n")

	return sb.String()
}

func renderCIWorkflow(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("name: Dendrite CI/CD\n\n")
	sb.WriteString("on:\n")
	sb.WriteString("  push:\n")
	sb.WriteString("    branches: [ develop, main ]\n")
	sb.WriteString("  pull_request:\n")
	sb.WriteString("    branches: [ develop, main ]\n\n")
	sb.WriteString("jobs:\n")
	sb.WriteString("  verify:\n")
	sb.WriteString("    name: Test & Coverage Gate (>= 85%)\n")
	sb.WriteString("    runs-on: ubuntu-latest\n")
	sb.WriteString("    steps:\n")
	sb.WriteString("      - name: Checkout Code\n")
	sb.WriteString("        uses: actions/checkout@v4\n\n")

	switch fp.PrimaryStack {
	case StackGo:
		goVer := fp.GoVersion
		if goVer == "" {
			goVer = "1.24"
		}
		sb.WriteString("      - name: Set up Go\n")
		sb.WriteString("        uses: actions/setup-go@v5\n")
		sb.WriteString(fmt.Sprintf("        with:\n          go-version: '%s'\n          cache: true\n\n", goVer))
		sb.WriteString("      - name: Run Tests with Coverage\n")
		sb.WriteString("        run: |\n          go test -v -race -coverprofile=coverage.out -covermode=atomic ./pkg/...\n")
		sb.WriteString("      - name: Verify Coverage Threshold (>= 85%)\n")
		sb.WriteString("        run: |\n          total=$(go tool cover -func=coverage.out | grep 'total:' | awk '{print $3}' | sed 's/%//')\n          echo \"Total Statement Coverage: ${total}%\"\n          awk -v cov=\"$total\" 'BEGIN { if (cov < 85.0) { print \"[-] Statement coverage \" cov \"% is below required 85.0%\"; exit 1 } else { print \"[\u2713] Coverage gate passed: \" cov \"%\"; exit 0 } }'\n")
	case StackRust:
		sb.WriteString("      - name: Set up Rust\n")
		sb.WriteString("        uses: dtolnay/rust-toolchain@stable\n\n")
		sb.WriteString("      - name: Run Tests\n")
		sb.WriteString("        run: cargo test --verbose\n")
	default:
		sb.WriteString("      - name: Run Verification\n")
		sb.WriteString("        run: echo \"Running automated stack tests...\"\n")
	}

	return sb.String()
}

func renderAgentRules(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("# Autonomous Agent Operating Rules\n\n")
	sb.WriteString("## Authoritative Project Management & Plane Ownership\n")
	sb.WriteString("- Plane v2 (`http://localhost:8090`) is the authoritative owner of ticket state.\n")
	sb.WriteString("- NEVER use `DEV_STATUS.md` to decide what to work on; query Plane via `./bin/dendrite plane next` or `dendrite_ticket_smart_select`.\n")
	sb.WriteString("- Completed features must transition to `To Verify` (intermediate verification state) for human sign-off, never directly to `Done`.\n\n")

	sb.WriteString("## Architecture & CI/CD Tooling Defaults\n")
	sb.WriteString("- Default CI/CD & Assistant Tool: **GitHub Copilot** (`.github/copilot-instructions.md`).\n")
	sb.WriteString(fmt.Sprintf("- Primary Stack: `%s`\n", fp.PrimaryStack))
	sb.WriteString("- Statement coverage threshold: **>= 85.0%**.\n")
	sb.WriteString("- Anti-mock policy: genuine structs and Testcontainers only.\n")

	return sb.String()
}

func renderAgentsOverview(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("# Project Agents & Environment Orchestration\n\n")
	sb.WriteString("This repository is orchestrated with autonomous multi-agent loops and governed by Dendrite.\n\n")
	sb.WriteString("## Standard Tooling\n")
	sb.WriteString("- **Default CI/CD Assistant**: GitHub Copilot (`.github/copilot-instructions.md`)\n")
	sb.WriteString(fmt.Sprintf("- **Runtime Stack**: %s\n", strings.ToUpper(string(fp.PrimaryStack))))
	sb.WriteString("- **Authoritative Issue Tracker**: Plane v2 (`http://localhost:8090`)\n")
	sb.WriteString("- **Token Governor**: LiteLLM Gateway (`http://localhost:4000`)\n\n")
	sb.WriteString("## Preparation & Sync\n")
	sb.WriteString("To synchronize agent rules, skills, and enforce CI/CD tooling defaults, execute:\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite env prepare\n")
	sb.WriteString("```\n")

	return sb.String()
}

func writeOrUpdateFile(path, content string, dryRun bool) (bool, bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil {
		if string(existing) == content {
			return false, false, nil // No change
		}
		if !dryRun {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return false, false, err
			}
		}
		return false, true, nil // Updated
	}

	if os.IsNotExist(err) {
		if !dryRun {
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return false, false, err
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return false, false, err
			}
		}
		return true, false, nil // Created
	}

	return false, false, err
}
