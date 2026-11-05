// Package synapsesearch answers one question across every project of a synapse. Each member has its
// own index and its own score scale, so scores from different members cannot be compared; the lists
// are merged by rank (reciprocal rank fusion), with a hit's score relative to the best of its own
// list breaking ties. Every hit is tagged with the member it came from.
package synapsesearch

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"Dendrite/pkg/rag"
	"Dendrite/pkg/synapse"
)

// rrfK is the reciprocal-rank-fusion constant; 60 is the value from the original paper.
const rrfK = 60

// Hit is one merged result.
type Hit struct {
	Member string `json:"member"`
	// Path is relative to the member's root.
	Path      string  `json:"path"`
	LineStart int     `json:"line_start"`
	LineEnd   int     `json:"line_end"`
	Heading   string  `json:"heading,omitempty"`
	Content   string  `json:"content,omitempty"`
	Rank      int     `json:"rank"`  // position in the member's own list, 1-based
	Score     float64 `json:"score"` // fused score; only the order is meaningful
}

// MemberNote says why a member contributed nothing.
type MemberNote struct {
	Member string `json:"member"`
	Reason string `json:"reason"`
}

// Options controls a federated search.
type Options struct {
	// TopK is the number of merged hits returned (default 10).
	TopK int
	// PerMember is how many hits are taken from each member before merging (default TopK).
	PerMember int
	// Corpus is the usual code/docs/all filter.
	Corpus string
	// Members restricts the search to these aliases (empty: all).
	Members []string
}

// Searcher runs one member's search and returns its chunks, best first, with absolute paths.
type Searcher func(ctx context.Context, dir, query string, topK int, corpus string) ([]rag.Chunk, error)

// Retrieve is the Searcher backed by the dendrite-rag sidecar.
func Retrieve(ctx context.Context, dir, query string, topK int, corpus string) ([]rag.Chunk, error) {
	res, err := rag.NewRetriever().Retrieve(ctx, dir, query, rag.SearchOptions{TopK: topK, Corpus: corpus}, false)
	if err != nil {
		return nil, err
	}
	return res.Chunks, nil
}

// Search queries each member of syn and merges the answers.
func Search(ctx context.Context, syn *synapse.Synapse, query string, opts Options, search Searcher) ([]Hit, []MemberNote, error) {
	if syn == nil {
		return nil, nil, fmt.Errorf("no synapse to search")
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil, fmt.Errorf("empty query")
	}
	if opts.TopK <= 0 {
		opts.TopK = 10
	}
	if opts.PerMember <= 0 {
		opts.PerMember = opts.TopK
	}
	if search == nil {
		search = Retrieve
	}
	wanted := map[string]bool{}
	for _, a := range opts.Members {
		wanted[a] = true
	}

	var all []Hit
	var notes []MemberNote
	for _, m := range syn.Members {
		if len(wanted) > 0 && !wanted[m.Alias] {
			continue
		}
		dir, err := synapse.Resolve(m)
		if err != nil {
			reason := "no local checkout"
			if !m.IsRemote() {
				reason = err.Error() + "; run `dendrite synapse heal`"
			}
			notes = append(notes, MemberNote{Member: m.Alias, Reason: reason})
			continue
		}
		chunks, err := search(ctx, dir, query, opts.PerMember, opts.Corpus)
		if err != nil {
			notes = append(notes, MemberNote{Member: m.Alias, Reason: firstLine(err.Error())})
			continue
		}
		all = append(all, fuseOne(m.Alias, dir, chunks)...)
	}
	if len(wanted) > 0 {
		have := map[string]bool{}
		for _, m := range syn.Members {
			have[m.Alias] = true
		}
		for a := range wanted {
			if !have[a] {
				return nil, nil, fmt.Errorf("%q is not a member of synapse %s", a, syn.ID)
			}
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		return all[i].Member < all[j].Member
	})
	if len(all) > opts.TopK {
		all = all[:opts.TopK]
	}
	return all, notes, nil
}

// fuseOne scores one member's ranked list. Rank sets the main term; the hit's score relative to the
// member's best nudges ties between members apart without making the scales comparable.
func fuseOne(alias, dir string, chunks []rag.Chunk) []Hit {
	var top float64
	for _, c := range chunks {
		if s := float64(c.Score); s > top {
			top = s
		}
	}
	out := make([]Hit, 0, len(chunks))
	for i, c := range chunks {
		rel := 1.0
		if top > 0 {
			rel = float64(c.Score) / top
		}
		path := c.FilePath
		if r, err := filepath.Rel(dir, c.FilePath); err == nil && !strings.HasPrefix(r, "..") {
			path = r
		}
		out = append(out, Hit{
			Member: alias, Path: path, LineStart: c.LineStart, LineEnd: c.LineEnd, Heading: c.Heading,
			Content: c.Content, Rank: i + 1,
			Score: 1.0/float64(rrfK+i+1) + rel*1e-6,
		})
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
