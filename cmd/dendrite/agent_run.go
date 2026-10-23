package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/agentsearch"
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/statedir"

	"github.com/spf13/cobra"
)

// newBoutonCmd builds `dendrite bouton` (alias agent): the coding agent harness and the agent job dispatcher.
func newBoutonCmd() *cobra.Command {
	cmd := newGroupCmd("bouton", "Autonomous coding agent harness and job dispatcher (Aider, OpenHands, OpenCode, AGY, Copilot)", "effectors",
		"agent", "effector-dispatch", "bouton-dispatch", "dispatch", "agt")
	cmd.Example = "dendrite bouton run aider --ticket DEN-030 --task \"fix the parser\"\n" +
		"dendrite bouton search \"refactor AST parser\" --local\n" +
		"dendrite bouton route --task \"add a CLI flag\" --exec\n" +
		"dendrite bouton jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate"
	cmd.AddCommand(newBoutonRunCmd(), newBoutonSearchCmd(), newBoutonRouteCmd(), newBoutonSessionsCmd(), newBoutonJobsCmd())
	return cmd
}

func sessionLedgerPath() string {
	return statedir.Resolve(filepath.Join(".dendrite", "agent_sessions.json"))
}

func loadSessionLedger() *agent.SessionLedger {
	path := sessionLedgerPath()
	ledger := agent.NewSessionLedger(nil, path)
	if data, err := os.ReadFile(path); err == nil {
		_ = ledger.LoadJSON(data)
	}
	return ledger
}

// defaultAgentModel picks the gateway model an agent type starts with.
func defaultAgentModel(t agent.AgentType) string {
	switch t {
	case agent.AgentAGY, agent.AgentAntigravity:
		m, _ := agentjobs.DefaultDispatcher.PickAGYModel(0)
		return m
	case agent.AgentCopilot, agent.AgentGitHubCopilot:
		return gateway.CopilotModel()
	}
	return "copilot-sonnet"
}

type boutonRunOptions struct {
	Ticket       string
	Model        string
	Task         string
	Sandbox      bool
	AllowInstall bool
}

func newBoutonRunCmd() *cobra.Command {
	var o boutonRunOptions
	cmd := &cobra.Command{
		Use:       "run <agent>",
		Short:     "Launch a coding agent (aider, openhands, opencode, agy, copilot) through the gateway",
		Args:      exactArgs(1),
		ValidArgs: []string{"aider", "openhands", "opencode", "agy", "copilot"},
		RunE: func(cmd *cobra.Command, args []string) error {
			t := agent.AgentType(strings.ToLower(args[0]))
			if !t.IsValid() {
				return usageErrorf("invalid agent %q: use aider, openhands, opencode, agy or copilot", args[0])
			}
			o.AllowInstall = o.AllowInstall || globals.Yes
			return runBoutonAgent(cmd.Context(), t, o, cmd.OutOrStdout())
		},
	}
	addTicketFlag(cmd, &o.Ticket, "")
	addModelFlag(cmd, &o.Model, "")
	cmd.Flags().StringVar(&o.Task, "task", "", "Task description for the agent")
	cmd.Flags().BoolVar(&o.Sandbox, "sandbox", false, "Run in an ephemeral cap-dropped container")
	cmd.Flags().BoolVar(&o.AllowInstall, "allow-install", false, "Install the agent CLI when missing (also --yes)")
	return cmd
}

func runBoutonAgent(ctx context.Context, t agent.AgentType, o boutonRunOptions, out io.Writer) error {
	cwd, _ := os.Getwd()
	cfg := agent.AgentConfig{
		Type:            t,
		LiteLLMBaseURL:  gatewayV1URL(),
		VirtualAPIKey:   "sk-dendrite-virtual-key",
		Model:           defaultAgentModel(t),
		WorkspaceDir:    cwd,
		TicketID:        o.Ticket,
		TaskDescription: o.Task,
		Sandbox:         o.Sandbox,
		Ephemeral:       o.Sandbox,
	}
	if o.Model != "" {
		cfg.Model = o.Model
	}
	if cfg.Type == agent.AgentOpenHands && !cfg.Sandbox {
		if _, err := exec.LookPath("openhands"); err != nil {
			fmt.Fprintln(out, "[+] OpenHands host binary not detected; auto-routing to hardened container sandbox...")
			cfg.Sandbox, cfg.Ephemeral = true, true
		}
	}

	env, err := agent.BuildAgentEnv(cfg)
	if err != nil {
		return fmt.Errorf("agent configuration: %w", err)
	}
	argv, err := agent.BuildAgentCommand(cfg)
	if err != nil {
		return fmt.Errorf("construct agent command: %w", err)
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
		if strings.Contains(k, "KEY") || strings.Contains(k, "TOKEN") {
			v = "********"
		}
		fmt.Fprintf(out, "    - %s=%s\n", k, v)
	}
	fmt.Fprintf(out, "[+] Command: %s\n", strings.Join(argv, " "))

	var runner agent.Runner
	if cfg.Sandbox {
		runner = agent.NewSandboxedRunner(nil, nil)
	} else {
		host := agent.NewHostRunner(true)
		host.AllowInstall = o.AllowInstall
		runner = host
	}
	summary, err := runner.Execute(ctx, cfg)
	if err != nil {
		return fmt.Errorf("agent execution: %w", err)
	}
	if err := loadSessionLedger().RecordSession(ctx, summary); err != nil {
		fmt.Fprintf(out, "[!] Session not recorded: %v\n", err)
	}

	fmt.Fprintf(out, "[✓] Agent session completed successfully (Session ID: %s, Duration: %v)\n", summary.SessionID, summary.Duration)
	if len(summary.CommitsGenerated) > 0 {
		fmt.Fprintln(out, "[+] Generated Commits:")
		for _, c := range summary.CommitsGenerated {
			fmt.Fprintf(out, "    - %s\n", c)
		}
	}
	return nil
}

func newBoutonSearchCmd() *cobra.Command {
	var pkg string
	var local bool
	var maxCost float64
	cmd := &cobra.Command{
		Use:   "search <query...>",
		Short: "Rank agents for a query through the dendrite-rag sidecar (BM25, embeddings, RRF)",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := agentsearch.NewQueryBuilder().
				WithText(strings.Join(args, " ")).
				WithTargetPackage(pkg).
				RequireLocalInference(local).
				WithMaxCost(maxCost).
				Build()
			results, err := agentsearch.NewSearchEngine(nil, nil, nil).Search(cmd.Context(), q)
			if err != nil {
				return fmt.Errorf("agent search: %w", err)
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, results)
			}
			fmt.Fprintf(out, "[+] Agent Search Results (%d matching candidates):\n\n", len(results))
			for i, res := range results {
				fmt.Fprintf(out, "  %d. [%s] %s\n", i+1, res.Manifest.ID, res.Manifest.Name)
				fmt.Fprintf(out, "     - Capability Score: %.2f (RRF: %.2f, Dense: %.2f, Sparse: %.2f)\n",
					res.FinalScore, res.RRFScore, res.DenseScore, res.SparseScore)
				fmt.Fprintf(out, "     - Preferred Tier:   %s (Engine: %s, Model: %s)\n",
					res.Manifest.PreferredTier, res.Manifest.PreferredEngine, res.Manifest.PreferredModel)
				fmt.Fprintf(out, "     - Zero-Cost:        %v\n", res.Manifest.ZeroCostCandidate)
				fmt.Fprintf(out, "     - Match Reasoning:  %s\n\n", res.Reasoning)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pkg, "pkg", "", "Target package")
	cmd.Flags().BoolVar(&local, "local", false, "Only agents that can run on local inference")
	cmd.Flags().Float64Var(&maxCost, "max-cost", 0, "Maximum estimated cost in USD (0: no limit)")
	return cmd
}

type boutonRouteOptions struct {
	Task    string
	Pkg     string
	Execute bool
	Local   bool
}

func newBoutonRouteCmd() *cobra.Command {
	var o boutonRouteOptions
	cmd := &cobra.Command{
		Use:   "route [task...]",
		Short: "Pick the best agent, tier and model for a task, and optionally run it",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				if o.Task != "" {
					return usageErrorf("give the task either as arguments or with --task, not both")
				}
				o.Task = strings.Join(args, " ")
			}
			if strings.TrimSpace(o.Task) == "" {
				return usageErrorf("give a task description")
			}
			return runBoutonRoute(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&o.Task, "task", "", "Task description")
	cmd.Flags().StringVar(&o.Pkg, "pkg", "", "Target package")
	cmd.Flags().BoolVar(&o.Execute, "exec", false, "Launch the selected agent")
	cmd.Flags().BoolVar(&o.Local, "local", false, "Require local inference (runs sandboxed with --exec)")
	return cmd
}

func runBoutonRoute(ctx context.Context, o boutonRouteOptions, out io.Writer) error {
	q := agentsearch.NewQueryBuilder().
		WithText(o.Task).
		WithTargetPackage(o.Pkg).
		RequireLocalInference(o.Local).
		Build()
	decision, err := agentsearch.NewSearchEngine(nil, nil, nil).Route(ctx, q)
	if err != nil {
		return fmt.Errorf("routing: %w", err)
	}
	if globals.JSON && !o.Execute {
		return writeJSON(out, decision)
	}
	fmt.Fprintln(out, "[+] Optimal Agent Routing Decision:")
	fmt.Fprintf(out, "    - Selected Agent:   %s (%s)\n", decision.SelectedAgent.ID, decision.SelectedAgent.Name)
	fmt.Fprintf(out, "    - Capability Score:  %.2f\n", decision.Score)
	fmt.Fprintf(out, "    - Recommended Tier:  %s\n", decision.Tier)
	fmt.Fprintf(out, "    - Target Engine:    %s\n", decision.EngineID)
	fmt.Fprintf(out, "    - Target Model:     %s\n", decision.Model)
	fmt.Fprintf(out, "    - Virtual Key:      %s\n", decision.VirtualKeyAlias)
	fmt.Fprintf(out, "    - Estimated Cost:   $%.2f\n", decision.EstimatedCost)
	fmt.Fprintf(out, "    - Routing Rationale: %s\n", decision.Reasoning)
	if !o.Execute {
		return nil
	}

	fmt.Fprintf(out, "\n[+] Auto-launching agent harness (%s)...\n", decision.SelectedAgent.ID)
	cwd, _ := os.Getwd()
	cfg := agent.AgentConfig{
		Type:            decision.SelectedAgent.Type,
		LiteLLMBaseURL:  gatewayV1URL(),
		VirtualAPIKey:   decision.VirtualKeyAlias,
		Model:           decision.Model,
		WorkspaceDir:    cwd,
		TaskDescription: o.Task,
	}
	var runner agent.Runner = agent.NewHostRunner(true)
	if o.Local {
		runner = agent.NewSandboxedRunner(nil, nil)
	}
	summary, err := runner.Execute(ctx, cfg)
	if err != nil {
		return fmt.Errorf("agent execution: %w", err)
	}
	fmt.Fprintf(out, "[✓] Executed successfully (Session ID: %s, Duration: %v)\n", summary.SessionID, summary.Duration)
	return nil
}

func newBoutonSessionsCmd() *cobra.Command {
	var ticket string
	cmd := &cobra.Command{
		Use:     "sessions",
		Aliases: []string{"session"},
		Short:   "List recorded agent sessions (duration, cost, status)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions := loadSessionLedger().ListSessionsByTicket(ticket)
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, sessions)
			}
			fmt.Fprintf(out, "[+] Agent Sessions Ledger (%d recorded sessions):\n", len(sessions))
			if len(sessions) == 0 {
				fmt.Fprintln(out, "    - None recorded")
			}
			for i, s := range sessions {
				fmt.Fprintf(out, "    %d. [%s] Agent: %s, Ticket: %s, Status: %s, Duration: %v, Cost: $%.4f\n",
					i+1, s.SessionID, s.AgentType, s.TicketID, s.Status, s.Duration, s.EstimatedCostUSD)
			}
			return nil
		},
	}
	addTicketFlag(cmd, &ticket, "")
	return cmd
}
