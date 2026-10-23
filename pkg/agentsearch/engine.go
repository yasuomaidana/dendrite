package agentsearch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/rag"
)

var (
	// ErrNoQueryText is returned when a search is attempted without text.
	ErrNoQueryText = errors.New("search query text cannot be empty")
	// ErrNoMatchingAgents indicates no agents satisfied the search filters.
	ErrNoMatchingAgents = errors.New("no matching agents found satisfying constraints")
)

// QueryBuilder provides a fluent builder for SearchQuery instances.
type QueryBuilder struct {
	query SearchQuery
}

// NewQueryBuilder starts a new fluent query construction.
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		query: SearchQuery{
			MaxResults: 5,
		},
	}
}

// WithText sets the primary prompt or task description.
func (b *QueryBuilder) WithText(text string) *QueryBuilder {
	b.query.Text = text
	return b
}

// WithTargetPackage sets the target Go/Rust package.
func (b *QueryBuilder) WithTargetPackage(pkg string) *QueryBuilder {
	b.query.TargetPackage = pkg
	return b
}

// WithTargetSymbols sets specific AST symbols involved.
func (b *QueryBuilder) WithTargetSymbols(symbols ...string) *QueryBuilder {
	b.query.TargetSymbols = append(b.query.TargetSymbols, symbols...)
	return b
}

// RequireLocalInference constrains routing to zero-cost local models.
func (b *QueryBuilder) RequireLocalInference(req bool) *QueryBuilder {
	b.query.RequireLocalInference = req
	return b
}

// WithMaxCost sets an upper cost ceiling in USD.
func (b *QueryBuilder) WithMaxCost(maxCost float64) *QueryBuilder {
	b.query.MaxCostUSD = maxCost
	return b
}

// WithMaxResults limits the number of returned matches.
func (b *QueryBuilder) WithMaxResults(max int) *QueryBuilder {
	if max > 0 {
		b.query.MaxResults = max
	}
	return b
}

// Build finalizes and returns the SearchQuery.
func (b *QueryBuilder) Build() SearchQuery {
	return b.query
}

// Ranker orders documents by relevance to a query. rag.Retriever is the implementation:
// the dendrite-rag sidecar's `rank` (code-aware BM25, the project's embedder and weighted
// Reciprocal Rank Fusion).
type Ranker interface {
	Rank(ctx context.Context, root, query string, docs []rag.RankDocument, topK int) ([]rag.Ranked, error)
}

// SearchEngine ranks agent manifests through a Ranker, then scores them with routing
// policy: session history, Leiden community affinity and cost constraints.
type SearchEngine struct {
	ranker Ranker
	// root is the project whose dendrite-rag settings (embedder, fusion weights) apply.
	root      string
	scorer    *Scorer
	manifests map[AgentID]AgentManifest
	documents []rag.RankDocument
}

// DefaultManifests returns the pre-registered platform agents.
func DefaultManifests() []AgentManifest {
	return []AgentManifest{
		{
			ID:                "aider",
			Type:              agent.AgentAider,
			Name:              "Aider Pair Programming Agent",
			Description:       "Terminal-based rapid file editing, git pair programming, TDD refactoring, and local chore execution.",
			Strengths:         []string{"rapid file editing", "git commits", "test-driven development", "local chore execution"},
			SupportedTools:    []string{"git", "file_edit", "run_tests"},
			DomainTags:        []string{"refactoring", "core", "test"},
			PreferredTier:     "local",
			PreferredEngine:   "ollama",
			PreferredModel:    "qwen2.5-coder:14b",
			ZeroCostCandidate: true,
			MaxCostCeilingUSD: 0.00,
		},
		{
			ID:                "openhands",
			Type:              agent.AgentOpenHands,
			Name:              "OpenHands Autonomous Software Engineer",
			Description:       "Containerized software engineering agent for complex multi-file features, browser journeys, and deep debugging.",
			Strengths:         []string{"full-stack feature implementation", "containerized sandbox execution", "multi-file debugging"},
			SupportedTools:    []string{"bash", "browser", "editor", "docker"},
			DomainTags:        []string{"fullstack", "sandbox", "integration"},
			PreferredTier:     "hybrid",
			PreferredEngine:   "vllm",
			PreferredModel:    "deepseek-coder",
			ZeroCostCandidate: false,
			MaxCostCeilingUSD: 1.50,
		},
		{
			ID:                "opencode",
			Type:              agent.AgentOpenCode,
			Name:              "OpenCode Polyglot & Doc Synthesizer",
			Description:       "Headless batch refactoring, polyglot language translation, AST transformation, and Docs-as-Code reflection.",
			Strengths:         []string{"polyglot translation", "docs-as-code reflection", "batch AST refactoring"},
			SupportedTools:    []string{"treesitter", "docusaurus", "ast_transform"},
			DomainTags:        []string{"docs", "translation", "ast"},
			PreferredTier:     "hybrid",
			PreferredEngine:   "ollama",
			PreferredModel:    "qwen2.5-coder:14b",
			ZeroCostCandidate: true,
			MaxCostCeilingUSD: 0.20,
		},
		{
			ID:                "agy",
			Type:              agent.AgentAGY,
			Name:              "Google Antigravity Lead Systems Architect",
			Description:       "Autonomous high-cognition strategic systems architect for macro architecture, security invariants, cross-package design, and token governance.",
			Strengths:         []string{"macro architecture", "system design", "token governance", "root-cause debugging", "security invariants"},
			SupportedTools:    []string{"mcp", "orchestrator", "git", "cloud_burst"},
			DomainTags:        []string{"architecture", "governance", "security", "infra"},
			PreferredTier:     "cloud",
			PreferredEngine:   "agy",
			PreferredModel:    "gemini-3.8-flash-high",
			ZeroCostCandidate: false,
			MaxCostCeilingUSD: 5.00,
		},
	}
}

// DefaultLeidenClusters provides default AST community clusters.
func DefaultLeidenClusters() []LeidenCluster {
	return []LeidenCluster{
		{
			ClusterID:   "cluster-ast-rea",
			Name:        "ast",
			Packages:    []string{"pkg/ast", "pkg/rea", "pkg/ai/importer", "pkg/ai/translate"},
			Symbols:     []string{"SymbolGraph", "TreeSitter", "Disassembler", "ISR"},
			PrimaryLang: "go",
		},
		{
			ClusterID:   "cluster-infra-gov",
			Name:        "governance",
			Packages:    []string{"pkg/gateway", "pkg/telemetry", "pkg/env", "pkg/perf"},
			Symbols:     []string{"TokenLimiter", "SlidingWindow", "LiteLLM", "Prometheus"},
			PrimaryLang: "go",
		},
		{
			ClusterID:   "cluster-docs-docusaurus",
			Name:        "docs",
			Packages:    []string{"pkg/docusaurus", "pkg/docgen", "pkg/docgen/reflect"},
			Symbols:     []string{"DocusaurusManager", "SharePack", "Catalog"},
			PrimaryLang: "typescript",
		},
		{
			ClusterID:   "cluster-agent-loop",
			Name:        "refactoring",
			Packages:    []string{"pkg/agent", "pkg/agent/triage", "pkg/sandbox", "pkg/testgen"},
			Symbols:     []string{"Runner", "SessionLedger", "TestSynthesizer"},
			PrimaryLang: "go",
		},
	}
}

// NewSearchEngine creates a SearchEngine over the default manifests. A nil ranker uses
// the dendrite-rag sidecar, with the settings of the current directory's project.
func NewSearchEngine(ranker Ranker, ledger *agent.SessionLedger, clusters []LeidenCluster) *SearchEngine {
	if ranker == nil {
		ranker = rag.NewRetriever()
	}
	if len(clusters) == 0 {
		clusters = DefaultLeidenClusters()
	}
	se := &SearchEngine{
		ranker:    ranker,
		root:      ".",
		scorer:    NewScorer(ledger, clusters),
		manifests: make(map[AgentID]AgentManifest),
	}
	for _, m := range DefaultManifests() {
		se.manifests[m.ID] = m
		se.documents = append(se.documents, rag.RankDocument{
			ID: string(m.ID),
			Text: strings.Join([]string{
				m.Name,
				m.Description,
				strings.Join(m.Strengths, " "),
				strings.Join(m.SupportedTools, " "),
				strings.Join(m.DomainTags, " "),
			}, " "),
		})
	}
	return se
}

// Search ranks the manifests for query through the Ranker and scores the candidates.
func (se *SearchEngine) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	if strings.TrimSpace(query.Text) == "" {
		return nil, ErrNoQueryText
	}

	ranked, err := se.ranker.Rank(ctx, se.root, query.Text, se.documents, 0)
	if err != nil {
		return nil, fmt.Errorf("ranking agents: %w", err)
	}
	// Fused scores are normalised to [0, 1] so they weigh against history and affinity.
	denseScores := make(map[string]float32, len(ranked))
	bm25Scores := make(map[string]float32, len(ranked))
	rrfScores := make(map[string]float32, len(ranked))
	var maxRRF float32
	for _, r := range ranked {
		bm25Scores[r.ID] = r.Lexical
		if r.Dense != nil {
			denseScores[r.ID] = *r.Dense
		}
		rrfScores[r.ID] = r.Score
		maxRRF = max(maxRRF, r.Score)
	}
	if maxRRF > 0 {
		for id, s := range rrfScores {
			rrfScores[id] = s / maxRRF
		}
	}

	// Score, filter, and rank results
	results := make([]SearchResult, 0, len(se.manifests))
	for id, manifest := range se.manifests {
		// Apply constraints
		if query.RequireLocalInference && !manifest.ZeroCostCandidate {
			continue
		}
		if query.MaxCostUSD > 0 && manifest.MaxCostCeilingUSD > query.MaxCostUSD {
			continue
		}

		rrf := rrfScores[string(id)]
		finalScore, boost, histScore, reason := se.scorer.EvaluateAgent(
			manifest,
			rrf,
			query.TargetPackage,
			query.TargetSymbols,
		)

		res := SearchResult{
			Manifest:        manifest,
			DenseScore:      denseScores[string(id)],
			SparseScore:     bm25Scores[string(id)],
			RRFScore:        rrf,
			HistoricalScore: histScore,
			GraphifyBoost:   boost,
			FinalScore:      finalScore,
			Reasoning:       reason,
		}
		results = append(results, res)
	}

	if len(results) == 0 {
		return nil, ErrNoMatchingAgents
	}

	// Sort descending by FinalScore
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	if query.MaxResults > 0 && len(results) > query.MaxResults {
		results = results[:query.MaxResults]
	}

	return results, nil
}

// Route automatically selects the optimal agent and configures execution routing.
func (se *SearchEngine) Route(ctx context.Context, query SearchQuery) (*RoutingDecision, error) {
	results, err := se.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	top := results[0]
	manifest := top.Manifest

	virtualKey := fmt.Sprintf("vk-%s-%s", manifest.ID, manifest.PreferredTier)
	cost := 0.0
	if !manifest.ZeroCostCandidate {
		cost = 0.05 // Baseline invocation estimate
	}

	decision := &RoutingDecision{
		SelectedAgent:   manifest,
		Score:           top.FinalScore,
		EngineID:        manifest.PreferredEngine,
		Model:           manifest.PreferredModel,
		Tier:            manifest.PreferredTier,
		VirtualKeyAlias: virtualKey,
		EstimatedCost:   cost,
		Reasoning:       top.Reasoning,
		CreatedAt:       time.Now(),
	}

	return decision, nil
}
