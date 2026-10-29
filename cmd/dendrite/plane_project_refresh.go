package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

type planeProjectRefreshOptions struct {
	Roots  []string
	Depth  int
	Prune  bool
	DryRun bool
}

func newChronosProjectRefreshCmd() *cobra.Command {
	var o planeProjectRefreshOptions
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Relocate or prune the entries of the global project index whose directory moved or vanished",
		Long: "Checks every entry of ~/.dendrite/projects.json (not only synapse members). A moved project is found\n" +
			"by its project ID, then its root commit, then a unique origin remote, under the IDE project folders and\n" +
			"--roots; its index entry, synapse path hints and pool files are rewritten. Entries that cannot be found\n" +
			"are reported, and removed with --prune. Several candidates are reported and never guessed.",
		Args:    noArgs,
		Example: "dendrite chronos project refresh --dry-run\ndendrite chronos project refresh --prune --roots ~/work",
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeProjectRefresh(o, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringSliceVar(&o.Roots, "roots", nil, "Extra directories to search (comma-separated)")
	f.IntVar(&o.Depth, "depth", 0, "Maximum search depth below each root")
	f.BoolVar(&o.Prune, "prune", false, "Remove entries whose project cannot be found")
	addDryRunFlag(cmd, &o.DryRun)
	return cmd
}

func planeProjectRefresh(o planeProjectRefreshOptions, out io.Writer) error {
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return err
	}
	ix, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return err
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	var roots []string
	for _, r := range o.Roots {
		roots = append(roots, expandHome(r))
	}
	rows, err := synapse.RefreshIndex(st, ix, synapse.RefreshOptions{Roots: roots, MaxDepth: o.Depth, Apply: !o.DryRun, Prune: o.Prune})
	if err != nil {
		return err
	}
	if globals.JSON {
		return writeJSON(out, rows)
	}

	counts := map[synapse.RefreshStatus]int{}
	for _, r := range rows {
		counts[r.Status]++
		switch r.Status {
		case synapse.RefreshOK:
		case synapse.RefreshRelocated:
			fmt.Fprintf(out, "[✓] %-32s %s -> %s (%s)\n", r.Name, r.OldPath, r.NewPath, r.Match)
		case synapse.RefreshPruned:
			fmt.Fprintf(out, "[-] %-32s pruned (was %s)\n", r.Name, r.OldPath)
		case synapse.RefreshAmbiguous:
			fmt.Fprintf(out, "[?] %-32s %d candidates, none applied:\n", r.Name, len(r.Candidates))
			for _, c := range r.Candidates {
				fmt.Fprintf(out, "      %s\n", filepath.Clean(c))
			}
		case synapse.RefreshMissing:
			fmt.Fprintf(out, "[-] %-32s not found (was %s)\n", r.Name, r.OldPath)
		}
	}
	fmt.Fprintf(out, "[i] %d ok, %d relocated, %d ambiguous, %d missing, %d pruned", counts[synapse.RefreshOK],
		counts[synapse.RefreshRelocated], counts[synapse.RefreshAmbiguous], counts[synapse.RefreshMissing], counts[synapse.RefreshPruned])
	if o.DryRun {
		fmt.Fprint(out, " (dry run: nothing was changed)")
	} else if counts[synapse.RefreshMissing] > 0 {
		fmt.Fprint(out, "; use --prune to drop the missing ones")
	}
	fmt.Fprintln(out)
	return nil
}

// refreshIndexPathOf repoints the project's index entry at root when the entry exists and points
// elsewhere (the repository moved). It never creates an entry, and failures are not fatal.
func refreshIndexPathOf(root string) {
	id, err := projectid.Read(root)
	if err != nil {
		return
	}
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return
	}
	ix, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return
	}
	if e, ok := ix.Lookup(id); ok && filepath.Clean(e.Path) != filepath.Clean(projectid.StateRoot(root)) {
		if _, err := ix.Register(root); err == nil {
			_ = ix.Save()
		}
	}
}

// warnStaleIndex prints one line when entries of the global project index point at directories
// that no longer exist. It is a cheap stat per entry and never fails the caller.
func warnStaleIndex(out io.Writer) {
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return
	}
	ix, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return
	}
	stale := 0
	for _, e := range ix.List() {
		if _, err := os.Stat(e.Path); err != nil {
			stale++
		}
	}
	if stale > 0 {
		fmt.Fprintf(out, "[!] %d project(s) in the global index point at paths that no longer exist; run `dendrite chronos project refresh --dry-run`\n", stale)
	}
}
