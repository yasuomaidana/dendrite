package immune

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/immune"
)

// NewImmuneCmd builds `dendrite immune`: the gate that decides whether an agent skill may be installed.
func NewImmuneCmd() *cobra.Command {
	cmd := cli.NewGroupCmd("immune", "Scan agent skills with SkillSpector before they are installed", "glia")
	cmd.Example = "dendrite immune scan .claude/skills/my-skill\n" +
		"dendrite immune scan ./skills --json\n" +
		"dendrite immune update\n" +
		"dendrite immune allow .claude/skills/my-skill --reason \"reviewed: the URL is our own mirror\""
	cmd.AddCommand(newImmuneScanCmd(), newImmuneUpdateCmd(), newImmuneAllowCmd())
	return cmd
}

// immuneScanOptions are the flags of `immune scan`; a band flag counts only when it was given.
type immuneScanOptions struct {
	immune.Thresholds
	LLM bool
	// Pin is the scanner version the directives require (immune.scanner_version); empty accepts any.
	Pin string
}

// immunePin is the scanner version the project's directives pin; empty when they pin none.
func immunePin() (string, error) {
	set, err := directive.Load(".")
	if err != nil {
		return "", err
	}
	return set.Immune.ScannerVersion, nil
}

// immuneSettings fills o from the project's directives: the bands to judge by (immune.approve_max and
// immune.hold_max, then any band a flag sets) and the scanner version they pin.
func immuneSettings(cmd *cobra.Command, o immuneScanOptions) (immuneScanOptions, error) {
	set, err := directive.Load(".")
	if err != nil {
		return o, err
	}
	t := immune.Thresholds{ApproveMax: set.Immune.ApproveMax, HoldMax: set.Immune.HoldMax}
	if cmd.Flags().Changed("approve-max") {
		t.ApproveMax = o.ApproveMax
	}
	if cmd.Flags().Changed("hold-max") {
		t.HoldMax = o.HoldMax
	}
	o.Thresholds, o.Pin = t, set.Immune.ScannerVersion
	return o, nil
}

func newImmuneScanCmd() *cobra.Command {
	o := immuneScanOptions{Thresholds: immune.DefaultThresholds}
	cmd := &cobra.Command{
		Use:   "scan <skill-path>",
		Short: "Scan a skill and print approve / hold / quarantine",
		Long: "Runs SkillSpector over a skill directory and maps its risk score to a verdict: up to immune.approve_max\n" +
			"(20) approve (exit 0), up to immune.hold_max (50) hold (exit 3, a human decision is needed), above that\n" +
			"quarantine (exit 4). The bands come from .github/dendrite/directives.toml; a flag overrides one. The scanner is static by\n" +
			"default; --llm adds its model pass, which needs a provider in the environment (SKILLSPECTOR_PROVIDER).\n" +
			"When the directives pin a scanner version (immune.scanner_version), a scanner of another version is refused:\n" +
			"run `dendrite immune update`. Every result is appended to the scan ledger of the repository that holds the\n" +
			"skill (.dendrite/immune/scans.jsonl); the pre-edit and pre-commit hooks read it to decide whether a skill under\n" +
			".claude/skills/ or .agents/skills/ may be changed or committed.",
		Args: cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := immuneSettings(cmd, o)
			if err != nil {
				return err
			}
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
	ledger := immune.LedgerFor(path)
	r, err := immune.Scanner{Thresholds: o.Thresholds, LLM: o.LLM, Pin: o.Pin, Ledger: &ledger}.Scan(ctx, path)
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

func newImmuneUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Install the pinned SkillSpector version, or the latest when none is pinned",
		Long: "Brings the installed scanner to the version the directives pin (immune.scanner_version in\n" +
			".github/dendrite/directives.toml, set with `dendrite directive set immune.scanner_version=<version>`) using\n" +
			"`uv tool install`. A scanner already at the pin is left alone. Without a pin it installs the scanner when\n" +
			"there is none and upgrades it to the latest otherwise.",
		Example: "dendrite immune update\ndendrite immune update --json",
		Args:    cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImmuneUpdate(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

func runImmuneUpdate(ctx context.Context, out io.Writer) error {
	pin, err := immunePin()
	if err != nil {
		return err
	}
	u, err := immune.Refresh(ctx, pin)
	if err != nil {
		return err
	}
	if cli.Globals.JSON {
		return cli.WriteJSON(out, u)
	}
	_, err = fmt.Fprintln(out, immuneUpdateLine(u))
	return err
}

// immuneUpdateLine says what `immune update` did.
func immuneUpdateLine(u immune.Update) string {
	switch {
	case u.Changed && u.Before == "":
		return fmt.Sprintf("[+] SkillSpector %s installed", u.After)
	case u.Changed:
		return fmt.Sprintf("[+] SkillSpector %s -> %s", u.Before, u.After)
	case u.Pin != "":
		return fmt.Sprintf("[✓] SkillSpector %s is the pinned version; nothing to do", u.After)
	}
	return fmt.Sprintf("[✓] SkillSpector %s is the latest; pin it with `dendrite directive set immune.scanner_version=%s`", u.After, u.After)
}

func newImmuneAllowCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "allow <skill-path>",
		Short: "Record a person's decision to accept a held scan of a skill",
		Long: "A scan that scores in the hold band needs a person. `immune allow` records that decision and its reason in\n" +
			"the scan ledger for the skill's content as it is now, and the hooks then let the skill through. Changing the\n" +
			"skill afterwards needs a new scan. Content that was not scanned or is quarantined cannot be allowed.",
		Example: "dendrite immune allow .claude/skills/my-skill --reason \"reviewed: the URL is our own mirror\"",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImmuneAllow(cmd.OutOrStdout(), args[0], reason)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the held skill is safe to use (required)")
	return cmd
}

func runImmuneAllow(out io.Writer, path, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return cli.UsageErrorf("--reason is required: say why the held skill is safe to use")
	}
	abs := path
	if a, err := filepath.Abs(path); err == nil {
		abs = a
	}
	e, added, err := immune.Allow(abs, reason, time.Now())
	if err != nil {
		return err
	}
	if cli.Globals.JSON {
		return cli.WriteJSON(out, e)
	}
	switch {
	case e.Verdict == immune.Approve:
		_, err = fmt.Fprintf(out, "[✓] %s is approved (score %d); no decision is needed\n", abs, e.Score)
	case added:
		_, err = fmt.Fprintf(out, "[+] %s: held scan (score %d) allowed: %s\n", abs, e.Score, e.Reason)
	default:
		_, err = fmt.Fprintf(out, "[✓] %s: that decision is already recorded\n", abs)
	}
	return err
}
