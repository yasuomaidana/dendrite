package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/llmchat"
	"Dendrite/pkg/llmchat/llmchattest"
)

// directiveRig is a project directory, -y off, a non-terminal stdin and a model that is never asked unless a
// test says so.
type directiveRig struct {
	t   *testing.T
	dir string
}

func newDirectiveRig(t *testing.T) *directiveRig {
	t.Helper()
	prevYes, prevTerm, prevChat := globals.Yes, isTerminal, directiveChat
	globals.Yes = false
	isTerminal = func(io.Reader) bool { return false }
	directiveChat = func(string) llmchat.Completer { return llmchat.Client{URL: "http://127.0.0.1:1"} } // nothing listens: asking fails
	t.Cleanup(func() { globals.Yes, isTerminal, directiveChat = prevYes, prevTerm, prevChat })
	return &directiveRig{t: t, dir: t.TempDir()}
}

func (r *directiveRig) run(args ...string) (string, error) {
	r.t.Helper()
	cmd := newDirectiveCmd()
	cmd.SetArgs(append(args, "--path", r.dir))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	return out.String(), err
}

func (r *directiveRig) read(rel string) string {
	r.t.Helper()
	data, err := os.ReadFile(filepath.Join(r.dir, rel))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(data)
}

func TestDirectiveRuleShowsTheDiffSavesAndRegenerates(t *testing.T) {
	r := newDirectiveRig(t)
	globals.Yes = true
	out, err := r.run("enforce 95% code coverage")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"coverage.min_percent = 95", "- min_percent = 100.0", "+ min_percent = 95.0", "does not exist yet", "[✓] Saved " + directive.Path, "[✓] Regenerated codecov.yml"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	set, _ := directive.Load(r.dir)
	if set.Coverage.MinPercent != 95 || !strings.Contains(r.read("codecov.yml"), "target: 95%") {
		t.Errorf("saved %+v", set)
	}
	out, err = r.run("enforce 95% code coverage")
	if err != nil || !strings.Contains(out, "Already so") {
		t.Errorf("a rule that changes nothing says so: %q, %v", out, err)
	}
}

func TestDirectiveAsksInATerminal(t *testing.T) {
	r := newDirectiveRig(t)
	if _, err := directive.EnsureDefaults(r.dir); err != nil {
		t.Fatal(err)
	}
	_, err := r.run("max 300 lines")
	var usage *usageError
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "pass -y") {
		t.Fatalf("without a terminal or -y it refuses: %v", err)
	}
	if set, _ := directive.Load(r.dir); set.Loc.MaxProductionLines != 500 {
		t.Error("a refused change writes nothing")
	}

	isTerminal = func(io.Reader) bool { return true }
	for _, tc := range []struct{ answer, wantLimit, want string }{{"n\n", "500", "Left as it was"}, {"y\n", "300", "Saved"}} {
		cmd := newDirectiveCmd()
		cmd.SetArgs([]string{"max 300 lines", "--path", r.dir})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetIn(strings.NewReader(tc.answer))
		cmd.SetContext(context.Background())
		if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), tc.want) || !strings.Contains(out.String(), "Apply? [y/N]") {
			t.Fatalf("answer %q: %v\n%s", tc.answer, err, out.String())
		}
		set, _ := directive.Load(r.dir)
		if got := set.Loc.MaxProductionLines; got != map[string]int{"500": 500, "300": 300}[tc.wantLimit] {
			t.Errorf("after %q the limit is %d", tc.answer, got)
		}
	}
}

func TestDirectiveReadsUnknownRulesThroughTheModel(t *testing.T) {
	r := newDirectiveRig(t)
	globals.Yes = true
	relax := llmchattest.New(t, `[{"key":"lint.strictness","value":"standard","rationale":"asked to relax lint"}]`)
	directiveChat = func(string) llmchat.Completer { return relax.Client }
	out, err := r.run("go easier on the linters")
	if err != nil || !strings.Contains(out, "asking the local model") || !strings.Contains(out, "lint.strictness = standard") {
		t.Fatalf("%v\n%s", err, out)
	}
	if set, _ := directive.Load(r.dir); set.Lint.Strictness != "standard" {
		t.Errorf("saved %+v", set)
	}
	down := llmchattest.Failing(t, 503)
	directiveChat = func(string) llmchat.Completer { return down.Client }
	if _, err := r.run("make it faster"); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Errorf("a model failure is reported: %v", err)
	}
	directiveChat = func(string) llmchat.Completer {
		return fakeCompleter{answer: `[{"key":"coverage.min_percent","value":900}]`}
	}
	if _, err := r.run("make it faster"); err == nil || !strings.Contains(err.Error(), "between 0 and 100") {
		t.Errorf("a proposal outside the limits is refused: %v", err)
	}
}

func TestDirectiveSetListAndApply(t *testing.T) {
	r := newDirectiveRig(t)
	globals.Yes = true
	out, err := r.run("set", "coverage.min_percent=90%", "loc.max_production_lines=400", "loc.warn_at=350")
	if err != nil || !strings.Contains(out, "[✓] Saved") {
		t.Fatalf("%v\n%s", err, out)
	}
	list, err := r.run("list")
	if err != nil || !strings.Contains(list, "min_percent = 90.0") || strings.Contains(list, "no .github") {
		t.Errorf("list:\n%s\n%v", list, err)
	}
	for _, bad := range [][]string{{"set", "loc.warn_at"}, {"set", "nope=1"}, {"set", "loc.warn_at=x"}, {"set", "loc.max_production_lines=100"}} {
		if _, err := r.run(bad...); err == nil {
			t.Errorf("%v must be refused", bad)
		}
	}

	if err := os.Remove(filepath.Join(r.dir, "codecov.yml")); err != nil {
		t.Fatal(err)
	}
	out, err = r.run("apply")
	if err != nil || !strings.Contains(out, "[✓] Regenerated codecov.yml") {
		t.Errorf("apply regenerates what is missing: %v\n%s", err, out)
	}

	fresh := newDirectiveRig(t)
	if out, err := fresh.run("list"); err != nil || !strings.Contains(out, "these are the defaults") {
		t.Errorf("list without a file:\n%s\n%v", out, err)
	}
	if out, err := fresh.run("apply"); err != nil || !strings.Contains(out, "Created "+directive.Path) || !directive.Exists(fresh.dir) {
		t.Errorf("apply without a file creates the defaults:\n%s\n%v", out, err)
	}
}

func TestDirectiveWithoutArgumentsShowsHelpAndBrokenFilesAreErrors(t *testing.T) {
	r := newDirectiveRig(t)
	if out, err := r.run(); err != nil || !strings.Contains(out, "dendrite directive set coverage.min_percent=95") {
		t.Errorf("help:\n%s\n%v", out, err)
	}
	if err := os.MkdirAll(filepath.Join(r.dir, ".github", "dendrite"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, filepath.FromSlash(directive.Path)), []byte("[loc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	globals.Yes = true
	for _, args := range [][]string{{"max 300 lines"}, {"list"}, {"apply"}, {"set", "loc.warn_at=1"}} {
		if _, err := r.run(args...); err == nil || !strings.Contains(err.Error(), directive.Path) {
			t.Errorf("%v with a broken file: %v", args, err)
		}
	}
}
