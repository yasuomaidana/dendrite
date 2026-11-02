package gate

import (
	"context"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/testkit"
)

func runOnly(t *testing.T, dir string, set directive.Set, scope *Scope, name string) Check {
	t.Helper()
	rep := Run(context.Background(), Options{Dir: dir, Set: set, Sidecar: rag.NewRetriever(), Scope: scope, Only: []string{name}})
	return byName(rep, name)
}

func TestComplexityFailsOnlyTheFunctionsOverTheLimit(t *testing.T) {
	ragtest.UseStandin(t, "1.12.0")
	dir := t.TempDir()
	write(t, dir, "old.go", "COMPLEX_FUNCTION\n")
	write(t, dir, "new.go", "COMPLEX_FUNCTION\n")
	write(t, dir, "ok.go", "x := 1\n")

	c := runOnly(t, dir, directive.Defaults(), nil, "complexity")
	testkit.Equal(t, "status", c.Status, Fail)
	testkit.Equal(t, "findings", len(c.Findings), 2)
	if !strings.Contains(c.Findings[0], "Complex: cyclomatic 99, cognitive 99") || !strings.Contains(c.Advice, "split the function") {
		t.Errorf("%+v", c)
	}

	scoped := runOnly(t, dir, directive.Defaults(), &Scope{lines: map[string][][2]int{"new.go": {{1, 1}}}}, "complexity")
	testkit.Equal(t, "scoped findings", len(scoped.Findings), 1)
	if !strings.HasPrefix(scoped.Findings[0], "new.go:1 Complex") {
		t.Errorf("only the touched file is judged: %+v", scoped)
	}

	untouched := runOnly(t, dir, directive.Defaults(), &Scope{lines: map[string][][2]int{"ok.go": {{1, 1}}}}, "complexity")
	testkit.Equal(t, "untouched", untouched.Status, Pass)
}

func TestStructureFailsAPileOfFilesAndAMissingReadme(t *testing.T) {
	ragtest.UseStandin(t, "1.12.0")
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		write(t, dir, "pkg/"+n+".go", "x := 1\n")
	}
	set := directive.Defaults()
	set.Structure.MaxFilesPerDir = 2

	c := runOnly(t, dir, set, nil, "structure")
	testkit.Equal(t, "status", c.Status, Fail)
	testkit.Equal(t, "findings", len(c.Findings), 2) // files in pkg, and no README.md at the root
	if !strings.Contains(c.Findings[0], "pkg [files 3, limit 2]") || !strings.Contains(c.Findings[1], "[docs 1, limit 0]") ||
		!strings.Contains(c.Advice, "sub-package") {
		t.Errorf("%+v", c)
	}

	scoped := runOnly(t, dir, set, &Scope{lines: map[string][][2]int{"pkg/a.go": {{1, 1}}}}, "structure")
	testkit.Equal(t, "scoped findings", len(scoped.Findings), 1)

	set.Structure.RequireDocs = false
	set.Structure.MaxFilesPerDir = 12
	testkit.Equal(t, "clean", runOnly(t, dir, set, nil, "structure").Status, Pass)
}

func TestTheNewChecksNeedARecentEnoughSidecar(t *testing.T) {
	ragtest.UseStandin(t, "1.11.0")
	dir := t.TempDir()
	write(t, dir, "a.go", "x := 1\n")
	for _, name := range []string{"complexity", "structure"} {
		c := runOnly(t, dir, directive.Defaults(), nil, name)
		if c.Status != Error || !strings.Contains(c.Summary, "needs 1.12") {
			t.Errorf("%s: %+v", name, c)
		}
	}
}

func TestTouchHelpers(t *testing.T) {
	scope := &Scope{lines: map[string][][2]int{"pkg/a.go": {{3, 4}}}}
	for name, tc := range map[string]struct {
		got, want bool
	}{
		"no scope touches everything":      {touchesSpan(nil, "x.go", 1, 2), true},
		"a file-only finding by file":      {touchesSpan(scope, "pkg/a.go", 0, 0), true},
		"lines inside the change":          {touchesSpan(scope, "pkg/a.go", 1, 3), true},
		"lines outside the change":         {touchesSpan(scope, "pkg/a.go", 5, 9), false},
		"an inverted span is one line":     {touchesSpan(scope, "pkg/a.go", 4, 1), true},
		"no scope, any file":               {touchesAnyFile(nil, "d", nil), true},
		"a listed file is touched":         {touchesAnyFile(scope, "pkg", []string{"pkg/a.go"}), true},
		"a bare name is joined to the dir": {touchesAnyFile(scope, "pkg", []string{"a.go"}), true},
		"nothing listed is touched":        {touchesAnyFile(scope, "pkg", []string{"pkg/b.go", "c.go"}), false},
	} {
		testkit.Equal(t, name, tc.got, tc.want)
	}
}
