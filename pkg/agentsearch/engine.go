package agentsearch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"Dendrite/pkg/agent"
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

// SearchEngine coordinates hybrid dense/sparse search, scoring, and semantic routing.
type SearchEngine struct {
	mu            sync.RWMutex
	embedClient   EmbeddingClient
	bm25          *BM25Index
	scorer        *Scorer
	manifests     map[AgentID]AgentManifest
	manifestTexts map[AgentID]string
	cachedVectors map[AgentID][]float32
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
			PreferredEngine:   "oxillama",
			PreferredModel:    "oxillama-rust-qwen7b",
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
			PreferredEngine:   "gemini-pro",
			PreferredModel:    "gemini-2.5-pro",
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

// NewSearchEngine creates and indexes a SearchEngine.
func NewSearchEngine(embedClient EmbeddingClient, ledger *agent.SessionLedger, clusters []LeidenCluster) *SearchEngine {
	if embedClient == nil {
		embedClient = NewMockEmbeddingClient(64)
	}
	if len(clusters) == 0 {
		clusters = DefaultLeidenClusters()
	}

	bm25 := NewBM25Index()
	manifests := make(map[AgentID]AgentManifest)
	manifestTexts := make(map[AgentID]string)

	defaults := DefaultManifests()
	for _, m := range defaults {
		manifests[m.ID] = m
		text := fmt.Sprintf("%s %s %s %s %s",
			m.Name,
			m.Description,
			strings.Join(m.Strengths, " "),
			strings.Join(m.SupportedTools, " "),
			strings.Join(m.DomainTags, " "),
		)
		manifestTexts[m.ID] = text
		bm25.AddDocument(string(m.ID), text)
	}

	scorer := NewScorer(ledger, clusters)

	se := &SearchEngine{
		embedClient:   embedClient,
		bm25:          bm25,
		scorer:        scorer,
		manifests:     manifests,
		manifestTexts: manifestTexts,
		cachedVectors: make(map[AgentID][]float32),
	}

	// Warm vector cache asynchronously or in-line
	_ = se.warmVectors(context.Background())

	return se
}

func (se *SearchEngine) warmVectors(ctx context.Context) error {
	texts := make([]string, 0, len(se.manifests))
	ids := make([]AgentID, 0, len(se.manifests))

	for id, text := range se.manifestTexts {
		ids = append(ids, id)
		texts = append(texts, text)
	}

	vecs, err := se.embedClient.Embed(ctx, texts)
	if err != nil {
		return err
	}

	se.mu.Lock()
	defer se.mu.Unlock()
	for i, id := range ids {
		se.cachedVectors[id] = vecs[i]
	}
	return nil
}

// Search executes hybrid vector + lexical search and scores candidates.
func (se *SearchEngine) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	if strings.TrimSpace(query.Text) == "" {
		return nil, ErrNoQueryText
	}

	// 1. Sparse Lexical Search (BM25)
	bm25Scores := se.bm25.Score(query.Text)

	// 2. Dense Vector Search (with graceful fallback to BM25 if embedding fails)
	denseScores := make(map[string]float32, len(se.manifests))
	queryVecs, err := se.embedClient.Embed(ctx, []string{query.Text})
	if err == nil && len(queryVecs) > 0 {
		se.mu.RLock()
		for id, mVec := range se.cachedVectors {
			sim, simErr := CosineSimilarity(queryVecs[0], mVec)
			if simErr == nil {
				denseScores[string(id)] = sim
			}
		}
		se.mu.RUnlock()
	} else {
		// Fallback: If TEI/embedding is unreachable, copy normalized BM25 scores
		for id, s := range bm25Scores {
			denseScores[id] = s
		}
	}

	// 3. Reciprocal Rank Fusion (RRF)
	rrfScores := ReciprocalRankFusion(denseScores, bm25Scores, 60.0)

	// 4. Score, filter, and rank results
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
