package agentsearch

import (
	"context"
	"errors"
	"testing"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/rag"
)

// orderedRanker answers every query with the same ranking, so the engine's own filtering and scoring is exercised without a sidecar.
type orderedRanker struct {
	ranked []rag.Ranked
	err    error
}

func (r orderedRanker) Rank(context.Context, string, string, []rag.RankDocument, int) ([]rag.Ranked, error) {
	return r.ranked, r.err
}

func TestSearchWithADenseRankingFiltersAndTruncates(t *testing.T) {
	dense := float32(0.4)
	ranker := orderedRanker{ranked: []rag.Ranked{{ID: "aider", Score: 0.5, Lexical: 2, Dense: &dense}}}
	engine := NewSearchEngine(ranker, nil, nil)
	ctx := context.Background()

	results, err := engine.Search(ctx, SearchQuery{Text: "refactor", MaxResults: 1})
	if err != nil || len(results) != 1 {
		t.Fatalf("results = %v, %v", results, err)
	}
	if results[0].Manifest.ID != "aider" || results[0].DenseScore != dense || results[0].RRFScore != 1 {
		t.Errorf("top result = %+v", results[0])
	}

	capped, err := engine.Search(ctx, SearchQuery{Text: "refactor", MaxCostUSD: 1e-9})
	if err != nil || len(capped) >= len(DefaultManifests()) {
		t.Errorf("a tiny cost ceiling kept %d of %d agents, %v", len(capped), len(DefaultManifests()), err)
	}

	engine.manifests = map[AgentID]AgentManifest{}
	if _, err := engine.Search(ctx, SearchQuery{Text: "refactor"}); !errors.Is(err, ErrNoMatchingAgents) {
		t.Errorf("an engine without agents: err = %v", err)
	}
}

func TestRouteReportsAndPricesTheChoice(t *testing.T) {
	ctx := context.Background()
	failing := NewSearchEngine(orderedRanker{err: errors.New("down")}, nil, nil)
	if _, err := failing.Route(ctx, SearchQuery{Text: "anything"}); err == nil {
		t.Error("a failing ranker was not an error")
	}

	var paid AgentManifest
	for _, m := range DefaultManifests() {
		if !m.ZeroCostCandidate {
			paid = m
		}
	}
	engine := NewSearchEngine(orderedRanker{ranked: []rag.Ranked{{ID: string(paid.ID), Score: 1}}}, nil, nil)
	decision, err := engine.Route(ctx, SearchQuery{Text: "anything"})
	if err != nil || decision.SelectedAgent.ID != paid.ID || decision.EstimatedCost != 0.05 {
		t.Fatalf("decision = %+v, %v", decision, err)
	}
}

func TestGraphifyBoostPerClusterMatch(t *testing.T) {
	scorer := NewScorer(nil, []LeidenCluster{{ClusterID: "c1", Name: "parsing", Packages: []string{"pkg/ast"}, Symbols: []string{"Walk"}}})
	tagged := func(tags ...string) AgentManifest { return AgentManifest{DomainTags: tags} }
	cases := []struct {
		name    string
		pkg     string
		symbols []string
		m       AgentManifest
		want    float32
	}{
		{"no targets", "", nil, tagged("parsing"), 0},
		{"no cluster", "pkg/other", []string{"Nope"}, tagged("parsing"), 0},
		{"package and one tag", "pkg/ast", nil, tagged("parsing"), 0.15},
		{"symbol and one tag", "pkg/other", []string{"walk"}, tagged("parsing"), 0.15},
		{"the boost is capped", "pkg/ast", nil, tagged("parsing", "PARSING", "c1"), 0.30},
		{"cluster without a shared tag", "pkg/ast", nil, tagged("other"), 0.05},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := scorer.ComputeGraphifyBoost(c.pkg, c.symbols, c.m); got != c.want {
				t.Errorf("boost = %v, want %v", got, c.want)
			}
		})
	}
}

func TestEvaluateAgentPenalisesCostlyHistoryAndCapsTheScore(t *testing.T) {
	ledger := agent.NewSessionLedger(nil, "")
	for id, cost := range map[string]float64{"a": 0.60, "b": 0.20} {
		_ = ledger.RecordSession(context.Background(), &agent.SessionSummary{SessionID: id, AgentType: agent.AgentType("costly-" + id), Status: "completed", EstimatedCostUSD: cost})
	}
	scorer := NewScorer(ledger, nil)
	for typ, want := range map[string]CapabilityScore{"costly-a": 0.7 + 0.3*0.70, "costly-b": 0.7 + 0.3*0.85} {
		_, _, hist, _ := scorer.EvaluateAgent(AgentManifest{Type: agent.AgentType(typ)}, 0.5, "", nil)
		if diff := hist - want; diff > 1e-5 || diff < -1e-5 {
			t.Errorf("%s history score = %v, want %v", typ, hist, want)
		}
	}
	free := AgentManifest{Type: "costly-a", ZeroCostCandidate: true}
	if final, _, _, _ := scorer.EvaluateAgent(free, 5, "", nil); final != 1 {
		t.Errorf("final = %v, want it capped at 1", final)
	}
}
