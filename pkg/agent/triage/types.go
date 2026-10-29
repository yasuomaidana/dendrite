package triage

import (
	"time"

	"Dendrite/pkg/rag"
)

// ExecutionTier represents the compute and cost classification of a task.
type ExecutionTier string

const (
	// TierLocal executes entirely on zero-cost local inference (Ollama Metal / OxiLLaMa / Candle).
	TierLocal ExecutionTier = "local"
	// TierHybrid performs local triage, context compaction, and skill enrichment before cloud escalation.
	TierHybrid ExecutionTier = "hybrid"
	// TierCloud runs high-cognition strategic reasoning on commercial paid models (Gemini 2.5 Pro / Sonnet).
	TierCloud ExecutionTier = "cloud"
)

// SupportedEngine represents the multi-provider inference backends.
type SupportedEngine string

const (
	EngineOllama          SupportedEngine = "ollama"
	EngineOllamaValidated SupportedEngine = "ollama-validated"
	EngineHuggingFace     SupportedEngine = "huggingface"
	EngineCandle          SupportedEngine = "candle"
	EngineOxiLLaMa        SupportedEngine = "oxillama"
)

// DeclarativePlan encapsulates structured, high-level directives synthesized by a paid orchestrator
// to be executed by a local intelligent worker.
type DeclarativePlan struct {
	Goal                        string   `json:"goal"`
	TargetFiles                 []string `json:"target_files,omitempty"`
	Instructions                []string `json:"instructions"`
	Constraints                 []string `json:"constraints"`
	EstimatedOrchestratorTokens int      `json:"estimated_orchestrator_tokens"`
	EstimatedDirectOutputTokens int      `json:"estimated_direct_output_tokens"`
	OutputTokenSavingsPct       float64  `json:"output_token_savings_pct"`
}

// TriageRequest is the input payload for the local agent triage engine.
type TriageRequest struct {
	Prompt                        string          `json:"prompt"`
	ActiveFile                    string          `json:"active_file,omitempty"`
	FileContent                   string          `json:"file_content,omitempty"`
	ForceTier                     ExecutionTier   `json:"force_tier,omitempty"`
	TargetEngine                  SupportedEngine `json:"target_engine,omitempty"`
	WorkspaceDir                  string          `json:"workspace_dir,omitempty"`
	ExecuteLocal                  bool            `json:"execute_local,omitempty"`
	EnableDeclarativeOrchestrator bool            `json:"enable_declarative_orchestrator,omitempty"`
	EnableRAG                     bool            `json:"enable_rag,omitempty"`
	RAGTopK                       int             `json:"rag_top_k,omitempty"`
}

// TriageResult encapsulates the routing decision, selected skills, token metrics, and execution result.
type TriageResult struct {
	Tier                  ExecutionTier   `json:"tier"`
	AssignedModel         string          `json:"assigned_model"`
	AssignedEngine        SupportedEngine `json:"assigned_engine"`
	Reason                string          `json:"reason"`
	LocalComputeAvailable bool            `json:"local_compute_available"`
	LocalEngineName       string          `json:"local_engine_name,omitempty"`
	SelectedSkills        []string        `json:"selected_skills"`
	EnhancedPrompt        string          `json:"enhanced_prompt"`
	OriginalTokens        int             `json:"original_tokens"`
	CompactedTokens       int             `json:"compacted_tokens"`
	EstimatedCostUSD      float64         `json:"estimated_cost_usd"`
	CostWithoutTriageUSD  float64         `json:"cost_without_triage_usd"`
	TokenSavingsPct       float64         `json:"token_savings_pct"`
	LocalExecutionResult  string          `json:"local_execution_result,omitempty"`
	ExecutionDuration     time.Duration   `json:"execution_duration"`
	// Declarative Orchestrator Fields
	IsDeclarativeOrchestrator  bool             `json:"is_declarative_orchestrator"`
	OrchestratorModel          string           `json:"orchestrator_model,omitempty"`
	WorkerModel                string           `json:"worker_model,omitempty"`
	WorkerEngine               SupportedEngine  `json:"worker_engine,omitempty"`
	DeclarativePlan            *DeclarativePlan `json:"declarative_plan,omitempty"`
	LocalWorkerExecutionResult string           `json:"local_worker_execution_result,omitempty"`
	// RAG Context Fields
	RetrievedRAGChunks []rag.Chunk `json:"retrieved_rag_chunks,omitempty"`
	RAGTokens          int         `json:"rag_tokens,omitempty"`
}

// SkillMatcher holds a skill's metadata and matching keywords.
type SkillMatcher struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Guideline   string   `json:"guideline"`
}
