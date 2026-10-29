package gate

import (
	"context"
	"errors"
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
	if c := coverageOf(Options{Dir: t.TempDir(), Set: directive.Defaults()}); c.Status != Skipped || !strings.Contains(c.Summary, "not a Go module") {
		t.Errorf("%+v", c)
	}

	failing := "FAIL\tmod/pkg/a\t0.1s\n"
	c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), CoverageRunner: runner(failing, errors.New("exit status 1"), args)})
	if c.Status != Fail || !reflect.DeepEqual(c.Findings, []string{"the tests did not pass"}) {
		t.Errorf("tests that fail fail the check: %+v", c)
	}

	write(t, dir, ".github/coverage_baseline.json", "{not json")
	scope := &Scope{lines: map[string][][2]int{"pkg/b/b.go": nil}}
	if c := coverageOf(Options{Dir: dir, Set: directive.Defaults(), Scope: scope, CoverageRunner: runner(goTestOutput, nil, args)}); c.Status != Error || !strings.Contains(c.Summary, "baseline") {
		t.Errorf("a broken baseline is an error: %+v", c)
	}
}
