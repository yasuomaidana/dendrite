package immune

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/immune"
)

// NewImmuneCmd builds `dendrite immune`: the gate that decides whether an agent skill may be installed.
func NewImmuneCmd() *cobra.Command {
	cmd := cli.NewGroupCmd("immune", "Scan agent skills with SkillSpector before they are installed", "glia")
	cmd.Example = "dendrite immune scan .claude/skills/my-skill\n" +
		"dendrite immune scan ./skills --json"
	cmd.AddCommand(newImmuneScanCmd())
	return cmd
}

// immuneScanOptions are the flags of `immune scan`; a band flag counts only when it was given.
type immuneScanOptions struct {
	immune.Thresholds
	LLM bool
}

// immuneThresholds are the bands to judge by: the project's directives (immune.approve_max and
// immune.hold_max), then any band a flag sets.
func immuneThresholds(cmd *cobra.Command, o immuneScanOptions) (immune.Thresholds, error) {
	set, err := directive.Load(".")
	if err != nil {
		return immune.Thresholds{}, err
	}
	t := immune.Thresholds{ApproveMax: set.Immune.ApproveMax, HoldMax: set.Immune.HoldMax}
	if cmd.Flags().Changed("approve-max") {
		t.ApproveMax = o.ApproveMax
	}
	if cmd.Flags().Changed("hold-max") {
		t.HoldMax = o.HoldMax
	}
	return t, nil
}

func newImmuneScanCmd() *cobra.Command {
	o := immuneScanOptions{Thresholds: immune.DefaultThresholds}
	cmd := &cobra.Command{
		Use:   "scan <skill-path>",
		Short: "Scan a skill and print approve / hold / quarantine",
		Long: "Runs SkillSpector over a skill directory and maps its risk score to a verdict: up to immune.approve_max\n" +
			"(20) approve (exit 0), up to immune.hold_max (50) hold (exit 3, a human decision is needed), above that\n" +
			"quarantine (exit 4). The bands come from .github/dendrite/directives.toml; a flag overrides one. The scanner is static by\n" +
			"default; --llm adds its model pass, which needs a provider in the environment (SKILLSPECTOR_PROVIDER).",
		Args: cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := immuneThresholds(cmd, o)
			if err != nil {
				return err
			}
			o.Thresholds = t
			return runImmuneScan(cmd.Context(), cmd.OutOrStdout(), args[0], o)
		},
	}
	cmd.Flags().IntVar(&o.ApproveMax, "approve-max", o.ApproveMax, "highest score that is approved (default: immune.approve_max of the directives)")
	cmd.Flags().IntVar(&o.HoldMax, "hold-max", o.HoldMax, "highest score that is held; above it is quarantined (default: immune.hold_max of the directives)")
	cmd.Flags().BoolVar(&o.LLM, "llm", false, "also run the scanner's model pass")
	return cmd
}

func runImmuneScan(ctx context.Context, out io.Writer, path string, o immuneScanOptions) error {
	if err := o.Validate(); err != nil {
		return cli.UsageErrorf("%v", err)
	}
	r, err := immune.Scanner{Thresholds: o.Thresholds, LLM: o.LLM}.Scan(ctx, path)
	if err != nil {
		return err
	}
	if cli.Globals.JSON {
		if err := cli.WriteJSON(out, r); err != nil {
			return err
		}
	} else {
		printImmuneReport(out, r)
	}
	switch r.Verdict {
	case immune.Hold:
		return &cli.VerdictError{Code: cli.ExitHold, Msg: fmt.Sprintf("held: score %d needs a recorded human decision", r.Score)}
	case immune.Quarantine:
		return &cli.VerdictError{Code: cli.ExitQuarantine, Msg: fmt.Sprintf("quarantined: score %d", r.Score)}
	}
	return nil
}

func printImmuneReport(out io.Writer, r immune.Report) {
	fmt.Fprintf(out, "[+] %s: score %d (%s) -> %s   [%s]\n", r.Path, r.Score, r.Severity, r.Verdict, r.ScannerVersion)
	for _, i := range r.Issues {
		fmt.Fprintf(out, "    - %s %s (%s) %s:%d\n", i.ID, i.Pattern, i.Severity, i.File, i.Line)
	}
}
