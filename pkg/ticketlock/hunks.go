package ticketlock

import (
	"Dendrite/pkg/rag"
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Hunk is one contiguous change of one file in a patch.
type Hunk struct {
	Path string
	Test bool
	Text string
}

// splitHunks cuts a patch into its hunks, in patch order. A file with no hunk header (a rename, a mode change)
// is one hunk holding its whole part of the patch.
func splitHunks(patch string) []Hunk {
	var out []Hunk
	for i, chunk := range strings.Split(patch, "\ndiff --git ") {
		if i == 0 {
			var found bool
			if chunk, found = strings.CutPrefix(chunk, "diff --git "); !found {
				continue
			}
		}
		head, body, _ := strings.Cut(chunk, "\n")
		fields := strings.Fields(head)
		if len(fields) < 2 {
			continue
		}
		path, isTest := strings.TrimPrefix(fields[1], "b/"), looksLikeTest(fields[1])
		// The lines before the first @@ are the file's header; a file with no @@ at all is one hunk of its header.
		lines := strings.Split(body, "\n")
		first := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "@@") })
		if first < 0 {
			out = append(out, Hunk{Path: path, Test: isTest, Text: strings.TrimSpace(strings.Join(lines, "\n"))})
			continue
		}
		var current []string
		for _, line := range lines[first:] {
			if strings.HasPrefix(line, "@@") && len(current) > 0 {
				out = append(out, Hunk{Path: path, Test: isTest, Text: strings.Join(current, "\n")})
				current = nil
			}
			current = append(current, line)
		}
		out = append(out, Hunk{Path: path, Test: isTest, Text: strings.Join(current, "\n")})
	}
	return out
}

// changeSummary lists the files of a change with the lines added and removed in each, one per line.
func changeSummary(hunks []Hunk) string {
	type counts struct {
		added, removed int
		test           bool
	}
	byPath := map[string]*counts{}
	for _, h := range hunks {
		c := byPath[h.Path]
		if c == nil {
			c = &counts{test: h.Test}
			byPath[h.Path] = c
		}
		for _, line := range strings.Split(h.Text, "\n") {
			switch {
			case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			case strings.HasPrefix(line, "+"):
				c.added++
			case strings.HasPrefix(line, "-"):
				c.removed++
			}
		}
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		kind := ""
		if byPath[p].test {
			kind = " (test)"
		}
		fmt.Fprintf(&b, "%s%s: +%d -%d\n", p, kind, byPath[p].added, byPath[p].removed)
	}
	return b.String()
}

// looksLikeTest says whether a path is a test file by name or directory.
func looksLikeTest(path string) bool {
	p := "/" + strings.ToLower(path)
	for _, marker := range []string{"_test.", ".test.", ".spec.", "/tests/", "/test/", "/__tests__/", "/test_", "/testdata/"} {
		if strings.Contains(p, marker) {
			return true
		}
	}
	return false
}

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

// wordCounts counts the words of s (letters, digits and underscores, three characters or more), lower-cased.
func wordCounts(s string) map[string]int {
	counts := map[string]int{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }) {
		if len(w) >= 3 {
			counts[w]++
		}
	}
	return counts
}

// lexicalRank orders documents by how many of the query's words they share, rarer words counting more. With no
// word in common the documents keep their order.
func lexicalRank(query string, docs []rag.RankDocument, topK int) []string {
	docWords := make([]map[string]int, len(docs))
	df := map[string]int{}
	for i, d := range docs {
		docWords[i] = wordCounts(d.Text)
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
		for w := range wordCounts(query) {
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
