package gate

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
)

const sampleDiff = `diff --git a/pkg/a.go b/pkg/a.go
index 111..222 100644
--- a/pkg/a.go
+++ b/pkg/a.go
@@ -3,0 +4,2 @@ func x() {
+added one
+added two
@@ -10 +12 @@ func y() {
-old
+new
@@ -20,2 +22,0 @@ func z() {
-gone
-gone
diff --git a/pkg/old.go b/pkg/old.go
deleted file mode 100644
--- a/pkg/old.go
+++ /dev/null
`

func TestScopeKnowsTheFilesAndAddedLinesOfAPatch(t *testing.T) {
	s := &Scope{lines: map[string][][2]int{}}
	s.addDiff(sampleDiff)
	files := s.Files()
	sort.Strings(files)
	if !reflect.DeepEqual(files, []string{"pkg/a.go", "pkg/old.go"}) {
		t.Fatalf("files = %v", files)
	}
	if !s.HasFile("pkg/a.go") || s.HasFile("pkg/b.go") {
		t.Error("HasFile")
	}
	for _, c := range []struct {
		start, end int
		want       bool
	}{{4, 5, true}, {1, 3, false}, {5, 9, true}, {6, 11, false}, {12, 12, true}, {13, 30, false}, {22, 22, false}} {
		if got := s.Touches("pkg/a.go", c.start, c.end); got != c.want {
			t.Errorf("Touches(a.go, %d, %d) = %v, want %v", c.start, c.end, got, c.want)
		}
	}
	if !s.Touches("pkg/old.go", 1, 1) || s.Touches("pkg/none.go", 1, 1) {
		t.Error("a deleted file counts as touched on every line; an untouched file as not at all")
	}
}

// repoWithChange has develop holding a clean base, and a branch that adds a duplicated block to one file while
// another file (already over the limit) is left alone.
func repoWithChange(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "develop")
	write(t, dir, "legacy.go", lines(30))
	write(t, dir, "twin.go", lines(12))
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	gitIn(t, dir, "checkout", "-q", "-b", "agent/den-7")
	write(t, dir, "fresh.go", lines(12))
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "[DEN-7] add fresh")
	return dir
}

func TestChangesSinceAndOfTicketFindWhatTheBranchDid(t *testing.T) {
	dir := repoWithChange(t)
	s := ChangesSince(dir, "develop")
	if got := s.Files(); !reflect.DeepEqual(got, []string{"fresh.go"}) || !s.Touches("fresh.go", 5, 6) {
		t.Fatalf("unmerged branch: %v", got)
	}
	write(t, dir, "twin.go", lines(12)+"y := 2\n")
	if s := ChangesSince(dir, "develop"); !s.HasFile("twin.go") || s.Touches("twin.go", 1, 12) || !s.Touches("twin.go", 13, 13) {
		t.Error("uncommitted changes count, down to the lines")
	}
	if s := ChangesOfTicket(dir, "DEN-7"); !s.HasFile("fresh.go") {
		t.Error("ChangesOfTicket on an unmerged branch")
	}

	// Once merged, the branch is no different from develop: the ticket's own commits show its work.
	gitIn(t, dir, "checkout", "-q", "--", ".")
	gitIn(t, dir, "checkout", "-q", "develop")
	gitIn(t, dir, "merge", "-q", "--no-ff", "-m", "merge", "agent/den-7")
	if s := ChangesOfTicket(dir, "DEN-7"); !reflect.DeepEqual(s.Files(), []string{"fresh.go"}) {
		t.Errorf("merged ticket: %v", s.Files())
	}
	if s := ChangesOfTicket(dir, "DEN-404"); len(s.Files()) != 0 {
		t.Errorf("a ticket with no commits changed nothing: %v", s.Files())
	}
	shas, lines := TicketCommits(dir, "DEN-7")
	if len(shas) != 1 || !strings.HasSuffix(lines[0], " [DEN-7] add fresh") {
		t.Errorf("%v %v", shas, lines)
	}
	if s, l := TicketCommits(t.TempDir(), "DEN-7"); s != nil || l != nil {
		t.Error("not a repository")
	}
}

func TestAScopedGateOnlyJudgesWhatTheChangeTouched(t *testing.T) {
	ragtest.UseStandin(t, "1.11.0")
	dir := repoWithChange(t)
	set := tightSet()
	rep := Run(context.Background(), Options{Dir: dir, Set: set, Sidecar: rag.NewRetriever(), Scope: ChangesSince(dir, "develop"), Skip: []string{"coverage"}})
	if rep.Scope != "changes" || rep.Passed {
		t.Fatalf("%+v", rep)
	}
	loc := byName(rep, "loc")
	if len(loc.Findings) != 1 || !strings.HasPrefix(loc.Findings[0], "fresh.go: 12") || !strings.Contains(loc.Summary, "1 files checked") {
		t.Errorf("legacy.go is over the limit too but this change did not touch it: %+v", loc)
	}
	clones := byName(rep, "clones")
	if clones.Status != Fail || len(clones.Findings) != 1 || !strings.Contains(clones.Findings[0], "fresh.go:1-12") || !strings.Contains(clones.Summary, "1 repeated blocks") {
		t.Errorf("the duplicated block involves lines the change added: %+v", clones)
	}

	untouched := &Scope{lines: map[string][][2]int{"legacy.go": {{1, 1}}}}
	rep = Run(context.Background(), Options{Dir: dir, Set: set, Sidecar: rag.NewRetriever(), Scope: untouched, Only: []string{"clones", "rstest"}})
	if byName(rep, "clones").Status != Pass {
		t.Errorf("a change that touched none of the repeated lines is not judged for them: %+v", byName(rep, "clones"))
	}
}

func TestAScopedRstestOnlyReportsTouchedFiles(t *testing.T) {
	ragtest.UseStandin(t, "1.11.0")
	dir := t.TempDir()
	write(t, dir, "old.rs", "REPEATED_TEST\n")
	write(t, dir, "new.rs", "REPEATED_TEST\n")
	scope := &Scope{lines: map[string][][2]int{"new.rs": {{1, 1}}}}
	c := byName(Run(context.Background(), Options{Dir: dir, Set: tightSet(), Sidecar: rag.NewRetriever(), Scope: scope, Only: []string{"rstest"}}), "rstest")
	if len(c.Findings) != 1 || !strings.HasPrefix(c.Findings[0], "new.rs:1") {
		t.Errorf("%+v", c)
	}
}
