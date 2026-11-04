package opstools

import . "Dendrite/pkg/mcpserver"

// Register registers the operations tools: audit, agents, canvas, scheduling, stores, story forks, migration, benchmarks and the smaller services on s.
func Register(s *Server, deps Dependencies) {
	registerAgentRunTool(s, deps)
	registerTelemetryAndInferenceTools(s, deps)
	registerPerfTools(s, deps)
	registerREATools(s, deps)
	registerDocusaurusTools(s, deps)
	registerTriageTools(s, deps)
	registerStoreTools(s, deps)
	registerSchedulerTools(s, deps)
	registerContractTools(s, deps)
	registerStoryForkTools(s, deps)
	registerAuditTools(s, deps)
	registerInferenceTools(s, deps)
	registerPenpotTools(s, deps)
	registerMigrateTools(s, deps)
	registerGreetingTools(s, deps)
	registerEnvPrepareTools(s, deps)
	RegisterCanvasTools(s, deps)
}
