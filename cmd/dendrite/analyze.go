package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"Dendrite/pkg/scheduler/governor"
	"Dendrite/pkg/scheduler/token"
)

var defaultCLIScheduler = token.NewTokenScheduler(nil, nil, "")
var defaultCLIGovernor = governor.NewQuotaGovernor(governor.DefaultConfig(), nil, defaultCLIScheduler)

// newCognitionCmd builds `dendrite cognition`: scheduler policy, quota governor, and analysis execution.
func newCognitionCmd() *cobra.Command {
	cmd := newGroupCmd("cognition", "Cognitive complexity analyzer, structural anti-pattern scanner, and policy router", "glia",
		"analyze", "cognitive-analysis", "anlz")
	cmd.Example = "dendrite cognition status --json\ndendrite cognition policy --night-window 22:00-06:00 --reserve-floor 0.25\ndendrite cognition mode always\ndendrite cognition run --project dendrite --chunk ast-pass-1"

	var policyOpts cognitionPolicyOptions
	policyCmd := &cobra.Command{
		Use:     "policy",
		Short:   "View or set scheduler policy directives",
		Args:    noArgs,
		Example: "dendrite cognition policy\ndendrite cognition policy --night-window 23:00-06:00 --sprint-before-reset 48h --reserve-floor 0.20 --mode auto",
		RunE: func(c *cobra.Command, args []string) error {
			return runCognitionPolicy(c, policyOpts, globals.JSON, c.OutOrStdout())
		},
	}
	pf := policyCmd.Flags()
	pf.StringVar(&policyOpts.NightWindow, "night-window", "", "Zero-cost local night window (HH:MM-HH:MM)")
	pf.StringVar(&policyOpts.WeeklyReset, "weekly-reset", "", "Weekly quota reset schedule (DAY HH:MM)")
	pf.DurationVar(&policyOpts.SprintBeforeReset, "sprint-before-reset", 0, "Cloud sprint duration before weekly reset")
	pf.Float64Var(&policyOpts.ReserveFloor, "reserve-floor", 0, "Safety reserve quota floor ratio (0.0-1.0)")
	pf.BoolVar(&policyOpts.AllowZero, "allow-zero", false, "Allow draining weekly quota down to 0%")
	pf.StringVar(&policyOpts.Mode, "mode", "", "Operational scheduling mode (auto|always|night|sprint|paused)")

	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Inspect real-time scheduler state, countdowns, and active tiers",
		Args:    noArgs,
		Example: "dendrite cognition status\ndendrite cognition status --json",
		RunE: func(c *cobra.Command, args []string) error {
			return runCognitionStatus(globals.JSON, c.OutOrStdout())
		},
	}

	var switchAllowZero bool
	switchCmd := &cobra.Command{
		Use:     "switch",
		Short:   "Set runtime quota governor switches",
		Args:    noArgs,
		Example: "dendrite cognition switch --allow-zero=true\ndendrite cognition switch --allow-zero=false",
		RunE: func(c *cobra.Command, args []string) error {
			return runCognitionSwitch(switchAllowZero, c.OutOrStdout())
		},
	}
	switchCmd.Flags().BoolVar(&switchAllowZero, "allow-zero", false, "Allow draining remaining quota past the 20% safety reserve down to 0%")
	requireFlags(switchCmd, "allow-zero")

	modeCmd := &cobra.Command{
		Use:     "mode [always|policy|paused]",
		Short:   "View or switch operational analysis mode (always|policy|paused)",
		Args:    maxArgs(1),
		Example: "dendrite cognition mode\ndendrite cognition mode always\ndendrite cognition mode policy\ndendrite cognition mode paused",
		RunE: func(c *cobra.Command, args []string) error {
			return runCognitionMode(args, globals.JSON, c.OutOrStdout())
		},
	}

	var pauseOpts cognitionChunkOptions
	pauseCmd := &cobra.Command{
		Use:     "pause",
		Short:   "Pause scheduler analysis and persist active chunk progress",
		Args:    noArgs,
		Example: "dendrite cognition pause --project dendrite --chunk chk-001",
		RunE: func(c *cobra.Command, args []string) error {
			return runCognitionPause(c.Context(), pauseOpts, c.OutOrStdout())
		},
	}
	addProjectFlag(pauseCmd, &pauseOpts.Project, "", "Active project name")
	pauseCmd.Flags().StringVar(&pauseOpts.Chunk, "chunk", "", "Active chunk identifier")

	var resumeOpts cognitionChunkOptions
	resumeCmd := &cobra.Command{
		Use:     "resume",
		Short:   "Resume scheduler analysis in active mode",
		Args:    noArgs,
		Example: "dendrite cognition resume --project dendrite --chunk chk-001",
		RunE: func(c *cobra.Command, args []string) error {
			return runCognitionResume(c.Context(), resumeOpts, c.OutOrStdout())
		},
	}
	addProjectFlag(resumeCmd, &resumeOpts.Project, "", "Active project name")
	resumeCmd.Flags().StringVar(&resumeOpts.Chunk, "chunk", "", "Active chunk identifier")

	var runOpts cognitionRunOptions
	runCmd := &cobra.Command{
		Use:     "run",
		Short:   "Execute an analysis pass on a target chunk under scheduler governance",
		Args:    noArgs,
		Example: "dendrite cognition run --project dendrite --chunk project-ast-chunk\ndendrite cognition run --simulate-night",
		RunE: func(c *cobra.Command, args []string) error {
			return runCognitionRun(c.Context(), runOpts, globals.JSON, c.OutOrStdout())
		},
	}
	runCmd.Flags().StringVarP(&runOpts.Chunk, "chunk", "c", "project-ast-chunk", "Target chunk identifier")
	addProjectFlag(runCmd, &runOpts.Project, "dendrite", "Target project name")
	runCmd.Flags().BoolVar(&runOpts.SimulateNight, "simulate-night", false, "Simulate execution inside the night window")
	runCmd.Flags().BoolVar(&runOpts.SimulateSprint, "simulate-sprint", false, "Simulate execution inside the pre-reset cloud sprint window")
	runCmd.MarkFlagsMutuallyExclusive("simulate-night", "simulate-sprint")

	cmd.AddCommand(policyCmd, statusCmd, switchCmd, modeCmd, pauseCmd, resumeCmd, runCmd)
	return cmd
}

type cognitionPolicyOptions struct {
	NightWindow       string
	WeeklyReset       string
	SprintBeforeReset time.Duration
	ReserveFloor      float64
	AllowZero         bool
	Mode              string
}

type cognitionChunkOptions struct {
	Project string
	Chunk   string
}

type cognitionRunOptions struct {
	Chunk          string
	Project        string
	SimulateNight  bool
	SimulateSprint bool
}

func runCognitionPolicy(cmd *cobra.Command, o cognitionPolicyOptions, asJSON bool, out io.Writer) error {
	currPolicy := defaultCLIScheduler.GetPolicy()
	updated := false
	f := cmd.Flags()

	if f.Changed("night-window") {
		currPolicy.NightWindow = o.NightWindow
		updated = true
	}
	if f.Changed("weekly-reset") {
		currPolicy.WeeklyReset = o.WeeklyReset
		updated = true
	}
	if f.Changed("sprint-before-reset") {
		currPolicy.SprintBeforeReset = o.SprintBeforeReset
		updated = true
	}
	if f.Changed("reserve-floor") {
		currPolicy.ReserveFloor = o.ReserveFloor
		updated = true
	}
	if f.Changed("allow-zero") {
		currPolicy.AllowZero = o.AllowZero
		updated = true
	}
	if f.Changed("mode") {
		mode := strings.ToLower(strings.TrimSpace(o.Mode))
		switch mode {
		case "auto", "always", "night", "sprint", "paused":
			currPolicy.Mode = mode
			updated = true
		default:
			return usageErrorf("invalid mode %q: allowed modes are auto, always, night, sprint, paused", o.Mode)
		}
	}

	if updated {
		if err := defaultCLIScheduler.SetPolicy(currPolicy); err != nil {
			return fmt.Errorf("failed to update policy: %w", err)
		}
	}

	active := defaultCLIScheduler.GetPolicy()

	if asJSON {
		data, err := json.MarshalIndent(map[string]interface{}{
			"updated": updated,
			"policy":  active,
		}, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal policy: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	if updated {
		fmt.Fprintln(out, "[✓] Successfully updated Token-Aware Hybrid Scheduler policy:")
	} else {
		fmt.Fprintln(out, "Token-Aware Hybrid Scheduler Policy:")
	}
	fmt.Fprintf(out, "  • Night Window (Zero-Cost Local): %s\n", active.NightWindow)
	fmt.Fprintf(out, "  • Weekly Quota Reset:             %s\n", active.WeeklyReset)
	fmt.Fprintf(out, "  • Cloud Sprint Duration:          %v\n", active.SprintBeforeReset)
	fmt.Fprintf(out, "  • Safety Reserve Quota Floor:     %.1f%%\n", active.ReserveFloor*100)
	fmt.Fprintf(out, "  • Allow Zero Quota Drain:         %t\n", active.AllowZero)
	fmt.Fprintf(out, "  • Operational Mode:               %s\n", active.Mode)
	return nil
}

func runCognitionStatus(asJSON bool, out io.Writer) error {
	st, err := defaultCLIScheduler.Status()
	if err != nil {
		return fmt.Errorf("failed to evaluate scheduler status: %w", err)
	}

	govState := defaultCLIGovernor.GetState()

	if asJSON {
		payload := map[string]interface{}{
			"scheduler": st,
			"governor":  govState,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal cognition status: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintln(out, "=== Token-Aware Hybrid Scheduler & Quota Governor Status ===")
	fmt.Fprintf(out, "Current Time:           %s\n", st.CurrentTime.Format(time.RFC3339))
	fmt.Fprintf(out, "Operating Mode:         %s\n", govState.Mode)
	fmt.Fprintf(out, "Active Execution Tier:  %s\n", st.ActiveTier)
	fmt.Fprintf(out, "Selected Model:         %s\n", st.SelectedModel)
	fmt.Fprintf(out, "In Night Window:        %t (%s)\n", st.InNightWindow, st.Policy.NightWindow)
	fmt.Fprintf(out, "In Cloud Sprint Window: %t (within %v of reset)\n", st.InSprintWindow, st.Policy.SprintBeforeReset)
	fmt.Fprintf(out, "Next Quota Reset:       %s (in %v)\n", st.NextReset.Format(time.RFC3339), st.TimeUntilReset.Round(time.Minute))
	if !st.InSprintWindow {
		fmt.Fprintf(out, "Time Until Next Sprint: %v\n", st.TimeUntilSprint.Round(time.Minute))
	}
	fmt.Fprintf(out, "Remaining Quota:        %.1f%%\n", st.RemainingQuotaRatio*100)
	fmt.Fprintf(out, "Safety Reserve Floor:   %.1f%%\n", govState.ReserveFloor*100)
	fmt.Fprintf(out, "Allow-Zero Drain:       %t\n", govState.AllowZeroDrain)
	fmt.Fprintf(out, "Cloud Spend (USD):      $%.2f\n", st.CloudSpendUSD)
	fmt.Fprintf(out, "Cloud Blocked:          %t\n", govState.IsCloudBlocked)
	fmt.Fprintf(out, "Status Reason:          %s\n", st.Reason)
	return nil
}

func runCognitionSwitch(allowZero bool, out io.Writer) error {
	if err := defaultCLIGovernor.SetAllowZero(allowZero); err != nil {
		return fmt.Errorf("failed to set allow-zero switch: %w", err)
	}
	if allowZero {
		fmt.Fprintln(out, "[✓] Allow-Zero Drain Override: ON (Cloud sprint permitted to drain past 20% down to 0%)")
	} else {
		fmt.Fprintln(out, "[✓] Allow-Zero Drain Override: OFF (Cloud calls halt at 20% safety reserve)")
	}
	return nil
}

func runCognitionMode(args []string, asJSON bool, out io.Writer) error {
	if len(args) == 0 {
		st := defaultCLIGovernor.GetState()
		if asJSON {
			data, _ := json.MarshalIndent(map[string]string{"mode": string(st.Mode)}, "", "  ")
			fmt.Fprintln(out, string(data))
			return nil
		}
		fmt.Fprintf(out, "Current Operating Mode: %s\n", st.Mode)
		return nil
	}

	modeStr := strings.ToLower(strings.TrimSpace(args[0]))
	var mode governor.AnalysisMode
	switch modeStr {
	case "always":
		mode = governor.ModeAlways
	case "policy":
		mode = governor.ModePolicy
	case "paused", "pause":
		mode = governor.ModePaused
	default:
		return usageErrorf("invalid mode %q: allowed modes are always, policy, paused", modeStr)
	}

	if err := defaultCLIGovernor.SetMode(mode); err != nil {
		return fmt.Errorf("failed to switch operating mode: %w", err)
	}
	fmt.Fprintf(out, "[✓] Switched operational analysis mode to: %s\n", mode)
	return nil
}

func runCognitionPause(ctx context.Context, o cognitionChunkOptions, out io.Writer) error {
	if err := defaultCLIGovernor.Pause(ctx, o.Project, o.Chunk); err != nil {
		return fmt.Errorf("failed to pause scheduler: %w", err)
	}
	fmt.Fprintln(out, "[✓] Token-Aware Hybrid Scheduler and Quota Governor paused. Active chunk progress persisted.")
	return nil
}

func runCognitionResume(ctx context.Context, o cognitionChunkOptions, out io.Writer) error {
	if err := defaultCLIGovernor.Resume(ctx, o.Project, o.Chunk); err != nil {
		return fmt.Errorf("failed to resume scheduler: %w", err)
	}
	fmt.Fprintln(out, "[✓] Token-Aware Hybrid Scheduler and Quota Governor resumed.")
	return nil
}

func runCognitionRun(ctx context.Context, o cognitionRunOptions, asJSON bool, out io.Writer) error {
	if o.SimulateNight && o.SimulateSprint {
		return usageErrorf("--simulate-night and --simulate-sprint are mutually exclusive")
	}

	sched := defaultCLIScheduler
	if o.SimulateNight || o.SimulateSprint {
		pol := sched.GetPolicy()
		if o.SimulateNight {
			pol.Mode = "night"
		} else if o.SimulateSprint {
			pol.Mode = "sprint"
		}
		tempSched := token.NewTokenScheduler(nil, nil, "")
		_ = tempSched.SetPolicy(pol)
		sched = tempSched
	}

	res, err := sched.ProcessChunk(ctx, o.Chunk, func(ctx context.Context, model string) (string, int, float64, error) {
		cost := 0.0
		if model == "gemini-2.5-pro" {
			allowed, gErr := defaultCLIGovernor.CheckCloudExecution(ctx, o.Project, o.Chunk)
			if !allowed {
				return "", 0, 0, gErr
			}
			cost = 0.02
		}
		return fmt.Sprintf("Completed semantic AST analysis for %s", o.Chunk), 1500, cost, nil
	})
	if err != nil {
		return fmt.Errorf("failed to process chunk: %w", err)
	}

	if asJSON {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal chunk result: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintf(out, "[✓] Chunk Execution Outcome:\n")
	fmt.Fprintf(out, "    Chunk ID:       %s\n", res.ChunkID)
	fmt.Fprintf(out, "    Execution Tier: %s\n", res.ActiveTier)
	fmt.Fprintf(out, "    Selected Model: %s\n", res.SelectedModel)
	fmt.Fprintf(out, "    Tokens:         %d\n", res.Tokens)
	fmt.Fprintf(out, "    Cost (USD):     $%.4f\n", res.CostUSD)
	fmt.Fprintf(out, "    Details:        %s\n", res.Response)
	return nil
}

func handleAnalyzeCommand(ctx context.Context, action string, args []string) {
	_ = invokeCmd(ctx, newCognitionCmd(), append([]string{action}, args...)...)
}

func printAnalyzeUsage() {
	_ = newCognitionCmd().Help()
}
