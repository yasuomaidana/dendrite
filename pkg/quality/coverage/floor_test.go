package coverage

import (
	"bytes"
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"Dendrite/pkg/fsutil"
)

func TestFloorListsPackagesBelowTheMinimumWorstFirst(t *testing.T) {
	current := map[string]PackageCoverage{
		"pkg/a":           {Package: "pkg/a", CoveragePct: 100},
		"pkg/b":           {Package: "pkg/b", CoveragePct: 82.4},
		"pkg/c":           {Package: "pkg/c", CoveragePct: 12},
		"pkg/d":           {Package: "pkg/d", CoveragePct: 82.4},
		"pkg/e":           {Package: "pkg/e", NoStatements: true},
		"pkg/f":           {Package: "pkg/f", CoveragePct: 0, NoTests: true},
		"pkg/x/planetest": {Package: "pkg/x/planetest", CoveragePct: 0},
		"gen/z":           {Package: "gen/z", CoveragePct: 1},
	}
	got := Floor(current, 90, func(p string) bool { return IsExempt(p, []string{"gen/**"}) })
	var order []string
	for _, r := range got {
		order = append(order, r.Package)
	}
	if !reflect.DeepEqual(order, []string{"pkg/f", "pkg/c", "pkg/b", "pkg/d"}) {
		t.Errorf("order = %v: lowest first, ties by name; no statements, test helpers and globbed packages left out", order)
	}
	if got[0].BaselinePct != 90 || got[2].CurrentPct != 82.4 {
		t.Errorf("rows = %+v", got)
	}
	if len(Floor(current, 90, nil)) != 6 {
		t.Error("with no exemption function only packages without statements are left out")
	}
	if len(Floor(current, 0, nil)) != 0 {
		t.Error("a floor of 0 lists nothing")
	}
}

func TestIsExemptByHelperNameAndGlob(t *testing.T) {
	for pkg, want := range map[string]bool{
		"pkg/planesvc/planetest": true, "pkg/rag/ragtest/standin": true, "pkg/x/testdata/y": true, "pkg/contest": true,
		"pkg/quality": false, "cmd/dendrite": false, "pkg/tester": false,
	} {
		if IsExempt(pkg, nil) != want {
			t.Errorf("IsExempt(%q) = %v", pkg, !want)
		}
	}
	globs := []string{"cmd/**", "internal/*/gen", "**/generated"}
	for pkg, want := range map[string]bool{
		"cmd/dendrite": true, "cmd/a/b": true, "internal/x/gen": true, "internal/x/y/gen": false, "pkg/generated": true, "generated": true, "pkg/gen": false,
	} {
		if IsExempt(pkg, globs) != want {
			t.Errorf("IsExempt(%q, globs) = %v", pkg, !want)
		}
	}
}

func TestRunAppliesTheFloorInsteadOfTheBaseline(t *testing.T) {
	output := "ok  \tmod/pkg/a\t0.1s\tcoverage: 100.0% of statements\nok  \tmod/pkg/b\t0.1s\tcoverage: 71.0% of statements\nok  \tmod/pkg/ctest\t0.1s\tcoverage: 3.0% of statements\n"
	run := func(min float64) (*Result, string) {
		var out, errBuf bytes.Buffer
		res, err := Run(context.Background(), Options{
			RepoDir: t.TempDir(), MinPercent: min, Stdout: &out, Stderr: &errBuf,
			CommandRunner: func(context.Context, string, string, ...string) ([]byte, error) { return []byte(output), nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		return res, errBuf.String()
	}
	res, stderr := run(80)
	if res.Passed || len(res.Offenders) != 1 || res.Offenders[0].Package != "mod/pkg/b" || !strings.Contains(stderr, "Coverage floor violation: 1 package(s) below 80.0%") {
		t.Errorf("below the floor: %+v\n%s", res, stderr)
	}
	if res, _ := run(70); !res.Passed || len(res.Offenders) != 0 {
		t.Errorf("above the floor: %+v", res)
	}
}

func TestTheRatchetDoesNotDemandAHundredPercentFromANewHelperPackage(t *testing.T) {
	output := "ok  \tmod/pkg/a\t0.1s\tcoverage: 100.0% of statements\nok  \tmod/pkg/atest\t0.1s\tcoverage: 0.0% of statements\nok  \tmod/pkg/b\t0.1s\tcoverage: 40.0% of statements\nok  \tmod/gen/c\t0.1s\tcoverage: 10.0% of statements\n"
	var out, errBuf bytes.Buffer
	dir := t.TempDir()
	if err := fsutil.WriteFileAtomic(filepath.Join(dir, ".github", "coverage_baseline.json"), []byte(`{"mod/pkg/a": 100.0}`)); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), Options{
		RepoDir: dir, Ratchet: true, ExemptGlobs: []string{"mod/gen/**"}, Stdout: &out, Stderr: &errBuf,
		CommandRunner: func(context.Context, string, string, ...string) ([]byte, error) { return []byte(output), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Offenders) != 1 || res.Offenders[0].Package != "mod/pkg/b" || !res.Offenders[0].IsNewPackage {
		t.Errorf("only the new package that is not a helper must start at 100%%: %+v", res.Offenders)
	}
}
