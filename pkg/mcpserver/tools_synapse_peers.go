package mcpserver

import (
	"context"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// registerSynapsePeersTool adds dendrite_synapse_peers to the default server: who the repository's
// synapse peers are, so an agent can ask one for a feature (`dendrite chronos request`).
func registerSynapsePeersTool(s *Server) {
	s.RegisterTool(Tool{
		Name: "dendrite_synapse_peers",
		Description: "Lists, per synapse that contains the repository, its other members: alias, role, Plane project identifier, " +
			"where the checkout resolves and its health. An empty list means the repository is in no synapse. " +
			"To ask a peer for a feature, file a ticket in its Plane project with `dendrite chronos request <peer> --name ... --desc ...`",
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{
			"dir": map[string]interface{}{"type": "string", "description": "Directory inside the repository; default the server's working directory"},
		}},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		dir, _ := args["dir"].(string)
		if dir == "" {
			dir = "."
		}
		root, err := plane.FindRepoRoot(dir)
		if err != nil {
			return errorResult("%s is not inside a git repository", dir), nil
		}
		id, err := projectid.Read(root)
		if err != nil {
			return jsonResult([]synapse.PeerGroup{}) // no project identity: in no synapse
		}
		st, err := synapse.DefaultStore()
		if err != nil {
			return errorResult("opening the synapse store: %v", err), nil
		}
		var ix *projectid.Index
		if p, err := projectid.DefaultIndexPath(); err == nil {
			ix, _ = projectid.LoadIndex(p)
		}
		groups, err := synapse.PeersOf(st, ix, id)
		if err != nil {
			return errorResult("listing peers: %v", err), nil
		}
		if groups == nil {
			groups = []synapse.PeerGroup{}
		}
		return jsonResult(groups)
	})
}
