package main

import (
	"fmt"
	"io"
	"strings"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// printSynapsePeers prints one line per synapse the repository containing dir belongs to, naming
// its peers, so an agent starting a session knows who it can ask for a feature. Anything missing
// (no repository, no project ID, no synapse) prints nothing.
func printSynapsePeers(dir string, out io.Writer) {
	root, err := plane.FindRepoRoot(dir)
	if err != nil {
		return
	}
	id, err := projectid.Read(root)
	if err != nil {
		return
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return
	}
	var ix *projectid.Index
	if p, err := projectid.DefaultIndexPath(); err == nil {
		ix, _ = projectid.LoadIndex(p)
	}
	groups, err := synapse.PeersOf(st, ix, id)
	if err != nil {
		return
	}
	for _, g := range groups {
		var peers []string
		for _, p := range g.Peers {
			ident := p.PlaneProject
			if ident == "" {
				ident = "?"
			}
			peers = append(peers, fmt.Sprintf("%s(%s,%s)", p.Alias, p.Role, ident))
		}
		fmt.Fprintf(out, "[i] synapse %s: you are %s; peers %s (ask with `dendrite chronos request`)\n", g.Synapse, g.Self, strings.Join(peers, " "))
	}
}
