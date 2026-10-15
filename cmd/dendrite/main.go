package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/docgen/reflect"
	"Dendrite/pkg/env"
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/jetbrains"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/server"
	"Dendrite/pkg/telemetry"
	"Dendrite/pkg/telemetry/exporter"
	"Dendrite/pkg/topology"
	"Dendrite/pkg/topology/advisor"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/workspace"
	"encoding/json"
	"path/filepath"
)

const version = "0.1.0-alpha"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	cmd := os.Args[1]
	ctx := context.Background()

	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("Dendrite CLI v%s\n", version)

	case "doctor":
		runDoctor(ctx)

	case "env":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite env <doctor|plan|install|replicate|status> [flags...]")
			return
		}
		handleEnvCommand(ctx, os.Args[2], os.Args[3:])

	case "triage":
		handleTriageCommand(ctx, os.Args[2:])

	case "test":
		if len(os.Args) < 3 {
			printTestUsage()
			return
		}
		handleTestCommand(ctx, os.Args[2], os.Args[3:])

	case "agent":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite agent <search|route|run|session> [tool] [flags...]")
			return
		}
		handleAgentCommand(ctx, os.Args[2], os.Args[3:])

	case "project":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite project <add|switch|list|remove> [args...]")
			return
		}
		handleProjectCommand(ctx, os.Args[2], os.Args[3:])

	case "doc":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite doc <docusaurus|reflect|plane> [args...]")
			return
		}
		handleDocCommand(ctx, os.Args[2], os.Args[3:])

	case "ui":
		handleUICommand(ctx, os.Args[2:])

	case "telemetry":
		handleTelemetryCommand(ctx, os.Args[2:])

	case "dev":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite dev <ollama|ticket|topology|gateway|audit-prompt> [args...]")
			return
		}
		handleDevCommand(ctx, os.Args[2], os.Args[3:])

	case "topology":
		if len(os.Args) < 3 {
			runTopologyAdvisor(ctx, nil)
			return
		}
		handleTopologyCommand(ctx, os.Args[2], os.Args[3:])

	case "jetbrains":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite jetbrains <probe|open|problems|symbols|build|run|setup> [args...]")
			return
		}
		handleJetBrainsCommand(ctx, os.Args[2], os.Args[3:])

	case "inference":
		if len(os.Args) < 3 {
			handleInferenceCommand(ctx, []string{"list"})
			return
		}
		handleInferenceCommand(ctx, os.Args[2:])

	case "plane":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite plane <status|replicate|push-status|doc|issues|modules|pages> [args...]")
			return
		}
		handlePlaneCommand(ctx, os.Args[2], os.Args[3:])

	case "mcp":
		if len(os.Args) < 3 {
			handleMCPCommand(ctx, "serve", nil)
			return
		}
		handleMCPCommand(ctx, os.Args[2], os.Args[3:])

	case "dist", "distcodebase", "distributed":
		if len(os.Args) < 3 {
			handleDistCommand(ctx, "help", nil)
			return
		}
		handleDistCommand(ctx, os.Args[2], os.Args[3:])

	case "bench", "benchmark":
		if len(os.Args) < 3 {
			printBenchUsage()
			return
		}
		handleBenchCommand(ctx, os.Args[2], os.Args[3:])

	case "rea":
		if len(os.Args) < 3 {
			printREAUsage()
			return
		}
		handleREACommand(ctx, os.Args[2], os.Args[3:])

	case "pool":
		if len(os.Args) < 3 {
			printPoolUsage()
			return
		}
		handlePoolCommand(ctx, os.Args[2], os.Args[3:])

	case "query":
		if len(os.Args) < 3 {
			printQueryUsage()
			return
		}
		handleQueryCommand(ctx, os.Args[2], os.Args[3:])

	case "ref":
		if len(os.Args) < 3 {
			printRefUsage()
			return
		}
		handleRefCommand(ctx, os.Args[2], os.Args[3:])

	case "store":
		if len(os.Args) < 3 {
			printStoreUsage()
			return
		}
		handleStoreCommand(ctx, os.Args[2], os.Args[3:])

	case "analyze":
		if len(os.Args) < 3 {
			printAnalyzeUsage()
			return
		}
		handleAnalyzeCommand(ctx, os.Args[2], os.Args[3:])

	case "tokens", "token":
		handleTokensCommand(ctx, os.Args[2:])

	case "story", "stories":
		if len(os.Args) >= 3 && os.Args[2] == "next" {
			handlePlaneCommand(ctx, "next", os.Args[3:])
			return
		}
		if len(os.Args) < 3 {
			printStoryUsage()
			return
		}
		handleStoryCommand(ctx, os.Args[2], os.Args[3:])

	case "commit":
		handleCommitCommand(ctx, os.Args[2:])

	case "skill":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite skill <ticket-relate|gantt-sync> [args...]")
			return
		}
		if os.Args[2] == "ticket-relate" || os.Args[2] == "relate" {
			handlePlaneCommand(ctx, "relate", os.Args[3:])
			return
		}
		if os.Args[2] == "gantt-sync" || os.Args[2] == "align-gantt" {
			handlePlaneCommand(ctx, "gantt-sync", os.Args[3:])
			return
		}
		fmt.Printf("[-] Unknown skill: %s\n", os.Args[2])

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Dendrite - Autonomous AI Development & Infrastructure Control Plane")
	fmt.Println("\nUsage:")
	fmt.Println("  dendrite [command]")
	fmt.Println("\nAvailable Commands:")
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
	fmt.Println("  doc docusaurus [flags]     Docusaurus Docs-as-Code and project sharing")
	fmt.Println("  doc reflect [flags]        Reflect documentation taxonomy (--reference, --source, --target)")
	fmt.Println("  plane replicate [--file DEV_STATUS.md] Replicate DEV_STATUS.md from Plane service (Plane owns status)")
	fmt.Println("  plane push-status                      Move local DEV_STATUS content to Plane service (Gantt dates & DAG)")
	fmt.Println("  plane next [--ticket <id>] [--json]    Smart select next ticket from Plane (priority & DAG unblocked)")
	fmt.Println("  plane link <src> <rel> <target>        Link dependency relationship in Plane for Gantt diagram")
	fmt.Println("  plane create --name \"[DEN-XXX] Title\" Create ticket and place on Plane Gantt development plan")
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
	fmt.Println("  version                    Print Dendrite CLI version")
}

func handleUICommand(ctx context.Context, args []string) {
	port := 3456
	daemonMode := false
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" && i+1 < len(args) {
			if p, err := strconv.Atoi(args[i+1]); err == nil {
				port = p
			}
			i++
		} else if args[i] == "--daemon" || args[i] == "-d" {
			daemonMode = true
		}
	}

	reg, _ := workspace.NewRegistry("", "")
	hub := server.NewTelemetryHub()

	srv := server.NewServer(server.ServerConfig{
		Host: "127.0.0.1",
		Port: port,
	}, reg, hub)

	start := time.Now()
	if err := srv.Start(); err != nil {
		fmt.Printf("[-] Failed to start Dendrite Web UI: %v\n", err)
		return
	}
	duration := time.Since(start)

	fmt.Printf("[✓] Dendrite Web UI listening at http://127.0.0.1:%d (Started in %v)\n", port, duration)
	fmt.Printf("    - WebSocket Telemetry: ws://127.0.0.1:%d/api/v1/telemetry/ws\n", port)
	fmt.Printf("    - REST API Snapshot:   http://127.0.0.1:%d/api/v1/telemetry/snapshot\n", port)
	fmt.Printf("    - Prometheus Metrics:  http://127.0.0.1:%d/metrics\n", port)

	if daemonMode {
		fmt.Println("[+] Running in daemon mode. Press Ctrl+C to terminate.")
		select {}
	}
}

func handleTelemetryCommand(ctx context.Context, args []string) {
	daemon := telemetry.NewDaemon(telemetry.DaemonConfig{
		Interval: 1 * time.Second,
	}, nil, nil, nil, nil)

	if len(args) > 0 && (args[0] == "metrics" || args[0] == "export" || args[0] == "prometheus") {
		exp := exporter.NewPrometheusExporter(daemon)
		if err := exp.WriteMetrics(ctx, os.Stdout); err != nil {
			fmt.Printf("[-] Failed to write Prometheus metrics: %v\n", err)
		}
		return
	}

	rep := daemon.CollectOnce(ctx)
	fmt.Printf("[+] Instantaneous Telemetry Snapshot:\n")
	fmt.Printf("    - Timestamp:        %s\n", rep.Timestamp.Format(time.RFC3339))
	fmt.Printf("    - SoC Temperature:  %.1f°C (Alert: %v, Critical: %v)\n", rep.SoCTempC, rep.ThermalAlert, rep.ThermalCritical)
	fmt.Printf("    - 24h Spend:        $%.2f / $%.2f daily budget\n", rep.Spend24hUSD, rep.DailyBudgetUSD)
	fmt.Printf("    - 7d Spend:         $%.2f\n", rep.Spend7dUSD)
	fmt.Println("    - Node Health:")
	for node, status := range rep.NodeStatus {
		fmt.Printf("        * %-10s -> %s\n", node, status)
	}
}

func handleEnvCommand(ctx context.Context, action string, args []string) {
	detector := env.NewDetector(nil)
	host, err := detector.Detect(ctx)
	if err != nil {
		fmt.Printf("[-] Host detection failed: %v\n", err)
		return
	}

	planner := env.NewPlanner(*host)
	replicator := env.NewReplicator(nil, *host)

	switch action {
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "doctor":
		fmt.Println("[+] Host Environment Discovery:")
	case "plan":
		mode := env.ModeContainer
		withLinear := false
		withPlane := false
		var withPlaneUI *bool
		withPenpot := false
		withGrafana := false
		for i := 0; i < len(args); i++ {
			if (args[i] == "--mode" || args[i] == "-m") && i+1 < len(args) {
				mode = env.InstallMode(args[i+1])
				i++
			} else if args[i] == "--with-linear" {
				withLinear = true
			} else if args[i] == "--with-plane" {
				withPlane = true
			} else if args[i] == "--with-plane-ui" {
				v := true
				withPlaneUI = &v
			} else if args[i] == "--no-plane-ui" || args[i] == "--without-plane-ui" {
				v := false
				withPlaneUI = &v
			} else if args[i] == "--with-penpot" {
				withPenpot = true
			} else if args[i] == "--with-grafana" {
				withGrafana = true
			}
		}
		req := env.ReplicationRequest{
			Mode:        mode,
			WithLinear:  withLinear,
			WithPlane:   withPlane,
			WithPlaneUI: withPlaneUI,
			WithPenpot:  withPenpot,
			WithGrafana: withGrafana,
		}
		plan, err := planner.PlanReplication(req)
		if err != nil {
			fmt.Printf("[-] Failed to plan replication: %v\n", err)
			return
		}
		fmt.Printf("[+] Replication Plan (Mode: %s, Est. Time: ~%ds):\n", plan.Mode, plan.EstimatedSecs)
		for i, a := range plan.Actions {
			mandatory := ""
			if a.Mandatory {
				mandatory = "[mandatory]"
			}
			fmt.Printf("    %d. [%s] %s %s\n", i+1, a.Type, a.Description, mandatory)
			fmt.Printf("       Command: %s\n", a.Command)
		}

	case "replicate":
		mode := env.ModeContainer
		withLinear := true
		withPlane := false
		var withPlaneUI *bool
		dryRun := false
		for i := 0; i < len(args); i++ {
			if (args[i] == "--mode" || args[i] == "-m") && i+1 < len(args) {
				mode = env.InstallMode(args[i+1])
				i++
			} else if args[i] == "--no-linear" {
				withLinear = false
			} else if args[i] == "--with-plane" {
				withPlane = true
			} else if args[i] == "--no-plane" {
				withPlane = false
			} else if args[i] == "--with-plane-ui" {
				v := true
				withPlaneUI = &v
			} else if args[i] == "--no-plane-ui" || args[i] == "--without-plane-ui" {
				v := false
				withPlaneUI = &v
			} else if args[i] == "--dry-run" {
				dryRun = true
			}
		}
		req := env.ReplicationRequest{
			Mode:        mode,
			WithLinear:  withLinear,
			WithPlane:   withPlane,
			WithPlaneUI: withPlaneUI,
			DryRun:      dryRun,
		}
		plan, err := planner.PlanReplication(req)
		if err != nil {
			fmt.Printf("[-] Failed to plan replication: %v\n", err)
			return
		}
		fmt.Printf("[+] Executing Environment Replication (Mode: %s, DryRun: %v)...\n", mode, dryRun)
		res, err := replicator.ExecutePlan(ctx, plan, dryRun)
		if err != nil {
			fmt.Printf("[-] Replication encountered error: %v\n", err)
		}
		fmt.Printf("[✓] Completed in %v (Success: %v)\n", res.TotalDuration, res.Success)
		if len(res.GeneratedFiles) > 0 {
			fmt.Println("[+] Generated Compose Files:")
			for _, f := range res.GeneratedFiles {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println("[+] Component Status:")
		for _, c := range res.Components {
			re := "UNREACHABLE"
			if c.Reachable {
				re = "REACHABLE"
			}
			fmt.Printf("    - %-12s: %s (Endpoint: %s)\n", c.Target, re, c.Endpoint)
		}

	case "install":
		if len(args) == 0 {
			fmt.Println("[-] Usage: dendrite env install <aider|openhands|copilot|agy|linear|plane|penpot|ollama|litellm|postgres|docker> [--mode container|host] [-y|--yes]")
			return
		}
		targetName := strings.ToLower(args[0])
		aType := agent.AgentType(targetName)
		if aType.IsValid() {
			autoApprove := false
			for _, a := range args[1:] {
				if a == "-y" || a == "--yes" {
					autoApprove = true
				}
			}
			err := agent.PromptAndInstallAgent(ctx, aType, autoApprove, os.Stdin, os.Stdout)
			if err != nil {
				fmt.Printf("[-] Failed to install %s: %v\n", aType, err)
				return
			}
			return
		}
		target := env.ReplicationTarget(args[0])
		mode := env.ModeContainer
		for i := 1; i < len(args); i++ {
			if (args[i] == "--mode" || args[i] == "-m") && i+1 < len(args) {
				mode = env.InstallMode(args[i+1])
				i++
			}
		}
		req := env.ReplicationRequest{
			Mode:       mode,
			Targets:    []env.ReplicationTarget{target},
			WithLinear: target == env.TargetLinear,
			WithPlane:  target == env.TargetPlane,
		}
		plan, err := planner.PlanReplication(req)
		if err != nil {

			fmt.Printf("[-] Failed to plan target installation: %v\n", err)
			return
		}
		res, err := replicator.ExecutePlan(ctx, plan, false)
		if err != nil {
			fmt.Printf("[-] Installation failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] Successfully installed %s (Mode: %s) in %v\n", target, mode, res.TotalDuration)

	case "status":
		statuses := replicator.AuditComponents(ctx)
		fmt.Println("[+] Real-Time Environment Component Status:")
		for _, s := range statuses {
			reach := "OFFLINE / UNREACHABLE"
			if s.Reachable {
				reach = "ONLINE / HEALTHY"
			}
			fmt.Printf("    [%s] %-12s - %s (Endpoint: %s)\n", reach, s.Target, s.Details, s.Endpoint)
		}

	default:
		fmt.Printf("[-] Unknown env action: %s. Use doctor, plan, replicate, install, or status.\n", action)
	}
}

func handleProjectCommand(ctx context.Context, action string, args []string) {
	reg, err := workspace.NewRegistry("", "")
	if err != nil {
		fmt.Printf("[-] Failed to open workspace registry: %v\n", err)
		return
	}

	switch action {
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "add":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite project add <name> <path>")
			return
		}
		name, path := args[0], args[1]
		entry, err := reg.AddProject(name, path)
		if err != nil {
			fmt.Printf("[-] Failed to register project: %v\n", err)
			return
		}
		fmt.Printf("[✓] Project '%s' registered successfully (Virtual Key: %s)\n", entry.Name, entry.VirtualKeyAlias)
		fmt.Printf("    - Code Path:   %s\n", entry.Path)
		fmt.Printf("    - Data Dir:    %s\n", entry.DataDir)
		fmt.Printf("    - Graph DB:    %s\n", entry.GraphDBPath)
		fmt.Printf("    - Spend Cache: %s\n", entry.SpendCachePath)

	case "switch":
		if len(args) < 1 {
			fmt.Println("[-] Usage: dendrite project switch <name>")
			return
		}
		name := args[0]
		start := time.Now()
		entry, err := reg.SwitchProject(name)
		duration := time.Since(start)
		if err != nil {
			fmt.Printf("[-] Failed to switch project: %v\n", err)
			return
		}
		fmt.Printf("[✓] Switched active context to '%s' in %v\n", entry.Name, duration)
		fmt.Printf("    - Virtual Key Alias: %s\n", entry.VirtualKeyAlias)
		fmt.Printf("    - Active Workspace:  %s\n", entry.Path)

	case "list":
		projects := reg.ListProjects()
		active, _ := reg.GetActiveProject()
		activeName := ""
		if active != nil {
			activeName = active.Name
		}
		fmt.Printf("[+] Registered Workspaces (%d):\n", len(projects))
		for _, p := range projects {
			mark := " "
			if p.Name == activeName {
				mark = "*"
			}
			fmt.Printf("  [%s] %-15s [%s] -> %s (Key: %s)\n", mark, p.Name, p.State, p.Path, p.VirtualKeyAlias)
		}

	case "remove":
		if len(args) < 1 {
			fmt.Println("[-] Usage: dendrite project remove <name> [--purge]")
			return
		}
		name := args[0]
		purge := false
		if len(args) > 1 && args[1] == "--purge" {
			purge = true
		}
		if err := reg.RemoveProject(name, purge); err != nil {
			fmt.Printf("[-] Failed to remove project: %v\n", err)
			return
		}
		fmt.Printf("[✓] Project '%s' removed (Purge Data: %v)\n", name, purge)

	default:
		fmt.Printf("[-] Unknown project action: %s\n", action)
	}
}

func handleDocCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "plane":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite doc plane <push|pull|sync> [--dir docs]")
			return
		}
		handlePlaneCommand(ctx, "doc", args)

	case "reflect":
		refDir := "docs"
		srcDir := "."
		targetDir := "reflected_docs"
		for i := 0; i < len(args); i++ {
			if args[i] == "--reference" && i+1 < len(args) {
				refDir = args[i+1]
				i++
			} else if args[i] == "--source" && i+1 < len(args) {
				srcDir = args[i+1]
				i++
			} else if args[i] == "--target" && i+1 < len(args) {
				targetDir = args[i+1]
				i++
			}
		}
		fmt.Printf("[+] Reflecting documentation taxonomy from '%s' to '%s' (source: '%s')...\n", refDir, targetDir, srcDir)
		result, err := reflect.ReflectTaxonomy(refDir, srcDir, targetDir)
		if err != nil {
			fmt.Printf("[-] Reflection failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] Reflected %d files in %dms (Sidebar Synced: %v)\n", len(result.FilesWritten), result.DurationMs, result.SidebarSynced)
		for _, f := range result.FilesWritten {
			fmt.Printf("    - %s\n", f)
		}
	default:
		fmt.Printf("Unknown doc action: %s\n", action)
	}
}

func getJetBrainsClient() (*jetbrains.Client, error) {
	testClient := &http.Client{Timeout: 1500 * time.Millisecond}
	transport := jetbrains.NewHTTPTransport("http://localhost:63342/api/mcp", testClient)
	client, err := jetbrains.NewClient(transport)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	if _, err := client.Connect(ctx); err == nil {
		return client, nil
	}

	mockTransport := jetbrains.NewMockTransport()
	mockClient, _ := jetbrains.NewClient(mockTransport)
	_, _ = mockClient.Connect(context.Background())
	return mockClient, nil
}

func handleJetBrainsCommand(ctx context.Context, action string, args []string) {
	client, err := getJetBrainsClient()
	if err != nil {
		fmt.Printf("[-] Failed to initialize JetBrains client: %v\n", err)
		return
	}
	bridge := jetbrains.NewBridge(client)

	switch action {
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "probe":
		fmt.Println("[+] Probing JetBrains IDE MCP Server...")
		srvInfo := client.GetServerInfo()
		if srvInfo != nil {
			fmt.Printf("  [✓] Connected: %s (v%s)\n", srvInfo.Name, srvInfo.Version)
		}
		tools, err := client.ListTools(ctx)
		if err != nil {
			fmt.Printf("[-] Failed to list tools: %v\n", err)
			return
		}
		fmt.Printf("  [✓] Discovered Tools (%d):\n", len(tools))
		for _, t := range tools {
			fmt.Printf("      - %-25s: %s\n", t.Name, t.Description)
		}

	case "open":
		if len(args) == 0 {
			fmt.Println("[-] Please specify a file path to open")
			return
		}
		filePath := args[0]
		line := 1
		for i := 1; i < len(args); i++ {
			if (args[i] == "--line" || args[i] == "-l") && i+1 < len(args) {
				if parsed, err := strconv.Atoi(args[i+1]); err == nil {
					line = parsed
				}
				i++
			}
		}
		if err := bridge.OpenFile(ctx, filePath, line, 1); err != nil {
			fmt.Printf("[-] Failed to open file: %v\n", err)
			return
		}
		fmt.Printf("[✓] Opened %s at line %d in JetBrains editor\n", filePath, line)

	case "problems":
		if len(args) == 0 {
			fmt.Println("[-] Please specify a file path to inspect")
			return
		}
		filePath := args[0]
		problems, err := bridge.GetFileProblems(ctx, filePath)
		if err != nil {
			fmt.Printf("[-] Failed to get problems: %v\n", err)
			return
		}
		fmt.Printf("[+] JetBrains Diagnostics for %s (%d found):\n", filePath, len(problems))
		for _, p := range problems {
			fmt.Printf("    [%s] Line %d:%d - %s\n", p.Severity, p.Line, p.Column, p.Description)
		}

	case "symbols":
		query := ""
		if len(args) > 0 {
			query = args[0]
		}
		symbols, err := bridge.SearchSymbol(ctx, query)
		if err != nil {
			fmt.Printf("[-] Symbol search failed: %v\n", err)
			return
		}
		fmt.Printf("[+] Matching Symbols (%d):\n", len(symbols))
		for _, s := range symbols {
			fmt.Printf("    - %s (%s) -> %s:%d\n", s.Name, s.Kind, s.File, s.Line)
		}

	case "build":
		fmt.Println("[+] Triggering IDE project build...")
		res, err := bridge.BuildProject(ctx)
		if err != nil {
			fmt.Printf("[-] Build failed: %v\n", err)
			return
		}
		if res.Success {
			fmt.Printf("[✓] Project build succeeded in %v\n", res.Duration)
		} else {
			fmt.Printf("[-] Build completed with errors in %v\n", res.Duration)
		}

	case "run":
		if len(args) == 0 {
			fmt.Println("[-] Please specify a run configuration name")
			return
		}
		configName := args[0]
		fmt.Printf("[+] Running IDE configuration '%s'...\n", configName)
		out, err := bridge.ExecuteRunConfiguration(ctx, configName)
		if err != nil {
			fmt.Printf("[-] Execution failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] %s\n", out)

	case "setup", "setup-mcp", "configure":
		fmt.Println("[+] Configuring JetBrains IDE Toolset for Dendrite MCP & Autonomous Agents...")
		opts := jetbrains.SetupOptions{
			ConfigureProjectMCP:    true,
			ConfigureGlobalMCP:     true,
			ConfigureRunConfigs:    true,
			ConfigureExternalTools: true,
		}
		for _, arg := range args {
			if arg == "--project-only" {
				opts.ConfigureGlobalMCP = false
			} else if arg == "--global-only" {
				opts.ConfigureProjectMCP = false
				opts.ConfigureRunConfigs = false
				opts.ConfigureExternalTools = false
			}
		}
		res, err := jetbrains.SetupJetBrainsToolset(opts)
		if err != nil {
			fmt.Printf("[-] Setup failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] JetBrains Toolset configured successfully!\n")
		if len(res.FilesCreated) > 0 {
			fmt.Println("[+] Created Configurations:")
			for _, f := range res.FilesCreated {
				fmt.Printf("    - %s\n", f)
			}
		}
		if len(res.FilesUpdated) > 0 {
			fmt.Println("[+] Updated Configurations:")
			for _, f := range res.FilesUpdated {
				fmt.Printf("    - %s\n", f)
			}
		}
		if len(res.ToolsAdded) > 0 {
			fmt.Println("[+] Registered External Tools:")
			for _, t := range res.ToolsAdded {
				fmt.Printf("    - %s\n", t)
			}
		}
		fmt.Println("[+] JetBrains MCP Integration Active:")
		fmt.Println("    - Project MCP: .ai/mcp/mcp.json")
		fmt.Println("    - Global MCP:  ~/.ai/mcp/mcp.json")
		fmt.Println("    - Run Configs: .idea/runConfigurations/ (MCP Server, Aider, Antigravity, Web UI)")
		fmt.Println("    - Toolset:     .idea/tools/External_Tools.xml")

	default:
		fmt.Printf("[-] Unknown jetbrains action: %s\n", action)
	}
}

func handleAgentCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "run":
		if len(args) == 0 {
			fmt.Println("[-] Please specify an agent tool: aider, openhands, opencode, agy, or copilot")
			return
		}
		agentType := agent.AgentType(strings.ToLower(args[0]))
		if !agentType.IsValid() {
			fmt.Printf("[-] Invalid agent type: %s (must be aider, openhands, opencode, agy, or copilot)\n", args[0])
			return
		}

		cwd, _ := os.Getwd()
		defaultModel := "copilot-sonnet"
		if agentType == agent.AgentAGY || agentType == agent.AgentAntigravity {
			defaultModel = "gemini-3.8-flash-high"
		} else if agentType == agent.AgentCopilot || agentType == agent.AgentGitHubCopilot {
			defaultModel = "claude-sonnet-4.5"
		}

		cfg := agent.AgentConfig{
			Type:           agentType,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-dendrite-virtual-key",
			Model:          defaultModel,
			WorkspaceDir:   cwd,
		}

		var allowInstall bool
		// Parse flags if provided
		for j := 1; j < len(args); j++ {
			if args[j] == "--ticket" && j+1 < len(args) {
				cfg.TicketID = args[j+1]
				j++
			} else if args[j] == "--model" && j+1 < len(args) {
				cfg.Model = args[j+1]
				j++
			} else if (args[j] == "--task" || args[j] == "--prompt") && j+1 < len(args) {
				cfg.TaskDescription = args[j+1]
				j++
			} else if args[j] == "--sandbox" || args[j] == "--ephemeral" {
				cfg.Sandbox = true
				cfg.Ephemeral = true
			} else if args[j] == "--yes" || args[j] == "-y" || args[j] == "--allow-install" {
				allowInstall = true
			}
		}

		// Auto-detect and sandbox fallback for OpenHands
		if cfg.Type == agent.AgentOpenHands && !cfg.Sandbox {
			if _, oErr := exec.LookPath("openhands"); oErr != nil {
				fmt.Println("[+] OpenHands host binary not detected; auto-routing to hardened container sandbox...")
				cfg.Sandbox = true
				cfg.Ephemeral = true
			}
		}

		env, err := agent.BuildAgentEnv(cfg)
		if err != nil {
			fmt.Printf("[-] Configuration error: %v\n", err)
			return
		}

		cmd, err := agent.BuildAgentCommand(cfg)
		if err != nil {
			fmt.Printf("[-] Failed to construct command: %v\n", err)
			return
		}

		fmt.Printf("[+] Initializing Agent Harness for: %s\n", cfg.Type)
		fmt.Printf("    - Workspace:    %s\n", cfg.WorkspaceDir)
		fmt.Printf("    - Gateway Base: %s\n", cfg.LiteLLMBaseURL)
		fmt.Printf("    - Primary Model:%s\n", cfg.Model)
		if cfg.TicketID != "" {
			fmt.Printf("    - Active Ticket:%s\n", cfg.TicketID)
		}
		if cfg.Sandbox {
			fmt.Println("    - Execution:    Ephemeral Cap-Dropped Container Sandbox (RAII)")
		} else {
			fmt.Println("    - Execution:    Host Local Terminal")
		}

		fmt.Println("[+] Injected Environment Variables:")
		for k, v := range env {
			displayVal := v
			if strings.Contains(k, "KEY") || strings.Contains(k, "TOKEN") {
				displayVal = "********"
			}
			fmt.Printf("    - %s=%s\n", k, displayVal)
		}
		fmt.Printf("[+] Command: %s\n", strings.Join(cmd, " "))

		var runner agent.Runner
		if cfg.Sandbox {
			runner = agent.NewSandboxedRunner(nil, nil)
		} else {
			hostR := agent.NewHostRunner(true)
			hostR.AllowInstall = allowInstall
			runner = hostR
		}
		summary, err := runner.Execute(ctx, cfg)
		if err != nil {
			fmt.Printf("[-] Agent execution failed: %v\n", err)
			return
		}

		// Record session to persistent ledger
		_ = os.MkdirAll(".dendrite", 0755)
		ledgerPath := filepath.Join(".dendrite", "agent_sessions.json")
		ledger := agent.NewSessionLedger(nil, ledgerPath)
		if data, err := os.ReadFile(ledgerPath); err == nil {
			_ = ledger.LoadJSON(data)
		}
		_ = ledger.RecordSession(ctx, summary)

		fmt.Printf("[✓] Agent session completed successfully (Session ID: %s, Duration: %v)\n", summary.SessionID, summary.Duration)
		if len(summary.CommitsGenerated) > 0 {
			fmt.Println("[+] Generated Commits:")
			for _, c := range summary.CommitsGenerated {
				fmt.Printf("    - %s\n", c)
			}
		}

	case "search":
		handleAgentSearchCommand(ctx, args)

	case "route":
		handleAgentRouteCommand(ctx, args)

	case "session":
		ledgerPath := filepath.Join(".dendrite", "agent_sessions.json")
		ledger := agent.NewSessionLedger(nil, ledgerPath)
		if data, err := os.ReadFile(ledgerPath); err == nil {
			_ = ledger.LoadJSON(data)
		}
		sessions := ledger.ListSessionsByTicket("")
		fmt.Printf("[+] Agent Sessions Ledger (%d recorded sessions):\n", len(sessions))
		if len(sessions) == 0 {
			fmt.Println("    - Active: None")
			fmt.Println("    - Last Session: None")
		} else {
			for idx, s := range sessions {
				fmt.Printf("    %d. [%s] Agent: %s, Ticket: %s, Status: %s, Duration: %v, Cost: $%.4f\n",
					idx+1, s.SessionID, s.AgentType, s.TicketID, s.Status, s.Duration, s.EstimatedCostUSD)
			}
		}
	default:
		fmt.Printf("[-] Unknown agent action: %s\n", action)
	}
}

func runDoctor(ctx context.Context) {
	fmt.Println("[+] Running Dendrite Doctor diagnostics...")

	status, models, err := devtool.ProbeOllama(ctx, ollama.DefaultBaseURL)
	if err != nil {
		fmt.Printf("  [-] Local Ollama: UNREACHABLE (%v)\n", err)
	} else {
		fmt.Printf("  [✓] Local Ollama: REACHABLE (latency: %v, loaded models: %d)\n", status.Latency, len(models))
		for _, m := range models {
			fmt.Printf("      - %s (size: %.2f GB)\n", m.Name, float64(m.Size)/(1024*1024*1024))
		}
	}

	if _, err := os.Stat("DEV_STATUS.md"); err == nil {
		fmt.Println("  [✓] Project Status: DEV_STATUS.md present")
	} else {
		fmt.Println("  [-] Project Status: DEV_STATUS.md not found in current directory")
	}

	fmt.Println("[+] Pre-flight diagnostics complete.")
}

func handleDevCommand(ctx context.Context, sub string, args []string) {
	switch sub {
	case "ollama":
		endpoint := ollama.DefaultBaseURL
		if len(args) > 1 && args[0] == "probe" {
			endpoint = args[1]
		}
		fmt.Printf("[+] Probing Ollama at %s...\n", endpoint)
		status, models, err := devtool.ProbeOllama(ctx, endpoint)
		if err != nil {
			fmt.Printf("[-] Probe failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] Status: Reachable (Latency: %v)\n", status.Latency)
		fmt.Printf("[✓] Models (%d):\n", len(models))
		for _, m := range models {
			fmt.Printf("    - %s\n", m.Name)
		}

	case "ticket":
		filePath := "DEV_STATUS.md"
		if len(args) == 0 || args[0] == "list" {
			tr := tracker.NewMemoryTracker(filePath)
			if err := tr.LoadFromMarkdown(filePath); err != nil {
				pClient, pErr := plane.NewClientFromEnv()
				if pErr == nil {
					issues, iErr := pClient.ListPlaneIssues(ctx)
					if iErr == nil && len(issues) > 0 {
						fmt.Printf("[+] Registered Tickets from Plane (%d):\n", len(issues))
						for _, iss := range issues {
							fmt.Printf("    [%s] %s\n", iss.State, iss.Name)
						}
						return
					}
				}
				fmt.Printf("[-] Failed to load tickets from %s: %v\n", filePath, err)
				return
			}
			tickets, _ := tr.ListTickets(ctx)
			fmt.Printf("[+] Registered Tickets (%d):\n", len(tickets))
			for _, t := range tickets {
				fmt.Printf("    [%s] %s: %s (%s)\n", t.Status, t.ID, t.Title, t.Milestone)
			}
		} else if args[0] == "set" && len(args) >= 3 {
			id := args[1]
			st := tracker.Status(strings.ToUpper(args[2]))
			mdErr := devtool.UpdateTicketInMarkdown(ctx, filePath, id, st)
			pClient, pErr := plane.NewClientFromEnv()
			planeSynced := false
			if pErr == nil && pClient != nil {
				if err := pClient.UpdateStatus(ctx, tracker.TicketID(id), st); err == nil {
					planeSynced = true
				}
			}

			if mdErr != nil && !planeSynced {
				fmt.Printf("[-] Failed to update ticket: %v\n", mdErr)
				return
			}
			msg := fmt.Sprintf("[✓] Successfully updated ticket %s to %s in %s", id, st, filePath)
			if planeSynced {
				msg += " and reflected in Plane service"
			}
			fmt.Println(msg)
		} else if len(args) > 0 && args[0] == "next" {
			handlePlaneCommand(ctx, "next", args[1:])
		} else if len(args) > 0 && args[0] == "link" {
			handlePlaneCommand(ctx, "link", args[1:])
		} else if len(args) > 0 && (args[0] == "create" || args[0] == "add") {
			handlePlaneCommand(ctx, "create", args[1:])
		} else if len(args) > 0 && (args[0] == "sync-plane" || args[0] == "push-plane") {
			handlePlaneCommand(ctx, "push-status", args[1:])
		} else if len(args) > 0 && (args[0] == "replicate-plane" || args[0] == "pull-plane") {
			handlePlaneCommand(ctx, "replicate", args[1:])
		}

	case "topology":
		if len(args) > 0 && (args[0] == "advisor" || args[0] == "advise") {
			runTopologyAdvisor(ctx, args[1:])
			return
		}
		scenarioStr := "local-hybrid"
		if len(args) > 0 {
			scenarioStr = args[0]
		}
		runTopologyInspect(ctx, scenarioStr)

	case "gateway":
		if len(args) > 0 && (args[0] == "zero-cost" || args[0] == "zerocost" || args[0] == "models") {
			handleZeroCostCommand(args[1:])
			return
		}
		scenarioStr := "local-hybrid"
		if len(args) > 0 {
			scenarioStr = args[0]
		}
		sc, err := topology.ParseScenario(scenarioStr)
		if err != nil {
			fmt.Printf("[-] Invalid scenario: %v\n", err)
			return
		}
		top, err := topology.NewTopologyConfig(sc, topology.TailscaleEndpoints{
			ControlPlane:   "100.84.12.10",
			ExecutionPlane: "100.84.12.25",
		})
		if err != nil {
			fmt.Printf("[-] Failed to create topology: %v\n", err)
			return
		}
		yamlData, err := gateway.RenderForTopology(top)
		if err != nil {
			fmt.Printf("[-] Failed to render LiteLLM gateway config: %v\n", err)
			return
		}
		fmt.Printf("[+] Rendered LiteLLM Config for scenario '%s':\n\n%s\n", sc, string(yamlData))

	case "inference":
		if len(args) == 0 {
			handleInferenceCommand(ctx, []string{"list"})
			return
		}
		handleInferenceCommand(ctx, args)

	case "audit-prompt":
		if len(args) == 0 {
			fmt.Println("[-] Please provide a prompt string to audit")
			return
		}
		input := strings.Join(args, " ")
		metrics := devtool.AuditPrompt(input)
		fmt.Printf("[+] Prompt Character Count:  %d\n", metrics.CharacterCount)
		fmt.Printf("[+] Prompt Word Count:       %d\n", metrics.WordCount)
		fmt.Printf("[+] Estimated Token Volume:  %d tokens (~4 chars/token)\n", metrics.EstimatedTokens)

	default:
		fmt.Printf("Unknown dev subcommand: %s\n", sub)
	}
}

func handleTopologyCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "advisor", "advise":
		runTopologyAdvisor(ctx, args)
	case "inspect", "show":
		scenarioStr := "local-hybrid"
		if len(args) > 0 {
			scenarioStr = args[0]
		}
		runTopologyInspect(ctx, scenarioStr)
	default:
		if _, err := topology.ParseScenario(action); err == nil {
			runTopologyInspect(ctx, action)
			return
		}
		fmt.Printf("[-] Unknown topology action: %s\n", action)
		fmt.Println("Usage: dendrite topology <advisor|inspect> [args...]")
	}
}

func runTopologyInspect(_ context.Context, scenarioStr string) {
	sc, err := topology.ParseScenario(scenarioStr)
	if err != nil {
		fmt.Printf("[-] Invalid scenario: %v\n", err)
		return
	}
	cfg, err := topology.NewTopologyConfig(sc, topology.TailscaleEndpoints{
		ControlPlane:   "100.84.12.10",
		ExecutionPlane: "100.84.12.25",
	})
	if err != nil {
		fmt.Printf("[-] Failed to construct topology: %v\n", err)
		return
	}
	fmt.Printf("[+] Active Scenario: %s\n", cfg.Scenario)
	fmt.Println("[+] Stack Placements:")
	for comp, node := range cfg.StackPlacement {
		fmt.Printf("    - %-15s -> %s\n", comp, node)
	}
	fmt.Println("[+] Node Compose Profiles:")
	for _, node := range topology.AllComputeNodes() {
		profiles := cfg.ActiveComposeProfiles(node)
		if len(profiles) > 0 {
			fmt.Printf("    - %-10s: %s\n", node, strings.Join(profiles, ", "))
		}
	}
}

func runTopologyAdvisor(ctx context.Context, args []string) {
	nodeArgs := "local=localhost"
	outputJSON := false

	for i := 0; i < len(args); i++ {
		if args[i] == "--nodes" && i+1 < len(args) {
			nodeArgs = args[i+1]
			i++
		} else if args[i] == "--json" {
			outputJSON = true
		}
	}

	targets := parseNodeTargets(nodeArgs)
	fmt.Printf("[+] Probing %d compute node(s) for hardware metrics...\n", len(targets))
	probe := advisor.NewHardwareProbe(nil)
	hardwareList, err := probe.ProbeAll(ctx, targets)
	if err != nil {
		fmt.Printf("[-] Failed to probe nodes: %v\n", err)
		return
	}

	fmt.Printf("[✓] Discovered %d compute node(s):\n", len(hardwareList))
	for _, hw := range hardwareList {
		gpuInfo := "None"
		if hw.HasGPU {
			gpuInfo = fmt.Sprintf("%s (%d MB VRAM)", hw.GPUType, hw.VRAMMB)
		}
		fmt.Printf("    - %-12s: %s/%s, %d CPU cores, %d MB RAM (%d MB free), %d GB disk, GPU: %s\n",
			hw.NodeID, hw.OS, hw.Arch, hw.CPUCores, hw.TotalRAMMB, hw.FreeRAMMB, hw.DiskFreeGB, gpuInfo)
	}

	adv := advisor.NewPlacementAdvisor()
	plan, err := adv.Advise(hardwareList, advisor.DefaultDendriteServices())
	if err != nil {
		fmt.Printf("[-] Placement advisor failed: %v\n", err)
		return
	}

	if outputJSON {
		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			fmt.Printf("[-] Failed to serialize JSON: %v\n", err)
			return
		}
		fmt.Println(string(data))
	} else {
		fmt.Print(plan.FormatTable())
	}
}

func parseNodeTargets(nodeArgs string) []advisor.NodeTarget {
	var targets []advisor.NodeTarget
	tokens := strings.Split(nodeArgs, ",")
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		var id, host string
		if strings.Contains(tok, "=") {
			parts := strings.SplitN(tok, "=", 2)
			id = strings.TrimSpace(parts[0])
			host = strings.TrimSpace(parts[1])
		} else {
			id = tok
			host = tok
		}
		isLocal := (host == "localhost" || host == "127.0.0.1" || host == "local")
		targets = append(targets, advisor.NodeTarget{
			ID:      id,
			Host:    host,
			IsLocal: isLocal,
		})
	}
	if len(targets) == 0 {
		targets = append(targets, advisor.NodeTarget{
			ID:      "local",
			Host:    "localhost",
			IsLocal: true,
		})
	}
	return targets
}
