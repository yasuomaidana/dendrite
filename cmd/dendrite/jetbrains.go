package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"Dendrite/pkg/jetbrains"
)

func getJetBrainsClient() (*jetbrains.Client, error) {
	testClient := &http.Client{Timeout: 1500 * time.Millisecond}
	transport := jetbrains.NewHTTPTransport("http://localhost:63342/api/mcp", testClient)
	client, err := jetbrains.NewClient(transport)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	connErr := error(nil)
	if _, connErr = client.Connect(ctx); connErr == nil {
		return client, nil
	}

	return nil, fmt.Errorf("failed to connect to live JetBrains IDE MCP server at http://localhost:63342/api/mcp: %w (Zero-Mock Policy: live IDE connection required)", connErr)
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
