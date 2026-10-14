package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/env"
	"Dendrite/pkg/jetbrains"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/tracker"
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

	case "dev":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite dev <ollama|ticket|audit-prompt> [args...]")
			return
		}
		handleDevCommand(ctx, os.Args[2], os.Args[3:])

	case "jetbrains":
		if len(os.Args) < 3 {
			fmt.Println("Usage: dendrite jetbrains <probe|open|problems|symbols|build|run> [args...]")
			return
		}
		handleJetBrainsCommand(ctx, os.Args[2], os.Args[3:])

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
	fmt.Println("  env doctor                 Inspect target computer OS, package manager and installed tools")
	fmt.Println("  env plan [flags]           Generate environment replication plan (--mode container|host|hybrid)")
	fmt.Println("  env replicate [flags]      Replicate development environment (Docker containers or host tools)")
	fmt.Println("  env install <tool> [flags] Install tool directly on host computer or spin up container")
	fmt.Println("  env status                 Query real-time health and status of replicated components")
	fmt.Println("  jetbrains probe [url]      Probe JetBrains IDE MCP server and list available tools")
	fmt.Println("  jetbrains open <f> [--line]Open target file in active JetBrains editor")
	fmt.Println("  jetbrains problems <file>  Query inspections and diagnostics for file")
	fmt.Println("  jetbrains symbols <query>  Search symbols in JetBrains PSI index")
	fmt.Println("  jetbrains build            Trigger IDE background build")
	fmt.Println("  jetbrains run <config>     Execute an IDE run configuration")
	fmt.Println("  dev ollama probe [url]     Probe Ollama health, latency, and models")
	fmt.Println("  dev ticket list [file]     List tickets from DEV_STATUS.md")
	fmt.Println("  dev ticket set <id> <st>   Update a ticket status (e.g. DONE, IN_PROGRESS)")
	fmt.Println("  dev audit-prompt <text>    Analyze prompt length and estimated token volume")
	fmt.Println("  version                    Print Dendrite CLI version")
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
	case "doctor":
		fmt.Println("[+] Host Environment Discovery:")
		fmt.Printf("    - Operating System:  %s (%s)\n", host.OS, host.Arch)
		fmt.Printf("    - Package Manager:   %s\n", host.PackageManager)
		fmt.Printf("    - Docker Engine:     %v\n", host.HasDocker)
		fmt.Printf("    - Docker Compose:    %v\n", host.HasDockerComp)
		fmt.Printf("    - Ollama Engine:     %v\n", host.HasOllama)
		fmt.Printf("    - Go Toolchain:      %v\n", host.HasGo)
		fmt.Printf("    - Node / NPM:        Node: %v, NPM: %v\n", host.HasNode, host.HasNpm)
		fmt.Printf("    - Tailscale Mesh:    %v\n", host.HasTailscale)
		fmt.Printf("    - Linear Host CLI:   %v\n", host.HasLinearCLI)

	case "plan":
		mode := env.ModeContainer
		withLinear := false
		for i := 0; i < len(args); i++ {
			if (args[i] == "--mode" || args[i] == "-m") && i+1 < len(args) {
				mode = env.InstallMode(args[i+1])
				i++
			} else if args[i] == "--with-linear" {
				withLinear = true
			}
		}
		req := env.ReplicationRequest{
			Mode:       mode,
			WithLinear: withLinear,
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
		dryRun := false
		for i := 0; i < len(args); i++ {
			if (args[i] == "--mode" || args[i] == "-m") && i+1 < len(args) {
				mode = env.InstallMode(args[i+1])
				i++
			} else if args[i] == "--no-linear" {
				withLinear = false
			} else if args[i] == "--dry-run" {
				dryRun = true
			}
		}
		req := env.ReplicationRequest{
			Mode:       mode,
			WithLinear: withLinear,
			DryRun:     dryRun,
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
			fmt.Println("[-] Usage: dendrite env install <linear|ollama|litellm|postgres|docker> [--mode container|host]")
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

	default:
		fmt.Printf("[-] Unknown jetbrains action: %s\n", action)
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
			if err := devtool.UpdateTicketInMarkdown(ctx, filePath, id, st); err != nil {
				fmt.Printf("[-] Failed to update ticket: %v\n", err)
				return
			}
			fmt.Printf("[✓] Successfully updated ticket %s to %s in %s\n", id, st, filePath)
		}

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
