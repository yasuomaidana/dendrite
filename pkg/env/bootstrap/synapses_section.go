package bootstrap

import (
	"fmt"
	"strings"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

// peerGroupsOf lists the synapses the repository at root belongs to, with its peers in each. It is empty when
// the repository has no project ID or is in no synapse. A seam for tests.
var peerGroupsOf = func(root string) []synapse.PeerGroup {
	id, err := projectid.Read(root)
	if err != nil {
		return nil
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return nil
	}
	var ix *projectid.Index
	if p, err := projectid.DefaultIndexPath(); err == nil {
		ix, _ = projectid.LoadIndex(p)
	}
	groups, err := synapse.PeersOf(st, ix, id)
	if err != nil {
		return nil
	}
	return groups
}

// renderSynapsesSection describes the synapses the repository at root belongs to and how to ask a
// peer for a feature. AGENTS.md is committed, so it names peers and their Plane projects but never
// filesystem paths. It returns "" when the repository has no project ID or is in no synapse.
func renderSynapsesSection(root string) string {
	groups := peerGroupsOf(root)
	if len(groups) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Synapses\n")
	sb.WriteString("This project belongs to the synapses below. Its peers are other repositories it depends on or is used by; ask a peer for a feature instead of editing its code.\n\n")
	for _, g := range groups {
		fmt.Fprintf(&sb, "### %s (you are `%s`)\n", g.Synapse, g.Self)
		for _, p := range g.Peers {
			plane := p.PlaneProject
			if plane == "" {
				plane = "unknown"
			}
			fmt.Fprintf(&sb, "- **%s** (%s) - Plane project %s\n", p.Alias, p.Role, plane)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("To ask a peer for a feature, file a ticket in its Plane project:\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite chronos request <peer-alias> --name \"...\" --desc \"goal and acceptance criteria\" [--synapse <id>] [--from <TICKET>]\n")
	sb.WriteString("```\n")
	sb.WriteString("A peer that is in several synapses with this project needs `--synapse`, which records where the request came from; the peer works through them with `dendrite chronos next --synapse <id>`.\n")
	return sb.String()
}
