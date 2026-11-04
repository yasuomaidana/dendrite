package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/env/pipeline"
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
	opts = normalizePrepareOptions(opts)

	fp, err := p.DetectStack(opts.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("stack detection failed: %w", err)
	}

	set, err := directiveSet(opts.ProjectPath)
	if err != nil {
		return nil, err
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
		if err := p.stepCICD(opts, fp, set, report); err != nil {
			return nil, err
		}
		if err := stepDirectives(opts, report); err != nil {
			return nil, err
		}
		stepRstest(opts, report)
	}

	// 2. Agent Rules & Skills Synchronization
	if opts.SyncSkills {
		if err := p.stepSyncSkills(opts, fp, set, report); err != nil {
			return nil, err
		}
	}

	// 3. The dendrite-rag shared tier: DENDRITE_RAG_PG_URL in $DENDRITE_HOME/rag.env.
	if err := stepRagEnv(opts, report); err != nil {
		return nil, err
	}

	report.Duration = time.Since(start)
	return report, nil
}

func normalizePrepareOptions(opts PrepareOptions) PrepareOptions {
	if opts.ProjectPath == "" {
		opts.ProjectPath = "."
	}
	if opts.DefaultTool == "" {
		opts.DefaultTool = DefaultToolName
	}
	return opts
}

func (p *Preparer) stepCICD(opts PrepareOptions, fp ProjectFingerprint, set directive.Set, report *PrepareReport) error {
	installed, bin, installCmd := p.ProbeGitHubCopilot()
	toolchains := p.ProbeToolchains()

	cicd := CICDStatus{
		DefaultTool:         opts.DefaultTool,
		ToolInstalled:       installed,
		ToolBinary:          bin,
		ToolInstallCommand:  installCmd,
		CoverageThreshold:   set.Coverage.MinPercent,
		CopilotInstructions: filepath.Join(".github", "copilot-instructions.md"),
		CIWorkflowPath:      filepath.Join(".github", "workflows", "ci.yml"),
		Toolchains:          toolchains,
		Enforced:            true,
	}

	created, updated, notes, err := p.scaffoldCICD(opts.ProjectPath, fp, set, opts)
	if err != nil {
		return fmt.Errorf("CI/CD scaffolding failed: %w", err)
	}
	cicd.FilesCreated = created
	cicd.FilesUpdated = updated
	report.CICD = cicd
	report.FilesCreated = append(report.FilesCreated, created...)
	report.FilesUpdated = append(report.FilesUpdated, updated...)
	report.Notes = append(report.Notes, notes...)
	return nil
}

func stepDirectives(opts PrepareOptions, report *PrepareReport) error {
	dirCreated, dirUpdated, dirNotes, err := ensureDirectives(opts.ProjectPath, opts.DryRun)
	if err != nil {
		return fmt.Errorf("quality directives failed: %w", err)
	}
	report.FilesCreated = append(report.FilesCreated, dirCreated...)
	report.FilesUpdated = append(report.FilesUpdated, dirUpdated...)
	report.Notes = append(report.Notes, dirNotes...)
	return nil
}

func stepRstest(opts PrepareOptions, report *PrepareReport) {
	rstestChanged, rstestNotes := ensureRstest(opts.ProjectPath, opts.DryRun)
	report.FilesUpdated = append(report.FilesUpdated, rstestChanged...)
	report.Notes = append(report.Notes, rstestNotes...)
}

func (p *Preparer) stepSyncSkills(opts PrepareOptions, fp ProjectFingerprint, set directive.Set, report *PrepareReport) error {
	created, updated, skills, err := p.syncAgentRulesAndSkills(opts.ProjectPath, fp, set, opts.DryRun)
	if err != nil {
		return fmt.Errorf("agent skills sync failed: %w", err)
	}
	report.FilesCreated = append(report.FilesCreated, created...)
	report.FilesUpdated = append(report.FilesUpdated, updated...)
	report.SkillsSynced = skills
	return nil
}

func stepRagEnv(opts PrepareOptions, report *PrepareReport) error {
	envCreated, envUpdated, err := ensureRagEnv(opts.ProjectPath, opts.DryRun)
	if err != nil {
		return fmt.Errorf("writing %s: %w", RagEnvFile, err)
	}
	if envCreated {
		report.FilesCreated = append(report.FilesCreated, "$DENDRITE_HOME/"+RagEnvFile)
	} else if envUpdated {
		report.FilesUpdated = append(report.FilesUpdated, "$DENDRITE_HOME/"+RagEnvFile)
	}
	return nil
}

// Sync is an alias for Prepare ensuring idempotent alignment of the workspace, skills, and CI/CD tools.
func (p *Preparer) Sync(ctx context.Context, opts PrepareOptions) (*PrepareReport, error) {
	return p.Prepare(ctx, opts)
}

// Update executes environment and CI/CD toolchain updates.
func (p *Preparer) Update(ctx context.Context, opts PrepareOptions) (*PrepareReport, error) {
	return p.Prepare(ctx, opts)
}

func (p *Preparer) scaffoldCICD(root string, fp ProjectFingerprint, set directive.Set, opts PrepareOptions) (created, updated, notes []string, err error) {
	dryRun := opts.DryRun
	githubDir := filepath.Join(root, ".github")
	workflowsDir := filepath.Join(githubDir, "workflows")

	if !dryRun {
		if err := os.MkdirAll(workflowsDir, 0755); err != nil {
			return nil, nil, nil, err
		}
	}

	// 1. .github/copilot-instructions.md
	copilotPath := filepath.Join(githubDir, "copilot-instructions.md")
	copilotContent := renderCopilotInstructions(fp, set)
	c, u, err := writeOrUpdateFile(copilotPath, copilotContent, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	if c {
		created = append(created, filepath.Join(".github", "copilot-instructions.md"))
	} else if u {
		updated = append(updated, filepath.Join(".github", "copilot-instructions.md"))
	}

	// 2. .github/workflows/ci.yml, from the project's components, directives and extension
	wf, err := pipeline.Scaffold(root, set, opts.AdoptCI, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	created = append(created, wf.Created...)
	updated = append(updated, wf.Updated...)
	notes = append(notes, wf.Notes...)

	// 3. Ensure .gitignore excludes coverage, build artifacts, lockfiles
	giCreated, giUpdated := ensureGitignoreEntries(root, dryRun)
	if giCreated {
		created = append(created, ".gitignore")
	} else if giUpdated {
		updated = append(updated, ".gitignore")
	}

	return created, updated, notes, nil
}

func (p *Preparer) syncAgentRulesAndSkills(root string, fp ProjectFingerprint, set directive.Set, dryRun bool) ([]string, []string, []string, error) {
	agentsDir := filepath.Join(root, ".agents")
	if !dryRun {
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			return nil, nil, nil, err
		}
	}

	created, updated, err := syncAgentRulesStep(root, fp, set, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}

	sc, su, skills, err := syncSkillsStep(root, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	created = append(created, sc...)
	updated = append(updated, su...)

	return created, updated, skills, nil
}

func syncAgentRulesStep(root string, fp ProjectFingerprint, set directive.Set, dryRun bool) (created, updated []string, err error) {
	c, u, err := syncAgentRulesFiles(root, fp, set, dryRun)
	if err != nil {
		return nil, nil, err
	}
	created, updated = append(created, c...), append(updated, u...)

	pc, err := syncAgentPointerFiles(root, dryRun)
	if err != nil {
		return nil, nil, err
	}
	created = append(created, pc...)

	cc, cu, err := syncClaudeHooks(root, dryRun)
	if err != nil {
		return nil, nil, err
	}
	created, updated = append(created, cc...), append(updated, cu...)

	gc, gu := syncGitAndMCP(root, dryRun)
	created, updated = append(created, gc...), append(updated, gu...)
	return created, updated, nil
}

func syncAgentRulesFiles(root string, fp ProjectFingerprint, set directive.Set, dryRun bool) (created, updated []string, err error) {
	rulesPath := filepath.Join(root, ".agents", "rules.md")
	c, u, err := writeOrUpdateFile(rulesPath, renderAgentRules(fp, set), dryRun)
	if err != nil {
		return nil, nil, err
	}
	appendFileChange(&created, &updated, filepath.Join(".agents", "rules.md"), c, u)

	agentsMDPath := filepath.Join(root, "AGENTS.md")
	agentsContent := renderAgentsOverview(fp) + renderSynapsesSection(root)
	c, u, err = writeOrUpdateFile(agentsMDPath, agentsContent, dryRun)
	if err != nil {
		return nil, nil, err
	}
	appendFileChange(&created, &updated, "AGENTS.md", c, u)
	return created, updated, nil
}

func syncAgentPointerFiles(root string, dryRun bool) (created []string, err error) {
	for _, name := range pointerFiles {
		target := filepath.Join(root, name)
		if fileExists(target) {
			continue
		}
		if c, _, err := writeOrUpdateFile(target, pointerFileContent, dryRun); err != nil {
			return nil, err
		} else if c {
			created = append(created, name)
		}
	}
	return created, nil
}

func syncClaudeHooks(root string, dryRun bool) (created, updated []string, err error) {
	settingsPath := filepath.Join(root, filepath.FromSlash(claudeSettingsPath))
	existing, _ := os.ReadFile(settingsPath)
	merged, changed, err := mergeClaudeHooks(existing)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[!] %v\n", err)
		return nil, nil, nil
	}
	if !changed {
		return nil, nil, nil
	}
	c, u, err := writeOrUpdateFile(settingsPath, string(merged), dryRun)
	if err != nil {
		return nil, nil, err
	}
	appendFileChange(&created, &updated, claudeSettingsPath, c, u)
	return created, updated, nil
}

func syncGitAndMCP(root string, dryRun bool) (created, updated []string) {
	gitCreated, gitUpdated, gitNote, gitErr := installGitHook(root, dryRun)
	created, updated = append(created, gitCreated...), append(updated, gitUpdated...)
	if gitErr != nil {
		fmt.Fprintf(os.Stderr, "[!] git pre-commit hook: %v\n", gitErr)
	}
	if gitNote != "" {
		fmt.Fprintf(os.Stderr, "[!] %s\n", gitNote)
	}

	mcpCreated, mcpUpdated, mcpErr := writeMCPConfigs(root, dryRun)
	created, updated = append(created, mcpCreated...), append(updated, mcpUpdated...)
	if mcpErr != nil {
		fmt.Fprintf(os.Stderr, "[!] MCP config left alone: %v\n", mcpErr)
	}
	return created, updated
}

func syncSkillsStep(root string, dryRun bool) (created, updated, skills []string, err error) {
	c, err := seedDefaultSkills(root, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	created = append(created, c...)

	wc, wu, err := syncWorkflowSkill(root, dryRun)
	if err != nil {
		return nil, nil, nil, err
	}
	created, updated = append(created, wc...), append(updated, wu...)
	return created, updated, discoverSkills(root), nil
}

func seedDefaultSkills(root string, dryRun bool) (created []string, err error) {
	defaultSkills := map[string]string{
		"copilot":    "---\nname: copilot\ndescription: GitHub Copilot Custom Instructions & CI/CD Tool Template\n---\n\n# GitHub Copilot Custom Instructions Template\n\nThis template defines the custom rules and system instructions placed in .github/copilot-instructions.md.\n",
		"compliance": "---\nname: compliance\ndescription: Enterprise Architecture And Compliance Reference Framework\n---\n\n# Enterprise Architecture And Compliance Reference Framework\n\nReference criteria for domain isolation, SOLID principles, zero placeholders, and token economy.\n",
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
	return created, nil
}

func syncWorkflowSkill(root string, dryRun bool) (created, updated []string, err error) {
	workflowSkill := renderDendriteWorkflowSkill(root)
	for _, dir := range skillDirs {
		target := filepath.Join(root, filepath.FromSlash(dir), DendriteWorkflowSkill, "SKILL.md")
		c, u, err := writeOrUpdateFile(target, workflowSkill, dryRun)
		if err != nil {
			return nil, nil, err
		}
		rel, _ := filepath.Rel(root, target)
		appendFileChange(&created, &updated, rel, c, u)
	}
	return created, updated, nil
}

func discoverSkills(root string) []string {
	var skills []string
	skillSet := make(map[string]bool)
	for _, dir := range skillDirs {
		dir = filepath.Join(root, filepath.FromSlash(dir))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
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
	return skills
}

func appendFileChange(created, updated *[]string, rel string, isCreated, isUpdated bool) {
	if isCreated {
		*created = append(*created, rel)
	} else if isUpdated {
		*updated = append(*updated, rel)
	}
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
