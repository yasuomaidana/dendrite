package planetools

import . "Dendrite/pkg/mcpserver"

// Register registers the Plane board, ticket and lifecycle tools on s.
func Register(s *Server, deps Dependencies) {
	registerStatusAndTicketTools(s, deps)
	registerOllamaAndGatewayTools(s, deps)
	registerPlaneSyncAndIDETools(s, deps)
	registerPlaneDAGTools(s, deps)
	RegisterPlaneTicketSetTool(s, deps)
	RegisterTicketLifecycleTools(s, deps)
	registerCommitTool(s, deps)
}
