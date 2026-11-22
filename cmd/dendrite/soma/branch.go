package soma

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/planecli"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/featurebranch"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/ticketparent"
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
	var clearParent bool
	parent := &cobra.Command{
		Use:   "parent <ticket> <parent> | parent <ticket> --clear",
		Short: "Make <parent> the parent of <ticket> in Plane, so their work shares the parent's feature branch; --clear removes the parent",
		Args:  cobra.RangeArgs(1, 2),
		Example: "dendrite branch parent DEN-841 DEN-840\n" +
			"dendrite branch parent DEN-841 --clear",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clearParent != (len(args) == 1) {
				return cli.UsageErrorf("give either <ticket> <parent> or <ticket> --clear")
			}
			return planecli.PlaneRun(func(c *plane.Client) error {
				if clearParent {
					return runBranchClearParent(cmd.Context(), c, args[0], cmd.OutOrStdout())
				}
				return runBranchParent(cmd.Context(), c, args[0], args[1], cmd.OutOrStdout())
			})
		},
	}
	parent.Flags().BoolVar(&clearParent, "clear", false, "Remove the parent of <ticket> instead of setting one")
	cmd.AddCommand(show, parent, newBranchRetireCmd())
	return cmd
}

func writeBranchTarget(out io.Writer, t branchpolicy.Target, plan featurebranch.Plan) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, map[string]any{"target": t, "plan": plan, "repairs": repairsOf(".")})
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
	for _, k := range repairsOf(".") {
		fmt.Fprintf(out, "Repair:             %s  [%s]  %s\n", k.Dependency, k.Stage, repairWhere(k))
	}
	return nil
}

// runBranchParent sets the Plane parent of child; a loop is a usage error.
func runBranchParent(ctx context.Context, client *plane.Client, child, parent string, out io.Writer) error {
	board := ticketlock.Board{Svc: planesvc.Default(), Client: client}
	if err := ticketparent.Set(ctx, board, child, parent); err != nil {
		if errors.Is(err, ticketparent.ErrLoop) {
			return cli.UsageErrorf("%v", err)
		}
		return err
	}
	fmt.Fprintf(out, "[✓] %s is now a sub-ticket of %s\n", strings.ToUpper(strings.TrimSpace(child)), strings.ToUpper(strings.TrimSpace(parent)))
	return nil
}

// runBranchClearParent removes the Plane parent of child; a ticket with no parent is reported and left alone.
func runBranchClearParent(ctx context.Context, client *plane.Client, child string, out io.Writer) error {
	child = strings.ToUpper(strings.TrimSpace(child))
	old, had, err := ticketparent.Clear(ctx, ticketlock.Board{Svc: planesvc.Default(), Client: client}, child)
	if err != nil {
		return err
	}
	if !had {
		fmt.Fprintf(out, "[i] %s has no parent\n", child)
		return nil
	}
	fmt.Fprintf(out, "[✓] %s is no longer a sub-ticket of %s\n", child, old.ID)
	return nil
}
