package mcpserver

import (
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

	registerStatusAndTicketTools(s, deps)
	registerOllamaAndGatewayTools(s, deps)
	registerAgentRunTool(s, deps)
	registerTelemetryAndInferenceTools(s, deps)
	registerPlaneSyncAndIDETools(s, deps)
	// 16-19: Distributed Codebase & Interoperability Tools
	registerDistCodebaseTools(s, deps)

	// 20: Performance Testing, Benchmarking & Gateway Load Tools
	registerPerfTools(s, deps)

	// 21: Open-Source REA Showcases & Cleanroom Parity Verification Tools
	registerREATools(s, deps)

	// 22: Docusaurus Documentation & Project Sharing Tools
	registerDocusaurusTools(s, deps)

	// 23: Local Agent Triage, Token Efficiency & Prompt Router
	registerTriageTools(s, deps)

	// 24: Hybrid Agent Search, TEI Embeddings & Semantic Routing Engine
	registerAgentSearchTools(s, deps)

	// 25: Automated Test Synthesizer, Self-Healing Loop & Web UI Test Harness
	registerTestGenTools(s, deps)

	// 26-28: Autonomous Pool DAG Synchronization & Multi-Directory Project Tools
	registerPoolTools(s, deps)

	// 29-31: Universal Intent Slicing & Query Engine Tools
	registerQueryTools(s, deps)

	// 38-40: Persistent Segmented Store & Chunk Checkpointing Tools
	registerStoreTools(s, deps)

	// 41-43: Token-Aware Hybrid Scheduler & Quota Governance Tools
	registerSchedulerTools(s, deps)

	// 45: Quad-IDE Cross-Language Schema & Contract Drift Tools
	registerContractTools(s, deps)

	// 47-50: Speculative Git Worktree Branching & Differential Benchmark Tools
	registerStoryForkTools(s, deps)
	// Ticket-Milestone DAG Linker, Smart Prioritization & Plane Gantt Tools
	registerPlaneDAGTools(s, deps)

	// 55-57: Git Guardrails, Speculative Stacked Branches & Vendor Audit Tools
	registerAuditTools(s, deps)

	registerCommitTool(s, deps)
	// Model Migration Tools
	registerInferenceTools(s, deps)
	// UI & Penpot Tools
	registerPenpotTools(s, deps)
	// Service Migration & Node Mobility Tools
	registerMigrateTools(s, deps)
	// Personal Context Greeting Tools
	registerGreetingTools(s, deps)
	// Project Environment Preparation & CI/CD Tools
	registerEnvPrepareTools(s, deps)
	// Canvas Interactive Writing & Penpot Creative Bridge Tools
	registerCanvasTools(s, deps)
	// Project RAG & AST Retrieval Tools
	registerRAGTools(s, deps)
	// Code Graph & Leiden Community Tools
	registerGraphTools(s, deps)
}
