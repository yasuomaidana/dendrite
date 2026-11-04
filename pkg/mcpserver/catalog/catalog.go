// Package catalog registers Dendrite's whole control-plane tool suite on an MCP server, from the tool packages.
package catalog

import (
	. "Dendrite/pkg/mcpserver"
	"Dendrite/pkg/mcpserver/codetools"
	"Dendrite/pkg/mcpserver/opstools"
	"Dendrite/pkg/mcpserver/planetools"
	"Dendrite/pkg/ollama"
)

// RegisterDefaultTools registers the standard suite of Dendrite MCP tools.
func RegisterDefaultTools(s *Server, deps Dependencies) {
	if deps.DevStatus == "" {
		deps.DevStatus = "DEV_STATUS.md"
	}
	if deps.OllamaURL == "" {
		deps.OllamaURL = ollama.DefaultBaseURL
	}
	planetools.Register(s, deps)
	codetools.Register(s, deps)
	opstools.Register(s, deps)
	RegisterSynapsePeersTool(s)
}
