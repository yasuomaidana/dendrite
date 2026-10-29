package gate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/sidecar"
)

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	if err := fsutil.WriteFileAtomic(filepath.Join(dir, filepath.FromSlash(rel)), []byte(body)); err != nil {
		t.Fatal(err)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.x")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func lines(n int) string { return strings.Repeat("x := 1\n", n) }

func tightSet() directive.Set {
	s := directive.Defaults()
	s.Loc.MaxProductionLines, s.Loc.WarnAt = 10, 5
	s.Duplication.MinLines = 5
	return s
}

func byName(r Report, name string) Check {
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	return Check{}
}

func TestRunReportsEveryCheckOfARepository(t *testing.T) {
	ragtest.UseStandin(t, "1.11.0")
	dir := t.TempDir()
	write(t, dir, "a.go", lines(12))
	write(t, dir, "b.go", lines(12))
	write(t, dir, "c.go", lines(2))
	write(t, dir, "r.rs", "REPEATED_TEST\n")
	rep := Run(context.Background(), Options{Dir: dir, Set: tightSet(), Sidecar: rag.NewRetriever()})
	if rep.Passed || rep.Scope != "repository" || len(rep.Checks) != 4 {
		t.Fatalf("%+v", rep)
	}
	loc := byName(rep, "loc")
	if loc.Status != Fail || len(loc.Findings) != 2 || !strings.HasPrefix(loc.Findings[0], "a.go: 12 production lines, the limit is 10") {
		t.Errorf("loc = %+v", loc)
	}
	if c := byName(rep, "clones"); c.Status != Fail || len(c.Findings) != 1 || !strings.Contains(c.Findings[0], "a.go:1-12, b.go:1-12") ||
		!strings.Contains(c.Advice, "helper function") || !strings.Contains(c.Advice, "macro") {
		t.Errorf("clones = %+v", c)
	}
	if c := byName(rep, "rstest"); c.Status != Fail || !strings.Contains(c.Findings[0], "r.rs:1 [repeated_test]") {
		t.Errorf("rstest = %+v", c)
	}
	if c := byName(rep, "coverage"); c.Status != Skipped || !strings.Contains(c.Summary, "not a Go module") {
		t.Errorf("coverage = %+v", c)
	}
}

func TestRunPassesACleanRepositoryAndOnlyRunsWhatWasAskedFor(t *testing.T) {
	ragtest.UseStandin(t, "1.11.0")
	dir := t.TempDir()
	write(t, dir, "a.go", lines(2))
	write(t, dir, "b.go", lines(3))
	rep := Run(context.Background(), Options{Dir: dir, Set: tightSet(), Sidecar: rag.NewRetriever()})
	if !rep.Passed || byName(rep, "rstest").Status != Skipped || !strings.Contains(byName(rep, "rstest").Summary, "no Rust files") {
		t.Errorf("%+v", rep)
	}
	only := Run(context.Background(), Options{Dir: dir, Set: tightSet(), Sidecar: rag.NewRetriever(), Only: []string{"loc", "CLONES"}, Skip: []string{"loc"}})
	if len(only.Checks) != 2 || only.Checks[0].Name != "loc" || only.Checks[1].Name != "clones" {
		t.Errorf("Only wins over Skip, in the gate's order: %+v", only.Checks)
	}
	skip := Run(context.Background(), Options{Dir: dir, Set: tightSet(), Sidecar: rag.NewRetriever(), Skip: []string{"loc", "clones", "coverage"}})
	if len(skip.Checks) != 1 || skip.Checks[0].Name != "rstest" {
		t.Errorf("%+v", skip.Checks)
	}
	if err := Validate([]string{"loc", "Clones"}); err != nil {
		t.Error(err)
	}
	if err := Validate([]string{"loc", "lint"}); err == nil || !strings.Contains(err.Error(), `unknown check "lint"`) {
		t.Errorf("%v", err)
	}
}

func TestACheckThatCannotRunFailsTheGate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(sidecar.EnvVar("dendrite-rag"), filepath.Join(dir, "no-such-binary"))
	write(t, dir, "a.go", lines(1))
	rep := Run(context.Background(), Options{Dir: dir, Set: tightSet(), Sidecar: rag.NewRetriever(), Skip: []string{"coverage"}})
	if rep.Passed {
		t.Fatal("a check that did not run proves nothing")
	}
	for _, name := range []string{"loc", "clones", "rstest"} {
		if c := byName(rep, name); c.Status != Error || !strings.Contains(c.Summary, "dendrite-rag") {
			t.Errorf("%s = %+v", name, c)
		}
	}
}

func TestRunWithinTheDuplicationAllowancePasses(t *testing.T) {
	ragtest.UseStandin(t, "1.11.0")
	dir := t.TempDir()
	write(t, dir, "a.go", lines(12))
	write(t, dir, "b.go", lines(12))
	set := tightSet()
	set.Duplication.MaxPercent = 100
	rep := Run(context.Background(), Options{Dir: dir, Set: set, Sidecar: rag.NewRetriever(), Only: []string{"clones"}})
	if c := byName(rep, "clones"); c.Status != Pass || !strings.Contains(c.Summary, "allowed 100.00%") {
		t.Errorf("%+v", c)
	}
}

func TestReportFormats(t *testing.T) {
	rep := Report{Schema: 1, Scope: "changes", Checks: []Check{
		{Name: "loc", Status: Fail, Summary: "1 over", Findings: []string{"a.go: 12 lines"}, Advice: "split the file"},
		{Name: "rstest", Status: Skipped, Summary: "no Rust files"},
	}}
	text := rep.Text()
	for _, want := range []string{"FAIL  loc       1 over", "        - a.go: 12 lines", "        > split the file", "skip  rstest", "quality gate (changes): FAILED"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	md := rep.Markdown()
	for _, want := range []string{"Quality gate (changes): **failed**", "- `loc` fail: 1 over", "  - a.go: 12 lines", "  - split the file"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	rep.Passed = true
	if !strings.Contains(rep.Text(), ": passed") || !strings.Contains(rep.Markdown(), "**passed**") || !strings.Contains(rep.JSON(), `"passed": true`) {
		t.Error("a passing report says so in every format")
	}
}
