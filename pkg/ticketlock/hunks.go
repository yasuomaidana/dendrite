package ticketlock

import (
	"fmt"
	"slices"
	"sort"
	"strings"
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
