package insight

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/rea"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/synapsesearch"
)

const (
	maxQueries   = 12
	perQuery     = 3
	defaultReuse = 10
	maxReuseText = 600
)

// systemLibrary matches the dependencies nearly every image has, which say nothing about what this one does.
var systemLibrary = regexp.MustCompile(`(?i)^(kernel32|user32|advapi32|ntdll|msvcrt|gdi32|shell32|ws2_32|ucrtbase|vcruntime\d*|libc(\.|-)|libm\.|libdl|libpthread|libgcc|libstdc|ld-linux|libsystem|/usr/lib/libsystem|/usr/lib/libc\+\+)`)

// ReuseHit is a place in the project or its synapse that matches what the analysis found.
type ReuseHit struct {
	// Source is "project", or the alias of the synapse member the passage is in.
	Source  string   `json:"source"`
	File    string   `json:"file"`
	Heading string   `json:"heading,omitempty"`
	Text    string   `json:"text"`
	Score   float64  `json:"score"`
	Queries []string `json:"queries"`
}

// ReuseOptions say where to look: the project directory (default ".") and, optionally, which synapse (default: the
// one the project belongs to, when it belongs to exactly one) and how many hits to return.
type ReuseOptions struct {
	Project string
	Synapse string
	K       int
}

// generatedName matches the names a disassembler invents for functions it could not name, which say nothing about
// what a function does.
var generatedName = regexp.MustCompile(`(?i)^(sub_|fun_|fcn\.|sym\.imp\.|loc_|nullsub_|thunk_|entry\d*$|_start$|0x)`)

// functionQueries are the names of the largest functions an engine found, without the generated ones, at most four.
func functionQueries(names []string) []string {
	var named []string
	for _, n := range names {
		if !generatedName.MatchString(n) {
			named = append(named, n)
		}
	}
	return firstN(distinct(named), 4)
}

// Queries are the questions an analysis asks the project: its exports, the names of the functions an engine found,
// its readable strings and the dependencies that are not ubiquitous system libraries, at most twelve, without
// repeats.
func Queries(img *analysis.Image, functions ...string) []string {
	var candidates []string
	candidates = append(candidates, firstN(distinct(img.Exports), 6)...)
	candidates = append(candidates, functionQueries(functions)...)
	for _, s := range img.Strings {
		if n := len(strings.TrimSpace(s)); n >= 6 && n <= 80 {
			candidates = append(candidates, strings.TrimSpace(s))
		}
	}
	for _, lib := range img.Libraries {
		if !systemLibrary.MatchString(lib.Name) {
			candidates = append(candidates, lib.Name)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range candidates {
		if c != "" && !seen[c] && len(out) < maxQueries {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// distinct drops empty items and repeats, keeping the order.
func distinct(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		if it != "" && !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return out
}

func firstN(items []string, n int) []string {
	if len(items) > n {
		return items[:n]
	}
	return items
}

// finder runs one query and returns the passages that match it, best first.
type finder func(ctx context.Context, query string) ([]ReuseHit, error)

// Reuse looks in the project (and the synapse it belongs to) for code that matches what analysis id found, so a
// cleanroom recreation starts from what already exists. Hits are ranked by how high and how often they matched,
// and capped at the 4,000-token evidence boundary.
func Reuse(ctx context.Context, svc *analysis.Service, id string, o ReuseOptions) ([]ReuseHit, error) {
	_, img, err := svc.Inspected(id)
	if err != nil {
		return nil, err
	}
	fns, err := svc.EngineFunctions(id)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(fns))
	for _, f := range fns {
		names = append(names, f.Name)
	}
	project := orDefault(o.Project, ".")
	syn, err := synapseFor(project, o.Synapse)
	if err != nil {
		return nil, err
	}
	find := projectFinder(svc, project)
	if syn != nil {
		find = synapseFinder(syn)
	}
	merged := map[string]*ReuseHit{}
	for _, q := range Queries(img, names...) {
		hits, err := find(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("search for %q: %w", q, err)
		}
		accumulate(merged, q, hits)
	}
	return ranked(merged, o.K), nil
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// projectFinder searches the project directory with the analysis's own RAG index port.
func projectFinder(svc *analysis.Service, project string) finder {
	return func(ctx context.Context, query string) ([]ReuseHit, error) {
		got, err := svc.Index.Search(ctx, project, query, perQuery)
		if err != nil {
			return nil, err
		}
		hits := make([]ReuseHit, 0, len(got))
		for _, h := range got {
			hits = append(hits, ReuseHit{Source: "project", File: h.File, Heading: h.Heading, Text: h.Text})
		}
		return hits, nil
	}
}

// synapseFinder searches every member of the synapse (the project is one of them).
func synapseFinder(syn *synapse.Synapse) finder {
	return func(ctx context.Context, query string) ([]ReuseHit, error) {
		got, _, err := synapsesearch.Search(ctx, syn, query, synapsesearch.Options{TopK: perQuery}, nil)
		hits := make([]ReuseHit, 0, len(got))
		for _, h := range got {
			hits = append(hits, ReuseHit{Source: h.Member, File: h.Path, Heading: h.Heading, Text: h.Content})
		}
		return hits, err
	}
}

// synapseFor finds the synapse to search: the one named, or the only one the project belongs to. A project in no
// synapse has none (nil, nil); one in several is an error that asks which.
func synapseFor(project, id string) (*synapse.Synapse, error) {
	store, err := synapse.DefaultStore()
	if err != nil {
		return nil, err
	}
	if id != "" {
		return store.Load(id)
	}
	pid, err := projectid.Read(project)
	if err != nil {
		return nil, nil
	}
	syns, err := store.ReferencingProject(pid)
	if len(syns) == 0 {
		return nil, err
	}
	if len(syns) > 1 {
		ids := make([]string, len(syns))
		for i, s := range syns {
			ids[i] = s.ID
		}
		return nil, fmt.Errorf("the project is in several synapses (%s): pass one", strings.Join(ids, ", "))
	}
	return syns[0], nil
}

// accumulate adds the hits of one query: a hit near the top scores more, and one found by several queries scores
// each time.
func accumulate(merged map[string]*ReuseHit, query string, hits []ReuseHit) {
	for i, h := range hits {
		key := h.Source + "|" + h.File
		cur, ok := merged[key]
		if !ok {
			h.Queries = nil
			cur = &h
			merged[key] = cur
		}
		cur.Score += 1 / float64(i+1)
		cur.Queries = append(cur.Queries, query)
	}
}

// ranked sorts the merged hits (best score, then name), keeps at most k and cuts them to the token boundary.
func ranked(merged map[string]*ReuseHit, k int) []ReuseHit {
	all := make([]ReuseHit, 0, len(merged))
	for _, h := range merged {
		all = append(all, *h)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		return all[i].Source+all[i].File < all[j].Source+all[j].File
	})
	if k <= 0 {
		k = defaultReuse
	}
	var out []ReuseHit
	tokens := 0
	for _, h := range all {
		if len(out) == k {
			break
		}
		if len(h.Text) > maxReuseText {
			h.Text = h.Text[:maxReuseText] + "..."
		}
		if tokens += rea.EstimateTokens(h.Text); tokens > rea.MaxEvidenceTokens {
			break
		}
		out = append(out, h)
	}
	return out
}
