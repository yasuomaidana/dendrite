package main

import (
	"Dendrite/pkg/devtool"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/workcopy"

	"github.com/spf13/cobra"
)

// newWorkManager is a seam so tests can point the CLI at a temporary repository.
var newWorkManager = func() *workcopy.Manager { return workcopy.New(".") }

// newWorkCmd builds `dendrite work`: model-owned working copies in dendrite-work/.
func newWorkCmd() *cobra.Command {
	cmd := newGroupCmd("work", "Manage model-owned working copies in dendrite-work/ (create, list, remove, auto-clean merged)", "soma",
		"workcopy", "wc")
	cmd.Long = "Model-owned working copies live in dendrite-work/ next to the main checkout ($DENDRITE_WORK_DIR overrides)."
	cmd.Example = "dendrite work new DEN-123\n" +
		"dendrite work list --json\n" +
		"dendrite work done DEN-123 --force\n" +
		"dendrite work gc --dry-run\n" +
		"dendrite work adopt"

	var branch, base string
	var noTicketGuard bool
	newCmd := &cobra.Command{
		Use:     "new <name>",
		Aliases: []string{"create"},
		Short:   "Create an isolated working copy (branch agent/<name>)",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkNew(cmd.Context(), args[0], branch, base, noTicketGuard, cmd.OutOrStdout())
		},
	}
	newCmd.Flags().BoolVar(&noTicketGuard, "no-ticket-guard", false, "Do not check or start the ticket in Plane (env "+devtool.TicketGuardEnv+"=off)")
	newCmd.Flags().StringVar(&branch, "branch", "", "Branch to create (default agent/<name>)")
	newCmd.Flags().StringVar(&base, "base", "", "Commit-ish to start from (default HEAD)")

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List working copies and their cleanup status",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkList(cmd.Context(), cmd.OutOrStdout())
		},
	}

	var force bool
	done := &cobra.Command{
		Use:     "done <name|path>",
		Aliases: []string{"remove", "rm"},
		Short:   "Remove a copy (refuses uncommitted/unpushed work unless --force)",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkDone(cmd.Context(), args[0], force, cmd.OutOrStdout())
		},
	}
	done.Flags().BoolVar(&force, "force", false, "Remove even with uncommitted or unpushed work, and delete the branch")

	var gcDryRun bool
	gc := &cobra.Command{
		Use:     "gc",
		Aliases: []string{"prune"},
		Short:   "Remove copies that are merged (or whose upstream is gone) and clean",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkGC(cmd.Context(), gcDryRun, cmd.OutOrStdout())
		},
	}
	addDryRunFlag(gc, &gcDryRun)

	var adoptDryRun bool
	adopt := &cobra.Command{
		Use:   "adopt",
		Short: "Move this repo's worktrees that live outside dendrite-work/ into it",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkAdopt(cmd.Context(), adoptDryRun, cmd.OutOrStdout())
		},
	}
	addDryRunFlag(adopt, &adoptDryRun)

	cmd.AddCommand(newCmd, list, done, gc, adopt)
	return cmd
}

// ticketShaped matches names like DEN-123 or KAKEBO-12.
var ticketShaped = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+$`)

func runWorkNew(ctx context.Context, name, branch, base string, noTicketGuard bool, out io.Writer) error {
	// A working copy named after a ticket starts that ticket: it must exist, and it moves to In Progress
	// before any code is written, so the board shows the work from its first minute.
	if ticketShaped.MatchString(name) && !noTicketGuard && devtool.GuardEnabled() {
		g, err := devtool.EnsureTicketInProgress(ctx, "", name, devtool.GuardStart, false)
		if err != nil {
			return err
		}
		switch {
		case g.Note != "":
			fmt.Fprintf(out, "[✓] %s\n", g.Note)
		case g.Skipped != "":
			fmt.Fprintf(out, "[!] ticket %s not checked: %s\n", name, g.Skipped)
		}
	}
	path, err := newWorkManager().Create(ctx, name, branch, base)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Working copy created: %s\n", path)
	if ticketShaped.MatchString(name) && !noTicketGuard && devtool.GuardEnabled() {
		claimInCopy(ctx, name, path, out)
	}
	fmt.Fprintf(out, "    cd %s\n", path)
	return nil
}

// claimInCopy claims the ticket for the new working copy, which opens the edit lock there. The copy already
// exists, so a failure is a warning that says how to claim by hand, not an error.
func claimInCopy(ctx context.Context, ticket, path string, out io.Writer) {
	client, err := newPlaneClient()
	if err == nil {
		var res ticketlock.ClaimResult
		if res, err = lifecycleIn(client, path).Claim(ctx, ticket, ticketlock.ClaimOptions{}); err == nil {
			fmt.Fprintf(out, "[✓] Claimed %s for this copy (session %s)\n", res.Ticket, res.SessionID)
			return
		}
	}
	fmt.Fprintf(out, "[!] %s is not claimed: %v\n    claim it before editing: dendrite chronos claim %s --worktree %s\n", ticket, err, ticket, path)
}

func runWorkList(ctx context.Context, out io.Writer) error {
	m := newWorkManager()
	copies, err := m.List(ctx)
	if err != nil {
		return err
	}
	if globals.JSON {
		if copies == nil {
			copies = []workcopy.Copy{}
		}
		return writeJSON(out, copies)
	}

	root, err := m.Root(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[+] Working copies in %s (%d):\n", root, len(copies))
	for _, c := range copies {
		fmt.Fprintf(out, "  - %-40s %-30s %s\n", c.Path, c.Branch, copyStatus(c))
	}
	return nil
}

func copyStatus(c workcopy.Copy) string {
	var parts []string
	if c.Story {
		parts = append(parts, "story")
	}
	if c.Dirty {
		parts = append(parts, "dirty")
	}
	if c.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d unpushed", c.Ahead))
	}
	if c.Untouched {
		parts = append(parts, "no commits")
	}
	if c.MergedInto != "" {
		parts = append(parts, "merged into "+c.MergedInto)
	}
	if c.UpstreamGone {
		parts = append(parts, "upstream gone")
	}
	if len(parts) == 0 {
		return "active"
	}
	return strings.Join(parts, ", ")
}

func runWorkDone(ctx context.Context, target string, force bool, out io.Writer) error {
	report, err := newWorkManager().Remove(ctx, target, workcopy.RemoveOptions{Force: force})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] %s %s (%s)\n", report.Action, report.Path, report.Reason)
	return nil
}

func runWorkGC(ctx context.Context, dryRun bool, out io.Writer) error {
	reports, err := newWorkManager().GC(ctx, workcopy.GCOptions{
		DryRun: dryRun,
		InUse:  agentjobs.NewLedger("").WorktreeInUse(),
	})
	if err != nil {
		return err
	}
	printWorkReports(out, reports, true)
	return nil
}

func runWorkAdopt(ctx context.Context, dryRun bool, out io.Writer) error {
	reports, err := newWorkManager().Adopt(ctx, workcopy.GCOptions{
		DryRun: dryRun,
		InUse:  agentjobs.NewLedger("").WorktreeInUse(),
	})
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		fmt.Fprintln(out, "[✓] No worktrees outside dendrite-work/ to adopt")
		return nil
	}
	printWorkReports(out, reports, true)
	return nil
}

// printWorkReports prints one line per report; with verbose=false only removals are shown.
func printWorkReports(out io.Writer, reports []workcopy.Report, verbose bool) {
	for _, r := range reports {
		if !verbose && r.Action != "removed" {
			continue
		}
		mark := "[✓]"
		if r.Action == "kept" {
			mark = "[=]"
		}
		fmt.Fprintf(out, "%s %s %s (%s)\n", mark, r.Action, r.Path, r.Reason)
	}
}

// autoCleanWorkCopies removes merged, clean working copies after a session
// command. It never fails the caller: problems are printed as warnings.
func autoCleanWorkCopies(ctx context.Context, out io.Writer) {
	reports, err := newWorkManager().GC(ctx, workcopy.GCOptions{InUse: agentjobs.NewLedger("").WorktreeInUse()})
	if err != nil {
		fmt.Fprintf(out, "[!] Working-copy cleanup skipped: %v\n", err)
		return
	}
	printWorkReports(out, reports, false)
}
