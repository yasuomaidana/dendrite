package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// requestSourcePrefix tags a request in the peer's Plane project with the synapse it came from
// (Plane's external_source), so a project shared by several synapses can tell them apart.
const requestSourcePrefix = "dendrite-synapse:"

type planeRequestOptions struct {
	Target  string // peer alias, Plane identifier or project ID
	Name    string
	Desc    string
	Synapse string
	From    string // ticket in this project that depends on the request
}

func newChronosRequestCmd() *cobra.Command {
	var o planeRequestOptions
	cmd := &cobra.Command{
		Use:   "request <peer-alias|IDENT>",
		Short: "Ask a synapse peer for a feature by creating a ticket in its Plane project",
		Long: "Creates a ticket in the Plane project of a peer of this repository: a member of a synapse this\n" +
			"repository belongs to. A peer outside every shared synapse is refused. When the peer shares several\n" +
			"synapses with this repository, --synapse says which one the request belongs to; the ticket records it\n" +
			"(external source dendrite-synapse:<id>) so the peer can filter its board with `chronos next --synapse`.\n" +
			"Asking twice with the same name returns the existing ticket. --from names the ticket here that\n" +
			"depends on the request.",
		Args:    exactArgs(1),
		Example: "dendrite chronos request oxipix --name \"Expose a batch resize API\" --desc \"docula needs ...\" --from DOC-12",
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Target = args[0]
			return planeRun(func(c *plane.Client) error {
				return planeRequest(cmd.Context(), c, planesvc.Default(), ".", o, cmd.OutOrStdout())
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Name, "name", "", "Title of the request")
	f.StringVar(&o.Desc, "desc", "", "What is needed and why (goal and acceptance criteria)")
	f.StringVar(&o.Synapse, "synapse", "", "The synapse the request belongs to (required when several are shared)")
	f.StringVar(&o.From, "from", "", "Ticket in this project that depends on the request")
	requireFlags(cmd, "name", "desc")
	return cmd
}

func planeRequest(ctx context.Context, client *plane.Client, svc *planesvc.Service, startDir string, o planeRequestOptions, out io.Writer) error {
	if strings.TrimSpace(o.Name) == "" || strings.TrimSpace(o.Desc) == "" {
		return usageErrorf("a request needs both --name and --desc")
	}
	root, err := plane.FindRepoRoot(startDir)
	if err != nil {
		return err
	}
	selfID, err := projectid.Read(root)
	if err != nil {
		return fmt.Errorf("%s has no project identity: run `dendrite chronos project ensure`: %w", root, err)
	}
	self, _, err := plane.LinkForDir(root)
	if err != nil {
		return fmt.Errorf("%s is not linked to a Plane project: run `dendrite chronos project ensure`: %w", root, err)
	}

	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	var ix *projectid.Index
	if p, err := projectid.DefaultIndexPath(); err == nil {
		ix, _ = projectid.LoadIndex(p)
	}
	groups, err := synapse.PeersOf(st, ix, selfID)
	if err != nil {
		return err
	}
	group, peer, err := pickPeer(groups, o.Target, o.Synapse)
	if err != nil {
		return err
	}
	if peer.PlaneProject == "" {
		return fmt.Errorf("peer %s of synapse %s has no Plane project: bind it with `dendrite synapse set-plane %s %s <IDENT>`", peer.Alias, group.Synapse, group.Synapse, peer.Alias)
	}

	peerClient, err := plane.ClientForProject(ctx, client, peer.PlaneProject)
	if err != nil {
		return fmt.Errorf("peer %s: %w", peer.Alias, err)
	}

	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(o.Name))))
	req := plane.CreateIssueRequest{
		Name:            strings.TrimSpace(o.Name),
		DescriptionHTML: requestBody(self.Identifier, group, o),
		ExternalSource:  requestSourcePrefix + group.Synapse,
		ExternalID:      self.Identifier + ":" + hex.EncodeToString(sum[:])[:12],
	}
	issue, created, err := svc.CreateIssue(ctx, peerClient, req)
	if err != nil {
		return fmt.Errorf("create the request in %s: %w", peer.PlaneProject, err)
	}
	ref := fmt.Sprintf("%s-%d", peer.PlaneProject, issue.SequenceID)
	if created {
		fmt.Fprintf(out, "[✓] Requested %q from %s in synapse %s: %s\n", req.Name, peer.Alias, group.Synapse, ref)
	} else {
		fmt.Fprintf(out, "[i] %s already has this request: %s\n", peer.Alias, ref)
	}

	if o.From != "" {
		linkRequestSource(ctx, client, o.From, issue.ID, ref, out)
	}
	return nil
}

// pickPeer finds the peer a request is addressed to among the groups of synapses this project
// belongs to. It never crosses synapses silently: a target that is a peer in several needs --synapse.
func pickPeer(groups []synapse.PeerGroup, target, only string) (synapse.PeerGroup, synapse.Peer, error) {
	if len(groups) == 0 {
		return synapse.PeerGroup{}, synapse.Peer{}, errors.New("this repository is in no synapse, so it has no peers to ask (see `dendrite synapse create`)")
	}
	if only != "" {
		var kept []synapse.PeerGroup
		for _, g := range groups {
			if g.Synapse == only {
				kept = append(kept, g)
			}
		}
		if len(kept) == 0 {
			var ids []string
			for _, g := range groups {
				ids = append(ids, g.Synapse)
			}
			return synapse.PeerGroup{}, synapse.Peer{}, fmt.Errorf("this repository is not in synapse %q (it is in: %s)", only, strings.Join(ids, ", "))
		}
		groups = kept
	}

	type hit struct {
		g synapse.PeerGroup
		p synapse.Peer
	}
	var hits []hit
	var known []string
	for _, g := range groups {
		for _, p := range g.Peers {
			known = append(known, g.Synapse+"/"+p.Alias)
			if strings.EqualFold(p.Alias, target) || strings.EqualFold(p.PlaneProject, target) || p.ProjectID == target {
				hits = append(hits, hit{g, p})
			}
		}
	}
	switch len(hits) {
	case 0:
		return synapse.PeerGroup{}, synapse.Peer{}, fmt.Errorf("%q is not a peer of this repository in any shared synapse (peers: %s)", target, strings.Join(known, ", "))
	case 1:
		return hits[0].g, hits[0].p, nil
	}
	var ids []string
	for _, h := range hits {
		ids = append(ids, h.g.Synapse)
	}
	return synapse.PeerGroup{}, synapse.Peer{}, fmt.Errorf("%q is a peer in several synapses (%s): pass --synapse to say which one this request belongs to", target, strings.Join(ids, ", "))
}

func requestBody(requester string, g synapse.PeerGroup, o planeRequestOptions) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<p>Requested by <b>%s</b> (%s) through synapse <code>%s</code>.</p>", html.EscapeString(requester), html.EscapeString(g.Self), html.EscapeString(g.Synapse))
	for _, para := range strings.Split(strings.TrimSpace(o.Desc), "\n") {
		if para = strings.TrimSpace(para); para != "" {
			fmt.Fprintf(&sb, "<p>%s</p>", html.EscapeString(para))
		}
	}
	if o.From != "" {
		fmt.Fprintf(&sb, "<p>Needed by <b>%s</b> in %s.</p>", html.EscapeString(o.From), html.EscapeString(requester))
	}
	return sb.String()
}

// linkRequestSource blocks the requesting ticket on the new one. Plane may refuse relations that
// cross projects; the request still stands (its description names the source ticket).
func linkRequestSource(ctx context.Context, client *plane.Client, from, requestUUID, requestRef string, out io.Writer) {
	src, err := resolvePlaneTicketUUID(ctx, client, from)
	if err != nil {
		fmt.Fprintf(out, "[-] could not find %s to link it: %v\n", from, err)
		return
	}
	if err := client.AddIssueRelation(ctx, src, "blocked_by", []string{requestUUID}); err != nil {
		fmt.Fprintf(out, "[i] %s was not linked to %s (Plane refused the cross-project relation: %v); the request names it in its description\n", from, requestRef, err)
		return
	}
	fmt.Fprintf(out, "[✓] %s is now blocked by %s\n", from, requestRef)
}
