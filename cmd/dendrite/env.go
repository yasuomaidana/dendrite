package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/env"
	"Dendrite/pkg/env/bootstrap"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/sidecar"

	"Dendrite/pkg/tracker/plane"
	"github.com/spf13/cobra"
	"time"
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
		newSomaMigrateStateCmd(),
		newSomaSecretsCmd(),
		newSomaStacksCmd(),
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
	AdoptCI  bool
	DryRun   bool
}

func newSomaPrepareCmd() *cobra.Command {
	var o somaPrepareOptions
	cmd := &cobra.Command{
		Use:     "prepare [path|project]",
		Aliases: []string{"sync", "update", "configure"},
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
	cmd.Flags().BoolVar(&o.AdoptCI, "adopt-ci", false, "Replace a hand-written CI workflow with the generated one (the old file is archived in .github/dendrite/)")
	addDryRunFlag(cmd, &o.DryRun)
	return cmd
}

// agyRegister and agyMCPStatus are the seams to Antigravity's own MCP registry; the tests replace them
// so no test changes the developer's real AGY configuration.
var (
	agyRegister  = bootstrap.RegisterAGY
	agyMCPStatus = bootstrap.AGYMCPStatus
)

func runSomaPrepare(ctx context.Context, o somaPrepareOptions, out io.Writer) error {
	target := resolvePrepareTarget(o.Path)
	report, err := bootstrap.NewPreparer().Prepare(ctx, bootstrap.PrepareOptions{
		ProjectPath: target,
		DefaultTool: o.Tool,
		EnforceCICD: !o.NoCICD,
		SyncSkills:  !o.NoSkills,
		DryRun:      o.DryRun,
		AdoptCI:     o.AdoptCI,
	})
	if err != nil {
		return fmt.Errorf("prepare project environment: %w", err)
	}
	var agyLines []string
	if !o.DryRun {
		agyLines = registerWithAGY(target)
	}
	if globals.JSON {
		return writeJSON(out, report)
	}
	printPrepareReport(out, report)
	for _, line := range agyLines {
		fmt.Fprintln(out, line)
	}
	return nil
}

// registerWithAGY adds the project's MCP servers to AGY's registry (AGY does not read .agents/mcp.json)
// and returns the lines to show; it is silent when AGY is not installed.
func registerWithAGY(target string) []string {
	registered, note := agyRegister(target)
	var lines []string
	if len(registered) > 0 {
		lines = append(lines, "[✓] AGY: registered MCP servers "+strings.Join(registered, ", "))
	}
	if note != "" {
		lines = append(lines, "[!] AGY MCP registration: "+note)
	}
	return lines
}

// resolvePrepareTarget maps a registered project name to its directory; any other value is a path.
func resolvePrepareTarget(target string) string {
	reg, err := newProjectRegistry()
	if err != nil {
		return target
	}
	if p, err := reg.GetProject(target); err == nil && p != nil {
		return p.Path
	}
	return target
}

func printPrepareReport(out io.Writer, report *bootstrap.PrepareReport) {
	fmt.Fprintln(out, "[+] Project Environment Preparation & CI/CD Enforcement:")
	fmt.Fprintf(out, "    - Project Path:       %s\n", report.ProjectPath)
	fmt.Fprintf(out, "    - Primary Stack:      %s\n", strings.ToUpper(string(report.Stack.PrimaryStack)))
	if len(report.Stack.SecondaryStacks) > 0 {
		fmt.Fprintf(out, "    - Secondary Stacks:   %s\n", upperJoin(report.Stack.SecondaryStacks))
	}
	printPrepareCICD(out, report.CICD)
	printPrepareFiles(out, report)
	fmt.Fprintf(out, "    - Execution Duration: %v\n", report.Duration)
}

func upperJoin(stacks []bootstrap.LanguageStack) string {
	names := make([]string, 0, len(stacks))
	for _, s := range stacks {
		names = append(names, strings.ToUpper(string(s)))
	}
	return strings.Join(names, ", ")
}

func printPrepareCICD(out io.Writer, c bootstrap.CICDStatus) {
	fmt.Fprintf(out, "    - Default Tool:       %s (Installed: %v)\n", c.DefaultTool, c.ToolInstalled)
	if !c.ToolInstalled && c.ToolInstallCommand != "" {
		fmt.Fprintf(out, "      * Tooling Guide:    %s\n", c.ToolInstallCommand)
	}
	fmt.Fprintf(out, "    - Coverage Gate:      >= %.1f%%\n", c.CoverageThreshold)
	fmt.Fprintf(out, "    - Copilot Rules:      %s\n", c.CopilotInstructions)
	fmt.Fprintf(out, "    - CI/CD Workflow:     %s\n", c.CIWorkflowPath)
}

func printPrepareFiles(out io.Writer, report *bootstrap.PrepareReport) {
	printCounted(out, "Files Created", report.FilesCreated)
	printCounted(out, "Files Updated", report.FilesUpdated)
	for _, note := range report.Notes {
		fmt.Fprintf(out, "    - [i] %s\n", note)
	}
	printCounted(out, "Skills Synced", report.SkillsSynced)
}

func printCounted(out io.Writer, label string, items []string) {
	if len(items) > 0 {
		fmt.Fprintf(out, "    - %s (%d): %s\n", label, len(items), strings.Join(items, ", "))
	}
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
	if !req.DryRun {
		adoptCheckoutStacks(out)
	}
	if !req.DryRun { // compose refuses to start without these keys
		res, err := env.EnsureSecrets(env.SecretsOptions{Dir: dotenv.ProjectRoot("."), GlobalPath: globalEnvPath()})
		if err != nil {
			return fmt.Errorf("prepare .env: %w", err)
		}
		if res.Created || len(res.Added) > 0 {
			_ = reportSecrets(out, res)
		}
	}
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
	diagnosePlane(ctx, out)
	diagnoseSidecars(out)
	diagnoseAGY(out)
	if _, err := os.Stat("DEV_STATUS.md"); err == nil {
		fmt.Fprintln(out, "  [✓] Project Status: DEV_STATUS.md present")
	} else {
		fmt.Fprintln(out, "  [-] Project Status: DEV_STATUS.md not found in current directory")
	}
	fmt.Fprintln(out, "[+] Pre-flight diagnostics complete.")
	return nil
}

// diagnoseSidecars reports where each Rust sidecar Dendrite requires was found, or how to get it.
func diagnoseSidecars(out io.Writer) {
	loc := sidecar.DefaultLocator()
	// dendrite-git is installed with the others but no Go code calls it yet (DGIT-1, DGIT-4, DGIT-7).
	for _, name := range []string{coreclient.BinaryName, planesvc.BinaryName, "dendrite-rag", "dendrite-git"} {
		note := ""
		if name == "dendrite-git" {
			note = " (not called by Dendrite yet)"
		}
		if path, err := loc.Locate(name); err == nil {
			fmt.Fprintf(out, "  [✓] Sidecar %s: %s%s\n", name, path, note)
		} else {
			fmt.Fprintf(out, "  [-] Sidecar %s: not found (%v); run `dendrite install`\n", name, err)
		}
	}
}

// diagnoseAGY reports whether Antigravity can reach the Dendrite MCP server. AGY keeps its own registry,
// so a project's .agents/mcp.json alone does not give it the dendrite tools.
func diagnoseAGY(out io.Writer) {
	state, err := agyMCPStatus("dendrite")
	switch state {
	case bootstrap.AGYAbsent:
		fmt.Fprintln(out, "  [i] AGY: not installed, nothing to register")
	case bootstrap.AGYRegistered:
		fmt.Fprintln(out, "  [✓] AGY: the dendrite MCP server is registered")
	case bootstrap.AGYNotRegistered:
		fmt.Fprintln(out, "  [-] AGY: the dendrite MCP server is not registered; run `dendrite soma prepare`")
	default:
		fmt.Fprintf(out, "  [-] AGY: cannot read its MCP registry (%v)\n", err)
	}
}

// diagnosePlane reports whether the Plane token is configured and the service answers with it.
func diagnosePlane(ctx context.Context, out io.Writer) {
	cfg, err := plane.ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(out, "  [-] Plane: %v\n", err)
		return
	}
	client, err := plane.NewClient(cfg, nil)
	if err != nil {
		fmt.Fprintf(out, "  [-] Plane: %v\n", err)
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if _, err := client.ListStates(pctx); err != nil {
		fmt.Fprintf(out, "  [-] Plane: token configured but %s did not answer (%v)\n", cfg.BaseURL, err)
		return
	}
	fmt.Fprintf(out, "  [✓] Plane: token configured, %s reachable (workspace %s)\n", cfg.BaseURL, cfg.WorkspaceSlug)
}
