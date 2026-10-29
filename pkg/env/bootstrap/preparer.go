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

// ProbeToolchains evaluates availability of canonical toolchains: go, cargo, bun, uv.
func (p *Preparer) ProbeToolchains() []ToolchainInfo {
	if p.lookPathFn == nil {
		p.lookPathFn = exec.LookPath
	}

	defs := []struct {
		name       string
		bin        string
		installCmd string
		testCmd    string
		buildCmd   string
	}{
		{"go", "go", "curl -fsSL https://go.dev/dl/ || brew install go", "go test -v -race -cover ./pkg/...", "go build -v ./..."},
		{"cargo", "cargo", "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh", "cargo test --verbose", "cargo build --release"},
		{"bun", "bun", "curl -fsSL https://bun.sh/install | bash", "bun test", "bun run build"},
		{"uv", "uv", "curl -LsSf https://astral.sh/uv/install.sh | sh", "uv run pytest", "uv build"},
	}

	var results []ToolchainInfo
	for _, d := range defs {
		path, err := p.lookPathFn(d.bin)
		installed := err == nil && path != ""
		info := ToolchainInfo{
			Name:           d.name,
			Binary:         d.bin,
			Installed:      installed,
			InstallCommand: d.installCmd,
			TestCommand:    d.testCmd,
			BuildCommand:   d.buildCmd,
		}
		if installed {
			info.Binary = path
		}
		results = append(results, info)
	}

	return results
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

	// 1. CI/CD & Toolchain Enforcement
	if opts.EnforceCICD {
		installed, bin, installCmd := p.ProbeGitHubCopilot()
		toolchains := p.ProbeToolchains()

		cicd := CICDStatus{
			DefaultTool:         opts.DefaultTool,
			ToolInstalled:       installed,
			ToolBinary:          bin,
			ToolInstallCommand:  installCmd,
			CoverageThreshold:   85.0,
			CopilotInstructions: filepath.Join(".github", "copilot-instructions.md"),
			CIWorkflowPath:      filepath.Join(".github", "workflows", "ci.yml"),
			Toolchains:          toolchains,
			Enforced:            true,
		}

		created, updated, notes, err := p.scaffoldCICD(opts.ProjectPath, fp, opts.DryRun)
		if err != nil {
			return nil, fmt.Errorf("CI/CD scaffolding failed: %w", err)
		}
		cicd.FilesCreated = created
		cicd.FilesUpdated = updated
		report.CICD = cicd
		report.FilesCreated = append(report.FilesCreated, created...)
		report.FilesUpdated = append(report.FilesUpdated, updated...)
		report.Notes = append(report.Notes, notes...)

		// A project without quality directives gets the defaults and the files derived from them.
		dirCreated, dirNotes, err := ensureDirectives(opts.ProjectPath, opts.DryRun)
		if err != nil {
			return nil, fmt.Errorf("quality directives failed: %w", err)
		}
		report.FilesCreated = append(report.FilesCreated, dirCreated...)
		report.Notes = append(report.Notes, dirNotes...)

		// Rust crates with tests use the rstest crate.
		rstestChanged, rstestNotes := ensureRstest(opts.ProjectPath, opts.DryRun)
		report.FilesUpdated = append(report.FilesUpdated, rstestChanged...)
		report.Notes = append(report.Notes, rstestNotes...)
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

	// 3. The dendrite-rag shared tier: DENDRITE_RAG_PG_URL in $DENDRITE_HOME/rag.env.
	envCreated, envUpdated, err := ensureRagEnv(opts.ProjectPath, opts.DryRun)
	if err != nil {
		return nil, fmt.Errorf("writing %s: %w", RagEnvFile, err)
	}
	if envCreated {
		report.FilesCreated = append(report.FilesCreated, "$DENDRITE_HOME/"+RagEnvFile)
	} else if envUpdated {
		report.FilesUpdated = append(report.FilesUpdated, "$DENDRITE_HOME/"+RagEnvFile)
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

func (p *Preparer) scaffoldCICD(root string, fp ProjectFingerprint, dryRun bool) (created, updated, notes []string, err error) {

	githubDir := filepath.Join(root, ".github")
	workflowsDir := filepath.Join(githubDir, "workflows")

	if !dryRun {
		if err := os.MkdirAll(workflowsDir, 0755); err != nil {
			return nil, nil, nil, err
		}
	}

	// 1. .github/copilot-instructions.md
	copilotPath := filepath.Join(githubDir, "copilot-instructions.md")
	copilotContent := renderCopilotInstructions(fp)
	c, u, err := writeOrUpdateFile(copilotPath, copilotContent, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	if c {
		created = append(created, filepath.Join(".github", "copilot-instructions.md"))
	} else if u {
		updated = append(updated, filepath.Join(".github", "copilot-instructions.md"))
	}

	// 2. .github/workflows/ci.yml
	workflowPath := filepath.Join(workflowsDir, "ci.yml")
	workflowContent := renderCIWorkflow(fp)
	c, u, kept, err := writeManagedFile(workflowPath, workflowContent, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	if kept {
		notes = append(notes, "kept the hand-written "+filepath.ToSlash(filepath.Join(".github", "workflows", "ci.yml"))+": it does not carry the managed header, so Dendrite does not overwrite it")
	}
	if c {
		created = append(created, filepath.Join(".github", "workflows", "ci.yml"))
	} else if u {
		updated = append(updated, filepath.Join(".github", "workflows", "ci.yml"))
	}

	// 3. Ensure .gitignore excludes coverage, build artifacts, lockfiles
	giCreated, giUpdated := ensureGitignoreEntries(root, dryRun)
	if giCreated {
		created = append(created, ".gitignore")
	} else if giUpdated {
		updated = append(updated, ".gitignore")
	}

	return created, updated, notes, nil
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
	agentsMDContent := renderAgentsOverview(fp) + renderSynapsesSection(root)
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

	// 4. Managed dendrite-workflow skill: identical in every agent family's skill dir.
	workflowSkill := renderDendriteWorkflowSkill()
	for _, dir := range skillDirs {
		target := filepath.Join(root, filepath.FromSlash(dir), DendriteWorkflowSkill, "SKILL.md")
		c, u, err := writeOrUpdateFile(target, workflowSkill, dryRun)
		if err != nil {
			return nil, nil, nil, err
		}
		rel, _ := filepath.Rel(root, target)
		if c {
			created = append(created, rel)
		} else if u {
			updated = append(updated, rel)
		}
	}

	// 5. Per-agent entry files importing AGENTS.md, only when missing.
	for _, name := range pointerFiles {
		target := filepath.Join(root, name)
		if fileExists(target) {
			continue
		}
		if c, _, err := writeOrUpdateFile(target, pointerFileContent, dryRun); err != nil {
			return nil, nil, nil, err
		} else if c {
			created = append(created, name)
		}
	}

	// 6. Claude Code hook: stops a raw recursive search once and points at Dendrite's retrieval.
	// An existing settings.json is merged into, never replaced; one that cannot be parsed is left alone.
	settingsPath := filepath.Join(root, filepath.FromSlash(claudeSettingsPath))
	existing, _ := os.ReadFile(settingsPath)
	if merged, changed, err := mergeClaudeHooks(existing); err != nil {
		fmt.Fprintf(os.Stderr, "[!] %v\n", err)
	} else if changed {
		if c, u, err := writeOrUpdateFile(settingsPath, string(merged), dryRun); err != nil {
			return nil, nil, nil, err
		} else if c {
			created = append(created, claudeSettingsPath)
		} else if u {
			updated = append(updated, claudeSettingsPath)
		}
	}

	// 7. Git pre-commit hook: the ticket lock for agents that do not go through Claude Code.
	gitCreated, gitUpdated, gitNote, gitErr := installGitHook(root, dryRun)
	created, updated = append(created, gitCreated...), append(updated, gitUpdated...)
	if gitErr != nil {
		fmt.Fprintf(os.Stderr, "[!] git pre-commit hook: %v\n", gitErr)
	}
	if gitNote != "" {
		fmt.Fprintf(os.Stderr, "[!] %s\n", gitNote)
	}

	// 8. MCP configs for Claude, AGY and JetBrains: dendrite (core profile) and one server per synapse.
	mcpCreated, mcpUpdated, mcpErr := writeMCPConfigs(root, dryRun)
	created, updated = append(created, mcpCreated...), append(updated, mcpUpdated...)
	if mcpErr != nil {
		fmt.Fprintf(os.Stderr, "[!] MCP config left alone: %v\n", mcpErr)
	}

	// Discover existing skills in every agent family's skill dir
	skillSet := make(map[string]bool)
	for _, dir := range skillDirs {
		dir = filepath.Join(root, filepath.FromSlash(dir))
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
		".venv/",
		"node_modules/",
		// .dendrite/ is local tool state (project identity, Plane link, caches): never committed.
		".dendrite/",
	}

	lines := map[string]bool{}
	for _, l := range strings.Split(existing, "\n") {
		lines[strings.TrimSpace(l)] = true
	}
	var toAppend []string
	for _, n := range needed {
		if !lines[n] && !(n == ".dendrite/" && lines[".dendrite"]) {
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

// EnsureGitignoreLine appends line to root/.gitignore unless a line equal to it is already
// there. It reports whether the file changed, or would change when dryRun is set.
func EnsureGitignoreLine(root, line string, dryRun bool) (bool, error) {
	path := filepath.Join(root, ".gitignore")
	var existing string
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	for _, l := range strings.Split(existing, "\n") {
		if strings.TrimSpace(l) == line {
			return false, nil
		}
	}
	if dryRun {
		return true, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		line = "\n" + line
	}
	_, err = f.WriteString(line + "\n")
	return err == nil, err
}
