package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/env"
	"Dendrite/pkg/env/bootstrap"
	"Dendrite/pkg/ollama"

	"github.com/spf13/cobra"
)

// newSomaCmd builds `dendrite soma` (alias env): environment preparation, replication and diagnostics.
func newSomaCmd() *cobra.Command {
	cmd := newGroupCmd("soma", "Manage and replicate development environments", "soma",
		"env", "homeostasis", "soma-homeostasis")
	cmd.Example = "dendrite soma prepare --dry-run\n" +
		"dendrite soma plan --mode host --with-plane\n" +
		"dendrite soma install litellm\n" +
		"dendrite soma status\n" +
		"dendrite soma diagnose"
	cmd.AddCommand(
		newSomaPrepareCmd(),
		newSomaPlanCmd(),
		newSomaReplicateCmd(),
		newSomaInstallCmd(),
		newSomaStatusCmd(),
		newSomaDiagnoseCmd(),
	)
	return cmd
}

// envTools detects the host and builds the planner and replicator for it.
func envTools(ctx context.Context) (*env.Planner, *env.Replicator, error) {
	host, err := env.NewDetector(nil).Detect(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("host detection failed: %w", err)
	}
	return env.NewPlanner(*host), env.NewReplicator(nil, *host), nil
}

// addModeFlag registers --mode/-m; parseMode validates the value inside RunE.
func addModeFlag(cmd *cobra.Command, p *string) {
	cmd.Flags().StringVarP(p, "mode", "m", string(env.ModeContainer), "Install mode: container, host or hybrid")
}

func parseMode(s string) (env.InstallMode, error) {
	m := env.InstallMode(s)
	if !m.IsValid() {
		return "", usageErrorf("invalid --mode %q: use container, host or hybrid", s)
	}
	return m, nil
}

func addPlaneUIFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("with-plane-ui", false, "Include the Plane web UI")
	cmd.Flags().Bool("no-plane-ui", false, "Skip the Plane web UI")
	cmd.MarkFlagsMutuallyExclusive("with-plane-ui", "no-plane-ui")
}

// planeUIFlag resolves the --with-plane-ui / --no-plane-ui pair; nil means "use the default".
func planeUIFlag(cmd *cobra.Command) *bool {
	f := cmd.Flags()
	switch {
	case f.Changed("no-plane-ui"):
		v := false
		return &v
	case f.Changed("with-plane-ui"):
		v := true
		return &v
	}
	return nil
}

type somaPrepareOptions struct {
	Path     string
	Tool     string
	NoCICD   bool
	NoSkills bool
	DryRun   bool
}

func newSomaPrepareCmd() *cobra.Command {
	var o somaPrepareOptions
	cmd := &cobra.Command{
		Use:     "prepare [path|project]",
		Aliases: []string{"sync", "update"},
		Short:   "Sync agent rules and skills, and enforce CI/CD tooling defaults",
		Args:    maxArgs(1),
		Example: "dendrite soma prepare\ndendrite soma prepare ../other-repo --dry-run\ndendrite soma prepare --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if cmd.Flags().Changed("path") {
					return usageErrorf("give the target either as an argument or with --path, not both")
				}
				o.Path = args[0]
			}
			if cmd.CalledAs() == "update" {
				fmt.Fprintln(cmd.ErrOrStderr(), "[i] 'update' here is an alias of 'prepare' (re-scaffolds this project); to update the dendrite binary and its sibling repos run 'dendrite update'")
			}
			return runSomaPrepare(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&o.Path, "path", ".", "Target project directory or registered project name")
	cmd.Flags().StringVar(&o.Tool, "tool", "copilot", "Default developer and CI/CD assistant tool")
	cmd.Flags().BoolVar(&o.NoCICD, "no-cicd", false, "Disable CI/CD and Copilot enforcement")
	cmd.Flags().BoolVar(&o.NoSkills, "no-skills", false, "Disable agent rules and skills synchronization")
	addDryRunFlag(cmd, &o.DryRun)
	return cmd
}

func runSomaPrepare(ctx context.Context, o somaPrepareOptions, out io.Writer) error {
	pathVal := o.Path
	if reg, err := newProjectRegistry(); err == nil {
		if p, err := reg.GetProject(pathVal); err == nil && p != nil {
			pathVal = p.Path
		}
	}

	report, err := bootstrap.NewPreparer().Prepare(ctx, bootstrap.PrepareOptions{
		ProjectPath: pathVal,
		DefaultTool: o.Tool,
		EnforceCICD: !o.NoCICD,
		SyncSkills:  !o.NoSkills,
		DryRun:      o.DryRun,
	})
	if err != nil {
		return fmt.Errorf("prepare project environment: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, report)
	}

	fmt.Fprintln(out, "[+] Project Environment Preparation & CI/CD Enforcement:")
	fmt.Fprintf(out, "    - Project Path:       %s\n", report.ProjectPath)
	fmt.Fprintf(out, "    - Primary Stack:      %s\n", strings.ToUpper(string(report.Stack.PrimaryStack)))
	if len(report.Stack.SecondaryStacks) > 0 {
		var sec []string
		for _, s := range report.Stack.SecondaryStacks {
			sec = append(sec, strings.ToUpper(string(s)))
		}
		fmt.Fprintf(out, "    - Secondary Stacks:   %s\n", strings.Join(sec, ", "))
	}
	fmt.Fprintf(out, "    - Default Tool:       %s (Installed: %v)\n", report.CICD.DefaultTool, report.CICD.ToolInstalled)
	if !report.CICD.ToolInstalled && report.CICD.ToolInstallCommand != "" {
		fmt.Fprintf(out, "      * Tooling Guide:    %s\n", report.CICD.ToolInstallCommand)
	}
	fmt.Fprintf(out, "    - Coverage Gate:      >= %.1f%%\n", report.CICD.CoverageThreshold)
	fmt.Fprintf(out, "    - Copilot Rules:      %s\n", report.CICD.CopilotInstructions)
	fmt.Fprintf(out, "    - CI/CD Workflow:     %s\n", report.CICD.CIWorkflowPath)
	if len(report.FilesCreated) > 0 {
		fmt.Fprintf(out, "    - Files Created (%d): %s\n", len(report.FilesCreated), strings.Join(report.FilesCreated, ", "))
	}
	if len(report.FilesUpdated) > 0 {
		fmt.Fprintf(out, "    - Files Updated (%d): %s\n", len(report.FilesUpdated), strings.Join(report.FilesUpdated, ", "))
	}
	if len(report.SkillsSynced) > 0 {
		fmt.Fprintf(out, "    - Skills Synced (%d): %s\n", len(report.SkillsSynced), strings.Join(report.SkillsSynced, ", "))
	}
	fmt.Fprintf(out, "    - Execution Duration: %v\n", report.Duration)
	return nil
}

func newSomaPlanCmd() *cobra.Command {
	var mode string
	var req env.ReplicationRequest
	cmd := &cobra.Command{
		Use:     "plan",
		Short:   "Show the replication plan for this host",
		Args:    noArgs,
		Example: "dendrite soma plan\ndendrite soma plan --mode host --with-plane --with-grafana",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := parseMode(mode)
			if err != nil {
				return err
			}
			req.Mode = m
			req.WithPlaneUI = planeUIFlag(cmd)
			return runSomaPlan(cmd.Context(), req, cmd.OutOrStdout())
		},
	}
	addModeFlag(cmd, &mode)
	cmd.Flags().BoolVar(&req.WithLinear, "with-linear", false, "Include Linear")
	cmd.Flags().BoolVar(&req.WithPlane, "with-plane", false, "Include Plane")
	cmd.Flags().BoolVar(&req.WithPenpot, "with-penpot", false, "Include Penpot")
	cmd.Flags().BoolVar(&req.WithGrafana, "with-grafana", false, "Include Grafana")
	addPlaneUIFlags(cmd)
	return cmd
}

func runSomaPlan(ctx context.Context, req env.ReplicationRequest, out io.Writer) error {
	planner, _, err := envTools(ctx)
	if err != nil {
		return err
	}
	plan, err := planner.PlanReplication(req)
	if err != nil {
		return fmt.Errorf("plan replication: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, plan)
	}
	fmt.Fprintf(out, "[+] Replication Plan (Mode: %s, Est. Time: ~%ds):\n", plan.Mode, plan.EstimatedSecs)
	for i, a := range plan.Actions {
		mandatory := ""
		if a.Mandatory {
			mandatory = "[mandatory]"
		}
		fmt.Fprintf(out, "    %d. [%s] %s %s\n", i+1, a.Type, a.Description, mandatory)
		fmt.Fprintf(out, "       Command: %s\n", a.Command)
	}
	return nil
}

func newSomaReplicateCmd() *cobra.Command {
	var mode string
	var noLinear bool
	var req env.ReplicationRequest
	cmd := &cobra.Command{
		Use:     "replicate",
		Short:   "Execute the replication plan (compose files, services, status)",
		Args:    noArgs,
		Example: "dendrite soma replicate --dry-run\ndendrite soma replicate --with-plane --no-plane-ui",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := parseMode(mode)
			if err != nil {
				return err
			}
			req.Mode = m
			req.WithLinear = !noLinear
			req.WithPlaneUI = planeUIFlag(cmd)
			return runSomaReplicate(cmd.Context(), req, cmd.OutOrStdout())
		},
	}
	addModeFlag(cmd, &mode)
	cmd.Flags().BoolVar(&noLinear, "no-linear", false, "Skip Linear")
	cmd.Flags().BoolVar(&req.WithPlane, "with-plane", false, "Include Plane")
	addPlaneUIFlags(cmd)
	addDryRunFlag(cmd, &req.DryRun)
	return cmd
}

func runSomaReplicate(ctx context.Context, req env.ReplicationRequest, out io.Writer) error {
	planner, replicator, err := envTools(ctx)
	if err != nil {
		return err
	}
	plan, err := planner.PlanReplication(req)
	if err != nil {
		return fmt.Errorf("plan replication: %w", err)
	}
	fmt.Fprintf(out, "[+] Executing Environment Replication (Mode: %s, DryRun: %v)...\n", req.Mode, req.DryRun)
	res, execErr := replicator.ExecutePlan(ctx, plan, req.DryRun)
	if res == nil {
		return fmt.Errorf("replication: %w", execErr)
	}
	fmt.Fprintf(out, "[✓] Completed in %v (Success: %v)\n", res.TotalDuration, res.Success)
	if len(res.GeneratedFiles) > 0 {
		fmt.Fprintln(out, "[+] Generated Compose Files:")
		for _, f := range res.GeneratedFiles {
			fmt.Fprintf(out, "    - %s\n", f)
		}
	}
	fmt.Fprintln(out, "[+] Component Status:")
	for _, c := range res.Components {
		re := "UNREACHABLE"
		if c.Reachable {
			re = "REACHABLE"
		}
		fmt.Fprintf(out, "    - %-12s: %s (Endpoint: %s)\n", c.Target, re, c.Endpoint)
	}
	if execErr != nil {
		return fmt.Errorf("replication: %w", execErr)
	}
	return nil
}

func newSomaInstallCmd() *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "install <target>",
		Short: "Install an agent (aider, openhands, copilot, agy) or a service (plane, litellm, ollama, ...)",
		Args:  exactArgs(1),
		Example: "dendrite soma install agy --yes\n" +
			"dendrite soma install litellm --mode host",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := parseMode(mode)
			if err != nil {
				return err
			}
			return runSomaInstall(cmd.Context(), args[0], m, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	addModeFlag(cmd, &mode)
	return cmd
}

func runSomaInstall(ctx context.Context, name string, mode env.InstallMode, in io.Reader, out io.Writer) error {
	if aType := agent.AgentType(strings.ToLower(name)); aType.IsValid() {
		if err := agent.PromptAndInstallAgent(ctx, aType, globals.Yes, in, out); err != nil {
			return fmt.Errorf("install %s: %w", aType, err)
		}
		return nil
	}
	planner, replicator, err := envTools(ctx)
	if err != nil {
		return err
	}
	target := env.ReplicationTarget(name)
	plan, err := planner.PlanReplication(env.ReplicationRequest{
		Mode:       mode,
		Targets:    []env.ReplicationTarget{target},
		WithLinear: target == env.TargetLinear,
		WithPlane:  target == env.TargetPlane,
	})
	if err != nil {
		return fmt.Errorf("plan installation of %s: %w", target, err)
	}
	res, err := replicator.ExecutePlan(ctx, plan, false)
	if err != nil {
		return fmt.Errorf("install %s: %w", target, err)
	}
	fmt.Fprintf(out, "[✓] Successfully installed %s (Mode: %s) in %v\n", target, mode, res.TotalDuration)
	return nil
}

func newSomaStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show real-time status of environment components",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSomaStatus(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

func runSomaStatus(ctx context.Context, out io.Writer) error {
	_, replicator, err := envTools(ctx)
	if err != nil {
		return err
	}
	statuses := replicator.AuditComponents(ctx)
	if globals.JSON {
		return writeJSON(out, statuses)
	}
	fmt.Fprintln(out, "[+] Real-Time Environment Component Status:")
	for _, s := range statuses {
		reach := "OFFLINE / UNREACHABLE"
		if s.Reachable {
			reach = "ONLINE / HEALTHY"
		}
		fmt.Fprintf(out, "    [%s] %-12s - %s (Endpoint: %s)\n", reach, s.Target, s.Details, s.Endpoint)
	}
	return nil
}

func newSomaDiagnoseCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "diagnose",
		Aliases: []string{"doctor", "diag"},
		Short:   "Run environment pre-flight diagnostics (local Ollama, project status)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSomaDiagnose(cmd.Context(), ollama.DefaultBaseURL, cmd.OutOrStdout())
		},
	}
}

func runSomaDiagnose(ctx context.Context, ollamaURL string, out io.Writer) error {
	fmt.Fprintln(out, "[+] Running Dendrite pre-flight diagnostics...")
	status, models, err := devtool.ProbeOllama(ctx, ollamaURL)
	if err != nil {
		fmt.Fprintf(out, "  [-] Local Ollama: UNREACHABLE (%v)\n", err)
	} else {
		fmt.Fprintf(out, "  [✓] Local Ollama: REACHABLE (latency: %v, loaded models: %d)\n", status.Latency, len(models))
		for _, m := range models {
			fmt.Fprintf(out, "      - %s (size: %.2f GB)\n", m.Name, float64(m.Size)/(1024*1024*1024))
		}
	}
	if _, err := os.Stat("DEV_STATUS.md"); err == nil {
		fmt.Fprintln(out, "  [✓] Project Status: DEV_STATUS.md present")
	} else {
		fmt.Fprintln(out, "  [-] Project Status: DEV_STATUS.md not found in current directory")
	}
	fmt.Fprintln(out, "[+] Pre-flight diagnostics complete.")
	return nil
}
