package soma

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/planecli"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/featurebranch"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

// NewBranchCmd builds `dendrite branch`.
func NewBranchCmd() *cobra.Command {
	cmd := cli.NewGroupCmd("branch", "Which branch a ticket is worked on and where it merges: the integration branch, a parent ticket's trunk, a fork", "soma")
	var ticket string
	show := &cobra.Command{
		Use:   "show",
		Short: "Explain the branch a ticket is worked on, the branch it merges into, and where that came from",
		Args:  cli.NoArgs,
		Example: "dendrite branch show\n" +
			"dendrite branch show --ticket DEN-843",
		RunE: func(cmd *cobra.Command, args []string) error {
			if ticket == "" {
				return writeBranchTarget(cmd.OutOrStdout(), branchpolicy.For("."), featurebranch.Plan{})
			}
			return planecli.PlaneRun(func(c *plane.Client) error {
				plan, err := featurebranch.Resolve(cmd.Context(), ".", ticket, ticketlock.Board{Svc: planesvc.Default(), Client: c})
				if err != nil {
					return err
				}
				t := branchpolicy.For(".")
				t.Branch = plan.Target
				if plan.Feature {
					t.Source = branchpolicy.SourceTicket
				}
				return writeBranchTarget(cmd.OutOrStdout(), t, plan)
			})
		},
	}
	show.Flags().StringVar(&ticket, "ticket", "", "Explain the target for this ticket (asks Plane for its parent)")
	parent := &cobra.Command{
		Use:     "parent <ticket> <parent>",
		Short:   "Make <parent> the parent of <ticket> in Plane, so their work shares the parent's feature branch",
		Args:    cli.ExactArgs(2),
		Example: "dendrite branch parent DEN-841 DEN-840",
		RunE: func(cmd *cobra.Command, args []string) error {
			return planecli.PlaneRun(func(c *plane.Client) error {
				return runBranchParent(cmd.Context(), c, args[0], args[1], cmd.OutOrStdout())
			})
		},
	}
	cmd.AddCommand(show, parent)
	return cmd
}

func writeBranchTarget(out io.Writer, t branchpolicy.Target, plan featurebranch.Plan) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, map[string]any{"target": t, "plan": plan})
	}
	if plan.Branch != "" {
		fmt.Fprintf(out, "Work branch:        %s\n", plan.Branch)
	}
	fmt.Fprintf(out, "Merges into:        %s  (%s)\n", t.Branch, t.Source)
	fmt.Fprintf(out, "Integration branch: %s\n", t.Integration)
	fmt.Fprintf(out, "Target remote:      %s  (pushes to %s)\n", t.Remote, t.PushRemote)
	fmt.Fprintf(out, "Feature prefix:     %s\n", t.FeaturePrefix)
	if plan.Feature {
		fmt.Fprintf(out, "Feature of:         %s  (%s)\n", plan.Parent.ID, plan.Parent.Title)
	}
	return nil
}

// runBranchParent sets the Plane parent of child. A parent that is the child itself or one of its descendants
// would make a loop, so it is refused.
func runBranchParent(ctx context.Context, client *plane.Client, child, parent string, out io.Writer) error {
	child, parent = strings.ToUpper(strings.TrimSpace(child)), strings.ToUpper(strings.TrimSpace(parent))
	if child == parent {
		return cli.UsageErrorf("a ticket cannot be its own parent")
	}
	board := ticketlock.Board{Svc: planesvc.Default(), Client: client}
	for at, depth := parent, 0; depth < 8; depth++ {
		up, ok, err := board.Parent(ctx, at)
		if err != nil || !ok {
			break
		}
		if up.ID == child {
			return cli.UsageErrorf("%s is below %s already; making it the parent would make a loop", parent, child)
		}
		at = up.ID
	}
	p, err := planesvc.Default().GetIssue(ctx, client, parent)
	if err != nil {
		return fmt.Errorf("parent %s: %w", parent, err)
	}
	if _, err := planesvc.Default().UpdateIssue(ctx, client, child, plane.UpdateIssueRequest{Parent: &p.ID}); err != nil {
		return fmt.Errorf("set the parent of %s: %w", child, err)
	}
	fmt.Fprintf(out, "[✓] %s is now a sub-ticket of %s\n", child, parent)
	return nil
}
