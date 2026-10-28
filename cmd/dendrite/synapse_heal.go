package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
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
// synapseHealOptions holds the flags of `synapse heal`.
type synapseHealOptions struct {
	Auto  bool
	Roots []string
	Depth int
}

func runSynapseHeal(ctx context.Context, o synapseHealOptions, jsonMode bool, out io.Writer) error {
	if o.Depth < 0 {
		return usageErrorf("invalid --depth %d: expected a positive integer", o.Depth)
	}
	ix, err := loadDefaultIndex()
	if err != nil {
		return err
	}
	opts := synapse.HealOptions{Index: ix, Apply: o.Auto, MaxDepth: o.Depth}
	for _, r := range o.Roots {
		opts.Roots = append(opts.Roots, splitList(r)...)
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
		case !r.Status.Healthy():
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
func runSynapseLocate(ctx context.Context, ref string, hints []string, jsonMode bool, out io.Writer) error {
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
func runSynapseIDESync(ctx context.Context, id, binary string, out io.Writer) error {
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
func runSynapseMove(ctx context.Context, ref, newPath string, jsonMode bool, out io.Writer) error {
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	ix, err := loadDefaultIndex()
	if err != nil {
		return err
	}
	res, err := synapse.Move(st, ref, newPath, synapse.MoveOptions{Index: ix})
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
