// Package insight puts a finished REA analysis to use in the rest of Dendrite: what already exists in the project
// and its synapse that does the same thing (reuse), what to read first (rank), a note for the ticket and a draft
// Architecture Decision Record.
package insight

import (
	"sort"

	"Dendrite/pkg/rea/analysis"
)

// Ranked is one document of an analysis in reading order.
type Ranked struct {
	Doc   string `json:"doc"`
	Edges int    `json:"edges"`
	Why   string `json:"why"`
}

// Rank orders the documents of an analysis for reading: the overview first, then the documents with the most
// edges (symbols, files, strings, sections) first, so the busiest dependency is read before the quiet ones.
// Documents with nothing in them are left out.
func Rank(st analysis.State, img *analysis.Image) []Ranked {
	var out []Ranked
	for _, d := range analysis.Documents(st, img) {
		if d.Edges > 0 || d.Path == "overview.md" {
			out = append(out, Ranked{Doc: d.Path, Edges: d.Edges, Why: d.Why})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		first, second := out[i].Doc == "overview.md", out[j].Doc == "overview.md"
		if first != second {
			return first
		}
		if out[i].Edges != out[j].Edges {
			return out[i].Edges > out[j].Edges
		}
		return out[i].Doc < out[j].Doc
	})
	return out
}
