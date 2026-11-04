package gate

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
)

// scopedOutput is what `go test -cover ./pkg/b` reports: only the requested package.
const scopedOutput = "ok  \tmod/pkg/b\t0.1s\tcoverage: 71.0% of statements\n"

const goTestOutput = "ok  \tmod/pkg/a\t0.1s\tcoverage: 100.0% of statements\nok  \tmod/pkg/b\t0.1s\tcoverage: 71.0% of statements\nok  \tmod/pkg/x/planetest\t0.1s\tcoverage: 2.0% of statements\n"

// goProject is a Go module and a recorder of the `go test` arguments the coverage check used.
func goProject(t *testing.T) (dir string, args *[]string) {
	t.Helper()
	dir = t.TempDir()
	write(t, dir, "go.mod", "module mod\n\ngo 1.22\n")
	for _, pkg := range []string{"pkg/b", "pkg/c"} {
		write(t, dir, pkg+"/x.go", "package x\n")
	}
	return dir, new([]string)
}

func runner(output string, err error, args *[]string) CommandRunner {
	return func(_ context.Context, _, _ string, a ...string) ([]byte, error) {
		*args = append([]string(nil), a...)
		return []byte(output), err
	}
}

func coverageOf(o Options) Check { return checkCoverage(context.Background(), o) }

func TestCoverageOfARepositoryIsHeldToTheFloor(t *testing.T) {
	dir, args := goProject(t)
	c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), CoverageRunner: runner(goTestOutput, nil, args)})
	if c.Status != Fail || !reflect.DeepEqual(c.Findings, []string{"pkg/b: 71.0%"}) || !strings.Contains(c.Summary, "floor of 100%") {
		t.Errorf("%+v", c)
	}
	if !reflect.DeepEqual(*args, []string{"test", "-cover", "./..."}) {
		t.Errorf("go test args = %v", *args)
	}
	set := directive.Defaults()
	set.Coverage.MinPercent = 70
	if c := coverageOf(Options{Dir: dir, Set: set, CoverageRunner: runner(goTestOutput, nil, args)}); c.Status != Pass {
		t.Errorf("above the floor: %+v", c)
	}
	set.Coverage.Exclude = []string{"pkg/b"}
	set.Coverage.MinPercent = 100
	if c := coverageOf(Options{Dir: dir, Set: set, CoverageRunner: runner(goTestOutput, nil, args)}); c.Status != Pass {
		t.Errorf("the directive can exclude a package: %+v", c)
	}
}

func TestCoverageOfAChangeCoversOnlyItsPackages(t *testing.T) {
	dir, args := goProject(t)
	scope := &Scope{lines: map[string][][2]int{"pkg/b/b.go": {{1, 2}}, "pkg/b/b_test.go": {{1, 1}}, "pkg/c/c.go": nil, "README.md": nil}}
	c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), Scope: scope, CoverageRunner: runner(scopedOutput, nil, args)})
	if !reflect.DeepEqual(*args, []string{"test", "-cover", "./pkg/b", "./pkg/c"}) {
		t.Errorf("only the touched packages are measured: %v", *args)
	}
	if c.Status != Fail || !strings.Contains(c.Summary, "floor of 100%") {
		t.Errorf("without a baseline the floor applies: %+v", c)
	}

	write(t, dir, ".github/coverage_baseline.json", `{"pkg/b": 80.0}`)
	c = coverageOf(Options{Dir: dir, Set: directive.Defaults(), Scope: scope, CoverageRunner: runner(scopedOutput, nil, args)})
	if c.Status != Fail || !reflect.DeepEqual(c.Findings, []string{"pkg/b: 71.0%, it was 80.0%"}) || !strings.Contains(c.Summary, "ratchet against .github/coverage_baseline.json") {
		t.Errorf("with a baseline a change may not lose coverage: %+v", c)
	}
	write(t, dir, ".github/coverage_baseline.json", `{"pkg/b": 71.0}`)
	if c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), Scope: scope, CoverageRunner: runner(scopedOutput, nil, args)}); c.Status != Pass {
		t.Errorf("a package that kept its coverage passes the ratchet: %+v", c)
	}
}

func TestCoverageSkipsAPackageTheChangeDeleted(t *testing.T) {
	dir, args := goProject(t)
	scope := &Scope{lines: map[string][][2]int{"pkg/b/b.go": nil, "pkg/gone/gone.go": nil}}
	coverageOf(Options{Dir: dir, Set: directive.Defaults(), Scope: scope, CoverageRunner: runner(scopedOutput, nil, args)})
	if !reflect.DeepEqual(*args, []string{"test", "-cover", "./pkg/b"}) {
		t.Errorf("a deleted package is not measured: %v", *args)
	}
	onlyGone := &Scope{lines: map[string][][2]int{"pkg/gone/gone.go": nil}}
	if c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), Scope: onlyGone, CoverageRunner: runner(scopedOutput, nil, args)}); c.Status != Skipped {
		t.Errorf("nothing left to measure is skipped: %+v", c)
	}
}

func TestCoverageIsSkippedOrFailedWhenItCannotRun(t *testing.T) {
	dir, args := goProject(t)
	noGo := &Scope{lines: map[string][][2]int{"README.md": nil}}
	if c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), Scope: noGo, CoverageRunner: runner("", nil, args)}); c.Status != Skipped || !strings.Contains(c.Summary, "no Go package changed") {
		t.Errorf("%+v", c)
	}
	off := directive.Defaults()
	off.Coverage.MinPercent = 0
	if c := coverageOf(Options{Dir: dir, Set: off, CoverageRunner: runner("", nil, args)}); c.Status != Skipped || !strings.Contains(c.Summary, "no coverage floor") {
		t.Errorf("%+v", c)
	}
	if c := coverageOf(Options{Dir: t.TempDir(), Set: directive.Defaults()}); c.Status != Skipped || !strings.Contains(c.Summary, "no Go module") {
		t.Errorf("%+v", c)
	}
}

func TestCoverageFailsWhenTheGoTestsFail(t *testing.T) {
	dir, args := goProject(t)
	failing := "FAIL\tmod/pkg/a\t0.1s\n"
	c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), CoverageRunner: runner(failing, errors.New("exit status 1"), args)})
	if c.Status != Fail || !reflect.DeepEqual(c.Findings, []string{"the tests did not pass"}) {
		t.Errorf("tests that fail fail the check: %+v", c)
	}
}

func TestCoverageIsAnErrorWhenTheBaselineIsBroken(t *testing.T) {
	dir, args := goProject(t)
	write(t, dir, ".github/coverage_baseline.json", "{not json")
	scope := &Scope{lines: map[string][][2]int{"pkg/b/b.go": nil}}
	if c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), Scope: scope, CoverageRunner: runner(goTestOutput, nil, args)}); c.Status != Error || !strings.Contains(c.Summary, "baseline") {
		t.Errorf("a broken baseline is an error: %+v", c)
	}
}

func TestTheCoverageRunWhereNoRunnerIsGivenDoesNotSeeTheLiveEnvironment(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "live-key")
	t.Setenv("DENDRITE_HOME", "/live/home")
	t.Setenv("LITELLM_MASTER_KEY", "live")
	t.Setenv("KEEP_ME", "yes")
	out, err := cleanRunner(context.Background(), t.TempDir(), "sh", "-c", `printf "%s|%s|%s|%s" "$PLANE_API_KEY" "$DENDRITE_HOME" "$LITELLM_MASTER_KEY" "$KEEP_ME"`)
	if err != nil || string(out) != "|||yes" {
		t.Errorf("child saw %q, %v", out, err)
	}
	got := CleanEnv([]string{"PATH=/bin", "PLANE_API_KEY=k", "DENDRITE_HOME=/h", "LITELLM_MASTER_KEY=m", "HOME=/u", "PLANEX=keep"})
	if strings.Join(got, " ") != "PATH=/bin HOME=/u PLANEX=keep" {
		t.Errorf("env = %v", got)
	}
}

func TestCoverageRunsTheRealGoTestWhenNoRunnerIsGiven(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module probe\n\ngo 1.22\n")
	write(t, dir, "p.go", "package probe\n\nfunc Add(a, b int) int { return a + b }\n")
	write(t, dir, "p_test.go", "package probe\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n")
	c := coverageOf(Options{Dir: dir, Set: directive.Defaults()})
	if c.Status != Pass || !strings.Contains(c.Summary, "floor of 100%") {
		t.Errorf("a fully covered module meets the floor: %+v", c)
	}
	write(t, dir, "q.go", "package probe\n\nfunc Sub(a, b int) int { return a - b }\n")
	if c := coverageOf(Options{Dir: dir, Set: directive.Defaults()}); c.Status != Fail || len(c.Findings) != 1 {
		t.Errorf("an uncovered function breaks it: %+v", c)
	}
}

// stackRun records the command and directory a component's coverage run used and answers with out and err.
func stackRun(out string, err error, got *[]string) CommandRunner {
	return func(_ context.Context, dir, name string, a ...string) ([]byte, error) {
		*got = append([]string{dir, name}, a...)
		return []byte(out), err
	}
}

func rustProject(t *testing.T, workspace bool) string {
	t.Helper()
	dir := t.TempDir()
	manifest := "[package]\nname = \"x\"\n"
	if workspace {
		manifest = "[workspace]\nmembers = []\n"
	}
	write(t, dir, "Cargo.toml", manifest)
	return dir
}

func TestCoverageRunsCargoLlvmCovForARustComponent(t *testing.T) {
	var got []string
	dir := rustProject(t, false)
	c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), CoverageRunner: stackRun("", nil, &got)})
	want := []string{dir, "cargo", "llvm-cov", "--fail-under-lines", "100"}
	if c.Status != Pass || !reflect.DeepEqual(got, want) || !strings.Contains(c.Summary, "rust . at or above 100%") {
		t.Errorf("%+v ran %v, want %v", c, got, want)
	}
	ws := rustProject(t, true)
	coverageOf(Options{Dir: ws, Set: directive.Defaults(), CoverageRunner: stackRun("", nil, &got)})
	if !reflect.DeepEqual(got, []string{ws, "cargo", "llvm-cov", "--workspace", "--fail-under-lines", "100"}) {
		t.Errorf("a workspace is measured whole: %v", got)
	}
}

func TestCoverageRunsPytestCovForAPythonComponent(t *testing.T) {
	var got []string
	dir := t.TempDir()
	write(t, dir, "pyproject.toml", "[project]\nname = \"x\"\n")
	write(t, dir, "tests/test_x.py", "def test_x(): pass\n")
	set := directive.Defaults()
	set.Coverage.MinPercent = 95.5
	c := coverageOf(Options{Dir: dir, Set: set, CoverageRunner: stackRun("", nil, &got)})
	want := []string{dir, "uv", "run", "--with", "pytest-cov", "pytest", "--cov", "--cov-fail-under=95.5"}
	if c.Status != Pass || !reflect.DeepEqual(got, want) {
		t.Errorf("%+v ran %v, want %v", c, got, want)
	}
}

func TestCoverageFailsAComponentBelowTheFloorWithTheEndOfItsOutput(t *testing.T) {
	var got []string
	dir := rustProject(t, false)
	out := "Compiling x\nFinished\nTotal 80%\nerror: coverage 80.00% < required 100.00%\n"
	c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), CoverageRunner: stackRun(out, errors.New("exit status 1"), &got)})
	if c.Status != Fail || len(c.Findings) != 1 || !strings.Contains(c.Findings[0], "coverage 80.00% < required 100.00%") {
		t.Errorf("%+v", c)
	}
}

func TestCoverageIsAnErrorWhenTheToolIsMissing(t *testing.T) {
	var got []string
	dir := rustProject(t, false)
	for name, run := range map[string]CommandRunner{
		"cargo subcommand": stackRun("error: no such command: `llvm-cov`", errors.New("exit status 101"), &got),
		"binary":           stackRun("", &exec.Error{Name: "cargo", Err: exec.ErrNotFound}, &got),
	} {
		c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), CoverageRunner: run})
		if c.Status != Error || !strings.Contains(c.Advice, "cargo install cargo-llvm-cov") {
			t.Errorf("%s: a tool that cannot run is not a skip: %+v", name, c)
		}
	}
	py := t.TempDir()
	write(t, py, "pyproject.toml", "[project]\nname = \"x\"\n")
	write(t, py, "tests/test_x.py", "def test_x(): pass\n")
	c := coverageOf(Options{Dir: py, Set: directive.Defaults(), CoverageRunner: stackRun("", &exec.Error{Name: "uv", Err: exec.ErrNotFound}, &got)})
	if c.Status != Error || !strings.Contains(c.Advice, "install uv") {
		t.Errorf("%+v", c)
	}
}

func TestCoverageOfAComponentIsSkippedWhenTheProjectOrTheChangeSaysSo(t *testing.T) {
	var got []string
	dir := rustProject(t, false)
	write(t, dir, ".github/dendrite/pipeline.yml", "skip_coverage_floor: true\n")
	c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), CoverageRunner: stackRun("", nil, &got)})
	if c.Status != Skipped || !strings.Contains(c.Summary, "its own coverage ratchet") || got != nil {
		t.Errorf("a project with its own ratchet is not measured: %+v ran %v", c, got)
	}

	other := t.TempDir()
	elsewhere := &Scope{lines: map[string][][2]int{"docs/readme.md": nil}}
	sub := filepath.Join(other, "crates")
	write(t, sub, "Cargo.toml", "[package]\nname = \"y\"\n")
	c = coverageOf(Options{Dir: other, Set: directive.Defaults(), Scope: elsewhere, CoverageRunner: stackRun("", nil, &got)})
	if got != nil {
		t.Errorf("a change that touched no file of the crate does not run it: ran %v, %+v", got, c)
	}
	touched := &Scope{lines: map[string][][2]int{"crates/src/lib.rs": nil}}
	c = coverageOf(Options{Dir: other, Set: directive.Defaults(), Scope: touched, CoverageRunner: stackRun("", nil, &got)})
	if c.Status != Pass || got[0] != sub {
		t.Errorf("a change that touched the crate runs it there: %+v ran %v", c, got)
	}

	off := directive.Defaults()
	off.Coverage.MinPercent = 0
	got = nil
	c = coverageOf(Options{Dir: rustProject(t, false), Set: off, CoverageRunner: stackRun("", nil, &got)})
	if c.Status != Skipped || got != nil {
		t.Errorf("without a floor nothing is measured: %+v", c)
	}
}

func TestCoverageRejectsABrokenPipelineExtension(t *testing.T) {
	dir := rustProject(t, false)
	write(t, dir, ".github/dendrite/pipeline.yml", "nonsense: [")
	if c := coverageOf(Options{Dir: dir, Set: directive.Defaults()}); c.Status != Error {
		t.Errorf("%+v", c)
	}
}

func TestMergeChecksRanksErrorOverFailOverPassOverSkip(t *testing.T) {
	skip := Check{Status: Skipped, Summary: "s"}
	pass := Check{Status: Pass, Summary: "p"}
	fail := Check{Status: Fail, Summary: "f", Findings: []string{"b", "a"}, Advice: "fix"}
	bad := Check{Status: Error, Summary: "e"}
	for name, tc := range map[string]struct {
		parts   []Check
		status  Status
		summary string
	}{
		"all skipped": {[]Check{skip, skip}, Skipped, "s; s"},
		"pass":        {[]Check{skip, pass}, Pass, "p"},
		"fail":        {[]Check{pass, fail, skip}, Fail, "p; f"},
		"error":       {[]Check{fail, bad, pass}, Error, "f; e; p"},
	} {
		got := mergeChecks("coverage", tc.parts)
		if got.Status != tc.status || got.Summary != tc.summary {
			t.Errorf("%s: %+v", name, got)
		}
	}
	got := mergeChecks("coverage", []Check{fail})
	if !reflect.DeepEqual(got.Findings, []string{"a", "b"}) || got.Advice != "fix" {
		t.Errorf("findings are sorted and advice kept: %+v", got)
	}
}

func TestCoverageRunsTheRealToolsWithoutARunnerAndFindsThemMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := coverageOf(Options{Dir: rustProject(t, false), Set: directive.Defaults()})
	if c.Status != Error || !strings.Contains(c.Summary, "cargo cannot run") {
		t.Errorf("%+v", c)
	}
}
