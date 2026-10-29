package inference

import (
	"context"
	"errors"
	"strings"

	"Dendrite/pkg/memfit"
	"Dendrite/pkg/oxilease"
)

func init() { memfit.Score = ModelIntelligenceScore }

// Choice is the best local model available right now.
type Choice struct {
	Engine    string `json:"engine"` // "ollama" or "oxillama"
	Model     string `json:"model"`
	Score     int    `json:"score"`
	SizeBytes int64  `json:"size_bytes"`
	// NeedsStart is true for an installed OxiLLaMa model whose server is stopped; a lease starts it.
	NeedsStart bool `json:"needs_start"`
}

// BestLocalModel picks the most capable installed chat model that fits the machine's memory right
// now: highest intelligence score first, then GPU (Ollama) over CPU (OxiLLaMa), then the larger
// model. Slower is fine; only a model that would starve the machine is excluded. Endpoints default
// to the local engines.
func BestLocalModel(ctx context.Context, ollamaEndpoint, oxiEndpoint string) (Choice, error) {
	r := memfit.Assess(ctx, ollamaEndpoint, oxiEndpoint)
	var cands []Choice
	for _, m := range r.Models {
		if !m.Fits || strings.Contains(strings.ToLower(m.Name), "embed") {
			continue
		}
		cands = append(cands, Choice{Engine: m.Engine, Model: m.Name, Score: ModelIntelligenceScore(m.Name), SizeBytes: m.SizeBytes})
	}
	if !r.OxiReachable { // installed but stopped: a lease can start it, if it fits next to what is loaded
		for _, m := range oxilease.Installed() {
			if memfit.CheckStartAt(ctx, ollamaEndpoint, "oxillama", m.SizeBytes) == nil {
				cands = append(cands, Choice{Engine: "oxillama", Model: m.ID, Score: ModelIntelligenceScore(m.ID), SizeBytes: m.SizeBytes, NeedsStart: true})
			}
		}
	}
	if len(cands) == 0 {
		return Choice{}, errors.New("no installed local model fits this machine's memory budget (see `dendrite spike models`)")
	}
	best := cands[0]
	for _, c := range cands[1:] {
		switch {
		case c.Score != best.Score:
			if c.Score > best.Score {
				best = c
			}
		case c.Engine != best.Engine:
			if c.Engine == "ollama" {
				best = c
			}
		case c.SizeBytes > best.SizeBytes:
			best = c
		}
	}
	return best, nil
}
