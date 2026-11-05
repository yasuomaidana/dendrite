package gate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// checkLint runs the formatting and lint steps of the project's own workflows (cargo fmt, cargo clippy, ruff,
// golangci-lint, bun run lint, loom-gate) with the commands CI runs, so local and CI cannot drift, and checks
// that the loom-gate revision a workflow installs exists and holds the loom-gate crate. Scoped to a change, only
// the steps of directories it touched run. A tool that is missing is an error: a lint that did not run proves
// nothing.
func checkLint(ctx context.Context, o Options) Check {
	steps, err := ciSteps(o.Dir)
	if err != nil {
		return failed("lint", err)
	}
	var parts []Check
	for _, s := range steps {
		if s.isCoverage() || !touchesDir(o.Scope, s.Dir) {
			continue
		}
		if pin, ok := loomGatePin(s); ok {
			parts = append(parts, checkLoomGatePin(ctx, o, s, pin))
		}
		if part, ok := lintStep(ctx, o, s); ok {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return Check{Name: "lint", Status: Skipped, Summary: "the workflows run no format or lint step the gate can run"}
	}
	return mergeChecks("lint", parts)
}

// stepLabel names a step in findings: "name (dir)".
func stepLabel(s ciStep) string {
	name := firstNonEmpty(s.Name, strings.TrimSpace(strings.SplitN(s.Run, "\n", 2)[0]), s.Uses)
	return fmt.Sprintf("%s (%s, %s)", name, s.File, s.Dir)
}

// installAdvice says how to get the tool a step needs.
func installAdvice(script string) string {
	switch {
	case strings.Contains(script, "cargo fmt"):
		return "install rustfmt with `rustup component add rustfmt`"
	case strings.Contains(script, "cargo clippy"):
		return "install clippy with `rustup component add clippy`"
	case strings.Contains(script, "ruff"):
		return "install uv (https://docs.astral.sh/uv/), which runs ruff"
	case strings.Contains(script, "bun"):
		return "install bun (https://bun.sh)"
	case strings.Contains(script, "golangci-lint"):
		return "install golangci-lint (https://golangci-lint.run)"
	case strings.Contains(script, "loom-gate"):
		return "install loom-gate with `cargo install --locked --git <repository in the workflow> loom-gate`"
	}
	return "install the tool the workflow step runs"
}

// lintStep runs one workflow step as written. ok is false when the gate does not run the step.
func lintStep(ctx context.Context, o Options, s ciStep) (Check, bool) {
	script, ok := s.script()
	if !ok {
		return Check{}, false
	}
	run := o.CoverageRunner
	if run == nil {
		run = cleanRunner
	}
	out, err := run(ctx, filepath.Join(o.Dir, filepath.FromSlash(s.Dir)), "sh", "-c", script)
	label := stepLabel(s)
	switch {
	case err == nil:
		return Check{Name: "lint", Status: Pass, Summary: label + " passes"}, true
	case toolMissing(err, out) && strings.HasPrefix(strings.TrimSpace(script), "loom-gate"):
		return Check{Name: "lint", Status: Skipped, Summary: label + ": loom-gate is not installed; the gate's loc, clones, complexity, structure and rstest checks run the same rules"}, true
	case toolMissing(err, out):
		return Check{Name: "lint", Status: Error, Summary: label + " cannot run: a tool is missing", Advice: installAdvice(script)}, true
	}
	return Check{Name: "lint", Status: Fail, Summary: label + " fails", Findings: []string{label + ": " + lastLines(out, 4)}}, true
}

// loomPin is the loom-gate source a workflow installs.
type loomPin struct {
	Repo string // the git URL or path after --git
	Ref  string // the revision after --rev, or the branch after --branch
}

var (
	loomInstall = regexp.MustCompile(`cargo install[^\n]*--git\s+(\S+)[^\n]*?--(rev|branch|tag)\s+("[^"\n]*"|'[^'\n]*'|\$\{\{[^}\n]*\}\}|\S+)[^\n]*\bloom-gate\b`)
	exprDefault = regexp.MustCompile(`\$\{\{[^}]*\|\|\s*'([^']*)'\s*\}\}`)
	envRef      = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}|\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
)

// expandEnv replaces ${{ env.X }}, ${X} and $X with the value the workflow, job or step sets for X. A name the
// workflow does not set stays as written, so the pin reads as unresolved and is reported.
func expandEnv(s string, env map[string]string) string {
	return envRef.ReplaceAllStringFunc(s, func(m string) string {
		g := envRef.FindStringSubmatch(m)
		if v, ok := env[firstNonEmpty(g[1], g[2], g[3])]; ok {
			return v
		}
		return m
	})
}

// loomGatePin reads the repository and revision a step installs loom-gate from. An expression such as
// ${{ vars.LOOM_GATE_REF || 'abc' }} counts as its default, which is what CI uses when the variable is unset;
// ${{ env.X }}, ${X} and $X take the value set by the step, its job or the workflow.
func loomGatePin(s ciStep) (loomPin, bool) {
	m := loomInstall.FindStringSubmatch(s.Run)
	if m == nil {
		return loomPin{}, false
	}
	ref := expandEnv(strings.Trim(m[3], `"'`), s.Env)
	if e := exprDefault.FindStringSubmatch(ref); e != nil {
		ref = e[1]
	}
	return loomPin{Repo: m[1], Ref: ref}, ref != "" && !strings.ContainsAny(ref, "$")
}

// checkLoomGatePin fetches the pinned revision into a scratch repository and looks for the loom-gate crate in
// it. A revision that does not exist, or that predates the crate, fails; a repository that cannot be reached is
// an error, because then nothing is known.
func checkLoomGatePin(ctx context.Context, o Options, s ciStep, pin loomPin) Check {
	run := o.CoverageRunner
	if run == nil {
		run = cleanRunner
	}
	label := fmt.Sprintf("loom-gate %s of %s (%s)", pin.Ref, pin.Repo, s.File)
	scratch, err := os.MkdirTemp("", "loom-pin-")
	if err != nil {
		return failed("lint", err)
	}
	defer os.RemoveAll(scratch)
	for _, args := range [][]string{{"init", "-q"}, {"fetch", "-q", "--depth=1", pin.Repo, pin.Ref}} {
		out, err := run(ctx, scratch, "git", args...)
		if err == nil {
			continue
		}
		if args[0] == "fetch" && missingRef(string(out)) {
			return Check{Name: "lint", Status: Fail, Summary: label + " does not exist", Findings: []string{label + ": " + lastLines(out, 2)},
				Advice: "pin loom-gate to a revision that exists in the repository"}
		}
		return Check{Name: "lint", Status: Error, Summary: label + " could not be checked: " + lastLines(out, 2)}
	}
	tree, err := run(ctx, scratch, "git", "ls-tree", "-r", "--name-only", "FETCH_HEAD")
	if err != nil {
		return failed("lint", err)
	}
	if !hasLoomGateCrate(string(tree)) {
		return Check{Name: "lint", Status: Fail, Summary: label + " has no loom-gate crate", Findings: []string{label + ": no loom-gate/Cargo.toml in that revision"},
			Advice: "pin loom-gate to a revision that contains the loom-gate crate"}
	}
	return Check{Name: "lint", Status: Pass, Summary: label + " exists and holds the crate"}
}

// missingRef reports whether git's fetch output says the revision is not in the repository.
func missingRef(out string) bool {
	for _, m := range []string{"couldn't find remote ref", "not our ref", "unadvertised object", "no such remote ref"} {
		if strings.Contains(out, m) {
			return true
		}
	}
	return false
}

// hasLoomGateCrate reports whether the file list holds a loom-gate crate manifest.
func hasLoomGateCrate(tree string) bool {
	for _, f := range strings.Split(tree, "\n") {
		if f == "loom-gate/Cargo.toml" || strings.HasSuffix(f, "/loom-gate/Cargo.toml") {
			return true
		}
	}
	return false
}
