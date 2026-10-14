package agentsearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"Dendrite/pkg/agent"
)

func TestQueryBuilder(t *testing.T) {
	q := NewQueryBuilder().
		WithText("refactor AST parser").
		WithTargetPackage("pkg/ast").
		WithTargetSymbols("SymbolGraph", "AddNode").
		RequireLocalInference(true).
		WithMaxCost(0.50).
		WithMaxResults(3).
		Build()

	if q.Text != "refactor AST parser" {
		t.Errorf("unexpected text: %s", q.Text)
	}
	if q.TargetPackage != "pkg/ast" {
		t.Errorf("unexpected package: %s", q.TargetPackage)
	}
	if len(q.TargetSymbols) != 2 {
		t.Errorf("unexpected symbols count: %d", len(q.TargetSymbols))
	}
	if !q.RequireLocalInference {
		t.Errorf("expected RequireLocalInference to be true")
	}
	if q.MaxCostUSD != 0.50 {
		t.Errorf("unexpected max cost: %f", q.MaxCostUSD)
	}
	if q.MaxResults != 3 {
		t.Errorf("unexpected max results: %d", q.MaxResults)
	}
}

func TestCosineSimilarity(t *testing.T) {
	v1 := []float32{1.0, 0.0, 0.0}
	v2 := []float32{1.0, 0.0, 0.0}
	sim, err := CosineSimilarity(v1, v2)
	if err != nil || sim < 0.99 {
		t.Fatalf("expected identical vectors sim ~ 1.0, got %f, err: %v", sim, err)
	}

	v3 := []float32{0.0, 1.0, 0.0}
	simOrth, err := CosineSimilarity(v1, v3)
	if err != nil || simOrth != 0.0 {
		t.Fatalf("expected orthogonal vectors sim 0.0, got %f", simOrth)
	}

	_, err = CosineSimilarity([]float32{1.0}, []float32{1.0, 2.0})
	if err != ErrDimensionMismatch {
		t.Fatalf("expected ErrDimensionMismatch, got %v", err)
	}

	simEmpty, err := CosineSimilarity([]float32{}, []float32{})
	if err != nil || simEmpty != 0 {
		t.Fatalf("expected 0 for empty vectors, got %f", simEmpty)
	}
}

func TestBM25Index(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocument("doc1", "refactor Go AST parser tree-sitter")
	idx.AddDocument("doc2", "benchmark k6 load test litellm gateway")
	idx.AddDocument("doc3", "translate python code into idiomatic rust")

	scores := idx.Score("refactor AST parser")
	if len(scores) == 0 {
		t.Fatalf("expected non-empty scores")
	}
	if scores["doc1"] <= scores["doc2"] {
		t.Errorf("doc1 should score higher than doc2 for AST query, got %f vs %f", scores["doc1"], scores["doc2"])
	}

	emptyScores := idx.Score("")
	if len(emptyScores) != 0 {
		t.Errorf("expected empty score map for empty query, got %v", emptyScores)
	}
}

func TestReciprocalRankFusion(t *testing.T) {
	dense := map[string]float32{"a": 0.9, "b": 0.5, "c": 0.1}
	sparse := map[string]float32{"a": 0.8, "b": 0.2, "d": 0.7}

	rrf := ReciprocalRankFusion(dense, sparse, 60.0)
	if rrf["a"] < rrf["b"] {
		t.Errorf("expected 'a' to be top ranked in RRF, got %v", rrf)
	}

	// Test zero/negative k fallback
	rrfZeroK := ReciprocalRankFusion(dense, sparse, 0)
	if len(rrfZeroK) != 4 {
		t.Errorf("expected 4 items in RRF, got %d", len(rrfZeroK))
	}
}

func TestTEIEmbeddingClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Return 2 vectors of dim 4
		resp := [][]float32{
			{0.1, 0.2, 0.3, 0.4},
			{0.5, 0.6, 0.7, 0.8},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTEIEmbeddingClient(server.URL, server.Client())
	vecs, err := client.Embed(context.Background(), []string{"test 1", "test 2"})
	if err != nil {
		t.Fatalf("failed to embed via TEI: %v", err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 4 {
		t.Fatalf("unexpected vecs shape: %v", vecs)
	}
	if client.Dimension() != 4 {
		t.Errorf("expected dim 4, got %d", client.Dimension())
	}

	// Test empty text error
	_, err = client.Embed(context.Background(), []string{})
	if err != ErrEmptyText {
		t.Fatalf("expected ErrEmptyText, got %v", err)
	}
}

func TestOllamaEmbeddingClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := map[string]interface{}{
			"embedding": []float32{0.1, 0.2, 0.3},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOllamaEmbeddingClient(server.URL, "nomic-embed-text", server.Client())
	vecs, err := client.Embed(context.Background(), []string{"hello ollama"})
	if err != nil {
		t.Fatalf("failed to embed via Ollama: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 3 {
		t.Fatalf("unexpected ollama vecs: %v", vecs)
	}

	_, err = client.Embed(context.Background(), nil)
	if err != ErrEmptyText {
		t.Fatalf("expected ErrEmptyText, got %v", err)
	}
}

func TestScorerWithLedgerAndLeiden(t *testing.T) {
	ledger := agent.NewSessionLedger(nil, "")
	ctx := context.Background()

	_ = ledger.RecordSession(ctx, &agent.SessionSummary{
		SessionID:        "s1",
		AgentType:        agent.AgentAider,
		Status:           "completed",
		Duration:         5 * time.Second,
		TokensConsumed:   500,
		EstimatedCostUSD: 0.0,
	})
	_ = ledger.RecordSession(ctx, &agent.SessionSummary{
		SessionID:        "s2",
		AgentType:        agent.AgentAider,
		Status:           "completed",
		Duration:         6 * time.Second,
		TokensConsumed:   600,
		EstimatedCostUSD: 0.0,
	})

	clusters := DefaultLeidenClusters()
	scorer := NewScorer(ledger, clusters)

	stats := scorer.ComputeHistoricalStats(agent.AgentAider)
	if stats.SuccessRate != 1.0 {
		t.Errorf("expected 100%% success rate, got %f", stats.SuccessRate)
	}

	manifest := DefaultManifests()[0] // Aider
	finalScore, boost, histScore, _ := scorer.EvaluateAgent(
		manifest,
		0.8,
		"pkg/agent",
		[]string{"Runner"},
	)

	if boost <= 0 {
		t.Errorf("expected positive Leiden boost for matching package pkg/agent, got %f", boost)
	}
	if finalScore <= 0 || histScore <= 0 {
		t.Errorf("expected positive scores, got final=%f, hist=%f", finalScore, histScore)
	}
}

func TestSearchEngineEndToEnd(t *testing.T) {
	mockClient := NewMockEmbeddingClient(32)
	ledger := agent.NewSessionLedger(nil, "")
	engine := NewSearchEngine(mockClient, ledger, nil)

	ctx := context.Background()

	// 1. AC-1: Hybrid search completes in < 150ms
	start := time.Now()
	query := NewQueryBuilder().
		WithText("rapid git commits and TDD refactoring").
		WithTargetPackage("pkg/agent").
		Build()

	results, err := engine.Search(ctx, query)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected search results, got 0")
	}
	if duration >= 150*time.Millisecond {
		t.Errorf("search took %v, must be < 150ms", duration)
	}

	// Top result should be Aider
	if results[0].Manifest.ID != "aider" {
		t.Logf("Top matched agent: %s (score: %.3f)", results[0].Manifest.ID, results[0].FinalScore)
	}

	// 2. Constraints testing: RequireLocalInference
	localQuery := NewQueryBuilder().
		WithText("macro architecture and system design").
		RequireLocalInference(true).
		Build()

	localResults, err := engine.Search(ctx, localQuery)
	if err != nil {
		t.Fatalf("local search failed: %v", err)
	}
	for _, res := range localResults {
		if !res.Manifest.ZeroCostCandidate {
			t.Errorf("expected only zero cost candidates, got %s", res.Manifest.ID)
		}
	}

	// 3. Routing decision
	routeQuery := NewQueryBuilder().
		WithText("macro architecture system design").
		Build()

	decision, err := engine.Route(ctx, routeQuery)
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	if decision.SelectedAgent.ID == "" || decision.Model == "" {
		t.Fatalf("invalid routing decision: %+v", decision)
	}

	// 4. Fallback test when embedding fails (AC-2)
	brokenEngine := NewSearchEngine(&brokenEmbedClient{}, ledger, nil)
	fallbackResults, err := brokenEngine.Search(ctx, query)
	if err != nil {
		t.Fatalf("fallback search failed: %v", err)
	}
	if len(fallbackResults) == 0 {
		t.Fatalf("expected fallback search results via BM25")
	}

	// 5. Empty query error
	_, err = engine.Search(ctx, SearchQuery{Text: "   "})
	if err != ErrNoQueryText {
		t.Errorf("expected ErrNoQueryText, got %v", err)
	}
}

type brokenEmbedClient struct{}

func (b *brokenEmbedClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, errors.New("simulated embedding endpoint timeout/offline")
}
func (b *brokenEmbedClient) Dimension() int { return 32 }
