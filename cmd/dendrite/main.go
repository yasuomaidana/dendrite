package main

import (
	"Dendrite/pkg/cli"
	"context"
	"fmt"
	"os"
)

const version = "0.1.0-alpha"

func main() {
	reg := buildRegistry()

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	ctx := context.Background()
	err := reg.Execute(ctx, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
		os.Exit(exitCodeFor(err))
	}
}

func buildRegistry() *cli.Registry {
	reg := cli.NewRegistry()

	reg.Register(&cli.Command{
		Name:        "version",
		Aliases:     []string{"--version", "-v"},
		Category:    "Cortex & Sensory Integration",
		Description: "Print Dendrite CLI version",
		Run: func(ctx context.Context, args []string) error {
			fmt.Printf("Dendrite CLI v%s\n", version)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "doctor",
		Aliases:     []string{"soma-diagnose", "probe", "diag"},
		Category:    "Soma & Axon Hillock",
		Description: "Run environment pre-flight diagnostics",
		Run: func(ctx context.Context, args []string) error {
			runDoctor(ctx)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "env",
		Aliases:     []string{"soma", "homeostasis", "soma-homeostasis"},
		Category:    "Soma & Axon Hillock",
		Description: "Manage and replicate development environments",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				fmt.Println("Usage: dendrite env <doctor|plan|install|replicate|status> [flags...]")
				return nil
			}
			handleEnvCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "triage",
		Aliases:     []string{"inhibitory-triage", "trg"},
		Category:    "Postsynaptic Effectors & Actuation",
		Description: "Resolve local agent task triage and priority routing",
		Run: func(ctx context.Context, args []string) error {
			handleTriageCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "test",
		Aliases:     []string{"myotatic-reflex", "reflex", "tst"},
		Category:    "Postsynaptic Effectors & Actuation",
		Description: "Table-driven test synthesis, Testcontainers & UI tests",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printTestUsage()
				return nil
			}
			handleTestCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "agent",
		Aliases:     []string{"effector-dispatch", "bouton-dispatch", "dispatch", "bouton", "agt"},
		Category:    "Postsynaptic Effectors & Actuation",
		Description: "Autonomous coding agent harness (Aider, OpenHands, OpenCode)",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				fmt.Println("Usage: dendrite agent <search|route|run|session> [tool] [flags...]")
				return nil
			}
			handleAgentCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "project",
		Aliases:     []string{"ganglion-workspace", "ganglion", "prj"},
		Category:    "Soma & Axon Hillock",
		Description: "Manage multi-project isolated workspaces",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				fmt.Println("Usage: dendrite project <add|switch|list|remove> [args...]")
				return nil
			}
			handleProjectCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "doc",
		Aliases:     []string{"docs", "engram-codex", "codex"},
		Category:    "Cortex & Sensory Integration",
		Description: "Docs-as-Code reflection, Docusaurus synthesis and portal",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				fmt.Println("Usage: dendrite doc <generate|audit|docusaurus|reflect|plane> [args...]")
				return nil
			}
			handleDocCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "ui",
		Aliases:     []string{"cortex-cockpit", "cortex"},
		Category:    "Cortex & Sensory Integration",
		Description: "Embedded Web Control Cockpit and telemetry server",
		Run: func(ctx context.Context, args []string) error {
			handleUICommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "telemetry",
		Aliases:     []string{"afferent-telemetry", "sensory", "tel"},
		Category:    "Cortex & Sensory Integration",
		Description: "Hardware thermal, token spend, and Prometheus telemetry",
		Run: func(ctx context.Context, args []string) error {
			handleTelemetryCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "dev",
		Category:    "Soma & Axon Hillock",
		Description: "Developer diagnostics, local Ollama probe, and ticket tools",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				fmt.Println("Usage: dendrite dev <ollama|ticket|topology|gateway|audit-prompt> [args...]")
				return nil
			}
			handleDevCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "gateway",
		Aliases:     []string{"synaptic-cleft", "cleft", "gate"},
		Category:    "Synaptic Cleft & Gating",
		Description: "LiteLLM token gateway rendering, dynamic routing updater & scheduler",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				handleGatewayCommand(ctx, []string{"update"})
				return nil
			}
			handleGatewayCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "topology",
		Aliases:     []string{"axon-pathway", "axon", "top"},
		Category:    "Axon & Myelin Sheath",
		Description: "Multi-node placement models and hardware service router",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				runTopologyAdvisor(ctx, nil)
				return nil
			}
			handleTopologyCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "jetbrains",
		Aliases:     []string{"neuromuscular-bridge", "bridge", "jb"},
		Category:    "Synaptic Cleft & Gating",
		Description: "JetBrains IDE MCP protocol integration and PSI symbol indexer",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				fmt.Println("Usage: dendrite jetbrains <probe|open|problems|symbols|build|run|setup> [args...]")
				return nil
			}
			handleJetBrainsCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "inference",
		Aliases:     []string{"action-potential", "spike", "inf"},
		Category:    "Synaptic Cleft & Gating",
		Description: "Unified multi-provider inference engine gateway (Ollama, OxiLLaMa, vLLM, TGI)",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				handleInferenceCommand(ctx, []string{"list"})
				return nil
			}
			handleInferenceCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "plane",
		Aliases:     []string{"chronos-tracker", "chronos", "pln"},
		Category:    "Soma & Axon Hillock",
		Description: "Sovereign issue tracking, Gantt alignment, and DAG scheduler",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				fmt.Println("Usage: dendrite plane <status|replicate|push-status|doc|issues|modules|pages|update|states> [args...]")
				return nil
			}
			handlePlaneCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "mcp",
		Aliases:     []string{"interneuron-mcp", "inter"},
		Category:    "Synaptic Cleft & Gating",
		Description: "Model Context Protocol (MCP) server over stdio",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				handleMCPCommand(ctx, "serve", nil)
				return nil
			}
			handleMCPCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "synapse",
		Aliases:     []string{"syn", "synapses"},
		Category:    "Synaptic Cleft & Gating",
		Description: "Typed cross-project links with scoped MCP servers and self-healing paths",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printSynapseUsage(os.Stdout)
				return nil
			}
			handleSynapseCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "dist",
		Aliases:     []string{"distcodebase", "distributed", "neural-mesh", "syn-mesh"},
		Category:    "Axon & Myelin Sheath",
		Description: "Distributed codebase spawner, modular REA, and repo coordinator",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				handleDistCommand(ctx, "help", nil)
				return nil
			}
			handleDistCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "bench",
		Aliases:     []string{"benchmark", "conduction-velocity", "velocity", "bnch"},
		Category:    "Axon & Myelin Sheath",
		Description: "TTFT, TPS, and gateway resilience benchmarking suite",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printBenchUsage()
				return nil
			}
			handleBenchCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "rea",
		Aliases:     []string{"retrograde-analysis", "retro"},
		Category:    "Dendritic Arbor",
		Description: "Reverse engineering cleanroom analysis and parity verification",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printREAUsage()
				return nil
			}
			handleREACommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "pool",
		Aliases:     []string{"connectome-pool", "plexus-pool", "connectome", "plexus", "plx"},
		Category:    "Soma & Axon Hillock",
		Description: "Multi-directory dependency inference & topological DAG solver",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printPoolUsage()
				return nil
			}
			handlePoolCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "query",
		Aliases:     []string{"dendritic-arbor", "arbor", "dend"},
		Category:    "Dendritic Arbor",
		Description: "Intent-driven CFG/AST reachability slicing, debloater, transpile & fuse",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printQueryUsage()
				return nil
			}
			handleQueryCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "ref",
		Aliases:     []string{"dendritic-afferent", "afferent", "aff"},
		Category:    "Dendritic Arbor",
		Description: "Manage Tier 1 source references for library reuse and cleanroom synthesis",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printRefUsage()
				return nil
			}
			handleRefCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "store",
		Aliases:     []string{"engram-store", "engram", "gram"},
		Category:    "Soma & Axon Hillock",
		Description: "Persistent segmented store, memory checkpointer, and chunk query",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printStoreUsage()
				return nil
			}
			handleStoreCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "analyze",
		Aliases:     []string{"cognitive-analysis", "cognition", "anlz"},
		Category:    "Glial Regulation & Pruning",
		Description: "Cognitive complexity analyzer, structural anti-pattern scanner, and policy router",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printAnalyzeUsage()
				return nil
			}
			handleAnalyzeCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "tokens",
		Aliases:     []string{"token", "vesicle-quota", "quota", "vesicle", "tok"},
		Category:    "Terminal Boutons & Vesicles",
		Description: "Inspect 24h & 7d token consumption windows and tier quotas",
		Run: func(ctx context.Context, args []string) error {
			handleTokensCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "story",
		Aliases:     []string{"stories", "synaptic-saga", "saga", "sty"},
		Category:    "Postsynaptic Effectors & Actuation",
		Description: "Autonomous multi-turn story execution and DAG branching",
		Run: func(ctx context.Context, args []string) error {
			if len(args) >= 1 && args[0] == "next" {
				handlePlaneCommand(ctx, "next", args[1:])
				return nil
			}
			if len(args) < 1 {
				printStoryUsage()
				return nil
			}
			handleStoryCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "commit",
		Aliases:     []string{"quantal-commit", "quantal", "cmt"},
		Category:    "Terminal Boutons & Vesicles",
		Description: "Autonomous Git commit with local model message generation",
		Run: func(ctx context.Context, args []string) error {
			handleCommitCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "skill",
		Category:    "Postsynaptic Effectors & Actuation",
		Description: "Execute skill integrations (ticket-relate, gantt-sync)",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				fmt.Println("Usage: dendrite skill <ticket-relate|gantt-sync> [args...]")
				return nil
			}
			if args[0] == "ticket-relate" || args[0] == "relate" {
				handlePlaneCommand(ctx, "relate", args[1:])
				return nil
			}
			if args[0] == "gantt-sync" || args[0] == "align-gantt" {
				handlePlaneCommand(ctx, "gantt-sync", args[1:])
				return nil
			}
			fmt.Printf("[-] Unknown skill: %s\n", args[0])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "audit-changes",
		Aliases:     []string{"microglial-prune", "synaptic-pruning", "prune", "prn"},
		Category:    "Glial Regulation & Pruning",
		Description: "Topological vendor audit and rebase engine for local/* branches",
		Run: func(ctx context.Context, args []string) error {
			handleAuditChangesCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "vcs",
		Aliases:     []string{"hillock-guard", "myelin-guard", "guard", "vcs"},
		Category:    "Soma & Axon Hillock",
		Description: "Git branch guardrails prohibiting main pushes and ensuring rebase integrity",
		Run: func(ctx context.Context, args []string) error {
			handleVCSCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "github-sync",
		Aliases:     []string{"sync-github", "saltatory-sync", "salt-sync", "gsync"},
		Category:    "Axon & Myelin Sheath",
		Description: "Synchronize prioritized tickets and DAG dependencies to GitHub Issues",
		Run: func(ctx context.Context, args []string) error {
			handleGitHubSyncCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "quality",
		Aliases:     []string{"homeostatic-audit", "allostasis-audit", "allostasis", "qlty"},
		Category:    "Glial Regulation & Pruning",
		Description: "Anti-mock scan, 100% coverage profiler, and HITL verification poller",
		Run: func(ctx context.Context, args []string) error {
			handleQualityCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "sync-remote",
		Category:    "Axon & Myelin Sheath",
		Description: "Detect, fetch, and rebase remote fixes with commit reconciliation",
		Run: func(ctx context.Context, args []string) error {
			handleVCSCommand(ctx, []string{"sync-remote"})
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "move",
		Aliases:     []string{"axonal-translocation", "translocate", "mv"},
		Category:    "Axon & Myelin Sheath",
		Description: "Zero-credential interactive service migration across SSH nodes",
		Run: func(ctx context.Context, args []string) error {
			handleMoveCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "service",
		Aliases:     []string{"motor-unit", "effector-service", "motor", "svc"},
		Category:    "Postsynaptic Effectors & Actuation",
		Description: "Manage microservices (Plane, Penpot, Ollama, LiteLLM)",
		Run: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				printServiceUsage()
				return nil
			}
			handleServiceCommand(ctx, args[0], args[1:])
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "canvas",
		Aliases:     []string{"creative-canvas", "interactive-canvas", "pcontext-canvas"},
		Category:    "Postsynaptic Effectors & Actuation",
		Description: "Interactive writing canvas and Penpot UI direct creative action bridge",
		Run: func(ctx context.Context, args []string) error {
			handleCanvasCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "ask",
		Aliases:     []string{"query-project", "rag-ask", "q"},
		Category:    "Cortex & Sensory Integration",
		Description: "Ask questions to Dendrite about a project with local RAG and interactive paid fallback",
		Run: func(ctx context.Context, args []string) error {
			handleAskCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "greeting",
		Aliases:     []string{"greet", "pcontext-greeting"},
		Category:    "Synaptic Cleft & Gating",
		Description: "Generate dynamic 10-style greeting with cliché suppression",
		Run: func(ctx context.Context, args []string) error {
			handleGreetingCommand(ctx, args)
			return nil
		},
	})

	reg.Register(&cli.Command{
		Name:        "session",
		Aliases:     []string{"handoff", "checkpoint"},
		Category:    "Synaptic Cleft & Gating",
		Description: "Session checkpoint and resume for multi-agent coordination",
		Run: func(ctx context.Context, args []string) error {
			return handleSessionCommand(ctx, args)
		},
	})

	reg.Register(&cli.Command{
		Name:        "work",
		Aliases:     []string{"workcopy", "wc"},
		Category:    "Synaptic Cleft & Gating",
		Description: "Manage model-owned working copies in dendrite-work/ (create, list, remove, auto-clean merged)",
		Run: func(ctx context.Context, args []string) error {
			return handleWorkCommand(ctx, args)
		},
	})

	return reg
}

func printUsage() {
	fmt.Println("Dendrite - Autonomous AI Development & Infrastructure Control Plane")
	fmt.Println("\nUsage:")
	fmt.Println("  dendrite [command]")
	fmt.Println("\nAvailable Commands (Neuro-Lore aliases supported, e.g. probe, arbor, cortex, axon, bouton, cleft):")
	fmt.Println("  doctor                     Run environment pre-flight diagnostics")
	fmt.Println("  ui [--port 3456] [--daemon]Launch Dendrite embedded web UI and dashboard (< 800ms)")
	fmt.Println("  telemetry [snapshot|metrics]Sample real-time SoC thermal & spend telemetry or raw Prometheus metrics")
	fmt.Println("  env doctor                 Inspect target computer OS, package manager and installed tools")
	fmt.Println("  env plan [flags]           Generate environment replication plan (--mode container|host|hybrid)")
	fmt.Println("  env replicate [flags]      Replicate development environment (Docker containers or host tools)")
	fmt.Println("  env install <tool> [flags] Install tool directly on host computer or spin up container")
	fmt.Println("  env status                 Query real-time health and status of replicated components")
	fmt.Println("  project add <name> <path>  Register a new workspace and prepare isolated state")
	fmt.Println("  project switch <name>      Switch active workspace context (< 50ms)")
	fmt.Println("  project list               List all registered workspaces")
	fmt.Println("  project remove <n> [--purge]Unregister workspace and optionally purge data")
	fmt.Println("  test auto --file <path>    Synthesize table-driven tests with Testcontainers & self-healing")
	fmt.Println("  test ui [--base-url <url>] Run Web UI browser journeys & WebSocket burst load tests")
	fmt.Println("  agent search <query>       Search and rank agents with hybrid vector & BM25 search")
	fmt.Println("  agent route --task <desc>  Auto-route task to optimal agent with LiteLLM virtual key")
	fmt.Println("  agent run <tool> [flags]   Launch an autonomous coding agent (aider, openhands, opencode, agy, copilot)")
	fmt.Println("  agent session [list]       View agent execution history and commits")
	fmt.Println("  commit [flags]             Autonomous Git commit with local model message generation")
	fmt.Println("  work <new|list|done|gc>    Model-owned working copies in dendrite-work/ (auto-removed once merged)")
	fmt.Println("  audit-changes [flags]      Topological vendor audit and rebase engine for local/* branches")
	fmt.Println("  vcs <guard|check|status>   Enforce Git branch guardrails (prohibit main, develop default)")
	fmt.Println("  github-sync [flags]        Synchronize prioritized tickets and DAG dependencies to GitHub Issues")
	fmt.Println("  sync-remote                Detect, fetch, and rebase remote fixes with commit reconciliation")
	fmt.Println("  quality <audit|hitl-sync>  Run anti-mock scan, 100% coverage profiler, and HITL verification poller")
	fmt.Println("  doc docusaurus [flags]     Docusaurus Docs-as-Code and project sharing")
	fmt.Println("  doc reflect [flags]        Reflect documentation taxonomy (--reference, --source, --target)")
	fmt.Println("  plane replicate [--file DEV_STATUS.md] Replicate DEV_STATUS.md from Plane service (Plane owns status)")
	fmt.Println("  plane push-status                      Move local DEV_STATUS content to Plane service (Gantt dates & DAG)")
	fmt.Println("  plane next [--ticket <id>] [--json]    Smart select next ticket from Plane (priority & DAG unblocked)")
	fmt.Println("  plane link <src> <rel> <target>        Link dependency relationship in Plane for Gantt diagram")
	fmt.Println("  plane create --name \"[DEN-XXX] Title\" --desc \"...\" Create ticket with context and place on Plane Gantt development plan")
	fmt.Println("  plane doc <push|pull|sync> [--dir docs] Synchronize documentation between Plane and local repository")
	fmt.Println("  plane <issues|modules|pages>           Inspect Plane issues, milestones, and documentation pages")
	fmt.Println("  dev ticket next [--ticket <id>]        Smart select next unblocked ticket by priority")
	fmt.Println("  jetbrains probe [url]      Probe JetBrains IDE MCP server and list available tools")
	fmt.Println("  jetbrains open <f> [--line]Open target file in active JetBrains editor")
	fmt.Println("  jetbrains problems <file>  Query inspections and diagnostics for file")
	fmt.Println("  jetbrains symbols <query>  Search symbols in JetBrains PSI index")
	fmt.Println("  jetbrains build            Trigger IDE background build")
	fmt.Println("  jetbrains run <config>     Execute an IDE run configuration")
	fmt.Println("  jetbrains setup            Configure JetBrains IDE toolset (MCP server, agents, run configs, external tools)")
	fmt.Println("  dev ollama probe [url]     Probe Ollama health, latency, and models")
	fmt.Println("  dev ticket list [file]     List tickets from DEV_STATUS.md")
	fmt.Println("  dev ticket set <id> <st>   Update a ticket status (e.g. DONE, IN_PROGRESS)")
	fmt.Println("  topology advisor [--nodes ..] Run hardware discovery & dynamic service placement router")
	fmt.Println("  topology [scenario]         Inspect and validate topology scenario placements")
	fmt.Println("  dev topology [scenario]    Inspect and validate topology scenario placements")
	fmt.Println("  dev gateway [scenario]     Render dynamic LiteLLM configuration YAML")
	fmt.Println("  gateway <update|schedule>  Dynamic LiteLLM routing table updater and daily scheduler")
	fmt.Println("  dev audit-prompt <text>    Analyze prompt length and estimated token volume")
	fmt.Println("  inference [list|probe|switch|render] Unified multi-provider inference engine gateway")
	fmt.Println("  dev inference <list|probe|switch|render> Multi-provider inference gateway control")
	fmt.Println("  mcp serve                  Launch Dendrite Model Context Protocol (MCP) server over stdio")
	fmt.Println("  mcp tools                  List registered Dendrite MCP tools")
	fmt.Println("  mcp call <tool> [json]     Invoke an MCP tool directly")
	fmt.Println("  bench inference [flags]    Benchmark TTFT and TPS generation velocity across engines")
	fmt.Println("  bench gateway [flags]      Stress-test LiteLLM gateway with fallback cascade fault injection")
	fmt.Println("  bench matrix [flags]       Multi-engine comparative benchmark matrix")
	fmt.Println("  bench stress [flags]       Concurrency stress test against sliding window budget limiter")
	fmt.Println("  rea showcase [target]      Open-source REA cleanroom analysis & parity verification")
	fmt.Println("  rea list                   List registered open-source showcase targets and metadata")
	fmt.Println("  pool <add|rm|list|resolve> Multi-directory dependency inference & topological DAG solver")
	fmt.Println("  ref <list|add|remove>      Manage Tier 1 source references for library reuse")
	fmt.Println("  store <status|chunks|query> Persistent segmented store & chunk checkpointer")
	fmt.Println("  query [slice] [flags]      Universal intent-driven CFG/AST reachability slicing & debloater")
	fmt.Println("  tokens [status] [flags]    Inspect day & week token windows and confirm AGY access")
	fmt.Println("  move <svc> to --ssh <tgt>  Zero-credential interactive service migration to remote SSH server")
	fmt.Println("  canvas <actions|dispatch|relate|sync|update-state> Interactive writing canvas & Penpot UI creative action bridge")
	fmt.Println("  service <start|stop|restart|health|list|ui-sync> Manage local and remote microservices (Penpot, Plane, Ollama, etc.)")
	fmt.Println("  ask [flags] \"<question>\"    Ask questions about the project with local RAG & interactive paid fallback")
	fmt.Println("  version                    Print Dendrite CLI version")
}
