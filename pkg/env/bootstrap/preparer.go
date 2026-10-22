package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/projectid"
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
		{"go", "go", "curl -fsSL https://go.dev/dl/ || brew install go", "go test -v -race -cover ./pkg/...", "go build -v -o bin/dendrite ./cmd/dendrite"},
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

	// 3. Ensure .gitignore excludes coverage, build artifacts, lockfiles
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

	sb.WriteString("## 3. Approved & Enforced Toolchains (Go, Cargo, Bun, uv)\n")
	sb.WriteString("All tooling and dependency operations must use project-approved toolchains:\n")
	sb.WriteString("- **Go**: `go` (`go test -v -race -cover ./pkg/...`, `golangci-lint`)\n")
	sb.WriteString("- **Rust**: `cargo` (`cargo test --verbose`, `cargo build --release`, `cargo clippy`)\n")
	sb.WriteString("- **JavaScript / TypeScript**: `bun` (`bun test`, `bun run build`, `bun install --frozen-lockfile`) - legacy `npm` and `yarn` are strictly banned\n")
	sb.WriteString("- **Python**: `uv` (`uv run pytest`, `uv pip list`, `uv venv`) - legacy `pip install` is strictly banned\n\n")

	sb.WriteString("## 4. Technology Stack & Coding Standards\n")
	sb.WriteString(fmt.Sprintf("- **Primary Stack**: %s\n", strings.ToUpper(string(fp.PrimaryStack))))
	if fp.GoVersion != "" {
		sb.WriteString(fmt.Sprintf("- **Go Toolchain**: %s\n", fp.GoVersion))
	}
	if len(fp.Toolchains) > 0 {
		sb.WriteString(fmt.Sprintf("- **Active Toolchains**: %s\n", strings.Join(fp.Toolchains, ", ")))
	}
	if len(fp.Linters) > 0 {
		sb.WriteString(fmt.Sprintf("- **Enforced Linters**: %s\n", strings.Join(fp.Linters, ", ")))
	}
	sb.WriteString("- **Complexity Limits**: Cyclomatic Complexity <= 15 per function, Cognitive Complexity <= 20, max function length <= 80 lines.\n\n")

	sb.WriteString("## 5. Model Precedence & Zero-Cost Delegation\n")
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

	// 1. Go toolchain (if Go is detected or is primary)
	if fp.HasGoMod || fp.PrimaryStack == StackGo {
		goVer := fp.GoVersion
		if goVer == "" {
			goVer = "1.25"
		}
		sb.WriteString("      - name: Set up Go\n")
		sb.WriteString("        uses: actions/setup-go@v5\n")
		sb.WriteString(fmt.Sprintf("        with:\n          go-version: '%s'\n          cache: true\n\n", goVer))
		sb.WriteString("      - name: Run Tests with Coverage\n")
		sb.WriteString("        run: |\n          go test -v -race -coverprofile=coverage.out -covermode=atomic ./pkg/...\n")
		sb.WriteString("      - name: Verify Coverage Threshold (>= 85%)\n")
		sb.WriteString("        run: |\n          total=$(go tool cover -func=coverage.out | grep 'total:' | awk '{print $3}' | sed 's/%//')\n          echo \"Total Statement Coverage: ${total}%\"\n          awk -v cov=\"$total\" 'BEGIN { if (cov < 85.0) { print \"[-] Statement coverage \" cov \"% is below required 85.0%\"; exit 1 } else { print \"[\u2713] Coverage gate passed: \" cov \"%\"; exit 0 } }'\n\n")
	}

	// 2. Rust & Cargo toolchain
	if fp.HasCargoToml || fp.HasCargo || fp.PrimaryStack == StackRust {
		sb.WriteString("      - name: Set up Rust (cargo)\n")
		sb.WriteString("        uses: dtolnay/rust-toolchain@stable\n\n")
		sb.WriteString("      - name: Run Cargo Tests\n")
		sb.WriteString("        run: cargo test --verbose\n\n")
	}

	// 3. JS/TS & Bun toolchain
	if fp.HasPackageJSON || fp.HasBun || fp.PrimaryStack == StackJavaScript || fp.PrimaryStack == StackTypeScript {
		sb.WriteString("      - name: Set up Bun\n")
		sb.WriteString("        uses: oven-sh/setup-bun@v2\n")
		sb.WriteString("        with:\n          bun-version: latest\n\n")
		sb.WriteString("      - name: Run Bun Tests & Build\n")
		sb.WriteString("        run: |\n          if [ -d \"website\" ]; then\n            cd website && bun install --frozen-lockfile && bun run build\n          else\n            bun test\n          fi\n\n")
	}

	// 4. Python & uv toolchain
	if fp.HasPyProject || fp.HasUV || fp.PrimaryStack == StackPython {
		sb.WriteString("      - name: Set up uv (Python)\n")
		sb.WriteString("        uses: astral-sh/setup-uv@v5\n")
		sb.WriteString("        with:\n          enable-cache: true\n\n")
		sb.WriteString("      - name: Verify Python Environment with uv\n")
		sb.WriteString("        run: |\n          uv --version\n          if [ -f \"scripts/pyproject.toml\" ]; then\n            cd scripts && uv pip list || true\n          fi\n")
	}

	return sb.String()
}

func renderAgentRules(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("# Autonomous Agent Operating Rules\n\n")
	sb.WriteString("## Authoritative Project Management & Plane Ownership\n")
	sb.WriteString("- Plane v2 (`http://localhost:8090`) is the authoritative owner of ticket state.\n")
	sb.WriteString("- NEVER use `DEV_STATUS.md` to decide what to work on; query Plane via `./bin/dendrite chronos next` or `dendrite_ticket_smart_select`.\n")
	sb.WriteString("- Completed features must transition to `To Verify` (intermediate verification state) for human sign-off, never directly to `Done`.\n\n")

	sb.WriteString("## Architecture & CI/CD Tooling Defaults\n")
	sb.WriteString("- Default CI/CD & Assistant Tool: **GitHub Copilot** (`.github/copilot-instructions.md`).\n")
	sb.WriteString(fmt.Sprintf("- Primary Stack: `%s`\n", fp.PrimaryStack))
	sb.WriteString("- Standardized Toolsets:\n")
	sb.WriteString("  - **Go**: `go test -v -race`, `golangci-lint`\n")
	sb.WriteString("  - **Rust**: `cargo test`, `cargo build`, `cargo clippy`\n")
	sb.WriteString("  - **JavaScript / TypeScript**: `bun test`, `bun run build` (zero npm)\n")
	sb.WriteString("  - **Python**: `uv run`, `uv pip` (zero legacy pip)\n")
	sb.WriteString("- Statement coverage threshold: **>= 85.0%**.\n")
	sb.WriteString("- Anti-mock policy: genuine structs and Testcontainers only.\n\n")

	sb.WriteString("## Dendrite-First Workflow (every model and session)\n")
	writeNumberedRules(&sb)

	return sb.String()
}

func renderAgentsOverview(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("# Project Agents & Environment Orchestration\n\n")
	sb.WriteString("This repository is orchestrated with autonomous multi-agent loops and governed by Dendrite.\n\n")
	sb.WriteString("## Standard Tooling\n")
	sb.WriteString("- **Default CI/CD Assistant**: GitHub Copilot (`.github/copilot-instructions.md`)\n")
	sb.WriteString(fmt.Sprintf("- **Runtime Stack**: %s\n", strings.ToUpper(string(fp.PrimaryStack))))
	sb.WriteString("- **Enforced Toolchains**: Go (`go`), Rust (`cargo`), Bun (`bun`), uv (`uv`)\n")
	sb.WriteString("- **Authoritative Issue Tracker**: Plane v2 (`http://localhost:8090`)\n")
	sb.WriteString("- **Token Governor**: LiteLLM Gateway (`http://localhost:4000`)\n\n")
	sb.WriteString("## Preparation & Sync\n")
	sb.WriteString("To synchronize agent rules, skills, and enforce CI/CD tooling defaults, execute:\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite soma prepare   # alias: dendrite env prepare\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Dendrite-First Workflow (all models: Claude, Antigravity/AGY, Gemini, Copilot, humans)\n")
	writeNumberedRules(&sb)
	sb.WriteString(fmt.Sprintf("\nSee the `%s` skill for commands and cleanup rules.\n\n", DendriteWorkflowSkill))

	sb.WriteString("## Commit & VCS Guidance\n")
	sb.WriteString("- `dendrite quantal` (alias `commit`) pushes the new commit to the configured remote (origin preferred) after every successful commit; use `--no-push` or `DENDRITE_COMMIT_PUSH=false` to opt out; it never force-pushes and refuses to push main/master.\n")
	sb.WriteString("- `dendrite quantal --synapse[=<id>]` commits the current project, then walks its synapse peers: dirty peers are committed through the same AI-assisted engine after confirmation (`--yes` skips the prompt), and clean peers ahead of their upstream are pushed.\n")
	sb.WriteString("- `dendrite sync` pushes unpushed commits and refreshes the Plane board (canonical states + DEV_STATUS when it changed) for the current project and every synapse peer, then records synapse health (`active`/`broken`). `dendrite sync --all --watch` keeps every synapse current (`DENDRITE_SYNC_INTERVAL`, default 10m).\n")
	sb.WriteString("- Plane workflow states are canonical across projects (`plane.CanonicalStates`); fix drift with `dendrite chronos states reconcile --all`.\n\n")

	sb.WriteString("## Session Start & Handoff\n")
	sb.WriteString("Every new session (Claude, AGY, or human) starts with:\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite session resume\n")
	sb.WriteString("```\n")
	sb.WriteString("Orchestrators run `dendrite session checkpoint` after each finished ticket and before long waits:\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite session checkpoint [--worktree dir] [--decision \"...\"]... [--next \"DEN-XXX\"]... [--note \"...\"]\n")
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

// RagEnvFile, under the Dendrite home, holds the DENDRITE_RAG_* variables the sidecar
// launcher passes to dendrite-rag (pkg/sidecar).
const RagEnvFile = "rag.env"

// ensureRagEnv adds DENDRITE_RAG_PG_URL, the shared-tier database of the control stack, to
// $DENDRITE_HOME/rag.env when it is missing; an existing value is never changed. The host
// port follows POSTGRES_PORT (environment, then the project's .env), default 5432.
func ensureRagEnv(projectRoot string, dryRun bool) (created, updated bool, err error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return false, false, err
	}
	path := filepath.Join(home, RagEnvFile)
	data, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, false, readErr
	}
	if dotEnvValue(string(data), "DENDRITE_RAG_PG_URL") != "" {
		return false, false, nil
	}
	port := strings.TrimSpace(os.Getenv("POSTGRES_PORT"))
	if port == "" {
		dotenv, _ := os.ReadFile(filepath.Join(projectRoot, ".env"))
		port = dotEnvValue(string(dotenv), "POSTGRES_PORT")
	}
	if port == "" {
		port = "5432"
	}
	created, updated = readErr != nil, readErr == nil
	if dryRun {
		return created, updated, nil
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += fmt.Sprintf("DENDRITE_RAG_PG_URL=postgresql://dendrite_rag:dendrite_rag@localhost:%s/dendrite_rag\n", port)
	if err := os.MkdirAll(home, 0o755); err != nil {
		return false, false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return false, false, err
	}
	return created, updated, nil
}

// dotEnvValue returns key's value in KEY=VALUE text, unquoted; "" when absent.
func dotEnvValue(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+"="); ok {
			return strings.Trim(strings.TrimSpace(v), "\"'")
		}
	}
	return ""
}
