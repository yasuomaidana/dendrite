package ticketlock

import (
	"context"
	"math"
	"sort"
	"strings"
	"unicode"

	"Dendrite/pkg/rag"
)

// Ranker orders documents by relevance to a query and returns the ids of the best topK, best first.
type Ranker func(ctx context.Context, query string, docs []rag.RankDocument, topK int) ([]string, error)

// sidecarRanker ranks with the dendrite-rag sidecar's hybrid (lexical and dense) `rank` for the project at root.
// A project without a reachable or recent enough sidecar is ranked by words alone, so a review never fails
// only because the sidecar is missing.
func sidecarRanker(root string) Ranker {
	retriever := rag.NewRetriever()
	return func(ctx context.Context, query string, docs []rag.RankDocument, topK int) ([]string, error) {
		ranked, err := retriever.Rank(ctx, root, query, docs, topK)
		if err != nil {
			return lexicalRank(query, docs, topK), nil
		}
		ids := make([]string, len(ranked))
		for i, r := range ranked {
			ids[i] = r.ID
		}
		return ids, nil
	}
}

// lexicalRank orders documents by how many of the query's words they share, rarer words counting more. With no
// word in common the documents keep their order.
func lexicalRank(query string, docs []rag.RankDocument, topK int) []string {
	words := func(s string) map[string]int {
		counts := map[string]int{}
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }) {
			if len(w) >= 3 {
				counts[w]++
			}
		}
		return counts
	}
	docWords := make([]map[string]int, len(docs))
	df := map[string]int{}
	for i, d := range docs {
		docWords[i] = words(d.Text)
		for w := range docWords[i] {
			df[w]++
		}
	}
	type scored struct {
		id    string
		score float64
		index int
	}
	rows := make([]scored, len(docs))
	for i, d := range docs {
		rows[i] = scored{id: d.ID, index: i}
		for w := range words(query) {
			if tf := docWords[i][w]; tf > 0 {
				rows[i].score += (1 + math.Log(float64(tf))) * math.Log(1+float64(len(docs))/float64(df[w]))
			}
		}
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].score > rows[b].score })
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.id)
	}
	return ids[:min(topK, len(ids))]
}
