package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"Dendrite/pkg/tracker/plane"

	"github.com/spf13/cobra"
)

// chronosStatesOptions are the `chronos states reconcile` flags.
type chronosStatesOptions struct {
	Project string
	All     bool
	DryRun  bool
}

// newChronosStatesCmd builds `chronos states`: converge workflow states onto plane.CanonicalStates.
func newChronosStatesCmd() *cobra.Command {
	cmd := newGroupCmd("states", "Manage the project's workflow states", "")
	var o chronosStatesOptions
	reconcile := &cobra.Command{
		Use:     "reconcile",
		Aliases: []string{"ensure"},
		Short:   "Converge workflow states (names, colors, order) onto the canonical set",
		Args:    noArgs,
		Example: "dendrite chronos states reconcile\ndendrite chronos states reconcile --project DGIT --dry-run\ndendrite chronos states reconcile --all",
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosStatesReconcile(cmd.Context(), c, o, cmd.OutOrStdout())
			})
		},
	}
	addProjectFlag(reconcile, &o.Project, "", "Plane project UUID or identifier (default: the configured project)")
	reconcile.Flags().BoolVar(&o.All, "all", false, "Reconcile every project in the workspace")
	reconcile.Flags().BoolVar(&o.DryRun, "dry-run", false, "Show the changes without applying them")
	cmd.AddCommand(reconcile)
	return cmd
}

// runChronosStatesReconcile converges one project (or every workspace project with All)
// onto plane.CanonicalStates. A project that fails is reported and the rest still run.
func runChronosStatesReconcile(ctx context.Context, client *plane.Client, o chronosStatesOptions, out io.Writer) error {
	type target struct{ id, label string }
	var targets []target
	switch {
	case o.All:
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return fmt.Errorf("list Plane projects: %w", err)
		}
		for _, p := range projects {
			targets = append(targets, target{p.ID, p.Identifier})
		}
	case o.Project != "":
		// Accept identifiers like DGIT; fall back to treating the value as a raw UUID.
		if p, err := client.ResolveProject(ctx, o.Project); err == nil {
			targets = append(targets, target{p.ID, p.Identifier})
		} else {
			targets = append(targets, target{o.Project, o.Project})
		}
	default:
		targets = append(targets, target{client.ProjectID(), client.ProjectID()})
	}

	failed := 0
	for _, t := range targets {
		rep, err := client.ReconcileStates(ctx, t.id, o.DryRun)
		if err != nil {
			fmt.Fprintf(out, "[-] Failed to reconcile states for %s: %v\n", t.label, err)
			failed++
			continue
		}
		verb := "Reconciled"
		if o.DryRun {
			verb = "Would reconcile"
		}
		if len(rep.Changes) == 0 {
			fmt.Fprintf(out, "[✓] %s: states already canonical\n", t.label)
		} else {
			fmt.Fprintf(out, "[✓] %s states for %s:\n", verb, t.label)
			for _, ch := range rep.Changes {
				switch {
				case ch.Action == "create":
					fmt.Fprintf(out, "    + create %s\n", ch.Name)
				case ch.From != "":
					fmt.Fprintf(out, "    ~ %s -> %s (%s)\n", ch.From, ch.Name, strings.Join(ch.Fields, ", "))
				default:
					fmt.Fprintf(out, "    ~ %s (%s)\n", ch.Name, strings.Join(ch.Fields, ", "))
				}
			}
		}
		if len(rep.Unmanaged) > 0 {
			fmt.Fprintf(out, "    ? unmanaged (left untouched): %s\n", strings.Join(rep.Unmanaged, ", "))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d project(s) failed to reconcile", failed, len(targets))
	}
	return nil
}
