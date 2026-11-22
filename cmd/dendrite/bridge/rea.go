package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/planecli"
	"Dendrite/pkg/rea/ownership"
	"Dendrite/pkg/rea/showcase"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"os"
	"strings"
)

// newRetroCmd builds `dendrite retro` (alias rea): cleanroom reverse-engineering showcases and parity checks.
func NewRetroCmd() *cobra.Command {
	cmd := cli.NewGroupCmd("retro", "Reverse engineering cleanroom analysis and parity verification", "arbor",
		"rea", "retrograde-analysis")
	cmd.Example = "dendrite retro list\n" +
		"dendrite retro showcase\n" +
		"dendrite retro showcase aegis --format markdown --output aegis.md\n" +
		"dendrite retro showcase ctf --json\n" +
		"dendrite retro handoff DEN-1062 --refused\n" +
		"dendrite retro analyze sample.exe\n" +
		"dendrite retro search <id> \"CreateFileW\""

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"targets", "ls"},
		Short:   "List the open-source showcase targets",
		Args:    cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRetroList(cmd.OutOrStdout())
		},
	}

	var format, output string
	targetIDs := []string{"all"}
	for _, t := range showcase.ListTargets() {
		targetIDs = append(targetIDs, t.ID)
	}
	showcaseCmd := &cobra.Command{
		Use:       "showcase [target]",
		Short:     "Run cleanroom analysis and parity verification for one target or all (" + strings.Join(targetIDs, ", ") + ")",
		Args:      cli.MaxArgs(1),
		ValidArgs: targetIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) == 1 {
				target = args[0]
			}
			f := strings.ToLower(format)
			if cli.Globals.JSON {
				f = "json"
			} else if f != "text" && f != "markdown" && f != "md" {
				return cli.UsageErrorf("invalid --format %q: use text or markdown (--json for JSON)", format)
			}
			return runRetroShowcase(cmd, target, f, output)
		},
	}
	showcaseCmd.Flags().StringVar(&format, "format", "text", "Report format: text or markdown (use --json for JSON)")
	showcaseCmd.Flags().StringVarP(&output, "output", "o", "", "Write the report to this file instead of stdout")

	cmd.AddCommand(list, showcaseCmd, newRetroHandoffCmd())
	cmd.AddCommand(reaCommands()...)
	cmd.AddCommand(insightCommands()...)
	cmd.AddCommand(engineCommands()...)
	cmd.AddCommand(reaExportCommand())
	return cmd
}

func runRetroList(out io.Writer) error {
	targets := showcase.ListTargets()
	if cli.Globals.JSON {
		return cli.WriteJSON(out, targets)
	}
	fmt.Fprintf(out, "[+] Registered Open-Source REA Showcase Targets (%d):\n\n", len(targets))
	fmt.Fprintf(out, "    %-12s | %-32s | %-12s | %-8s | %s\n", "ID", "Name", "License (SPDX)", "Format", "Upstream Repository")
	fmt.Fprintln(out, "    "+strings.Repeat("-", 100))
	for _, t := range targets {
		fmt.Fprintf(out, "    %-12s | %-32s | %-12s | %-8s | %s\n", t.ID, t.Name, t.LicenseSPDX, t.BinaryFormat, t.UpstreamRepo)
		fmt.Fprintf(out, "      -> Capability: %s\n", t.ExtractedCapability)
		fmt.Fprintf(out, "      -> Oracle:     %s\n\n", t.GroundTruthOracle)
	}
	return nil
}

func runRetroShowcase(cmd *cobra.Command, target, format, output string) error {
	out := cmd.OutOrStdout()
	// Progress goes to stderr so stdout holds only the report.
	fmt.Fprintf(cmd.ErrOrStderr(), "[*] Launching Open-Source REA Showcase Verification for '%s'...\n", target)
	report, err := showcase.RunShowcase(cmd.Context(), target)
	if err != nil {
		return fmt.Errorf("showcase run: %w", err)
	}

	var content string
	switch format {
	case "json":
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode JSON report: %w", err)
		}
		content = string(data)
	case "markdown", "md":
		content = showcase.RenderMarkdownScorecard(report)
	default:
		content = renderRetroText(report)
	}

	if output == "" {
		fmt.Fprintln(out, content)
	} else {
		if err := os.WriteFile(output, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write report to %s: %w", output, err)
		}
		fmt.Fprintf(out, "[+] Scorecard written to %s\n", output)
	}
	if report.FailedTargets > 0 {
		return fmt.Errorf("%d of %d showcase targets failed validation", report.FailedTargets, report.TotalTargets)
	}
	return nil
}

func renderRetroText(report *showcase.ShowcaseReport) string {
	var sb strings.Builder
	rule := "================================================================================\n"
	sb.WriteString("\n" + rule)
	fmt.Fprintf(&sb, "  DENDRITE REA CLEANROOM VERIFICATION SCORECARD: %s\n", report.OverallStatus)
	sb.WriteString(rule)
	fmt.Fprintf(&sb, "  Timestamp:             %s\n", report.Timestamp)
	fmt.Fprintf(&sb, "  Targets Evaluated:     %d (Passed: %d, Failed: %d)\n", report.TotalTargets, report.SucceededTargets, report.FailedTargets)
	fmt.Fprintf(&sb, "  Aggregate Parity:      %.2f%%\n\n", report.OverallParityPercent)
	for _, r := range report.Results {
		icon := "[+]"
		if !r.PassedValidation {
			icon = "[-]"
		}
		fmt.Fprintf(&sb, "  %s %s (%s)\n", icon, r.Metadata.Name, r.Target)
		fmt.Fprintf(&sb, "      Upstream:          %s (%s)\n", r.Metadata.UpstreamRepo, r.Metadata.LicenseSPDX)
		fmt.Fprintf(&sb, "      Capability:        %s\n", r.Metadata.ExtractedCapability)
		fmt.Fprintf(&sb, "      Vectors Tested:    %d/%d passed (%.2f%% parity)\n", r.Parity.PassedVectors, r.Parity.TotalVectors, r.Parity.ParityPercent)
		fmt.Fprintf(&sb, "      SPDX Audit:        Compliant: %t (OSI: %t)\n", r.SPDX.Compliant, r.SPDX.OSIApproved)
		if r.Evidence != nil {
			fmt.Fprintf(&sb, "      Evidence Bundle:   %d tokens (Truncated: %t)\n", r.Evidence.TokenEstimate, r.Evidence.Truncated)
		}
		fmt.Fprintf(&sb, "      Execution Time:    %d ms\n", r.ExecutionTimeMs)
		fmt.Fprintf(&sb, "      ADR Document:      %s: %s\n\n", r.ADR.ID, r.ADR.Title)
	}
	return sb.String()
}

// retroPorts builds the effects of a handoff (a seam for tests).
var retroPorts = ownership.BoardPorts

// newRetroHandoffCmd builds `dendrite retro handoff`: the explicit way a session that refuses an REA ticket
// hands it to AGY, as the project's [rea] directive says.
func newRetroHandoffCmd() *cobra.Command {
	var refused bool
	var reason, worktree string
	cmd := &cobra.Command{
		Use:   "handoff <ticket> --refused",
		Short: "Hand an REA ticket this session refuses to AGY (rule: the [rea] directive)",
		Long: "A Claude session that will not do an REA ticket runs this instead of leaving it half done. It records the\n" +
			"refusal on the ticket, starts an AGY job in an isolated working copy, labels the ticket owner:agy and\n" +
			"releases the Claude claim. A Claude session cannot claim an owner:agy ticket again without --steal.\n" +
			"Running it for a ticket AGY already owns changes nothing. The same handoff runs from the Stop hook when a\n" +
			"transcript ends in a refusal.",
		Example: "dendrite retro handoff DEN-1062 --refused --reason \"declined to analyse the binary\"",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !refused {
				return cli.UsageErrorf("--refused is required: handoff is only for an REA ticket this session refuses")
			}
			return planecli.PlaneRun(func(c *plane.Client) error {
				return runRetroHandoff(cmd.Context(), planecli.LifecycleIn(c, worktree), args[0], reason, cmd.OutOrStdout())
			})
		},
	}
	f := cmd.Flags()
	f.BoolVar(&refused, "refused", false, "This session refuses the ticket (required)")
	f.StringVar(&reason, "reason", "", "Why, in a sentence; kept on the ticket")
	f.StringVar(&worktree, "worktree", "", "Working copy of this session (default: the current directory)")
	return cmd
}

func runRetroHandoff(ctx context.Context, l *ticketlock.Lifecycle, ticket, reason string, out io.Writer) error {
	rule, err := ownership.LoadRule(l.Dir)
	if err != nil {
		return err
	}
	res, err := ownership.Transfer(ctx, retroPorts(l), rule.Rea, ticket, ownership.Refusal{Reason: "explicit", Excerpt: reason})
	if err != nil {
		return err
	}
	if cli.Globals.JSON {
		return cli.WriteJSON(out, res)
	}
	if res.AlreadyOwned {
		fmt.Fprintf(out, "[✓] %s is already owned by AGY: nothing to do\n", res.Ticket)
		return nil
	}
	fmt.Fprintf(out, "[✓] %s handed to AGY (job %s); the Claude claim is released\n", res.Ticket, res.JobID)
	return nil
}
