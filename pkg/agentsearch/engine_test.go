package agentsearch

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/sidecar"
)

func TestMain(m *testing.M) { ragtest.Main(m) }

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

// standInEngine ranks through the dendrite-rag stand-in, which orders documents by the
// number of query words they contain.
func standInEngine(t *testing.T, ledger *agent.SessionLedger) *SearchEngine {
	t.Helper()
	if os.Getenv(sidecar.EnvVar(rag.SidecarBinary)) == "" {
		t.Fatal("TestMain did not install the dendrite-rag stand-in")
	}
	engine := NewSearchEngine(nil, ledger, nil)
	engine.root = t.TempDir()
	return engine
}

func TestSearchScoresTheSidecarRanking(t *testing.T) {
	engine := standInEngine(t, agent.NewSessionLedger(nil, ""))

	results, err := engine.Search(context.Background(), NewQueryBuilder().WithText("macro architecture system design security").Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(DefaultManifests()) {
		t.Fatalf("got %d results", len(results))
	}
	top := results[0]
	if top.Manifest.ID != "agy" || top.RRFScore != 1 || top.SparseScore < 4 || top.DenseScore != 0 {
		t.Fatalf("top result = %+v", top)
	}
	for _, r := range results[1:] {
		if r.RRFScore >= 1 || r.RRFScore <= 0 {
			t.Fatalf("RRF of %s = %f, want it normalised below the top score", r.Manifest.ID, r.RRFScore)
		}
	}
}

func TestSearchAppliesConstraints(t *testing.T) {
	engine := standInEngine(t, nil)

	results, err := engine.Search(context.Background(), NewQueryBuilder().WithText("macro architecture").RequireLocalInference(true).Build())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if !r.Manifest.ZeroCostCandidate {
			t.Errorf("expected only zero cost candidates, got %s", r.Manifest.ID)
		}
	}
	if _, err := engine.Search(context.Background(), SearchQuery{Text: "   "}); err != ErrNoQueryText {
		t.Errorf("expected ErrNoQueryText, got %v", err)
	}
}

func TestSearchReportsSidecarFailure(t *testing.T) {
	engine := standInEngine(t, nil)
	t.Setenv("STANDIN_FAIL", "rank")
	if _, err := engine.Search(context.Background(), NewQueryBuilder().WithText("anything").Build()); err == nil || !strings.Contains(err.Error(), "ranking agents") {
		t.Fatalf("err = %v", err)
	}
	t.Setenv(sidecar.EnvVar(rag.SidecarBinary), "")
	engine.ranker = &rag.Retriever{Locator: sidecar.Locator{LookPath: func(string) (string, error) { return "", errors.New("not on PATH") }}}
	if _, err := engine.Search(context.Background(), NewQueryBuilder().WithText("anything").Build()); !errors.Is(err, rag.ErrSidecarUnavailable) {
		t.Fatalf("err = %v, want ErrSidecarUnavailable", err)
	}
}

func TestRouteThroughTheSidecar(t *testing.T) {
	engine := standInEngine(t, agent.NewSessionLedger(nil, ""))

	start := time.Now()
	decision, err := engine.Route(context.Background(), NewQueryBuilder().WithText("rapid git commits and TDD refactoring").WithTargetPackage("pkg/agent").Build())
	if err != nil {
		t.Fatal(err)
	}
	if decision.SelectedAgent.ID != "aider" || decision.Model == "" || decision.EstimatedCost != 0 {
		t.Fatalf("decision = %+v", decision)
	}
	if took := time.Since(start); took >= 2*time.Second {
		t.Errorf("routing took %v", took)
	}
}
