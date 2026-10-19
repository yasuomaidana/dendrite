package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/statedir"
)

type boutonRunOptions struct {
	Ticket  string
	Model   string
	Task    string
	Sandbox bool
}

func newBoutonRunCmd() *cobra.Command {
	var o boutonRunOptions
	cmd := &cobra.Command{
		Use:     "run <agent-type>",
		Short:   "Launch an autonomous coding agent (aider, openhands, opencode, agy, copilot)",
		Args:    exactArgs(1),
		Example: "dendrite bouton run aider --ticket DEN-030 --task \"fix parser\"\ndendrite bouton run openhands --sandbox",
		RunE: func(c *cobra.Command, args []string) error {
			return runBoutonRun(c.Context(), args[0], o, c.OutOrStdout())
		},
	}
	f := cmd.Flags()
	addTicketFlag(cmd, &o.Ticket, "")
	addModelFlag(cmd, &o.Model, "")
	f.StringVar(&o.Task, "task", "", "Task description")
	f.StringVar(&o.Task, "prompt", "", "Alias for --task")
	f.BoolVar(&o.Sandbox, "sandbox", false, "Execute inside an ephemeral container sandbox")
	return cmd
}

func runBoutonRun(ctx context.Context, rawType string, o boutonRunOptions, out io.Writer) error {
	agentType := agent.AgentType(strings.ToLower(rawType))
	if !agentType.IsValid() {
		return usageErrorf("invalid agent type: %s (must be aider, openhands, opencode, agy, or copilot)", rawType)
	}

	cwd, _ := os.Getwd()
	defaultModel := "copilot-sonnet"
	if agentType == agent.AgentAGY || agentType == agent.AgentAntigravity {
		defaultModel = "gemini-3.8-flash-high"
	} else if agentType == agent.AgentCopilot || agentType == agent.AgentGitHubCopilot {
		defaultModel = gateway.CopilotModel()
	}

	model := o.Model
	if model == "" {
		model = defaultModel
	}

	cfg := agent.AgentConfig{
		Type:            agentType,
		LiteLLMBaseURL:  gatewayV1URL(),
		VirtualAPIKey:   "sk-dendrite-virtual-key",
		Model:           model,
		TicketID:        o.Ticket,
		WorkspaceDir:    cwd,
		TaskDescription: o.Task,
		Sandbox:         o.Sandbox,
		Ephemeral:       o.Sandbox,
	}

	// Auto-detect and sandbox fallback for OpenHands
	if cfg.Type == agent.AgentOpenHands && !cfg.Sandbox {
		if _, oErr := exec.LookPath("openhands"); oErr != nil {
			fmt.Fprintln(out, "[+] OpenHands host binary not detected; auto-routing to hardened container sandbox...")
			cfg.Sandbox = true
			cfg.Ephemeral = true
		}
	}

	env, err := agent.BuildAgentEnv(cfg)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	cmdArgs, err := agent.BuildAgentCommand(cfg)
	if err != nil {
		return fmt.Errorf("failed to construct command: %w", err)
	}

	fmt.Fprintf(out, "[+] Initializing Agent Harness for: %s\n", cfg.Type)
	fmt.Fprintf(out, "    - Workspace:    %s\n", cfg.WorkspaceDir)
	fmt.Fprintf(out, "    - Gateway Base: %s\n", cfg.LiteLLMBaseURL)
	fmt.Fprintf(out, "    - Primary Model:%s\n", cfg.Model)
	if cfg.TicketID != "" {
		fmt.Fprintf(out, "    - Active Ticket:%s\n", cfg.TicketID)
	}
	if cfg.Sandbox {
		fmt.Fprintln(out, "    - Execution:    Ephemeral Cap-Dropped Container Sandbox (RAII)")
	} else {
		fmt.Fprintln(out, "    - Execution:    Host Local Terminal")
	}

	fmt.Fprintln(out, "[+] Injected Environment Variables:")
	for k, v := range env {
		displayVal := v
		if strings.Contains(k, "KEY") || strings.Contains(k, "TOKEN") {
			displayVal = "********"
		}
		fmt.Fprintf(out, "    - %s=%s\n", k, displayVal)
	}
	fmt.Fprintf(out, "[+] Command: %s\n", strings.Join(cmdArgs, " "))

	var runner agent.Runner
	if cfg.Sandbox {
		runner = agent.NewSandboxedRunner(nil, nil)
	} else {
		hostR := agent.NewHostRunner(true)
		hostR.AllowInstall = globals.Yes
		hostR.Stdout = out
		hostR.Stderr = out
		runner = hostR
	}
	summary, err := runner.Execute(ctx, cfg)
	if err != nil {
		return fmt.Errorf("agent execution failed: %w", err)
	}

	// Record session to persistent ledger
	_ = os.MkdirAll(".dendrite", 0755)
	ledgerPath := statedir.Resolve(filepath.Join(".dendrite", "agent_sessions.json"))
	ledger := agent.NewSessionLedger(nil, ledgerPath)
	if data, err := os.ReadFile(ledgerPath); err == nil {
		_ = ledger.LoadJSON(data)
	}
	_ = ledger.RecordSession(ctx, summary)

	fmt.Fprintf(out, "[✓] Agent session completed successfully (Session ID: %s, Duration: %v)\n", summary.SessionID, summary.Duration)
	if len(summary.CommitsGenerated) > 0 {
		fmt.Fprintln(out, "[+] Generated Commits:")
		for _, c := range summary.CommitsGenerated {
			fmt.Fprintf(out, "    - %s\n", c)
		}
	}
	return nil
}

func newBoutonSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "sessions",
		Aliases: []string{"session"},
		Short:   "View agent execution history, token spend, and commits",
		Args:    noArgs,
		Example: "dendrite bouton sessions",
		RunE: func(c *cobra.Command, args []string) error {
			return runBoutonSessions(c.Context(), c.OutOrStdout())
		},
	}
}

func runBoutonSessions(ctx context.Context, out io.Writer) error {
	ledgerPath := statedir.Resolve(filepath.Join(".dendrite", "agent_sessions.json"))
	ledger := agent.NewSessionLedger(nil, ledgerPath)
	if data, err := os.ReadFile(ledgerPath); err == nil {
		_ = ledger.LoadJSON(data)
	}
	sessions := ledger.ListSessionsByTicket("")
	fmt.Fprintf(out, "[+] Agent Sessions Ledger (%d recorded sessions):\n", len(sessions))
	if len(sessions) == 0 {
		fmt.Fprintln(out, "    - Active: None")
		fmt.Fprintln(out, "    - Last Session: None")
	} else {
		for idx, s := range sessions {
			fmt.Fprintf(out, "    %d. [%s] Agent: %s, Ticket: %s, Status: %s, Duration: %v, Cost: $%.4f\n",
				idx+1, s.SessionID, s.AgentType, s.TicketID, s.Status, s.Duration, s.EstimatedCostUSD)
		}
	}
	return nil
}

func handleAgentCommand(ctx context.Context, action string, args []string) {
	_ = invokeCmd(ctx, newBoutonCmd(), append([]string{action}, args...)...)
}

