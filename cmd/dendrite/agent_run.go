package main

import (
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/statedir"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"Dendrite/pkg/agent"
	"path/filepath"
)

func handleAgentCommand(ctx context.Context, action string, args []string) {
	switch action {
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
			defaultModel = gateway.CopilotModel()
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
		ledgerPath := statedir.Resolve(filepath.Join(".dendrite", "agent_sessions.json"))
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
		ledgerPath := statedir.Resolve(filepath.Join(".dendrite", "agent_sessions.json"))
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
