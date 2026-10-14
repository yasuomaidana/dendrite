package agentsearch

import (
	"sort"
)

const defaultRRFConstant = 60.0

// ScoredItem pairs an item ID with a raw relevance score.
type ScoredItem struct {
	ID    string
	Score float32
}

// ReciprocalRankFusion merges dense vector and sparse lexical rankings.
func ReciprocalRankFusion(denseScores, sparseScores map[string]float32, k float64) map[string]float32 {
	if k <= 0 {
		k = defaultRRFConstant
	}

	denseRanks := rankScores(denseScores)
	sparseRanks := rankScores(sparseScores)

	allIDs := make(map[string]bool)
	for id := range denseScores {
		allIDs[id] = true
	}
	for id := range sparseScores {
		allIDs[id] = true
	}

	rrfScores := make(map[string]float32, len(allIDs))
	var maxRRF float64

	for id := range allIDs {
		var sum float64
		if rank, ok := denseRanks[id]; ok {
			sum += 1.0 / (k + float64(rank))
		}
		if rank, ok := sparseRanks[id]; ok {
			sum += 1.0 / (k + float64(rank))
		}

		rrfScores[id] = float32(sum)
		if sum > maxRRF {
			maxRRF = sum
		}
	}

	// Normalize to [0.0, 1.0]
	if maxRRF > 0 {
		for id, val := range rrfScores {
			rrfScores[id] = float32(float64(val) / maxRRF)
		}
	}

	return rrfScores
}

func rankScores(scores map[string]float32) map[string]int {
	items := make([]ScoredItem, 0, len(scores))
	for id, s := range scores {
		items = append(items, ScoredItem{ID: id, Score: s})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})

	ranks := make(map[string]int, len(items))
	for i, item := range items {
		ranks[item.ID] = i + 1 // 1-indexed rank
	}
	return ranks
}
