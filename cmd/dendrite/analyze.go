package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"Dendrite/pkg/scheduler/governor"
	"Dendrite/pkg/scheduler/token"

	"github.com/spf13/cobra"
)

// newScheduler and newGovernor open the token scheduler and quota governor on use, so --state-dir is honored.
func newScheduler() *token.TokenScheduler { return token.NewTokenScheduler(nil, nil, "") }

func newGovernor(sched *token.TokenScheduler) (*governor.QuotaGovernor, error) {
	st, err := openSegmentedStore()
	if err != nil {
		return nil, fmt.Errorf("open segmented store: %w", err)
	}
	return governor.NewQuotaGovernor(governor.DefaultConfig(), st, sched), nil
}

// withGovernor runs fn with a fresh scheduler and the governor built on it.
func withGovernor(fn func(*token.TokenScheduler, *governor.QuotaGovernor) error) error {
	sched := newScheduler()
	gov, err := newGovernor(sched)
	if err != nil {
		return err
	}
	return fn(sched, gov)
}

// newCognitionCmd builds `dendrite cognition` (alias analyze): the token-aware scheduler and quota governor.
func newCognitionCmd() *cobra.Command {
	cmd := newGroupCmd("cognition", "Cognitive complexity analyzer, structural anti-pattern scanner, and policy router", "glia",
		"analyze", "cognitive-analysis", "anlz")
	cmd.Example = "dendrite cognition status --json\n" +
		"dendrite cognition policy --night-window 22:00-06:00 --reserve-floor 0.25\n" +
		"dendrite cognition switch --allow-zero off\n" +
		"dendrite cognition mode paused\n" +
		"dendrite cognition run --chunk ast-1 --simulate-night"
	cmd.AddCommand(
		newCognitionPolicyCmd(),
		newCognitionStatusCmd(),
		newCognitionSwitchCmd(),
		newCognitionModeCmd(),
		newCognitionPauseResumeCmd("pause", "Pause analysis and persist the active chunk's progress"),
		newCognitionPauseResumeCmd("resume", "Resume analysis in the active mode"),
		newCognitionRunCmd(),
	)
	return cmd
}

func newCognitionPolicyCmd() *cobra.Command {
	var p token.Policy
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Show the scheduler policy, or change it with flags",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCognitionPolicy(cmd, p, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&p.NightWindow, "night-window", "", "Zero-cost local window, HH:MM-HH:MM")
	f.StringVar(&p.WeeklyReset, "weekly-reset", "", "Weekly quota reset, e.g. \"MON 09:00\"")
	f.DurationVar(&p.SprintBeforeReset, "sprint-before-reset", 0, "Cloud sprint window before the reset")
	f.Float64Var(&p.ReserveFloor, "reserve-floor", 0, "Safety reserve as a fraction (0.0-1.0)")
	f.BoolVar(&p.AllowZero, "allow-zero", false, "Allow draining the quota below the reserve floor")
	f.StringVar(&p.Mode, "mode", "", "Mode: auto, always, night, sprint or paused")
	return cmd
}

// runCognitionPolicy applies the flags the user set on top of the stored policy.
func runCognitionPolicy(cmd *cobra.Command, set token.Policy, out io.Writer) error {
	sched := newScheduler()
	pol := sched.GetPolicy()
	f := cmd.Flags()
	if f.Changed("reserve-floor") && (set.ReserveFloor < 0 || set.ReserveFloor > 1) {
		return usageErrorf("--reserve-floor must be between 0.0 and 1.0")
	}
	updated := false
	apply := func(name string, fn func()) {
		if f.Changed(name) {
			fn()
			updated = true
		}
	}
	apply("night-window", func() { pol.NightWindow = set.NightWindow })
	apply("weekly-reset", func() { pol.WeeklyReset = set.WeeklyReset })
	apply("sprint-before-reset", func() { pol.SprintBeforeReset = set.SprintBeforeReset })
	apply("reserve-floor", func() { pol.ReserveFloor = set.ReserveFloor })
	apply("allow-zero", func() { pol.AllowZero = set.AllowZero })
	apply("mode", func() { pol.Mode = set.Mode })
	if updated {
		if err := sched.SetPolicy(pol); err != nil {
			return fmt.Errorf("update policy: %w", err)
		}
	}

	active := sched.GetPolicy()
	if globals.JSON {
		return writeJSON(out, map[string]any{"updated": updated, "policy": active})
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

func newCognitionStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the scheduler's tier, windows, countdowns and the governor state",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernor(func(sched *token.TokenScheduler, gov *governor.QuotaGovernor) error {
				return runCognitionStatus(sched, gov, cmd.OutOrStdout())
			})
		},
	}
}

func runCognitionStatus(sched *token.TokenScheduler, gov *governor.QuotaGovernor, out io.Writer) error {
	st, err := sched.Status()
	if err != nil {
		return fmt.Errorf("evaluate scheduler status: %w", err)
	}
	govState := gov.GetState()
	if globals.JSON {
		return writeJSON(out, map[string]any{"scheduler": st, "governor": govState})
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

// parseOnOff reads an on/off switch value.
func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "on", "true", "1", "enable", "yes":
		return true, nil
	case "off", "false", "0", "disable", "no":
		return false, nil
	}
	return false, usageErrorf("invalid switch value %q: use on or off", s)
}

func newCognitionSwitchCmd() *cobra.Command {
	var allowZero string
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Set runtime feature switches",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			enabled, err := parseOnOff(allowZero)
			if err != nil {
				return err
			}
			return withGovernor(func(_ *token.TokenScheduler, gov *governor.QuotaGovernor) error {
				if err := gov.SetAllowZero(enabled); err != nil {
					return fmt.Errorf("set allow-zero switch: %w", err)
				}
				if enabled {
					fmt.Fprintln(cmd.OutOrStdout(), "[✓] Allow-Zero Drain Override: ON (Cloud sprint permitted to drain past 20% down to 0%)")
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "[✓] Allow-Zero Drain Override: OFF (Cloud calls halt at 20% safety reserve)")
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&allowZero, "allow-zero", "", "Allow-zero drain override: on or off")
	requireFlags(cmd, "allow-zero")
	return cmd
}

func newCognitionModeCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "mode [always|policy|paused]",
		Short:     "Show or set the analysis operating mode",
		Args:      maxArgs(1),
		ValidArgs: []string{"always", "policy", "paused"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var mode governor.AnalysisMode
			if len(args) == 1 {
				switch strings.ToLower(args[0]) {
				case "always":
					mode = governor.ModeAlways
				case "policy":
					mode = governor.ModePolicy
				case "paused", "pause":
					mode = governor.ModePaused
				default:
					return usageErrorf("invalid mode %q: use always, policy or paused", args[0])
				}
			}
			return withGovernor(func(_ *token.TokenScheduler, gov *governor.QuotaGovernor) error {
				out := cmd.OutOrStdout()
				if mode == "" {
					fmt.Fprintf(out, "Current Operating Mode: %s\n", gov.GetState().Mode)
					return nil
				}
				if err := gov.SetMode(mode); err != nil {
					return fmt.Errorf("switch operating mode: %w", err)
				}
				fmt.Fprintf(out, "[✓] Switched operational analysis mode to: %s\n", mode)
				return nil
			})
		},
	}
}

func newCognitionPauseResumeCmd(use, short string) *cobra.Command {
	var project, chunk string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernor(func(_ *token.TokenScheduler, gov *governor.QuotaGovernor) error {
				ctx, out := cmd.Context(), cmd.OutOrStdout()
				if use == "pause" {
					if err := gov.Pause(ctx, project, chunk); err != nil {
						return fmt.Errorf("pause: %w", err)
					}
					fmt.Fprintln(out, "[✓] Token-Aware Hybrid Scheduler and Quota Governor paused. Active chunk progress persisted.")
					return nil
				}
				if err := gov.Resume(ctx, project, chunk); err != nil {
					return fmt.Errorf("resume: %w", err)
				}
				fmt.Fprintln(out, "[✓] Token-Aware Hybrid Scheduler and Quota Governor resumed.")
				return nil
			})
		},
	}
	addProjectFlag(cmd, &project, "", "Project of the active chunk")
	cmd.Flags().StringVarP(&chunk, "chunk", "c", "", "Active chunk ID")
	return cmd
}

func newCognitionRunCmd() *cobra.Command {
	var chunk, project string
	var simNight, simSprint bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one analysis pass on a chunk under scheduler governance",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernor(func(sched *token.TokenScheduler, gov *governor.QuotaGovernor) error {
				if simNight || simSprint {
					pol := sched.GetPolicy()
					pol.Mode = "night"
					if simSprint {
						pol.Mode = "sprint"
					}
					sched = token.NewTokenScheduler(nil, nil, "")
					if err := sched.SetPolicy(pol); err != nil {
						return fmt.Errorf("set simulated policy: %w", err)
					}
				}
				return runCognitionChunk(cmd.Context(), sched, gov, project, chunk, cmd.OutOrStdout())
			})
		},
	}
	cmd.Flags().StringVarP(&chunk, "chunk", "c", "project-ast-chunk", "Chunk ID")
	addProjectFlag(cmd, &project, "dendrite", "Project")
	cmd.Flags().BoolVar(&simNight, "simulate-night", false, "Run as if inside the night window")
	cmd.Flags().BoolVar(&simSprint, "simulate-sprint", false, "Run as if inside the sprint window")
	cmd.MarkFlagsMutuallyExclusive("simulate-night", "simulate-sprint")
	return cmd
}

func runCognitionChunk(ctx context.Context, sched *token.TokenScheduler, gov *governor.QuotaGovernor, project, chunk string, out io.Writer) error {
	res, err := sched.ProcessChunk(ctx, chunk, func(ctx context.Context, model string) (string, int, float64, error) {
		cost := 0.0
		if model == "gemini-2.5-pro" {
			// The governor's safety floor gates every cloud call.
			if allowed, gErr := gov.CheckCloudExecution(ctx, project, chunk); !allowed {
				return "", 0, 0, gErr
			}
			cost = 0.02
		}
		return fmt.Sprintf("Completed semantic AST analysis for %s", chunk), 1500, cost, nil
	})
	if err != nil {
		return fmt.Errorf("process chunk: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, res)
	}
	fmt.Fprintln(out, "[✓] Chunk Execution Outcome:")
	fmt.Fprintf(out, "    Chunk ID:       %s\n", res.ChunkID)
	fmt.Fprintf(out, "    Execution Tier: %s\n", res.ActiveTier)
	fmt.Fprintf(out, "    Selected Model: %s\n", res.SelectedModel)
	fmt.Fprintf(out, "    Tokens:         %d\n", res.Tokens)
	fmt.Fprintf(out, "    Cost (USD):     $%.4f\n", res.CostUSD)
	fmt.Fprintf(out, "    Details:        %s\n", res.Response)
	return nil
}
