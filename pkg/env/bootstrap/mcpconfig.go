package bootstrap

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/jetbrains"

	"Dendrite/pkg/synapseide"
)

// mcpConfigPaths are the MCP configuration files of the agent families, relative to the repository root:
// Claude Code, Antigravity (AGY) and JetBrains. All three get the same servers.
var mcpConfigPaths = []string{".mcp.json", ".agents/mcp.json", ".ai/mcp/mcp.json"}

type mcpServer struct {
	name  string
	entry jetbrains.MCPServerEntry
}

// mcpServersFor lists the servers an agent in the repository at root should have: dendrite itself with the
// core profile (retrieval, the code graph, Plane and the ticket lifecycle tools), and one scoped server per
// synapse the repository belongs to. The command is "dendrite" on PATH, so the committed file works on any machine.
func mcpServersFor(root string) []mcpServer {
	servers := []mcpServer{{
		name:  "dendrite",
		entry: jetbrains.MCPServerEntry{Command: "dendrite", Args: []string{"inter", "serve", "--profile", "core"}},
	}}
	for _, g := range peerGroupsOf(root) {
		servers = append(servers, mcpServer{
			name:  synapseide.SynapseServerName(g.Synapse),
			entry: jetbrains.MCPServerEntry{Command: "dendrite", Args: []string{"inter", "serve", "--synapse", g.Synapse}},
		})
	}
	return servers
}

// writeMCPConfigs adds the servers to every MCP config file, keeping the other servers and keys. A file that
// cannot be parsed is left alone and reported; the others are still written.
func writeMCPConfigs(root string, dryRun bool) (created, updated []string, err error) {
	if dryRun {
		return nil, nil, nil
	}
	servers := mcpServersFor(root)
	var errs []error
	for _, rel := range mcpConfigPaths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		before, readErr := os.ReadFile(path)
		existed := readErr == nil
		var failed error
		for _, s := range servers {
			if _, upsertErr := jetbrains.UpsertMCPServer(path, s.name, s.entry); upsertErr != nil {
				failed = fmt.Errorf("%s: %w", rel, upsertErr)
				break
			}
		}
		if failed != nil {
			errs = append(errs, failed)
			continue
		}
		after, _ := os.ReadFile(path)
		switch {
		case !existed:
			created = append(created, rel)
		case !bytes.Equal(before, after):
			updated = append(updated, rel)
		}
	}
	return created, updated, errors.Join(errs...)
}
