package soma

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/repair"
	"Dendrite/pkg/workcopy"
)

// withLedger runs fn on the repair ledger of the Dendrite home.
func withLedger(fn func(l *repair.Ledger) error) error {
	l, err := repair.Open()
	if err != nil {
		return err
	}
	return fn(l)
}

func newRepairStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"list", "ls"},
		Short:   "Show every repair chain: who uses which repair branch, its pull request and its stage",
		Args:    cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withLedger(func(l *repair.Ledger) error {
				links, err := l.Links()
				if err != nil {
					return err
				}
				return writeRepairTree(cmd.OutOrStdout(), repair.Tree(links))
			})
		},
	}
}

func writeRepairTree(out io.Writer, rows []repair.Row) error {
	if cli.Globals.JSON {
		if rows == nil {
			rows = []repair.Row{}
		}
		return cli.WriteJSON(out, rows)
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "[✓] No repairs in progress")
		return nil
	}
	for _, r := range rows {
		k := r.Link
		indent := strings.Repeat("    ", r.Depth)
		if r.Depth == 0 {
			fmt.Fprintf(out, "%s  (%s)\n", filepath.Base(k.Consumer), k.Ticket)
		}
		fmt.Fprintf(out, "%s  └─ %s  [%s]  %s\n", indent, k.Dependency, k.Stage, repairWhere(k))
		if k.PullRequest != "" {
			fmt.Fprintf(out, "%s       pull request: %s\n", indent, k.PullRequest)
		}
	}
	return nil
}

// repairWhere says what the consumer of a link points at in its stage.
func repairWhere(k repair.Link) string {
	switch k.Stage {
	case repair.StageMerged:
		return "pinned to " + k.Upstream + " at " + k.MergeSHA
	case repair.StageReleased:
		return "released as " + k.Version
	}
	return "branch " + k.Branch + " in " + k.Copy
}

func newRepairPRCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pr",
		Short: "Open the pull request of every repair that has none, the bottom of each chain first",
		Args:  cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withLedger(func(l *repair.Ledger) error {
				opened, err := repair.Propose(cmd.Context(), l)
				out := cmd.OutOrStdout()
				for _, k := range opened {
					fmt.Fprintf(out, "[✓] %s: %s\n", k.ID, k.PullRequest)
				}
				if err == nil && len(opened) == 0 {
					fmt.Fprintln(out, "[✓] Every repair has its pull request")
				}
				return err
			})
		},
	}
}

func newRepairAdvanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "advance",
		Short: "Move each repair on: pin the consumer to the merge commit once merged, restore it once released",
		Long: "repairing -> merged: the pull request was merged; the consumer is pinned to the dependency's repository at\n" +
			"the merge commit. merged -> released: a tag holds that commit; a consumer that used a published version gets\n" +
			"the release, any other gets back exactly what it had. A repair waits for the repairs below it to be merged.",
		Args: cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withLedger(func(l *repair.Ledger) error {
				steps, err := repair.Advance(cmd.Context(), l)
				if err != nil {
					return err
				}
				return writeRepairSteps(cmd.OutOrStdout(), steps)
			})
		},
	}
}

// writeRepairSteps prints one line per step.
func writeRepairSteps(out io.Writer, steps []repair.Step) error {
	if cli.Globals.JSON {
		if steps == nil {
			steps = []repair.Step{}
		}
		return cli.WriteJSON(out, steps)
	}
	if len(steps) == 0 {
		fmt.Fprintln(out, "[✓] No repairs to advance")
	}
	for _, s := range steps {
		fmt.Fprintln(out, s.Line())
	}
	return nil
}

func newRepairDropCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drop <id>",
		Short: "Forget a repair (e.g. DEN-890/embedag); manifests, branches and copies are left as they are",
		Args:  cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withLedger(func(l *repair.Ledger) error {
				if _, err := l.Get(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Dropped %s\n", args[0])
				return l.Remove(args[0])
			})
		},
	}
}

// repairsOf lists the repairs whose consumer is the project at dir.
func repairsOf(dir string) []repair.Link {
	var mine []repair.Link
	_ = withLedger(func(l *repair.Ledger) error {
		links, err := l.Links()
		abs, _ := filepath.Abs(dir)
		for _, k := range links {
			if k.Consumer == workcopy.Canon(abs) {
				mine = append(mine, k)
			}
		}
		return err
	})
	return mine
}
