package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/planecli"
	"Dendrite/pkg/rea/ownership"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

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
