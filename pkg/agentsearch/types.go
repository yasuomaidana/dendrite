package agentsearch

import (
	"time"

	"Dendrite/pkg/agent"
)

// AgentID represents a strongly-typed identifier for an agent.
type AgentID string

// CapabilityScore represents a normalized capability score between 0.0 and 1.0.
type CapabilityScore float32

// TaskVector represents dense floating-point embedding coordinates.
type TaskVector []float32

// LeidenCluster represents an AST community cluster discovered by Graphify.
type LeidenCluster struct {
	ClusterID   string   `json:"cluster_id"`
	Name        string   `json:"name"`
	Packages    []string `json:"packages"`
	Symbols     []string `json:"symbols"`
	PrimaryLang string   `json:"primary_lang"`
}

// AgentManifest defines an autonomous agent's capabilities, supported tools, and optimal model tiers.
type AgentManifest struct {
	ID                AgentID         `json:"id"`
	Type              agent.AgentType `json:"type"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Strengths         []string        `json:"strengths"`
	SupportedTools    []string        `json:"supported_tools"`
	DomainTags        []string        `json:"domain_tags"`
	PreferredTier     string          `json:"preferred_tier"` // "local", "hybrid", "cloud"
	PreferredEngine   string          `json:"preferred_engine"`
	PreferredModel    string          `json:"preferred_model"`
	ZeroCostCandidate bool            `json:"zero_cost_candidate"`
	MaxCostCeilingUSD float64         `json:"max_cost_ceiling_usd"`
}

// SearchQuery specifies the search criteria and constraints for agent selection.
type SearchQuery struct {
	Text                  string   `json:"text"`
	TargetPackage         string   `json:"target_package,omitempty"`
	TargetSymbols         []string `json:"target_symbols,omitempty"`
	RequireLocalInference bool     `json:"require_local_inference"`
	MaxCostUSD            float64  `json:"max_cost_usd,omitempty"`
	MaxResults            int      `json:"max_results"`
}

// SearchResult represents a scored agent match from the hybrid search engine.
type SearchResult struct {
	Manifest        AgentManifest   `json:"manifest"`
	DenseScore      float32         `json:"dense_score"`
	SparseScore     float32         `json:"sparse_score"`
	RRFScore        float32         `json:"rrf_score"`
	HistoricalScore CapabilityScore `json:"historical_score"`
	GraphifyBoost   float32         `json:"graphify_boost"`
	FinalScore      CapabilityScore `json:"final_score"`
	Reasoning       string          `json:"reasoning"`
}

// RoutingDecision encapsulates the auto-selected agent and recommended execution topology.
type RoutingDecision struct {
	SelectedAgent   AgentManifest   `json:"selected_agent"`
	Score           CapabilityScore `json:"score"`
	EngineID        string          `json:"engine_id"`
	Model           string          `json:"model"`
	Tier            string          `json:"tier"`
	VirtualKeyAlias string          `json:"virtual_key_alias"`
	EstimatedCost   float64         `json:"estimated_cost"`
	Reasoning       string          `json:"reasoning"`
	CreatedAt       time.Time       `json:"created_at"`
}
