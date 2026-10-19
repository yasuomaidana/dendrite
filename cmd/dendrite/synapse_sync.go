package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"Dendrite/pkg/synapse"
)

// runSynapseSyncCommand implements `synapse status` (pull=false) and `synapse sync` (pull=true).
//
// status fetches every member and reports where it stands against its tracked ref without
// touching any checkout; sync additionally fast-forwards local members, resolves remote
// members, records each member's commit as its lock and applies the sync strategy
// (go.work / cargo-patch). Both persist the refreshed locks and lifecycle state.
func runSynapseSyncCommand(ctx context.Context, subArgs []string, out io.Writer, pull bool) error {
	opts := synapse.SyncOptions{Pull: pull, Link: pull, Merge: map[string]string{}}
	jsonMode := false
	var id string
	for i := 0; i < len(subArgs); i++ {
		arg := subArgs[i]
		switch {
		case arg == "--json":
			jsonMode = true
		case arg == "--no-fetch":
			opts.NoFetch = true
		case pull && arg == "--autostash":
			opts.Autostash = true
		case pull && arg == "--rebase":
			opts.Rebase = true
		case pull && arg == "--enforce":
			opts.Enforce = true
		case pull && arg == "--no-link":
			opts.Link = false
		case pull && (arg == "--merge" || strings.HasPrefix(arg, "--merge=")):
			val := strings.TrimPrefix(arg, "--merge=")
			if arg == "--merge" {
				if i+1 >= len(subArgs) {
					return errors.New("--merge requires <alias>=<branch>")
				}
				i++
				val = subArgs[i]
			}
			alias, branch, ok := strings.Cut(val, "=")
			if !ok || alias == "" || branch == "" {
				return fmt.Errorf("invalid --merge %q: expected <alias>=<branch>", val)
			}
			opts.Merge[alias] = branch
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag: %s", arg)
		case id == "":
			id = arg
		default:
			return fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	if id == "" {
		return errors.New("synapse id is required")
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

	if jsonMode {
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
