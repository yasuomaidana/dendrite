package knowledge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/tokens"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// newVesicleCmd builds `dendrite vesicle` (alias tokens): token windows, quotas and prompt size estimates.
func NewVesicleCmd() *cobra.Command {
	cmd := cli.NewGroupCmd("vesicle", "Inspect 24h & 7d token consumption windows and tier quotas", "boutons",
		"tokens", "token", "vesicle-quota", "quota", "tok")
	cmd.Example = "dendrite vesicle status\n" +
		"dendrite vesicle status --probe --json\n" +
		"dendrite vesicle estimate \"Refactor the planner to use the DAG solver\""
	cmd.AddCommand(newVesicleStatusCmd(), newVesicleEstimateCmd())
	return cmd
}

func newVesicleStatusCmd() *cobra.Command {
	cfg := tokens.DefaultConfig()
	var probe bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show day and week token windows against capacity, and AGY access",
		Args:  cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.DayCapacityTokens <= 0 || cfg.WeekCapacityTokens <= 0 {
				return cli.UsageErrorf("--day-tokens and --week-tokens must be positive")
			}
			cfg.GatewayBaseURL = cli.GatewayV1URL()
			report, err := tokens.NewEvaluator(nil, nil).Evaluate(cmd.Context(), cfg, probe)
			if err != nil {
				return fmt.Errorf("evaluate token status: %w", err)
			}
			if cli.Globals.JSON {
				return cli.WriteJSON(cmd.OutOrStdout(), report)
			}
			fmt.Fprint(cmd.OutOrStdout(), tokens.FormatReport(report))
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&probe, "probe", false, "Send a probe request to confirm AGY access")
	cli.AddModelFlag(cmd, &cfg.Model, cfg.Model)
	f.Int64Var(&cfg.DayCapacityTokens, "day-tokens", cfg.DayCapacityTokens, "Day window capacity in tokens")
	f.Int64Var(&cfg.WeekCapacityTokens, "week-tokens", cfg.WeekCapacityTokens, "Week window capacity in tokens")
	f.StringVar(&cfg.SpendFile, "spend-file", cfg.SpendFile, "Spend ledger file")
	f.StringVar(&cfg.PolicyFile, "policy-file", cfg.PolicyFile, "Scheduler policy file")
	return cmd
}

func newVesicleEstimateCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "estimate <prompt...>",
		Aliases: []string{"audit-prompt"},
		Short:   "Estimate a prompt's size in characters, words and tokens",
		Args:    cli.MinArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVesicleEstimate(strings.Join(args, " "), cmd.OutOrStdout())
		},
	}
}

func runVesicleEstimate(prompt string, out io.Writer) error {
	m := devtool.AuditPrompt(prompt)
	if cli.Globals.JSON {
		return cli.WriteJSON(out, m)
	}
	fmt.Fprintf(out, "[+] Prompt Character Count:  %d\n", m.CharacterCount)
	fmt.Fprintf(out, "[+] Prompt Word Count:       %d\n", m.WordCount)
	fmt.Fprintf(out, "[+] Estimated Token Volume:  %d tokens (~4 chars/token)\n", m.EstimatedTokens)
	return nil
}
