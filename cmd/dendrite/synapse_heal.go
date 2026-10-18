package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/synapseide"
)

// loadDefaultIndex opens the global project index (~/.dendrite/projects.json or $DENDRITE_HOME).
func loadDefaultIndex() (*projectid.Index, error) {
	path, err := projectid.DefaultIndexPath()
	if err != nil {
		return nil, err
	}
	return projectid.LoadIndex(path)
}

// splitList splits a comma-separated flag value, dropping empty items.
func splitList(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// runSynapseHeal executes `synapse heal [--auto] [--roots a,b] [--depth N] [--json]`.
func runSynapseHeal(ctx context.Context, subArgs []string, out io.Writer) error {
	ix, err := loadDefaultIndex()
	if err != nil {
		return err
	}
	opts := synapse.HealOptions{Index: ix}
	jsonMode := false
	for i := 0; i < len(subArgs); i++ {
		switch arg := subArgs[i]; {
		case arg == "--auto":
			opts.Apply = true
		case arg == "--json":
			jsonMode = true
		case arg == "--roots" || arg == "--depth":
			if i+1 >= len(subArgs) {
				return fmt.Errorf("flag %s requires an argument", arg)
			}
			i++
			if arg == "--roots" {
				opts.Roots = append(opts.Roots, splitList(subArgs[i])...)
				continue
			}
			if opts.MaxDepth, err = strconv.Atoi(subArgs[i]); err != nil || opts.MaxDepth < 1 {
				return fmt.Errorf("invalid --depth %q: expected a positive integer", subArgs[i])
			}
		default:
			return fmt.Errorf("unexpected argument: %s", arg)
		}
	}

	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	rows, err := synapse.Heal(st, opts)
	if err != nil {
		return err
	}
	if jsonMode {
		if rows == nil {
			rows = []synapse.HealRow{}
		}
		data, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "[+] No synapses defined.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SYNAPSE\tMEMBER\tSTATUS\tPATH")
	pending, broken := 0, 0
	for _, r := range rows {
		path := r.OldPath
		switch {
		case r.NewPath != "":
			path = r.OldPath + " -> " + r.NewPath
			if r.Applied {
				path += " (applied)"
			} else {
				pending++
			}
		case len(r.Candidates) > 0:
			path = r.OldPath + " ? " + strings.Join(r.Candidates, " | ")
			broken++
		case r.Status != synapse.StatusOK:
			broken++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Synapse, r.Alias, r.Status, path)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if pending > 0 {
		fmt.Fprintf(out, "[!] %d relocation(s) found; re-run with --auto to apply.\n", pending)
	}
	if broken > 0 {
		fmt.Fprintf(out, "[!] %d member(s) unresolved; pass --roots or use `dendrite synapse locate <alias> <dir>`.\n", broken)
	}
	return nil
}

// findMember resolves an alias or project ID to one member across all synapses.
// An alias bound to different projects in different synapses is ambiguous.
func findMember(st *synapse.Store, ref string) (synapse.Member, error) {
	all, err := st.List()
	if err != nil {
		return synapse.Member{}, err
	}
	var found *synapse.Member
	owners := map[string][]string{}
	for _, s := range all {
		if m, ok := s.Member(ref); ok {
			if found == nil {
				found = m
			}
			owners[m.ProjectID] = append(owners[m.ProjectID], s.ID)
		}
	}
	if found == nil {
		return synapse.Member{}, fmt.Errorf("no synapse member %q", ref)
	}
	if len(owners) > 1 {
		var parts []string
		for id, syns := range owners {
			parts = append(parts, fmt.Sprintf("%s in %s", id, strings.Join(syns, ",")))
		}
		sort.Strings(parts)
		return synapse.Member{}, fmt.Errorf("alias %q is ambiguous (%s); use the project id", ref, strings.Join(parts, "; "))
	}
	return *found, nil
}

// runSynapseLocate executes `synapse locate <alias|project-id> [hint-dir...] [--json]`.
func runSynapseLocate(ctx context.Context, subArgs []string, out io.Writer) error {
	var ref string
	var hints []string
	jsonMode := false
	for _, arg := range subArgs {
		switch {
		case arg == "--json":
			jsonMode = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag: %s", arg)
		case ref == "":
			ref = arg
		default:
			hints = append(hints, arg)
		}
	}
	if ref == "" {
		return errors.New("usage: dendrite synapse locate <alias|project-id> [hint-dir]")
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	m, err := findMember(st, ref)
	if err != nil {
		return err
	}
	ix, err := loadDefaultIndex()
	if err != nil {
		return err
	}
	loc := synapse.Locate(m, synapse.HealOptions{Index: ix}, hints...)
	if jsonMode {
		data, _ := json.MarshalIndent(loc, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}
	switch {
	case loc.Path != "":
		fmt.Fprintf(out, "[✓] %s (%s) found at %s via %s match\n", m.Alias, m.ProjectID, loc.Path, loc.Match)
	case len(loc.Candidates) > 0:
		fmt.Fprintf(out, "[?] %s: %d candidate(s) by %s, not applied:\n", m.Alias, len(loc.Candidates), loc.Match)
		for _, c := range loc.Candidates {
			fmt.Fprintf(out, "    - %s\n", c)
		}
	default:
		return fmt.Errorf("%s (%s) not found; pass a hint directory", m.Alias, m.ProjectID)
	}
	return nil
}

// runSynapseIDESync executes `synapse ide-sync <id> [--binary path]`.
func runSynapseIDESync(ctx context.Context, subArgs []string, out io.Writer) error {
	var id, binary string
	for i := 0; i < len(subArgs); i++ {
		switch arg := subArgs[i]; {
		case arg == "--binary":
			if i+1 >= len(subArgs) {
				return errors.New("flag --binary requires an argument")
			}
			i++
			binary = subArgs[i]
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag: %s", arg)
		case id == "":
			id = arg
		default:
			return fmt.Errorf("unexpected argument: %s", arg)
		}
	}
	if id == "" {
		return errors.New("usage: dendrite synapse ide-sync <id> [--binary path]")
	}
	if binary == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		binary = exe
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	s, err := st.Load(id)
	if err != nil {
		return fmt.Errorf("synapse %q: %w", id, err)
	}
	written, skipped, err := synapseide.SyncSynapseIDE(s, binary)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Registered MCP server %s and IDE scope %s\n", synapseide.SynapseServerName(id), synapseide.SynapseScopeName(id))
	for _, f := range written {
		fmt.Fprintf(out, "    + %s\n", f)
	}
	for _, a := range skipped {
		fmt.Fprintf(out, "[!] skipped unresolved member %s (run `dendrite synapse heal`)\n", a)
	}
	return nil
}

// runSynapseMove executes `synapse move <alias|project-id> <new-path> [--json]`.
func runSynapseMove(ctx context.Context, subArgs []string, out io.Writer) error {
	var positional []string
	jsonMode := false
	for _, arg := range subArgs {
		switch {
		case arg == "--json":
			jsonMode = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag: %s", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return errors.New("usage: dendrite synapse move <alias|project-id> <new-path>")
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	ix, err := loadDefaultIndex()
	if err != nil {
		return err
	}
	res, err := synapse.Move(st, positional[0], positional[1], synapse.MoveOptions{Index: ix})
	if err != nil {
		return err
	}
	if jsonMode {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(out, string(data))
		return nil
	}
	fmt.Fprintf(out, "[✓] Moved %s -> %s\n", res.OldPath, res.NewPath)
	if res.CrossDevice {
		fmt.Fprintln(out, "    (cross-device: copied then removed the source)")
	}
	fmt.Fprintf(out, "    Rebound synapses: %s\n", strings.Join(res.Synapses, ", "))
	return nil
}
