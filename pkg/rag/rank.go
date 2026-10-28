package rag

import (
	"context"
	"fmt"
)

// RankProtocolMinor is the first sidecar protocol minor that serves `rank` (1.3).
const RankProtocolMinor = 3

// RankDocument is one caller-supplied text for Rank.
type RankDocument struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Ranked is one document of a Rank result.
type Ranked struct {
	ID string `json:"id"`
	// Score is the weighted Reciprocal Rank Fusion of the lexical and dense rankings.
	Score float32 `json:"score"`
	// Lexical is the document's BM25 score.
	Lexical float32 `json:"lexical"`
	// Dense is the cosine similarity, nil when the project has no reachable embedder.
	Dense *float32 `json:"dense,omitempty"`
}

// Rank orders documents by relevance to query through the sidecar's `rank` method: no
// project index is built, but the settings (embedder, fusion weights) of the project at
// root apply. topK 0 returns every document, best first.
func (r *Retriever) Rank(ctx context.Context, root, query string, docs []RankDocument, topK int) ([]Ranked, error) {
	if root == "" {
		root = "."
	}
	dir, _, err := projectRoot(root)
	if err != nil {
		return nil, err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return nil, fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := r.connect(ctx, bin)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	if protocolMinor(c.Health.Protocol) < RankProtocolMinor {
		return nil, fmt.Errorf("%w: %s speaks protocol %s; rank needs 1.%d or later; %s",
			ErrSidecarTooOld, c.Path, c.Health.Protocol, RankProtocolMinor, InstallHint)
	}
	params := map[string]interface{}{"root": dir, "query": query, "documents": docs, "top_k": topK}
	var res struct {
		Ranked []Ranked `json:"ranked"`
	}
	if err := c.Call(ctx, "rank", params, &res); err != nil {
		return nil, fmt.Errorf("rank: %w", err)
	}
	return res.Ranked, nil
}
