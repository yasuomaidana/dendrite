package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"Dendrite/pkg/synapse"
)

// synapseSyncOptions are the `synapse status` / `synapse sync` flags.
type synapseSyncOptions struct {
	NoFetch   bool
	Autostash bool
	Rebase    bool
	Enforce   bool
	NoLink    bool
	Merge     []string
}

// newSynapseStatusCmd builds `synapse status`: fetch every member and report where it stands.
func newSynapseStatusCmd() *cobra.Command {
	var o synapseSyncOptions
	cmd := &cobra.Command{
		Use:     "status <id>",
		Short:   "Fetch every member and report ahead/behind, dirty files and drift against its tracked ref",
		Args:    exactArgs(1),
		Example: "dendrite synapse status dendrite-ecosystem\ndendrite synapse status dendrite-ecosystem --no-fetch --json",
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseSync(c.Context(), args[0], o, false, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&o.NoFetch, "no-fetch", false, "Use the last fetched remote state")
	return cmd
}

// newSynapseSyncCmd builds `synapse sync`: bring every member to its tracked ref and relink.
func newSynapseSyncCmd() *cobra.Command {
	var o synapseSyncOptions
	cmd := &cobra.Command{
		Use:   "sync <id>",
		Short: "Fast-forward members to their tracked ref, lock their commits and relink (go.work / cargo patches)",
		Long: "Fast-forwards every local member that is on its tracked branch, resolves remote members,\n" +
			"records each member's commit as its lock and applies the sync strategy. --enforce also\n" +
			"switches clean members that sit on another branch back to their tracked branch.",
		Args:    exactArgs(1),
		Example: "dendrite synapse sync dendrite-ecosystem\ndendrite synapse sync dendrite-ecosystem --enforce\ndendrite synapse sync dendrite-ecosystem --merge mcp-hub=feat/x",
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseSync(c.Context(), args[0], o, true, c.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.NoFetch, "no-fetch", false, "Use the last fetched remote state")
	f.BoolVar(&o.Autostash, "autostash", false, "Stash uncommitted changes around member fast-forwards")
	f.BoolVar(&o.Rebase, "rebase", false, "Replay local commits with rebase instead of fast-forward only")
	f.BoolVar(&o.Enforce, "enforce", false, "Switch clean members on another branch to their tracked branch")
	f.BoolVar(&o.NoLink, "no-link", false, "Do not apply the sync strategy")
	f.StringArrayVar(&o.Merge, "merge", nil, "Merge <alias>=<branch> into that member after updating it (repeatable)")
	return cmd
}

// runSynapseSync implements `synapse status` (pull=false) and `synapse sync` (pull=true).
//
// status fetches every member and reports where it stands against its tracked ref without
// touching any checkout; sync additionally fast-forwards local members, resolves remote
// members, records each member's commit as its lock and applies the sync strategy
// (go.work / cargo-patch). Both persist the refreshed locks and lifecycle state.
func runSynapseSync(ctx context.Context, id string, o synapseSyncOptions, pull bool, out io.Writer) error {
	opts := synapse.SyncOptions{
		Pull: pull, Link: pull && !o.NoLink, NoFetch: o.NoFetch,
		Autostash: o.Autostash, Rebase: o.Rebase, Enforce: o.Enforce, Merge: map[string]string{},
	}
	for _, val := range o.Merge {
		alias, branch, ok := strings.Cut(val, "=")
		if !ok || alias == "" || branch == "" {
			return usageErrorf("invalid --merge %q: expected <alias>=<branch>", val)
		}
		opts.Merge[alias] = branch
	}

	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	s, err := st.Load(id)
	if err != nil {
		return fmt.Errorf("synapse %q: %w", id, err)
	}
	for alias := range opts.Merge {
		if _, ok := s.Member(alias); !ok {
			return fmt.Errorf("--merge: synapse %q has no member %q", id, alias)
		}
	}

	rep := synapse.SyncSynapse(ctx, s, opts)
	if rep.Changed {
		if err := st.Save(s); err != nil {
			return fmt.Errorf("saving synapse %s: %w", s.ID, err)
		}
	}

	if globals.JSON {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
		return nil
	}
	printSynapseSyncReport(out, rep)
	return nil
}

func printSynapseSyncReport(out io.Writer, rep *synapse.SyncReport) {
	fmt.Fprintf(out, "[+] Synapse %s: %s\n", rep.Synapse, rep.State)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MEMBER\tSOURCE\tTRACKING\tHEAD\tAHEAD/BEHIND\tDIRTY\tSTATUS")
	for _, m := range rep.Members {
		tracking, head, ab, dirty := m.Ref, short(m.LockedSHA), "-", "-"
		status := "ok"
		if r := m.Repo; r != nil {
			if r.Target != "" {
				tracking = r.Target
			}
			ab = fmt.Sprintf("%d/%d", r.Ahead, r.Behind)
			dirty = fmt.Sprintf("%d", r.DirtyFiles)
			switch {
			case r.Switched != "" && r.Merged != "":
				status = "switched from " + r.Switched + ", merged " + r.Merged
			case r.Switched != "":
				status = "switched from " + r.Switched
			case r.Merged != "":
				status = "merged " + r.Merged
			case r.Updated:
				status = "updated"
			}
		}
		if tracking == "" {
			tracking = "-"
		}
		if len(m.Drift) > 0 {
			status = strings.Join(m.Drift, "; ")
		}
		if m.Error != "" {
			status = "error: " + m.Error
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", m.Alias, m.Source, tracking, head, ab, dirty, status)
	}
	_ = tw.Flush()

	for _, m := range rep.Members {
		if m.Repo == nil {
			continue
		}
		for _, n := range m.Repo.Notes {
			fmt.Fprintf(out, "    [i] %s: %s\n", m.Alias, n)
		}
		if len(m.Repo.Pending) > 0 {
			fmt.Fprintf(out, "    [i] %s: branches with changes not in this checkout:\n", m.Alias)
			for _, p := range m.Repo.Pending {
				fmt.Fprintf(out, "        - %s (%d commit(s)) -> merge with: dendrite synapse sync %s --merge %s=%s\n",
					p.Ref, p.Commits, rep.Synapse, m.Alias, strings.TrimPrefix(p.Ref, "origin/"))
			}
		}
	}
	for _, l := range rep.Links {
		line := fmt.Sprintf("    [%s] %s %s", l.Action, l.Strategy, l.File)
		if l.File == "" {
			line = fmt.Sprintf("    [%s] %s", l.Action, l.Strategy)
		}
		if len(l.Uses) > 0 {
			line += " (local: " + strings.Join(l.Uses, ", ") + ")"
		}
		if l.Reason != "" {
			line += " - " + l.Reason
		}
		fmt.Fprintln(out, line)
	}
}

func short(sha string) string {
	if sha == "" {
		return "-"
	}
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
