package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"Dendrite/pkg/immune"
)

// newImmuneCmd builds `dendrite immune`: the gate that decides whether an agent skill may be installed.
func newImmuneCmd() *cobra.Command {
	cmd := newGroupCmd("immune", "Scan agent skills with SkillSpector before they are installed", "glia")
	cmd.Example = "dendrite immune scan .claude/skills/my-skill\n" +
		"dendrite immune scan ./skills --json"
	cmd.AddCommand(newImmuneScanCmd())
	return cmd
}

type immuneScanOptions struct {
	immune.Thresholds
	LLM bool
}

func newImmuneScanCmd() *cobra.Command {
	o := immuneScanOptions{Thresholds: immune.DefaultThresholds}
	cmd := &cobra.Command{
		Use:   "scan <skill-path>",
		Short: "Scan a skill and print approve / hold / quarantine",
		Long: "Runs SkillSpector over a skill directory and maps its risk score to a verdict: 0-20 approve (exit 0),\n" +
			"21-50 hold (exit 3, a human decision is needed), 51+ quarantine (exit 4). The scanner is static by\n" +
			"default; --llm adds its model pass, which needs a provider in the environment (SKILLSPECTOR_PROVIDER).",
		Args: asUsage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImmuneScan(cmd.Context(), cmd.OutOrStdout(), args[0], o)
		},
	}
	cmd.Flags().IntVar(&o.ApproveMax, "approve-max", o.ApproveMax, "highest score that is approved")
	cmd.Flags().IntVar(&o.HoldMax, "hold-max", o.HoldMax, "highest score that is held; above it is quarantined")
	cmd.Flags().BoolVar(&o.LLM, "llm", false, "also run the scanner's model pass")
	return cmd
}

func runImmuneScan(ctx context.Context, out io.Writer, path string, o immuneScanOptions) error {
	if err := o.Validate(); err != nil {
		return usageErrorf("%v", err)
	}
	r, err := immune.Scanner{Thresholds: o.Thresholds, LLM: o.LLM}.Scan(ctx, path)
	if err != nil {
		return err
	}
	if globals.JSON {
		if err := writeJSON(out, r); err != nil {
			return err
		}
	} else {
		printImmuneReport(out, r)
	}
	switch r.Verdict {
	case immune.Hold:
		return &verdictError{ExitHold, fmt.Sprintf("held: score %d needs a recorded human decision", r.Score)}
	case immune.Quarantine:
		return &verdictError{ExitQuarantine, fmt.Sprintf("quarantined: score %d", r.Score)}
	}
	return nil
}

func printImmuneReport(out io.Writer, r immune.Report) {
	fmt.Fprintf(out, "[+] %s: score %d (%s) -> %s   [%s]\n", r.Path, r.Score, r.Severity, r.Verdict, r.ScannerVersion)
	for _, i := range r.Issues {
		fmt.Fprintf(out, "    - %s %s (%s) %s:%d\n", i.ID, i.Pattern, i.Severity, i.File, i.Line)
	}
}
