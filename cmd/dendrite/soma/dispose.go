package soma

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/dispose"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/repair"
	"Dendrite/pkg/synapse"
)

// disposal says how `work dispose` runs.
type disposal struct {
	// DryRun lists and deletes nothing; Yes deletes without asking.
	DryRun, Yes bool
	// Interactive says a person can answer the questions.
	Interactive bool
	// Quiet is for the offer other commands make: nothing is printed unless something can be deleted, and the
	// network is not asked.
	Quiet bool
}

func newWorkDisposeCmd() *cobra.Command {
	var d disposal
	cmd := &cobra.Command{
		Use:   "dispose",
		Short: "Ask to delete what merged work left behind: repair copies and branches, finished features, a retired develop",
		Long: "Lists the working copies and remote branches of repairs whose fix is merged, the branches of features whose\n" +
			"trunk is merged into the integration branch, and a retired develop, then asks once per group before deleting.\n" +
			"Nothing with uncommitted, unpushed or unmerged work is deleted, and neither is a fork: those are listed as kept.",
		Example: "dendrite work dispose\ndendrite work dispose --dry-run\ndendrite work dispose --yes",
		Args:    cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d.Yes = d.Yes || cli.Globals.Yes
			d.Interactive = cli.IsTerminal(cmd.InOrStdin())
			return runWorkDispose(cmd.Context(), d, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cli.AddDryRunFlag(cmd, &d.DryRun)
	cmd.Flags().BoolVar(&d.Yes, "yes", false, "Delete every disposable item without asking")
	return cmd
}

// trackedBy says which synapse still follows branch for the project in the current directory, so that deleting
// it would break `dendrite sync`; "" when none does.
func trackedBy(branch string) string {
	id, err := projectid.Read(projectid.StateRoot("."))
	st, stErr := synapse.DefaultStore()
	if err != nil || stErr != nil {
		return ""
	}
	syns, _ := st.ReferencingProject(id)
	for _, s := range syns {
		m, _ := s.Member(id)
		if s.State != synapse.StateArchived && m.TrackedRef(s.Channel) == branch {
			return fmt.Sprintf("synapse %s still tracks it for this project: set the member's branch to %s first", s.ID, branchpolicy.Resolve(".", branchpolicy.Override{}).Integration)
		}
	}
	return ""
}

func runWorkDispose(ctx context.Context, d disposal, in io.Reader, out io.Writer) error {
	ledger, err := repair.Open()
	if err != nil {
		return err
	}
	items, err := dispose.Find(ctx, ledger, dispose.Options{Dir: ".", Offline: d.Quiet, Tracked: trackedBy})
	if err != nil {
		return err
	}
	if cli.Globals.JSON && !d.Quiet {
		return cli.WriteJSON(out, append([]dispose.Item{}, items...))
	}
	groups := dispose.Grouped(items)
	if d.Quiet {
		groups = withDisposable(groups)
	}
	if len(groups) == 0 && !d.Quiet {
		fmt.Fprintln(out, "[✓] Nothing to dispose of")
	}
	d.offer(ctx, groups, in, out)
	return nil
}

// offer lists each group and deletes the ones that are confirmed.
func (d disposal) offer(ctx context.Context, groups []dispose.Group, in io.Reader, out io.Writer) {
	answers := bufio.NewReader(in)
	offered := 0
	for _, g := range groups {
		printDisposalGroup(out, g)
		offered += len(g.Disposable)
		if len(g.Disposable) > 0 && d.confirms(answers, out, g) {
			deleteDisposalGroup(ctx, out, g)
		}
	}
	if offered > 0 && !d.DryRun && !d.Yes && !d.Interactive {
		fmt.Fprintln(out, "[i] Nothing was deleted: run `dendrite work dispose` in a terminal to choose, or `dendrite work dispose --yes`.")
	}
}

// withDisposable keeps the groups that have something to delete.
func withDisposable(groups []dispose.Group) []dispose.Group {
	var out []dispose.Group
	for _, g := range groups {
		if len(g.Disposable) > 0 {
			out = append(out, g)
		}
	}
	return out
}

func printDisposalGroup(out io.Writer, g dispose.Group) {
	fmt.Fprintf(out, "[+] %s (%d disposable, %d kept):\n", g.Kind, len(g.Disposable), len(g.Kept))
	for _, it := range g.Disposable {
		fmt.Fprintf(out, "    - %s  (%s)\n", it.Name, it.Why)
	}
	for _, it := range g.Kept {
		fmt.Fprintf(out, "    = %s  kept: %s\n", it.Name, it.Kept)
	}
}

// confirms reports whether the group is to be deleted: never on a dry run, always with --yes, otherwise only
// when a person answers yes.
func (d disposal) confirms(answers *bufio.Reader, out io.Writer, g dispose.Group) bool {
	if d.DryRun || d.Yes || !d.Interactive {
		return d.Yes && !d.DryRun
	}
	fmt.Fprintf(out, "[?] Delete these %d (%s)? [y/N] ", len(g.Disposable), g.Kind)
	answer, _ := answers.ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(answer))
	if a != "y" && a != "yes" {
		fmt.Fprintln(out, "[i] Left as they are.")
		return false
	}
	return true
}

func deleteDisposalGroup(ctx context.Context, out io.Writer, g dispose.Group) {
	for _, it := range g.Disposable {
		if err := it.Delete(ctx); err != nil {
			fmt.Fprintf(out, "[!] %s not deleted: %v\n", it.Name, err)
			continue
		}
		fmt.Fprintf(out, "[✓] deleted %s\n", it.Name)
	}
}

// OfferDisposal is the offer other commands make after their own work: when merged work left something behind
// it is listed, and a person at a terminal is asked whether to delete it. It never fails the caller.
func OfferDisposal(ctx context.Context, out io.Writer) {
	if cli.Globals.JSON {
		return // the caller's output is JSON: an offer in prose would break it
	}
	d := disposal{Quiet: true, Interactive: cli.IsTerminal(os.Stdin)}
	if err := runWorkDispose(ctx, d, os.Stdin, out); err != nil {
		fmt.Fprintf(out, "[!] Disposal not offered: %v\n", err)
	}
}
