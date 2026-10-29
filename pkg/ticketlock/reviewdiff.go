package ticketlock

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// generatedPaths are left out of a review diff: files Dendrite or a package manager writes, which say nothing
// about the work and drown a small model.
var generatedPaths = []string{
	":(exclude)*.lock", ":(exclude)go.sum", ":(exclude)package-lock.json", ":(exclude)pnpm-lock.yaml", ":(exclude)bun.lockb",
	":(exclude)docs/cli/**", ":(exclude)docs/mcp/**", ":(exclude).agents/**", ":(exclude).claude/**", ":(exclude).gemini/**",
}

// declaration matches an added line that declares a test, a function or a test case.
var declaration = regexp.MustCompile(`^\+\s*(func |fn |def |async fn |it\(|test\(|describe\(|#\[case|#\[test|@Test)`)

// fileDiff is one file's part of a patch.
type fileDiff struct {
	path         string
	body         string
	added, gone  int
	declarations []string
	isTest       bool
}

// reviewView turns a patch into what a local model can judge: a summary line per file, the changes to
// production files in full, and for test files only the declarations they add (the test names are what show a
// criterion is covered; their bodies are most of the bytes). The result fits budget: the summary and the test
// names are capped, and the production files share what is left, each keeping its start.
func reviewView(patch string, budget int) string {
	if strings.TrimSpace(patch) == "" {
		return ""
	}
	byPath := map[string]*fileDiff{}
	var order []string
	for i, chunk := range strings.Split(patch, "\ndiff --git ") {
		if i == 0 && !strings.HasPrefix(chunk, "diff --git ") {
			continue
		}
		chunk = strings.TrimPrefix(chunk, "diff --git ")
		head, _, _ := strings.Cut(chunk, "\n")
		fields := strings.Fields(head)
		if len(fields) < 2 {
			continue
		}
		path := strings.TrimPrefix(fields[1], "b/")
		f := byPath[path]
		if f == nil {
			f = &fileDiff{path: path, isTest: looksLikeTest(path)}
			byPath[path] = f
			order = append(order, path)
		}
		f.body += "diff --git " + chunk + "\n"
		for _, line := range strings.Split(chunk, "\n") {
			switch {
			case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			case strings.HasPrefix(line, "+"):
				f.added++
				if declaration.MatchString(line) {
					f.declarations = append(f.declarations, line)
				}
			case strings.HasPrefix(line, "-"):
				f.gone++
			}
		}
	}
	sort.Strings(order)
	var summary, tests strings.Builder
	var production []*fileDiff
	for _, path := range order {
		f := byPath[path]
		kind := ""
		if f.isTest {
			kind = " (test)"
		}
		fmt.Fprintf(&summary, "%s%s: +%d -%d\n", f.path, kind, f.added, f.gone)
		if f.isTest {
			fmt.Fprintf(&tests, "%s:\n%s\n", f.path, strings.Join(f.declarations[:min(len(f.declarations), 40)], "\n"))
		} else {
			production = append(production, f)
		}
	}
	out := "Files changed:\n" + capText(summary.String(), budget/5)
	if tests.Len() > 0 {
		out += "\nTests added (declarations only):\n" + capText(tests.String(), budget/5)
	}
	out += "\nChanges to production files:\n"
	if len(production) == 0 {
		return out
	}
	// Every shown file gets at least minFileBytes; files that do not fit are named, not shown.
	room := max(budget-len(out), minFileBytes)
	shown := min(len(production), room/minFileBytes)
	per := room / shown
	for _, f := range production[:shown] {
		out += capText(f.body, per)
	}
	if hidden := production[shown:]; len(hidden) > 0 {
		out += fmt.Sprintf("\n[%d more production files not shown:", len(hidden))
		for _, f := range hidden {
			out += " " + f.path
		}
		out += "]\n"
	}
	return out
}

// minFileBytes is the least of a production file's patch worth showing to a reviewer.
const minFileBytes = 700

// capText keeps the first n bytes of s and says when it cut.
func capText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[cut]\n"
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
