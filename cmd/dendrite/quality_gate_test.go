package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/quality/coverage"
	"Dendrite/pkg/rag/ragtest"
)

// gateProject is a directory with a few files; dir is also the --path of the command.
func gateProject(t *testing.T, files map[string]string) string {
	t.Helper()
	ragtest.UseStandin(t, "1.11.0")
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAllostasisGateReportsAndExitsNonZeroOnAFailure(t *testing.T) {
	long := strings.Repeat("x := 1\n", 60)
	dir := gateProject(t, map[string]string{
		"a.go": long, "b.go": long,
		directive.Path: "schema = 1\n[loc]\nmax_production_lines = 50\nwarn_at = 40\n[duplication]\nmax_percent = 100.0\n",
	})
	out, code := cliOutput(t, "allostasis", "gate", "--path", dir, "--skip", "coverage")
	if code != ExitError || !strings.Contains(out, "FAIL  loc") || !strings.Contains(out, "a.go: 60 production lines, the limit is 50") || !strings.Contains(out, "quality gate (repository): FAILED") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "PASS  clones") {
		t.Errorf("the duplication allowance is read from the directives:\n%s", out)
	}
	for _, format := range []string{"markdown", "json"} {
		o, c := cliOutput(t, "allostasis", "gate", "--path", dir, "--only", "loc", "--format", format)
		want := map[string]string{"markdown": "Quality gate (repository): **failed**", "json": `"passed": false`}[format]
		if c != ExitError || !strings.Contains(o, want) {
			t.Errorf("%s: exit %d\n%s", format, c, o)
		}
	}
	if o, c := cliOutput(t, "--json", "allostasis", "gate", "--path", dir, "--only", "loc"); c != ExitError || !strings.Contains(o, `"scope": "repository"`) {
		t.Errorf("--json: exit %d\n%s", c, o)
	}
	clean := gateProject(t, map[string]string{"a.go": "x := 1\n"})
	if o, c := cliOutput(t, "allostasis", "gate", "--path", clean, "--skip", "coverage"); c != ExitOK || !strings.Contains(o, "quality gate (repository): passed") {
		t.Errorf("a clean project: exit %d\n%s", c, o)
	}
}

func TestAllostasisGateRefusesBadInput(t *testing.T) {
	dir := gateProject(t, map[string]string{"a.go": "x := 1\n"})
	for name, args := range map[string][]string{
		"unknown check":    {"--only", "lint"},
		"unknown skip":     {"--skip", "nope"},
		"unknown format":   {"--format", "xml"},
		"since and ticket": {"--since", "develop", "--ticket", "DEN-1"},
		"stray argument":   {"extra"},
	} {
		if out, code := cliOutput(t, append([]string{"allostasis", "gate", "--path", dir}, args...)...); code != ExitUsage {
			t.Errorf("%s: exit %d\n%s", name, code, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".github", "dendrite"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(directive.Path)), []byte("[loc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := cliOutput(t, "allostasis", "gate", "--path", dir); code != ExitError || !strings.Contains(out, directive.Path) {
		t.Errorf("broken directives: exit %d\n%s", code, out)
	}
}

func TestAllostasisGateCanJudgeOnlyAChange(t *testing.T) {
	long := strings.Repeat("x := 1\n", 60)
	dir := gateProject(t, map[string]string{"old.go": long, directive.Path: "schema = 1\n[loc]\nmax_production_lines = 50\nwarn_at = 40\n"})
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "develop")
	run("add", ".")
	run("commit", "-q", "-m", "base")
	run("checkout", "-q", "-b", "agent/den-3")
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("y := 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "[DEN-3] add new")

	for _, args := range [][]string{{"--since", "develop"}, {"--ticket", "DEN-3"}} {
		out, code := cliOutput(t, append([]string{"allostasis", "gate", "--path", dir, "--only", "loc"}, args...)...)
		if code != ExitOK || !strings.Contains(out, "quality gate (changes): passed") || !strings.Contains(out, "1 files checked") {
			t.Errorf("%v: old.go is over the limit but the change did not touch it: exit %d\n%s", args, code, out)
		}
	}
	if out, code := cliOutput(t, "allostasis", "gate", "--path", dir, "--only", "loc"); code != ExitError {
		t.Errorf("the whole repository does fail: exit %d\n%s", code, out)
	}
}

func TestAllostasisCoverageFloorFlags(t *testing.T) {
	var got coverage.Options
	useCoverage(t, func(_ context.Context, o coverage.Options) (*coverage.Result, error) {
		got = o
		return &coverage.Result{Passed: false}, nil
	})
	if out, code := cliOutput(t, "allostasis", "coverage", "--min", "85"); code != ExitError || got.MinPercent != 85 || !strings.Contains(out, "coverage floor failed: a package is below 85.0%") {
		t.Fatalf("--min: exit %d %+v\n%s", code, got, out)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "dendrite"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(directive.Path)), []byte("schema = 1\n[coverage]\nmin_percent = 92.5\nexclude = [\"cmd/**\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, code := cliOutput(t, "allostasis", "coverage", "--floor"); code != ExitError || got.MinPercent != 92.5 || len(got.ExemptGlobs) != 1 || got.ExemptGlobs[0] != "cmd/**" {
		t.Fatalf("--floor reads the directives: exit %d %+v", code, got)
	}
	if _, code := cliOutput(t, "allostasis", "coverage", "--floor", "--min", "50"); code != ExitUsage {
		t.Errorf("--min and --floor exclude each other: exit %d", code)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(directive.Path)), []byte("[coverage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := cliOutput(t, "allostasis", "coverage", "--floor"); code != ExitError || !strings.Contains(out, directive.Path) {
		t.Errorf("--floor with broken directives: exit %d\n%s", code, out)
	}
}
