package ticketlock

import (
	"fmt"
	"strings"
	"testing"
)

const mixedPatch = `diff --git a/pkg/directive/parse.go b/pkg/directive/parse.go
new file mode 100644
--- /dev/null
+++ b/pkg/directive/parse.go
@@ -0,0 +1,3 @@
+package directive
+func Parse(text string) []Change { return nil }
-old line
diff --git a/pkg/directive/parse_test.go b/pkg/directive/parse_test.go
new file mode 100644
--- /dev/null
+++ b/pkg/directive/parse_test.go
@@ -0,0 +1,4 @@
+func TestParseKnownSentences(t *testing.T) {
+	got := Parse("enforce 95% code coverage")
+	assert(got)
+}
diff --git a/pkg/directive/parse.go b/pkg/directive/parse.go
--- a/pkg/directive/parse.go
+++ b/pkg/directive/parse.go
@@ -1 +1,2 @@
+// second commit touching the same file
`

func TestReviewViewSummarisesFilesKeepsProductionAndOnlyNamesTests(t *testing.T) {
	got := reviewView(mixedPatch, MaxReviewDiff)
	for _, want := range []string{
		"Files changed:\n",
		"pkg/directive/parse.go: +3 -1\n", // both commits' changes to one file are one line
		"pkg/directive/parse_test.go (test): +4 -0\n",
		"Tests added (declarations only):\npkg/directive/parse_test.go:\n+func TestParseKnownSentences(t *testing.T) {",
		"Changes to production files:\ndiff --git a/pkg/directive/parse.go",
		"+func Parse(text string) []Change { return nil }",
		"+// second commit touching the same file",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("view lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{`+	got := Parse("enforce 95% code coverage")`, "+	assert(got)"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("test bodies stay out of the view: %q\n%s", unwanted, got)
		}
	}
	if len(got) >= len(mixedPatch)+200 {
		t.Errorf("the view is not much longer than the patch it summarises (%d vs %d)", len(got), len(mixedPatch))
	}
}

func TestReviewViewHandlesNothingAndOddInput(t *testing.T) {
	if reviewView("  \n", 1000) != "" {
		t.Error("no patch, no view")
	}
	if got := reviewView("garbage before\ndiff --git broken\nmore", 1000); !strings.Contains(got, "Changes to production files:") {
		t.Errorf("a header with no paths is skipped, not fatal: %q", got)
	}
	if got := reviewView("diff --git a/x.go b/x.go\n+a\n", 1000); !strings.Contains(got, "x.go: +1 -0") || strings.Contains(got, "Tests added") {
		t.Errorf("no tests, no tests section: %q", got)
	}
}

func TestLooksLikeTestByNameAndDirectory(t *testing.T) {
	for path, want := range map[string]bool{
		"a_test.go": true, "web/a.test.ts": true, "web/a.spec.tsx": true, "x/tests/it.rs": true, "x/test/it.rs": true,
		"x/__tests__/a.js": true, "py/test_a.py": true, "pkg/x/testdata/a.toml": true,
		"main.go": false, "pkg/attest.go": false, "pkg/contest/a.go": false,
	} {
		if looksLikeTest(path) != want {
			t.Errorf("looksLikeTest(%q) = %v", path, !want)
		}
	}
}

func TestBranchDiffLeavesOutGeneratedDocsAndAgentFiles(t *testing.T) {
	repo := repoWithMergedWork(t)
	writeFile(t, repo, "docs/cli/x.md", "generated reference\n")
	writeFile(t, repo, ".claude/skills/s/SKILL.md", "generated skill\n")
	writeFile(t, repo, "real.go", "package real\n")
	run := func(args ...string) {
		t.Helper()
		if out, err := gitCommand(repo, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", ".")
	run("-c", "user.name=t", "-c", "user.email=t@e.x", "commit", "-q", "-m", "[DEN-9] generated files too")
	got := branchDiff(repo, "DEN-9")
	if !strings.Contains(got, "real.go") || strings.Contains(got, "docs/cli/x.md") || strings.Contains(got, ".claude/skills") {
		t.Errorf("generated files are not shown:\n%s", got)
	}
}

func TestReviewViewSharesItsBudgetAmongProductionFiles(t *testing.T) {
	var patch strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&patch, "diff --git a/pkg/f%d.go b/pkg/f%d.go\n+first line of file %d\n%s", i, i, i, strings.Repeat("+filler line that goes on and on\n", 200))
	}
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&patch, "diff --git a/pkg/t%d_test.go b/pkg/t%d_test.go\n+func TestNumber%d(t *testing.T) {}\n", i, i, i)
	}
	got := reviewView(patch.String(), 8000)
	if len(got) > 8000+1500 {
		t.Errorf("the view is %d bytes for a budget of 8000", len(got))
	}
	shown := 0
	for i := 0; i < 10; i++ {
		if strings.Contains(got, fmt.Sprintf("+first line of file %d", i)) {
			shown++
		}
	}
	if shown < 4 || shown == 10 || !strings.Contains(got, "more production files not shown: pkg/f") {
		t.Errorf("%d files shown; the rest must be named:\n%.400s", shown, got[len(got)-300:])
	}
	if !strings.Contains(got, "[cut]") || !strings.Contains(got, "TestNumber0") {
		t.Errorf("long sections say they were cut and keep their start")
	}
}
