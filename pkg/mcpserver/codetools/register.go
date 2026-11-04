package codetools

import . "Dendrite/pkg/mcpserver"

// Register registers the retrieval, code graph, query and distributed-codebase tools on s.
func Register(s *Server, deps Dependencies) {
	registerDistCodebaseTools(s, deps)
	registerAgentSearchTools(s, deps)
	registerTestGenTools(s, deps)
	registerPoolTools(s, deps)
	registerQueryTools(s, deps)
	registerRAGTools(s, deps)
	registerGraphTools(s, deps)
}
