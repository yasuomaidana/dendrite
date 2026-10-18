package reflect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// SubcommandDoc describes a (possibly nested) subcommand of a CLI command.
type SubcommandDoc struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Usage       string    `json:"usage,omitempty"`
	Aliases     []string  `json:"aliases,omitempty"`
	Flags       []FlagDoc `json:"flags,omitempty"`
	Examples    []string  `json:"examples,omitempty"`
}

// FlagDoc describes a CLI command line flag.
type FlagDoc struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// CLICommandDoc holds structured documentation data for a primary CLI command.
type CLICommandDoc struct {
	Name        string          `json:"name"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
	Aliases     []string        `json:"aliases"`
	Subcommands []SubcommandDoc `json:"subcommands,omitempty"`
	Flags       []FlagDoc       `json:"flags,omitempty"`
	Examples    []string        `json:"examples,omitempty"`
}

// CLIRootDoc documents the root command: its global flags and the command groups.
type CLIRootDoc struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	GlobalFlags []FlagDoc         `json:"global_flags,omitempty"`
	Groups      []CLIRootGroupDoc `json:"groups,omitempty"`
}

// CLIRootGroupDoc lists the commands of one help group.
type CLIRootGroupDoc struct {
	Title    string          `json:"title"`
	Commands []SubcommandDoc `json:"commands"`
}

// documentedCommands returns the user-facing children of cmd (no help, completion or hidden commands).
func documentedCommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// IntrospectCLITree introspects a live cobra command tree and extracts documentation models for all primary commands.
func IntrospectCLITree(root *cobra.Command) []CLICommandDoc {
	if root == nil {
		return nil
	}
	cmds := documentedCommands(root)
	docs := make([]CLICommandDoc, 0, len(cmds))
	for _, cmd := range cmds {
		docs = append(docs, IntrospectCLICommand(cmd))
	}
	return docs
}

// IntrospectCLICommand extracts documentation metadata from a *cobra.Command and its nested subcommands.
func IntrospectCLICommand(cmd *cobra.Command) CLICommandDoc {
	if cmd == nil {
		return CLICommandDoc{}
	}

	doc := CLICommandDoc{
		Name:        cmd.Name(),
		Category:    categoryOf(cmd),
		Description: cmd.Short,
		Aliases:     append([]string(nil), cmd.Aliases...),
		Flags:       flagDocs(cmd.LocalNonPersistentFlags()),
		Examples:    exampleLines(cmd.Example),
	}

	var walk func(c *cobra.Command, prefix string)
	walk = func(c *cobra.Command, prefix string) {
		for _, sub := range documentedCommands(c) {
			name := strings.TrimSpace(prefix + " " + sub.Name())
			doc.Subcommands = append(doc.Subcommands, SubcommandDoc{
				Name:        name,
				Description: sub.Short,
				Usage:       sub.UseLine(),
				Aliases:     append([]string(nil), sub.Aliases...),
				Flags:       flagDocs(sub.LocalNonPersistentFlags()),
				Examples:    exampleLines(sub.Example),
			})
			walk(sub, name)
		}
	}
	walk(cmd, "")

	enrichCommandDoc(&doc)
	return doc
}

// IntrospectCLIRoot documents the root command's global flags and its command groups.
func IntrospectCLIRoot(root *cobra.Command) CLIRootDoc {
	if root == nil {
		return CLIRootDoc{}
	}
	doc := CLIRootDoc{
		Name:        root.Name(),
		Description: root.Short,
		GlobalFlags: flagDocs(root.PersistentFlags()),
	}
	byGroup := map[string][]SubcommandDoc{}
	for _, c := range documentedCommands(root) {
		byGroup[c.GroupID] = append(byGroup[c.GroupID], SubcommandDoc{Name: c.Name(), Description: c.Short, Aliases: c.Aliases})
	}
	for _, g := range root.Groups() {
		if cmds := byGroup[g.ID]; len(cmds) > 0 {
			doc.Groups = append(doc.Groups, CLIRootGroupDoc{Title: strings.TrimSuffix(g.Title, ":"), Commands: cmds})
		}
	}
	if cmds := byGroup[""]; len(cmds) > 0 {
		doc.Groups = append(doc.Groups, CLIRootGroupDoc{Title: "Other", Commands: cmds})
	}
	return doc
}

// categoryOf returns the help-group title of a top-level command.
func categoryOf(cmd *cobra.Command) string {
	if cmd.GroupID == "" || !cmd.HasParent() {
		return ""
	}
	for _, g := range cmd.Parent().Groups() {
		if g.ID == cmd.GroupID {
			return strings.TrimSuffix(g.Title, ":")
		}
	}
	return ""
}

// flagDocs converts a pflag set into sorted FlagDocs, skipping the built-in help flag.
func flagDocs(fs *pflag.FlagSet) []FlagDoc {
	var out []FlagDoc
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" || f.Hidden {
			return
		}
		name := "--" + f.Name
		if f.Shorthand != "" {
			name += ", -" + f.Shorthand
		}
		def := f.DefValue
		switch def {
		case "false", "[]", "0", "0s":
			def = ""
		}
		out = append(out, FlagDoc{Name: name, Type: f.Value.Type(), Description: f.Usage, Default: def})
	})
	return out
}

// exampleLines splits a cobra Example block into trimmed, non-empty lines.
func exampleLines(example string) []string {
	var out []string
	for _, l := range strings.Split(example, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// RenderCLIRootMarkdown renders docs/cli/dendrite.md: global flags and the grouped command index.
func RenderCLIRootMarkdown(doc CLIRootDoc) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", doc.Name))
	sb.WriteString(fmt.Sprintf("title: \"%s\"\n", doc.Name))
	sb.WriteString(fmt.Sprintf("sidebar_label: \"%s\"\n", doc.Name))
	sb.WriteString("sidebar_position: 1\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", doc.Name))
	if doc.Description != "" {
		sb.WriteString(doc.Description + "\n\n")
	}
	sb.WriteString("Commands use Neuro-Lore names; the plain names listed as aliases work too.\n\n")

	sb.WriteString("## Global Flags\n\n")
	writeFlagTable(&sb, doc.GlobalFlags)

	sb.WriteString("## Commands\n\n")
	for _, g := range doc.Groups {
		sb.WriteString(fmt.Sprintf("### %s\n\n", g.Title))
		sb.WriteString("| Command | Aliases | Description |\n")
		sb.WriteString("| :--- | :--- | :--- |\n")
		for _, c := range g.Commands {
			sb.WriteString(fmt.Sprintf("| [`%s`](%s.md) | %s | %s |\n", c.Name, c.Name, formatAliases(c.Aliases), c.Description))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatAliases(aliases []string) string {
	if len(aliases) == 0 {
		return ""
	}
	return "`" + strings.Join(aliases, "`, `") + "`"
}

// writeFlagTable writes a flag table, or "None" when there are no flags.
func writeFlagTable(sb *strings.Builder, flags []FlagDoc) {
	if len(flags) == 0 {
		sb.WriteString("None\n\n")
		return
	}
	sb.WriteString("| Flag | Description |\n")
	sb.WriteString("| :--- | :--- |\n")
	for _, flag := range flags {
		desc := flag.Description
		if flag.Default != "" {
			desc = fmt.Sprintf("%s (default: %s)", desc, flag.Default)
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s |\n", flag.Name, desc))
	}
	sb.WriteString("\n")
}

// RenderCLICommandMarkdown generates the markdown documentation string for a CLI command.
func RenderCLICommandMarkdown(doc CLICommandDoc) string {
	var sb strings.Builder

	// Frontmatter
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", doc.Name))
	sb.WriteString(fmt.Sprintf("title: \"dendrite %s\"\n", doc.Name))
	sb.WriteString(fmt.Sprintf("sidebar_label: \"%s\"\n", doc.Name))
	sb.WriteString("---\n\n")

	// Heading & Description
	sb.WriteString(fmt.Sprintf("# dendrite %s\n\n", doc.Name))
	if doc.Description != "" {
		sb.WriteString(doc.Description + "\n\n")
	} else {
		sb.WriteString("No description available.\n\n")
	}

	// Category
	sb.WriteString("## Category\n\n")
	if doc.Category != "" {
		sb.WriteString(doc.Category + "\n\n")
	} else {
		sb.WriteString("General\n\n")
	}

	// Aliases
	sb.WriteString("## Aliases\n\n")
	if len(doc.Aliases) > 0 {
		for _, alias := range doc.Aliases {
			sb.WriteString(fmt.Sprintf("- `%s`\n", alias))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("None\n\n")
	}

	// Subcommands
	sb.WriteString("## Subcommands\n\n")
	if len(doc.Subcommands) > 0 {
		sb.WriteString("| Subcommand | Description |\n")
		sb.WriteString("| :--- | :--- |\n")
		for _, sub := range doc.Subcommands {
			sb.WriteString(fmt.Sprintf("| `%s` | %s |\n", sub.Name, sub.Description))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("None\n\n")
	}

	// Flags
	sb.WriteString("## Flags\n\n")
	writeFlagTable(&sb, doc.Flags)

	// Subcommand reference: usage, flags and examples of every subcommand that declares any
	var detailed []SubcommandDoc
	for _, sub := range doc.Subcommands {
		if len(sub.Flags) > 0 || len(sub.Examples) > 0 || len(sub.Aliases) > 0 {
			detailed = append(detailed, sub)
		}
	}
	if len(detailed) > 0 {
		sb.WriteString("## Subcommand Reference\n\n")
		for _, sub := range detailed {
			sb.WriteString(fmt.Sprintf("### `dendrite %s %s`\n\n", doc.Name, sub.Name))
			if sub.Description != "" {
				sb.WriteString(sub.Description + "\n\n")
			}
			if sub.Usage != "" {
				sb.WriteString(fmt.Sprintf("```bash\n%s\n```\n\n", sub.Usage))
			}
			if len(sub.Aliases) > 0 {
				sb.WriteString("Aliases: " + formatAliases(sub.Aliases) + "\n\n")
			}
			if len(sub.Flags) > 0 {
				writeFlagTable(&sb, sub.Flags)
			}
			if len(sub.Examples) > 0 {
				sb.WriteString("```bash\n" + strings.Join(sub.Examples, "\n") + "\n```\n\n")
			}
		}
	}

	// Examples
	sb.WriteString("## Examples\n\n")
	if len(doc.Examples) > 0 {
		sb.WriteString("```bash\n")
		for _, ex := range doc.Examples {
			sb.WriteString(ex + "\n")
		}
		sb.WriteString("```\n")
	} else {
		sb.WriteString(fmt.Sprintf("```bash\ndendrite %s\n```\n", doc.Name))
	}

	return sb.String()
}

// enrichCommandDoc fills subcommands, flags and examples from a static catalog for commands that are not
// yet native cobra trees (legacy adapters, DEN-950). The catalog is keyed by plain name; any alias matches.
func enrichCommandDoc(doc *CLICommandDoc) {
	catalog := map[string]struct {
		Subcommands []SubcommandDoc
		Flags       []FlagDoc
		Examples    []string
	}{
		"version": {
			Examples: []string{"dendrite version", "dendrite --version", "dendrite -v"},
		},
		"doctor": {
			Examples: []string{"dendrite doctor", "dendrite probe", "dendrite diag"},
		},
		"env": {
			Subcommands: []SubcommandDoc{
				{Name: "doctor", Description: "Inspect target computer OS, package manager and installed tools"},
				{Name: "plan", Description: "Generate environment replication plan (--mode container|host|hybrid)"},
				{Name: "replicate", Description: "Replicate development environment (Docker containers or host tools)"},
				{Name: "install", Description: "Install tool directly on host computer or spin up container"},
				{Name: "status", Description: "Query real-time health and status of replicated components"},
				{Name: "prepare", Description: "Synchronize project environment, agent rules, and CI/CD tooling"},
			},
			Flags: []FlagDoc{
				{Name: "--mode", Description: "Replication execution mode: 'container', 'host', or 'hybrid'", Default: "container"},
				{Name: "--with-linear", Description: "Include Linear issue tracker container"},
				{Name: "--with-plane", Description: "Include Plane v2 project management container"},
				{Name: "--with-penpot", Description: "Include Penpot design system container"},
				{Name: "--with-grafana", Description: "Include Prometheus & Grafana observability suite"},
				{Name: "--dry-run", Description: "Simulate actions without modifying local disk or services"},
			},
			Examples: []string{
				"dendrite env doctor",
				"dendrite env plan --mode hybrid --with-plane --with-penpot",
				"dendrite env replicate --mode hybrid",
				"dendrite env status",
			},
		},
		"triage": {
			Examples: []string{"dendrite triage", "dendrite trg"},
		},
		"test": {
			Subcommands: []SubcommandDoc{
				{Name: "auto", Description: "Synthesize table-driven tests with Testcontainers & self-healing"},
				{Name: "ui", Description: "Run Web UI browser journeys & WebSocket burst load tests"},
			},
			Flags: []FlagDoc{
				{Name: "--file", Description: "Target Go source file for table-driven test synthesis"},
				{Name: "--base-url", Description: "Base target URL for UI browser test journeys"},
			},
			Examples: []string{
				"dendrite test auto --file ./pkg/env/detector.go",
				"dendrite test ui --base-url http://localhost:3456",
			},
		},
		"agent": {
			Subcommands: []SubcommandDoc{
				{Name: "run", Description: "Launch an autonomous coding agent (aider, openhands, opencode, agy, copilot)"},
				{Name: "search", Description: "Search and rank agents with hybrid vector & BM25 search"},
				{Name: "route", Description: "Auto-route task to optimal agent with LiteLLM virtual key"},
				{Name: "session", Description: "View agent execution history, token spend, and commits"},
			},
			Flags: []FlagDoc{
				{Name: "--ticket", Description: "Target ticket identifier (e.g. DEN-030)"},
				{Name: "--model", Description: "Target LLM model name"},
				{Name: "--spend-cap", Description: "Maximum USD budget ceiling for session"},
				{Name: "--sandbox", Description: "Execute inside ephemeral container sandbox"},
			},
			Examples: []string{
				"dendrite agent run aider --ticket DEN-030",
				"dendrite agent search \"refactor AST parser\"",
				"dendrite agent session list",
			},
		},
		"work": {
			Subcommands: []SubcommandDoc{
				{Name: "new", Description: "Create an isolated working copy in dendrite-work/ on branch agent/<name>"},
				{Name: "list", Description: "List working copies with dirty, unpushed, merged and upstream-gone status"},
				{Name: "done", Description: "Remove one copy (refuses uncommitted or unpushed work without --force)"},
				{Name: "gc", Description: "Remove copies that are merged (or whose upstream is gone) and clean"},
				{Name: "adopt", Description: "Move this repository's worktrees that live outside dendrite-work/ into it"},
			},
			Flags: []FlagDoc{
				{Name: "--branch", Description: "Branch for a new copy (default agent/<name>)"},
				{Name: "--base", Description: "Commit-ish a new copy starts from (default HEAD)"},
				{Name: "--force", Description: "Remove a copy even with unsaved work, deleting its branch"},
				{Name: "--dry-run", Description: "Report what gc or adopt would do without changing anything"},
				{Name: "--json", Description: "Output the copy list as JSON"},
			},
			Examples: []string{
				"dendrite work new DEN-123",
				"dendrite work list",
				"dendrite work gc --dry-run",
				"dendrite work adopt --dry-run",
			},
		},
		"project": {
			Subcommands: []SubcommandDoc{
				{Name: "add", Description: "Register a new workspace and prepare isolated state"},
				{Name: "switch", Description: "Switch active workspace context (< 50ms)"},
				{Name: "list", Description: "List all registered workspaces"},
				{Name: "remove", Description: "Unregister workspace and optionally purge data"},
			},
			Flags: []FlagDoc{
				{Name: "--purge", Description: "Purge persistent data directory when removing workspace"},
			},
			Examples: []string{
				"dendrite project add backend /path/to/repo",
				"dendrite project switch backend",
				"dendrite project list",
				"dendrite project remove backend --purge",
			},
		},
		"doc": {
			Subcommands: []SubcommandDoc{
				{Name: "generate", Description: "Synthesize Docusaurus site or reference documentation"},
				{Name: "audit", Description: "Audit documentation coverage and completeness against registries"},
				{Name: "docusaurus", Description: "Complete Docusaurus site operations (generate, init, share, audit, serve, build)"},
				{Name: "reflect", Description: "Reflect documentation taxonomy from reference to target directory"},
				{Name: "plane", Description: "Synchronize documentation pages between Plane and local repository"},
			},
			Flags: []FlagDoc{
				{Name: "--reference", Description: "Generate per-command and per-tool reference pages from live registries"},
				{Name: "--strict", Description: "Exit 1 if any registered command or tool is undocumented or stale"},
				{Name: "--docs", Description: "Documentation source directory", Default: "docs"},
				{Name: "--out", Description: "Target output directory"},
			},
			Examples: []string{
				"dendrite docs generate --reference",
				"dendrite docs audit --strict",
				"dendrite doc docusaurus serve --port 3000",
			},
		},
		"ui": {
			Subcommands: []SubcommandDoc{
				{Name: "sync", Description: "Synchronize UI component design tokens and Penpot frames"},
			},
			Flags: []FlagDoc{
				{Name: "--port", Description: "Local HTTP port to bind Web UI", Default: "3456"},
				{Name: "--daemon", Description: "Run in continuous background daemon mode"},
				{Name: "--file", Description: "Path to UI component file for synchronization"},
				{Name: "--ticket", Description: "Associated ticket ID for UI synchronization"},
			},
			Examples: []string{
				"dendrite ui",
				"dendrite ui --port 8080 --daemon",
				"dendrite ui sync --file src/Button.tsx --ticket DEN-810",
			},
		},
		"telemetry": {
			Subcommands: []SubcommandDoc{
				{Name: "snapshot", Description: "Instantaneous hardware thermal and token budget metrics"},
				{Name: "metrics", Description: "Export raw metrics in Prometheus text format"},
			},
			Examples: []string{
				"dendrite telemetry",
				"dendrite telemetry metrics",
			},
		},
		"dev": {
			Subcommands: []SubcommandDoc{
				{Name: "ollama", Description: "Probe local Ollama inference engine health, latency, and models"},
				{Name: "ticket", Description: "List, inspect, or transition tickets in DEV_STATUS.md"},
				{Name: "topology", Description: "Inspect service placement across compute nodes"},
				{Name: "gateway", Description: "Render dynamic LiteLLM configuration YAML"},
				{Name: "audit-prompt", Description: "Analyze prompt text length and estimated token volume"},
			},
			Examples: []string{
				"dendrite dev ollama probe",
				"dendrite dev ticket list",
				"dendrite dev audit-prompt \"Explain Dendrite architecture\"",
			},
		},
		"gateway": {
			Subcommands: []SubcommandDoc{
				{Name: "update", Description: "Dynamically discover live inference engines and update LiteLLM routing table"},
				{Name: "schedule", Description: "Run dynamic routing table updater on daily background schedule"},
			},
			Flags: []FlagDoc{
				{Name: "--file", Description: "Target litellm_config.yaml path", Default: "stacks/litellm_config.yaml"},
				{Name: "--dry-run", Description: "Simulate update without modifying disk"},
			},
			Examples: []string{
				"dendrite gateway update",
				"dendrite gateway schedule",
			},
		},
		"topology": {
			Subcommands: []SubcommandDoc{
				{Name: "advisor", Description: "Run dynamic service placement advisor across compute nodes"},
				{Name: "inspect", Description: "Inspect and validate topology scenario placements"},
			},
			Flags: []FlagDoc{
				{Name: "--nodes", Description: "Comma-separated node definitions (e.g. 'local=localhost,worker=10.0.0.2')"},
				{Name: "--json", Description: "Output placement plan in JSON format"},
			},
			Examples: []string{
				"dendrite topology advisor",
				"dendrite topology inspect local-hybrid",
			},
		},
		"jetbrains": {
			Subcommands: []SubcommandDoc{
				{Name: "probe", Description: "Discover IDE MCP capabilities and tools"},
				{Name: "open", Description: "Open file at specified line in active JetBrains editor"},
				{Name: "problems", Description: "Query IDE inspection errors and diagnostics for file"},
				{Name: "symbols", Description: "Search symbol index in JetBrains PSI tree"},
				{Name: "build", Description: "Trigger IDE background build"},
				{Name: "run", Description: "Execute an IDE run configuration"},
				{Name: "setup", Description: "Configure JetBrains IDE toolset (.ai/mcp/mcp.json, agents, run configs)"},
			},
			Flags: []FlagDoc{
				{Name: "--line", Description: "Line number to navigate to", Default: "1"},
			},
			Examples: []string{
				"dendrite jetbrains probe",
				"dendrite jetbrains open main.go --line 42",
				"dendrite jetbrains symbols \"Registry\"",
			},
		},
		"inference": {
			Subcommands: []SubcommandDoc{
				{Name: "list", Description: "List registered engines, endpoints, and failover cascade order"},
				{Name: "probe", Description: "Probe inference engines for reachability, latency, and loaded models"},
				{Name: "switch", Description: "Set primary inference engine"},
				{Name: "render", Description: "Render engine configuration"},
			},
			Examples: []string{
				"dendrite inference list",
				"dendrite inference probe ollama",
			},
		},
		"plane": {
			Subcommands: []SubcommandDoc{
				{Name: "status", Description: "Display synchronization status with self-hosted Plane instance"},
				{Name: "replicate", Description: "Replicate DEV_STATUS.md directly from Plane service state"},
				{Name: "push-status", Description: "Push local DEV_STATUS.md content into Plane service"},
				{Name: "next", Description: "Smart select next ticket unblocked by dependencies"},
				{Name: "link", Description: "Create dependency relationship between tickets for Gantt alignment"},
				{Name: "create", Description: "Create a new ticket placed on Plane Gantt development plan"},
				{Name: "doc", Description: "Synchronize documentation pages between Plane and local repository"},
				{Name: "issues", Description: "List open issues and milestones from Plane"},
			},
			Examples: []string{
				"dendrite plane status",
				"dendrite plane replicate",
				"dendrite plane next",
			},
		},
		"mcp": {
			Subcommands: []SubcommandDoc{
				{Name: "serve", Description: "Launch Dendrite MCP server listening on stdio"},
				{Name: "tools", Description: "List all registered Dendrite MCP tools"},
				{Name: "call", Description: "Invoke an MCP tool directly with JSON arguments"},
			},
			Examples: []string{
				"dendrite mcp serve",
				"dendrite mcp tools",
				"dendrite mcp call dendrite_env_status",
			},
		},
		"dist": {
			Subcommands: []SubcommandDoc{
				{Name: "spawn", Description: "Spawn distributed workspace node"},
				{Name: "interop", Description: "Verify inter-node messaging and RPC contracts"},
				{Name: "help", Description: "Display distributed codebase command guidance"},
			},
			Examples: []string{"dendrite dist help"},
		},
		"bench": {
			Subcommands: []SubcommandDoc{
				{Name: "inference", Description: "Benchmark Time-to-First-Token and tokens/sec generation velocity"},
				{Name: "gateway", Description: "Stress-test LiteLLM gateway with cascade fault injection"},
				{Name: "matrix", Description: "Multi-engine comparative benchmark matrix"},
				{Name: "stress", Description: "Concurrency stress test against sliding window budget limiter"},
			},
			Examples: []string{
				"dendrite bench inference",
				"dendrite bench gateway",
			},
		},
		"rea": {
			Subcommands: []SubcommandDoc{
				{Name: "showcase", Description: "Open-source cleanroom analysis and parity verification target"},
				{Name: "list", Description: "List registered open-source showcase targets and metadata"},
			},
			Examples: []string{
				"dendrite rea list",
				"dendrite rea showcase",
			},
		},
		"pool": {
			Subcommands: []SubcommandDoc{
				{Name: "add", Description: "Add repository directory to connectome pool"},
				{Name: "rm", Description: "Remove directory from pool"},
				{Name: "list", Description: "List registered directories and dependencies"},
				{Name: "resolve", Description: "Compute topological DAG execution order across directories"},
			},
			Examples: []string{
				"dendrite pool list",
				"dendrite pool resolve",
			},
		},
		"query": {
			Subcommands: []SubcommandDoc{
				{Name: "slice", Description: "Universal intent-driven CFG/AST reachability slicing and debloater"},
				{Name: "swap", Description: "AST-aware pattern and framework replacement engine"},
				{Name: "transpile", Description: "Polyglot cross-language code translation with dual test parity"},
				{Name: "fuse", Description: "Multi-source AST blending and syntax tree synthesis"},
				{Name: "reuse-audit", Description: "AST token reuse audit and anti-duplication scanner"},
			},
			Examples: []string{
				"dendrite query slice \"verify docker container mounts\"",
				"dendrite query reuse-audit",
			},
		},
		"ref": {
			Subcommands: []SubcommandDoc{
				{Name: "list", Description: "List registered Tier 1 source references"},
				{Name: "add", Description: "Add external reference repository"},
				{Name: "remove", Description: "Remove external reference repository"},
			},
			Examples: []string{
				"dendrite ref list",
				"dendrite ref add github.com/gin-gonic/gin",
			},
		},
		"store": {
			Subcommands: []SubcommandDoc{
				{Name: "status", Description: "Inspect persistent segmented store status and chunk count"},
				{Name: "chunks", Description: "List stored memory chunks and vector embeddings"},
				{Name: "query", Description: "Semantic similarity query over stored code memory"},
				{Name: "commit", Description: "Checkpoint active workspace state into persistent store"},
			},
			Examples: []string{
				"dendrite store status",
				"dendrite store chunks",
			},
		},
		"analyze": {
			Subcommands: []SubcommandDoc{
				{Name: "repo", Description: "Analyze repository files for cyclomatic and cognitive complexity"},
				{Name: "complexity", Description: "Detailed function-level complexity breakdown"},
				{Name: "policy", Description: "Evaluate codebase against enterprise compliance policies"},
			},
			Examples: []string{
				"dendrite analyze repo .",
			},
		},
		"tokens": {
			Subcommands: []SubcommandDoc{
				{Name: "status", Description: "Display token spend in 24h and 7d sliding windows"},
				{Name: "quota", Description: "Display tier quotas and model rates"},
			},
			Examples: []string{
				"dendrite tokens status",
			},
		},
		"story": {
			Subcommands: []SubcommandDoc{
				{Name: "next", Description: "Execute next story task from Plane DAG"},
				{Name: "fork", Description: "Create speculative branch for alternative implementation"},
				{Name: "resolve", Description: "Merge winning branch and reconcile ticket state"},
			},
			Examples: []string{
				"dendrite story next",
			},
		},
		"commit": {
			Flags: []FlagDoc{
				{Name: "--ticket", Description: "Target ticket ID (e.g. DEN-098)"},
				{Name: "--message", Description: "Explicit commit message (overrides model synthesis)"},
				{Name: "--dry-run", Description: "Preview staged changes and message without modifying git history"},
				{Name: "--no-push", Description: "Opt out of automatic remote push"},
			},
			Examples: []string{
				"dendrite commit --ticket DEN-985",
				"dendrite commit -m \"feat(docs): generate cli reference docs\"",
			},
		},
		"skill": {
			Subcommands: []SubcommandDoc{
				{Name: "ticket-relate", Description: "Link tickets and sync dependencies to Plane"},
				{Name: "gantt-sync", Description: "Align schedule dates and milestones with Plane Gantt"},
			},
			Examples: []string{
				"dendrite skill ticket-relate DEN-030 depends-on DEN-029",
				"dendrite skill gantt-sync",
			},
		},
		"audit-changes": {
			Flags: []FlagDoc{
				{Name: "--branch", Description: "Target local branch to audit"},
				{Name: "--rebase", Description: "Automatically rebase clean changes onto develop"},
			},
			Examples: []string{
				"dendrite audit-changes",
			},
		},
		"vcs": {
			Subcommands: []SubcommandDoc{
				{Name: "check", Description: "Verify active branch complies with guardrails"},
				{Name: "guard", Description: "Enforce branch policy and halt if on main/master"},
				{Name: "status", Description: "Show current VCS status and branch policy"},
				{Name: "sync-remote", Description: "Fetch and rebase remote fixes with commit reconciliation"},
				{Name: "evacuate", Description: "Force evacuate working tree to develop branch"},
			},
			Examples: []string{
				"dendrite vcs check",
				"dendrite vcs sync-remote",
			},
		},
		"github-sync": {
			Flags: []FlagDoc{
				{Name: "--repo", Description: "Target GitHub repository (owner/repo)"},
				{Name: "--dry-run", Description: "Preview issue sync without modifying GitHub"},
			},
			Examples: []string{
				"dendrite github-sync",
			},
		},
		"quality": {
			Subcommands: []SubcommandDoc{
				{Name: "audit", Description: "Run anti-mock scan and verify coverage gates"},
				{Name: "hitl-sync", Description: "Poll human-in-the-loop verification status"},
			},
			Examples: []string{
				"dendrite quality audit",
			},
		},
		"sync-remote": {
			Examples: []string{
				"dendrite sync-remote",
			},
		},
		"move": {
			Flags: []FlagDoc{
				{Name: "--ssh", Description: "Target SSH host for service migration"},
			},
			Examples: []string{
				"dendrite move plane to --ssh worker-node",
			},
		},
		"service": {
			Subcommands: []SubcommandDoc{
				{Name: "start", Description: "Start specified microservice"},
				{Name: "stop", Description: "Stop specified microservice"},
				{Name: "restart", Description: "Restart specified microservice"},
				{Name: "health", Description: "Check health of specified microservice"},
				{Name: "list", Description: "List all managed microservices and status"},
				{Name: "ui-sync", Description: "Synchronize service state with Web UI"},
			},
			Examples: []string{
				"dendrite service list",
				"dendrite service health plane",
			},
		},
		"canvas": {
			Subcommands: []SubcommandDoc{
				{Name: "actions", Description: "List available creative canvas actions"},
				{Name: "dispatch", Description: "Dispatch canvas action to Penpot"},
				{Name: "relate", Description: "Relate canvas element to Plane ticket"},
				{Name: "sync", Description: "Synchronize canvas state"},
				{Name: "update-state", Description: "Update canvas element state"},
			},
			Examples: []string{
				"dendrite canvas actions",
			},
		},
		"ask": {
			Flags: []FlagDoc{
				{Name: "--paid-fallback", Description: "Fallback to commercial paid model when local model fails"},
			},
			Examples: []string{
				"dendrite ask \"How does token budget governance work in Dendrite?\"",
			},
		},
		"greeting": {
			Flags: []FlagDoc{
				{Name: "--topic", Description: "User's current topic or project"},
				{Name: "--decision", Description: "Decision point being weighed"},
				{Name: "--milestone", Description: "Milestone achieved or in-flight"},
				{Name: "--json", Description: "Output greeting result in JSON format"},
			},
			Examples: []string{
				"dendrite greeting --topic \"DEN-985\"",
				"dendrite greeting --json",
			},
		},
	}

	entry, found := catalog[doc.Name]
	for _, alias := range doc.Aliases {
		if found {
			break
		}
		entry, found = catalog[alias]
	}
	if found {
		if len(doc.Subcommands) == 0 && len(entry.Subcommands) > 0 {
			doc.Subcommands = append([]SubcommandDoc(nil), entry.Subcommands...)
		}
		if len(doc.Flags) == 0 && len(entry.Flags) > 0 {
			doc.Flags = append([]FlagDoc(nil), entry.Flags...)
		}
		if len(doc.Examples) == 0 && len(entry.Examples) > 0 {
			doc.Examples = append([]string(nil), entry.Examples...)
		}
	}

	if len(doc.Examples) == 0 {
		doc.Examples = []string{fmt.Sprintf("dendrite %s", doc.Name)}
	}
}
