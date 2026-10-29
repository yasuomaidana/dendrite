package gate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"Dendrite/pkg/quality/coverage"
)

func failed(name string, err error) Check {
	return Check{Name: name, Status: Error, Summary: err.Error()}
}

func verdict(name, summary string, findings []string) Check {
	if len(findings) > 0 {
		return Check{Name: name, Status: Fail, Summary: summary, Findings: findings}
	}
	return Check{Name: name, Status: Pass, Summary: summary}
}

// checkLoc fails a file with more production lines than the directive allows; scoped to a change, only the
// files it touched.
func checkLoc(ctx context.Context, o Options) Check {
	var rep struct {
		Totals struct{ Files int }
		Files  []struct {
			Path       string
			Production int
			Status     string
		}
	}
	params := map[string]interface{}{
		"max_production_lines": o.Set.Loc.MaxProductionLines, "warn_at": o.Set.Loc.WarnAt, "exclude": o.Set.Loc.Exclude,
	}
	if err := o.Sidecar.Quality(ctx, o.Dir, "quality.loc", params, &rep); err != nil {
		return failed("loc", err)
	}
	var findings []string
	checked := 0
	for _, f := range rep.Files {
		if o.Scope != nil && !o.Scope.HasFile(f.Path) {
			continue
		}
		checked++
		if f.Status == "fail" {
			findings = append(findings, fmt.Sprintf("%s: %d production lines, the limit is %d", f.Path, f.Production, o.Set.Loc.MaxProductionLines))
		}
	}
	return verdict("loc", fmt.Sprintf("%d files checked, %d over %d production lines", checked, len(findings), o.Set.Loc.MaxProductionLines), findings)
}

// checkClones fails when repeated code exceeds the allowance; scoped to a change, when a repeated block has a
// place in lines the change added.
func checkClones(ctx context.Context, o Options) Check {
	var rep struct {
		Totals struct {
			Percent float64
			Groups  int
		}
		Groups []struct {
			ID        int
			Locations []struct {
				Path      string `json:"path"`
				StartLine int    `json:"start_line"`
				EndLine   int    `json:"end_line"`
			}
		}
	}
	params := map[string]interface{}{
		"max_percent": o.Set.Duplication.MaxPercent, "min_lines": o.Set.Duplication.MinLines,
		"min_tokens": o.Set.Duplication.MinTokens, "include_tests": o.Set.Duplication.IncludeTests,
	}
	if err := o.Sidecar.Quality(ctx, o.Dir, "quality.clones", params, &rep); err != nil {
		return failed("clones", err)
	}
	var findings []string
	for _, g := range rep.Groups {
		var places []string
		touched := o.Scope == nil
		for _, l := range g.Locations {
			places = append(places, fmt.Sprintf("%s:%d-%d", l.Path, l.StartLine, l.EndLine))
			if o.Scope != nil && o.Scope.Touches(l.Path, l.StartLine, l.EndLine) {
				touched = true
			}
		}
		if touched {
			findings = append(findings, fmt.Sprintf("the same code in %s", strings.Join(places, ", ")))
		}
	}
	if o.Scope == nil && rep.Totals.Percent <= o.Set.Duplication.MaxPercent {
		findings = nil // within the allowance
	}
	summary := fmt.Sprintf("%.2f%% of the code is repeated (allowed %.2f%%) in %d groups", rep.Totals.Percent, o.Set.Duplication.MaxPercent, rep.Totals.Groups)
	if o.Scope != nil {
		summary = fmt.Sprintf("%d repeated blocks involve lines this change added", len(findings))
	}
	return verdict("clones", summary, findings)
}

// checkRstest applies to Rust projects only.
func checkRstest(ctx context.Context, o Options) Check {
	var rep struct {
		Files      int
		Violations []struct {
			Kind, Path, Message string
			Line                int
		}
	}
	if err := o.Sidecar.Quality(ctx, o.Dir, "quality.rstest", nil, &rep); err != nil {
		return failed("rstest", err)
	}
	if rep.Files == 0 {
		return Check{Name: "rstest", Status: Skipped, Summary: "no Rust files"}
	}
	var findings []string
	for _, v := range rep.Violations {
		if o.Scope == nil || o.Scope.HasFile(v.Path) {
			findings = append(findings, fmt.Sprintf("%s:%d [%s] %s", v.Path, v.Line, v.Kind, v.Message))
		}
	}
	return verdict("rstest", fmt.Sprintf("%d Rust files checked, %d violations", rep.Files, len(findings)), findings)
}

// baselineFile is where the coverage ratchet keeps its per-package baseline.
const baselineFile = ".github/coverage_baseline.json"

// checkCoverage measures Go test coverage. The whole repository is held to the directive's floor. A change is
// held to the ratchet when the project keeps a baseline (its packages may not lose coverage, new ones start at
// 100%), so a ticket is not failed for the coverage debt of a package it only touched, and to the floor when it
// does not.
func checkCoverage(ctx context.Context, o Options) Check {
	if _, err := os.Stat(filepath.Join(o.Dir, "go.mod")); err != nil {
		return Check{Name: "coverage", Status: Skipped, Summary: "not a Go module (coverage for other languages is not wired yet)"}
	}
	packages := []string{"./..."}
	if o.Scope != nil {
		packages = changedGoPackages(o.Scope)
		if len(packages) == 0 {
			return Check{Name: "coverage", Status: Skipped, Summary: "no Go package changed"}
		}
	}
	opts := coverage.Options{RepoDir: o.Dir, Packages: packages, Stdout: io.Discard, Stderr: io.Discard, CommandRunner: o.CoverageRunner}
	mode := fmt.Sprintf("floor of %.0f%%", o.Set.Coverage.MinPercent)
	_, baselineErr := os.Stat(filepath.Join(o.Dir, baselineFile))
	if o.Scope != nil && baselineErr == nil {
		opts.Ratchet, mode = true, "ratchet against "+baselineFile
	} else {
		opts.MinPercent, opts.ExemptGlobs = o.Set.Coverage.MinPercent, o.Set.Coverage.Exclude
		if opts.MinPercent <= 0 {
			return Check{Name: "coverage", Status: Skipped, Summary: "the directives set no coverage floor"}
		}
	}
	res, err := coverage.Run(ctx, opts)
	if err != nil {
		return failed("coverage", err)
	}
	var findings []string
	for _, off := range res.Offenders {
		if opts.Ratchet {
			findings = append(findings, fmt.Sprintf("%s: %.1f%%, it was %.1f%%", off.Package, off.CurrentPct, off.BaselinePct))
		} else {
			findings = append(findings, fmt.Sprintf("%s: %.1f%%", off.Package, off.CurrentPct))
		}
	}
	if len(findings) == 0 && !res.Passed {
		findings = []string{"the tests did not pass"}
	}
	sort.Strings(findings)
	return verdict("coverage", fmt.Sprintf("%d packages measured against a %s", res.TotalPackages, mode), findings)
}

// changedGoPackages lists the packages (as `go test` arguments) holding Go files the change touched.
func changedGoPackages(s *Scope) []string {
	seen := map[string]bool{}
	for _, f := range s.Files() {
		if strings.HasSuffix(f, ".go") {
			seen["./"+filepath.ToSlash(filepath.Dir(f))] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
