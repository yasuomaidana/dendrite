package agentsearch

import (
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/agent"
)

// HistoricalStats stores computed performance statistics for an agent type.
type HistoricalStats struct {
	TotalSessions    int
	CompletedCount   int
	SuccessRate      float32
	AvgDuration      time.Duration
	TotalTokens      int
	TotalCostUSD     float64
	AvgCostPerRunUSD float64
}

// Scorer calculates holistic capability scores blending RRF, historical reliability, and Graphify community tags.
type Scorer struct {
	ledger   *agent.SessionLedger
	clusters map[string]LeidenCluster
}

// NewScorer constructs a Scorer with session history and Graphify community clusters.
func NewScorer(ledger *agent.SessionLedger, clusters []LeidenCluster) *Scorer {
	clusterMap := make(map[string]LeidenCluster, len(clusters))
	for _, c := range clusters {
		clusterMap[c.ClusterID] = c
	}
	return &Scorer{
		ledger:   ledger,
		clusters: clusterMap,
	}
}

// ComputeHistoricalStats computes empirical performance metrics from the session ledger.
func (s *Scorer) ComputeHistoricalStats(agentType agent.AgentType) HistoricalStats {
	if s.ledger == nil {
		return HistoricalStats{SuccessRate: 0.85} // Baseline prior
	}

	sessions := s.ledger.ListSessionsByAgent(agentType)
	if len(sessions) == 0 {
		return HistoricalStats{SuccessRate: 0.85}
	}

	var completed int
	var totalDur time.Duration
	var totalTokens int
	var totalCost float64

	for _, sess := range sessions {
		if sess.Status == "completed" {
			completed++
		}
		totalDur += sess.Duration
		totalTokens += sess.TokensConsumed
		totalCost += sess.EstimatedCostUSD
	}

	n := len(sessions)
	rate := float32(completed) / float32(n)
	avgDur := totalDur / time.Duration(n)
	avgCost := totalCost / float64(n)

	return HistoricalStats{
		TotalSessions:    n,
		CompletedCount:   completed,
		SuccessRate:      rate,
		AvgDuration:      avgDur,
		TotalTokens:      totalTokens,
		TotalCostUSD:     totalCost,
		AvgCostPerRunUSD: avgCost,
	}
}

// ComputeGraphifyBoost evaluates Leiden AST community clusters to determine domain affinity.
func (s *Scorer) ComputeGraphifyBoost(targetPkg string, targetSymbols []string, manifest AgentManifest) (float32, string) {
	if targetPkg == "" && len(targetSymbols) == 0 {
		return 0, "no AST targets provided"
	}

	var matchingCluster *LeidenCluster
	for _, c := range s.clusters {
		for _, p := range c.Packages {
			if strings.EqualFold(p, targetPkg) || strings.Contains(strings.ToLower(targetPkg), strings.ToLower(p)) {
				matchingCluster = &c
				break
			}
		}
		if matchingCluster != nil {
			break
		}
		for _, sym := range targetSymbols {
			for _, cs := range c.Symbols {
				if strings.EqualFold(cs, sym) {
					matchingCluster = &c
					break
				}
			}
			if matchingCluster != nil {
				break
			}
		}
	}

	if matchingCluster == nil {
		return 0, "no matching Leiden AST cluster found"
	}

	// Check if manifest has matching domain tags
	matchedTags := 0
	for _, tag := range manifest.DomainTags {
		if strings.EqualFold(tag, matchingCluster.Name) || strings.EqualFold(tag, matchingCluster.ClusterID) {
			matchedTags++
		}
	}

	if matchedTags > 0 {
		boost := float32(0.15) * float32(matchedTags)
		if boost > 0.30 {
			boost = 0.30
		}
		return boost, fmt.Sprintf("affinity with Leiden cluster %s (%s)", matchingCluster.ClusterID, matchingCluster.Name)
	}

	return 0.05, fmt.Sprintf("relevant to Leiden cluster %s", matchingCluster.ClusterID)
}

// EvaluateAgent calculates the composite capability score.
func (s *Scorer) EvaluateAgent(
	manifest AgentManifest,
	rrfScore float32,
	targetPkg string,
	targetSymbols []string,
) (CapabilityScore, float32, CapabilityScore, string) {
	stats := s.ComputeHistoricalStats(manifest.Type)

	// Historical score combines success rate and cost efficiency
	histSuccess := stats.SuccessRate
	histCostEff := float32(1.0)
	if stats.AvgCostPerRunUSD > 0.50 {
		histCostEff = 0.70
	} else if stats.AvgCostPerRunUSD > 0.10 {
		histCostEff = 0.85
	}
	if manifest.ZeroCostCandidate {
		histCostEff = 1.0
	}

	historicalScore := CapabilityScore((histSuccess * 0.7) + (histCostEff * 0.3))

	graphifyBoost, reason := s.ComputeGraphifyBoost(targetPkg, targetSymbols, manifest)

	// Final weighted score: RRF (50%), Historical (30%), Graphify (20%)
	final := (rrfScore * 0.50) + (float32(historicalScore) * 0.30) + (graphifyBoost * 0.20)
	if final > 1.0 {
		final = 1.0
	}

	fullReason := fmt.Sprintf("RRF=%.2f, History(success=%.0f%%)=%.2f; Graphify: %s",
		rrfScore, histSuccess*100, historicalScore, reason)

	return CapabilityScore(final), graphifyBoost, historicalScore, fullReason
}
