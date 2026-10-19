package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/tokens"
)

// newVesicleCmd builds `dendrite vesicle`: 24h and 7d token quota windows and prompt token estimation.
func newVesicleCmd() *cobra.Command {
	cmd := newGroupCmd("vesicle", "Inspect 24h & 7d token consumption windows and tier quotas", "boutons",
		"tokens", "token", "vesicle-quota", "quota", "tok")
	cmd.Example = "dendrite vesicle status --json\ndendrite vesicle status --probe --model gemini-3.8-flash-high\ndendrite vesicle estimate \"Explain Dendrite architecture\""

	defCfg := tokens.DefaultConfig()
	var statusOpts vesicleStatusOptions
	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Inspect 24h and 7d token availability and AGY access confirmation",
		Args:    noArgs,
		Example: "dendrite vesicle status\ndendrite vesicle status --probe --day-tokens 1000000 --week-tokens 7000000 --json",
		RunE: func(c *cobra.Command, args []string) error {
			return runVesicleStatus(c.Context(), statusOpts, globals.JSON, c.OutOrStdout())
		},
	}
	sf := statusCmd.Flags()
	sf.BoolVar(&statusOpts.Probe, "probe", false, "Run a live ping probe against AGY CLI")
	addModelFlag(statusCmd, &statusOpts.Model, defCfg.Model)
	sf.Int64Var(&statusOpts.DayTokens, "day-tokens", defCfg.DayCapacityTokens, "24h token capacity ceiling")
	sf.Int64Var(&statusOpts.WeekTokens, "week-tokens", defCfg.WeekCapacityTokens, "7d token capacity ceiling")
	sf.StringVar(&statusOpts.SpendFile, "spend-file", "", "Path to token spend JSON file")
	sf.StringVar(&statusOpts.PolicyFile, "policy-file", "", "Path to scheduler policy JSON file")

	estimateCmd := &cobra.Command{
		Use:     "estimate <text...>",
		Short:   "Estimate character, word, and token counts for a prompt",
		Args:    minArgs(1),
		Example: "dendrite vesicle estimate \"Explain Dendrite architecture\"\ndendrite vesicle estimate \"Refactor AST parser\" --json",
		RunE: func(c *cobra.Command, args []string) error {
			return runVesicleEstimate(args, globals.JSON, c.OutOrStdout())
		},
	}

	cmd.AddCommand(statusCmd, estimateCmd)
	return cmd
}

type vesicleStatusOptions struct {
	Probe      bool
	Model      string
	DayTokens  int64
	WeekTokens int64
	SpendFile  string
	PolicyFile string
}

func runVesicleStatus(ctx context.Context, o vesicleStatusOptions, asJSON bool, out io.Writer) error {
	cfg := tokens.DefaultConfig()
	cfg.GatewayBaseURL = gatewayV1URL()
	cfg.Model = o.Model
	cfg.DayCapacityTokens = o.DayTokens
	cfg.WeekCapacityTokens = o.WeekTokens
	if o.SpendFile != "" {
		cfg.SpendFile = o.SpendFile
	}
	if o.PolicyFile != "" {
		cfg.PolicyFile = o.PolicyFile
	}

	eval := tokens.NewEvaluator(nil, nil)
	report, err := eval.Evaluate(ctx, cfg, o.Probe)
	if err != nil {
		return fmt.Errorf("failed to evaluate token status: %w", err)
	}

	if asJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal token status: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprint(out, tokens.FormatReport(report))
	return nil
}

func runVesicleEstimate(args []string, asJSON bool, out io.Writer) error {
	input := strings.Join(args, " ")
	metrics := devtool.AuditPrompt(input)
	if asJSON {
		data, err := json.MarshalIndent(metrics, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal prompt metrics: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintf(out, "[+] Prompt Character Count:  %d\n", metrics.CharacterCount)
	fmt.Fprintf(out, "[+] Prompt Word Count:       %d\n", metrics.WordCount)
	fmt.Fprintf(out, "[+] Estimated Token Volume:  %d tokens (~4 chars/token)\n", metrics.EstimatedTokens)
	return nil
}

func handleTokensCommand(ctx context.Context, args []string) {
	_ = invokeCmd(ctx, newVesicleCmd(), args...)
}
