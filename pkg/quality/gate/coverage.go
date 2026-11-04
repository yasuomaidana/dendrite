package gate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"Dendrite/pkg/env/pipeline"
	"Dendrite/pkg/quality/coverage"
)

// checkCoverage holds every component of the project to the coverage rule CI enforces for its toolchain: the Go
// module at the root by package (see checkGoCoverage), each Rust crate or workspace by `cargo llvm-cov` and each
// Python project by `pytest --cov`. A check that cannot run because the tool is missing is an error, never a
// skip: a skipped coverage check is how a change looked done locally and then failed in CI.
func checkCoverage(ctx context.Context, o Options) Check {
	ext, err := pipeline.LoadExtension(o.Dir)
	if err != nil {
		return failed("coverage", err)
	}
	parts := []Check{checkGoCoverage(ctx, o)}
	components, _ := pipeline.Detect(o.Dir)
	for _, c := range components {
		if c.Kind == pipeline.Rust || c.Kind == pipeline.UV {
			parts = append(parts, checkComponentCoverage(ctx, o, c, ext))
		}
	}
	return mergeChecks("coverage", parts)
}

// mergeChecks is one check out of several: an error beats a failure, a failure beats a pass, and a pass beats a
// skip. The summary lists the parts that ran (all of them when none did).
func mergeChecks(name string, parts []Check) Check {
	out := Check{Name: name, Status: Skipped}
	var ran, all []string
	rank := map[Status]int{Skipped: 0, Pass: 1, Fail: 2, Error: 3}
	for _, p := range parts {
		all = append(all, p.Summary)
		if p.Status != Skipped {
			ran = append(ran, p.Summary)
		}
		if rank[p.Status] > rank[out.Status] {
			out.Status = p.Status
		}
		out.Findings = append(out.Findings, p.Findings...)
		if out.Advice == "" {
			out.Advice = p.Advice
		}
	}
	if len(ran) == 0 {
		ran = all
	}
	out.Summary = strings.Join(ran, "; ")
	sort.Strings(out.Findings)
	return out
}

// componentCommand is the command CI runs to test a Rust or Python component against the floor.
func componentCommand(c pipeline.Component, floor string) (name string, args []string) {
	if c.Kind == pipeline.Rust {
		args = []string{"llvm-cov"}
		if c.Workspace {
			args = append(args, "--workspace")
		}
		return "cargo", append(args, "--fail-under-lines", floor)
	}
	return "uv", []string{"run", "--with", "pytest-cov", "pytest", "--cov", "--cov-fail-under=" + floor}
}

// passedFloor words a coverage pass: against the directive's floor, or against the workflow's own (floor "").
func passedFloor(label, floor string) string {
	if floor == "" {
		return label + " meets the coverage floor of its workflow"
	}
	return fmt.Sprintf("%s at or above %s%%", label, floor)
}

// workflowCoverage is the coverage command the project's own workflow runs for the component, so a floor set
// there (cargo llvm-cov --fail-under-lines 90) is the floor the gate holds it to.
func workflowCoverage(root string, c pipeline.Component) (script string, found bool) {
	steps, err := ciSteps(root)
	if err != nil {
		return "", false
	}
	for _, s := range steps {
		if s.Dir != filepath.ToSlash(filepath.Clean(c.Dir)) || !s.isCoverage() {
			continue
		}
		if script, ok := s.script(); ok {
			return script, true
		}
	}
	return "", false
}

// toolAdvice says how to install what a component's coverage run needs.
var toolAdvice = map[pipeline.Kind]string{
	pipeline.Rust: "install it with `cargo install cargo-llvm-cov` and `rustup component add llvm-tools-preview`",
	pipeline.UV:   "install uv (https://docs.astral.sh/uv/)",
}

// touchesDir reports whether the change touched a file under dir ("." is the whole project).
func touchesDir(s *Scope, dir string) bool {
	if s == nil || dir == "." {
		return true
	}
	for _, f := range s.Files() {
		if strings.HasPrefix(f, dir+"/") {
			return true
		}
	}
	return false
}

func toolMissing(err error, out []byte) bool {
	return errors.Is(err, exec.ErrNotFound) || strings.Contains(string(out), "no such command") || strings.Contains(string(out), "command not found") || strings.Contains(string(out), ": not found")
}

// lastLines is the end of a command's output, enough to see why it failed.
func lastLines(out []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// checkComponentCoverage runs the coverage floor of one Rust or Python component, as CI does.
func checkComponentCoverage(ctx context.Context, o Options, c pipeline.Component, ext pipeline.Extension) Check {
	label := string(c.Kind) + " " + c.Dir
	skip := func(why string) Check { return Check{Name: "coverage", Status: Skipped, Summary: label + ": " + why} }
	switch {
	case ext.SkipCoverageFloor:
		return skip("the project keeps its own coverage ratchet (skip_coverage_floor in the pipeline extension)")
	case o.Set.Coverage.MinPercent <= 0:
		return skip("the directives set no coverage floor")
	case !touchesDir(o.Scope, c.Dir):
		return skip("the change touched none of its files")
	}
	floor := strconv.FormatFloat(o.Set.Coverage.MinPercent, 'f', -1, 64)
	name, args := componentCommand(c, floor)
	if script, found := workflowCoverage(o.Dir, c); found {
		name, args, floor = "sh", []string{"-c", script}, ""
	}
	run := o.CoverageRunner
	if run == nil {
		run = cleanRunner
	}
	out, err := run(ctx, filepath.Join(o.Dir, filepath.FromSlash(c.Dir)), name, args...)
	switch {
	case err == nil:
		return Check{Name: "coverage", Status: Pass, Summary: passedFloor(label, floor)}
	case toolMissing(err, out):
		return Check{Name: "coverage", Status: Error, Summary: label + ": " + name + " cannot run, so coverage was not measured", Advice: toolAdvice[c.Kind]}
	}
	return Check{Name: "coverage", Status: Fail, Summary: label + " is below its coverage floor or its tests fail",
		Findings: []string{label + ": " + lastLines(out, 8)}}
}

const baselineFile = ".github/coverage_baseline.json"

// checkGoCoverage measures Go test coverage of the root module. The whole repository is held to the directive's floor. A change is
// held to the ratchet when the project keeps a baseline (its packages may not lose coverage, new ones start at
// 100%), so a ticket is not failed for the coverage debt of a package it only touched, and to the floor when it
// does not.
func checkGoCoverage(ctx context.Context, o Options) Check {
	if _, err := os.Stat(filepath.Join(o.Dir, "go.mod")); err != nil {
		return Check{Name: "coverage", Status: Skipped, Summary: "no Go module at the root"}
	}
	packages := coveragePackages(o)
	if len(packages) == 0 {
		return Check{Name: "coverage", Status: Skipped, Summary: "no Go package changed"}
	}
	run := o.CoverageRunner
	if run == nil {
		run = cleanRunner
	}
	opts := coverage.Options{RepoDir: o.Dir, Packages: packages, Stdout: io.Discard, Stderr: io.Discard, CommandRunner: run}
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
	findings := coverageFindings(res, opts.Ratchet)
	sort.Strings(findings)
	return verdict("coverage", fmt.Sprintf("%d packages measured against a %s", res.TotalPackages, mode), findings)
}

// coverageFindings describes each package that fell short, or says the tests failed when none did.
func coverageFindings(res *coverage.Result, ratchet bool) []string {
	var findings []string
	for _, off := range res.Offenders {
		if ratchet {
			findings = append(findings, fmt.Sprintf("%s: %.1f%%, it was %.1f%%", off.Package, off.CurrentPct, off.BaselinePct))
		} else {
			findings = append(findings, fmt.Sprintf("%s: %.1f%%", off.Package, off.CurrentPct))
		}
	}
	if len(findings) == 0 && !res.Passed {
		findings = []string{"the tests did not pass"}
	}
	return findings
}

// coveragePackages are the `go test` arguments to measure: the whole module, or the packages a change touched
// that still exist.
func coveragePackages(o Options) []string {
	if o.Scope == nil {
		return []string{"./..."}
	}
	return existingPackages(o.Dir, changedGoPackages(o.Scope))
}

// existingPackages keeps the `go test` arguments whose directory still exists: a package the change deleted has
// nothing to measure, and `go test` fails on it.
func existingPackages(dir string, packages []string) []string {
	var out []string
	for _, p := range packages {
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err == nil && info.IsDir() {
			out = append(out, p)
		}
	}
	return out
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
