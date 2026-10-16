package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/env"
	"Dendrite/pkg/jetbrains"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/tracker"

	"github.com/spf13/cobra"
)

const version = "0.1.0-alpha"

// HostDetector abstracts host environment discovery.
type HostDetector interface {
	Detect(ctx context.Context) (*env.HostInfo, error)
}

var (
	jetBrainsEndpoint       = "http://localhost:63342/api/mcp"
	jetBrainsClientProvider = getJetBrainsClient
	hostDetectorProvider    = func() HostDetector { return env.NewDetector(nil) }
)

func runMain(ctx context.Context, args []string) error {
	rootCmd := buildRootCommand()
	rootCmd.SetArgs(args)
	return rootCmd.ExecuteContext(ctx)
}

func main() {
	if err := runMain(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
		os.Exit(1)
	}
}

func buildRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "dendrite",
		Version: version,
		Short:   "Autonomous AI Development & Infrastructure Control Plane",
		Long:    "Dendrite - Autonomous AI Development & Infrastructure Control Plane",
	}
	rootCmd.SetVersionTemplate("Dendrite CLI v{{.Version}}\n")

	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newDoctorCommand())
	rootCmd.AddCommand(newEnvCommand())
	rootCmd.AddCommand(newDevCommand())
	rootCmd.AddCommand(newJetBrainsCommand())

	return rootCmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print Dendrite CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Dendrite CLI v%s\n", version)
		},
	}
}

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run environment pre-flight diagnostics",
		Run: func(cmd *cobra.Command, args []string) {
			runDoctor(cmd.Context())
		},
	}
}

func newEnvCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage and replicate development environments",
	}

	cmd.AddCommand(newEnvDoctorCommand())
	cmd.AddCommand(newEnvPlanCommand())
	cmd.AddCommand(newEnvReplicateCommand())
	cmd.AddCommand(newEnvInstallCommand())
	cmd.AddCommand(newEnvStatusCommand())

	return cmd
}

func newEnvDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Inspect target computer OS, package manager and installed tools",
		Run: func(cmd *cobra.Command, args []string) {
			detector := hostDetectorProvider()
			host, err := detector.Detect(cmd.Context())
			if err != nil {
				fmt.Printf("[-] Host detection failed: %v\n", err)
				return
			}
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
		},
	}
}

func newEnvPlanCommand() *cobra.Command {
	var mode string
	var withLinear bool

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Generate environment replication plan (--mode container|host|hybrid)",
		Run: func(cmd *cobra.Command, args []string) {
			detector := hostDetectorProvider()
			host, err := detector.Detect(cmd.Context())
			if err != nil {
				fmt.Printf("[-] Failed to detect host: %v\n", err)
				return
			}
			planner := env.NewPlanner(*host)
			req := env.ReplicationRequest{
				Mode:       env.InstallMode(mode),
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
		},
	}

	cmd.Flags().StringVarP(&mode, "mode", "m", string(env.ModeContainer), "Replication mode (container, host, hybrid)")
	cmd.Flags().BoolVar(&withLinear, "with-linear", false, "Include Linear tracker in replication plan")
	return cmd
}

func newEnvReplicateCommand() *cobra.Command {
	var mode string
	var noLinear bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "replicate",
		Short: "Replicate development environment (Docker containers or host tools)",
		Run: func(cmd *cobra.Command, args []string) {
			detector := hostDetectorProvider()
			host, err := detector.Detect(cmd.Context())
			if err != nil {
				fmt.Printf("[-] Failed to detect host: %v\n", err)
				return
			}
			planner := env.NewPlanner(*host)
			replicator := env.NewReplicator(nil, *host)
			req := env.ReplicationRequest{
				Mode:       env.InstallMode(mode),
				WithLinear: !noLinear,
				DryRun:     dryRun,
			}
			plan, err := planner.PlanReplication(req)
			if err != nil {
				fmt.Printf("[-] Failed to plan replication: %v\n", err)
				return
			}
			fmt.Printf("[+] Executing Environment Replication (Mode: %s, DryRun: %v)...\n", mode, dryRun)
			res, err := replicator.ExecutePlan(cmd.Context(), plan, dryRun)
			if err != nil {
				fmt.Printf("[-] Replication encountered error: %v\n", err)
			}
			fmt.Printf("[\u2713] Completed in %v (Success: %v)\n", res.TotalDuration, res.Success)
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
		},
	}

	cmd.Flags().StringVarP(&mode, "mode", "m", string(env.ModeContainer), "Replication mode (container, host, hybrid)")
	cmd.Flags().BoolVar(&noLinear, "no-linear", false, "Exclude Linear tracker")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Simulate execution without running commands")
	return cmd
}

func newEnvInstallCommand() *cobra.Command {
	var mode string

	cmd := &cobra.Command{
		Use:   "install <tool>",
		Short: "Install tool directly on host computer or spin up container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			target := env.ReplicationTarget(args[0])
			detector := hostDetectorProvider()
			host, err := detector.Detect(cmd.Context())
			if err != nil {
				fmt.Printf("[-] Failed to detect host: %v\n", err)
				return
			}
			planner := env.NewPlanner(*host)
			replicator := env.NewReplicator(nil, *host)
			req := env.ReplicationRequest{
				Mode:       env.InstallMode(mode),
				Targets:    []env.ReplicationTarget{target},
				WithLinear: target == env.TargetLinear,
			}
			plan, err := planner.PlanReplication(req)
			if err != nil {
				fmt.Printf("[-] Failed to plan target installation: %v\n", err)
				return
			}
			res, err := replicator.ExecutePlan(cmd.Context(), plan, false)
			if err != nil {
				fmt.Printf("[-] Installation failed: %v\n", err)
				return
			}
			fmt.Printf("[\u2713] Successfully installed %s (Mode: %s) in %v\n", target, mode, res.TotalDuration)
		},
	}

	cmd.Flags().StringVarP(&mode, "mode", "m", string(env.ModeContainer), "Replication mode (container, host)")
	return cmd
}

func newEnvStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Query real-time health and status of replicated components",
		Run: func(cmd *cobra.Command, args []string) {
			detector := hostDetectorProvider()
			host, err := detector.Detect(cmd.Context())
			if err != nil {
				fmt.Printf("[-] Failed to detect host: %v\n", err)
				return
			}
			replicator := env.NewReplicator(nil, *host)
			statuses := replicator.AuditComponents(cmd.Context())
			fmt.Println("[+] Real-Time Environment Component Status:")
			for _, s := range statuses {
				reach := "OFFLINE / UNREACHABLE"
				if s.Reachable {
					reach = "ONLINE / HEALTHY"
				}
				fmt.Printf("    [%s] %-12s - %s (Endpoint: %s)\n", reach, s.Target, s.Details, s.Endpoint)
			}
		},
	}
}

func newDevCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Developer utility and local AI commands",
	}

	cmd.AddCommand(newDevOllamaCommand())
	cmd.AddCommand(newDevTicketCommand())
	cmd.AddCommand(newDevAuditPromptCommand())

	return cmd
}

func newDevOllamaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ollama [probe] [endpoint]",
		Short: "Probe Ollama health, latency, and models",
		Run: func(cmd *cobra.Command, args []string) {
			endpoint := ollama.DefaultBaseURL
			if len(args) > 1 && args[0] == "probe" {
				endpoint = args[1]
			} else if len(args) == 1 && args[0] != "probe" {
				endpoint = args[0]
			}
			fmt.Printf("[+] Probing Ollama at %s...\n", endpoint)
			status, models, err := devtool.ProbeOllama(cmd.Context(), endpoint)
			if err != nil {
				fmt.Printf("[-] Probe failed: %v\n", err)
				return
			}
			fmt.Printf("[\u2713] Status: Reachable (Latency: %v)\n", status.Latency)
			fmt.Printf("[\u2713] Models (%d):\n", len(models))
			for _, m := range models {
				fmt.Printf("    - %s\n", m.Name)
			}
		},
	}
}

func newDevTicketCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ticket",
		Short: "Manage development status tickets",
		Run: func(cmd *cobra.Command, args []string) {
			filePath := "DEV_STATUS.md"
			if len(args) > 0 {
				filePath = args[0]
			}
			listTickets(cmd.Context(), filePath)
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list [file]",
		Short: "List tickets from DEV_STATUS.md",
		Run: func(cmd *cobra.Command, args []string) {
			filePath := "DEV_STATUS.md"
			if len(args) > 0 {
				filePath = args[0]
			}
			listTickets(cmd.Context(), filePath)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set <id> <status> [file]",
		Short: "Update a ticket status (e.g. DONE, IN_PROGRESS)",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			id := args[0]
			st := tracker.Status(strings.ToUpper(args[1]))
			filePath := "DEV_STATUS.md"
			if len(args) > 2 {
				filePath = args[2]
			}
			if err := devtool.UpdateTicketInMarkdown(cmd.Context(), filePath, id, st); err != nil {
				fmt.Printf("[-] Failed to update ticket: %v\n", err)
				return
			}
			fmt.Printf("[\u2713] Successfully updated ticket %s to %s in %s\n", id, st, filePath)
		},
	})

	return cmd
}

func listTickets(ctx context.Context, filePath string) {
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
}

func newDevAuditPromptCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "audit-prompt <text...>",
		Short: "Analyze prompt length and estimated token volume",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			input := strings.Join(args, " ")
			metrics := devtool.AuditPrompt(input)
			fmt.Printf("[+] Prompt Character Count:  %d\n", metrics.CharacterCount)
			fmt.Printf("[+] Prompt Word Count:       %d\n", metrics.WordCount)
			fmt.Printf("[+] Estimated Token Volume:  %d tokens (~4 chars/token)\n", metrics.EstimatedTokens)
		},
	}
}

func newJetBrainsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jetbrains",
		Short: "JetBrains IDE MCP bridge and automation commands",
	}

	cmd.AddCommand(newJetBrainsProbeCommand())
	cmd.AddCommand(newJetBrainsOpenCommand())
	cmd.AddCommand(newJetBrainsProblemsCommand())
	cmd.AddCommand(newJetBrainsSymbolsCommand())
	cmd.AddCommand(newJetBrainsBuildCommand())
	cmd.AddCommand(newJetBrainsRunCommand())

	return cmd
}

func newJetBrainsProbeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "probe [url]",
		Short: "Probe JetBrains IDE MCP server and list available tools",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := jetBrainsClientProvider()
			if err != nil {
				fmt.Printf("[-] Failed to initialize JetBrains client: %v\n", err)
				return
			}
			fmt.Println("[+] Probing JetBrains IDE MCP Server...")
			srvInfo := client.GetServerInfo()
			if srvInfo != nil {
				fmt.Printf("  [\u2713] Connected: %s (v%s)\n", srvInfo.Name, srvInfo.Version)
			}
			tools, err := client.ListTools(cmd.Context())
			if err != nil {
				fmt.Printf("[-] Failed to list tools: %v\n", err)
				return
			}
			fmt.Printf("  [\u2713] Discovered Tools (%d):\n", len(tools))
			for _, t := range tools {
				fmt.Printf("      - %-25s: %s\n", t.Name, t.Description)
			}
		},
	}
}

func newJetBrainsOpenCommand() *cobra.Command {
	var line int

	cmd := &cobra.Command{
		Use:   "open <file>",
		Short: "Open target file in active JetBrains editor",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := jetBrainsClientProvider()
			if err != nil {
				fmt.Printf("[-] Failed to initialize JetBrains client: %v\n", err)
				return
			}
			bridge := jetbrains.NewBridge(client)
			filePath := args[0]
			if err := bridge.OpenFile(cmd.Context(), filePath, line, 1); err != nil {
				fmt.Printf("[-] Failed to open file: %v\n", err)
				return
			}
			fmt.Printf("[\u2713] Opened %s at line %d in JetBrains editor\n", filePath, line)
		},
	}

	cmd.Flags().IntVarP(&line, "line", "l", 1, "Line number to navigate to")
	return cmd
}

func newJetBrainsProblemsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "problems <file>",
		Short: "Query inspections and diagnostics for file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := jetBrainsClientProvider()
			if err != nil {
				fmt.Printf("[-] Failed to initialize JetBrains client: %v\n", err)
				return
			}
			bridge := jetbrains.NewBridge(client)
			filePath := args[0]
			problems, err := bridge.GetFileProblems(cmd.Context(), filePath)
			if err != nil {
				fmt.Printf("[-] Failed to get problems: %v\n", err)
				return
			}
			fmt.Printf("[+] JetBrains Diagnostics for %s (%d found):\n", filePath, len(problems))
			for _, p := range problems {
				fmt.Printf("    [%s] Line %d:%d - %s\n", p.Severity, p.Line, p.Column, p.Description)
			}
		},
	}
}

func newJetBrainsSymbolsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "symbols [query]",
		Short: "Search symbols in JetBrains PSI index",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := jetBrainsClientProvider()
			if err != nil {
				fmt.Printf("[-] Failed to initialize JetBrains client: %v\n", err)
				return
			}
			bridge := jetbrains.NewBridge(client)
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			symbols, err := bridge.SearchSymbol(cmd.Context(), query)
			if err != nil {
				fmt.Printf("[-] Symbol search failed: %v\n", err)
				return
			}
			fmt.Printf("[+] Matching Symbols (%d):\n", len(symbols))
			for _, s := range symbols {
				fmt.Printf("    - %s (%s) -> %s:%d\n", s.Name, s.Kind, s.File, s.Line)
			}
		},
	}
}

func newJetBrainsBuildCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Trigger IDE background build",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := jetBrainsClientProvider()
			if err != nil {
				fmt.Printf("[-] Failed to initialize JetBrains client: %v\n", err)
				return
			}
			bridge := jetbrains.NewBridge(client)
			fmt.Println("[+] Triggering IDE project build...")
			res, err := bridge.BuildProject(cmd.Context())
			if err != nil {
				fmt.Printf("[-] Build failed: %v\n", err)
				return
			}
			if res.Success {
				fmt.Printf("[\u2713] Project build succeeded in %v\n", res.Duration)
			} else {
				fmt.Printf("[-] Build completed with errors in %v\n", res.Duration)
			}
		},
	}
}

func newJetBrainsRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run <config>",
		Short: "Execute an IDE run configuration",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client, err := jetBrainsClientProvider()
			if err != nil {
				fmt.Printf("[-] Failed to initialize JetBrains client: %v\n", err)
				return
			}
			bridge := jetbrains.NewBridge(client)
			configName := args[0]
			fmt.Printf("[+] Running IDE configuration '%s'...\n", configName)
			out, err := bridge.ExecuteRunConfiguration(cmd.Context(), configName)
			if err != nil {
				fmt.Printf("[-] Execution failed: %v\n", err)
				return
			}
			fmt.Printf("[\u2713] %s\n", out)
		},
	}
}

func runDoctor(ctx context.Context) {
	fmt.Println("[+] Running Dendrite Doctor diagnostics...")

	status, models, err := devtool.ProbeOllama(ctx, ollama.DefaultBaseURL)
	if err != nil {
		fmt.Printf("  [-] Local Ollama: UNREACHABLE (%v)\n", err)
	} else {
		fmt.Printf("  [\u2713] Local Ollama: REACHABLE (latency: %v, loaded models: %d)\n", status.Latency, len(models))
		for _, m := range models {
			fmt.Printf("      - %s (size: %.2f GB)\n", m.Name, float64(m.Size)/(1024*1024*1024))
		}
	}

	if _, err := os.Stat("DEV_STATUS.md"); err == nil {
		fmt.Println("  [\u2713] Project Status: DEV_STATUS.md present")
	} else {
		fmt.Println("  [-] Project Status: DEV_STATUS.md not found in current directory")
	}

	fmt.Println("[+] Pre-flight diagnostics complete.")
}

func getJetBrainsClient() (*jetbrains.Client, error) {
	testClient := &http.Client{Timeout: 1500 * time.Millisecond}
	transport := jetbrains.NewHTTPTransport(jetBrainsEndpoint, testClient)
	client, err := jetbrains.NewClient(transport)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	if _, err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to JetBrains IDE MCP at %s: %w", jetBrainsEndpoint, err)
	}
	return client, nil
}
