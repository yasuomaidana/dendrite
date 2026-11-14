package pipeline

import (
	"Dendrite/pkg/branchpolicy"
	"bytes"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"Dendrite/pkg/directive"
)

// ManagedHeader marks a workflow Dendrite generates and may rewrite. A file without it was written by a
// person and is left alone.
const ManagedHeader = "# managed by dendrite — edit .github/dendrite/directives.toml"

// loomGateRepo builds the quality gates every generated workflow runs.
const loomGateRepo = "https://github.com/yasuomaidana/dendrite-rag-rs"

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

var plain = regexp.MustCompile(`^[A-Za-z0-9 _.()/=,>-]+$`)

func scalar(s string) string {
	if plain.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func indent(text string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// writeStep writes a step at the indentation of a job's step list; workDir applies to run steps.
func writeStep(b *strings.Builder, s Step, workDir string) {
	b.WriteString("      - name: " + scalar(s.Name) + "\n")
	if s.Uses != "" {
		b.WriteString("        uses: " + s.Uses + "\n")
	}
	if len(s.With) > 0 {
		b.WriteString("        with:\n")
		for _, kv := range s.With {
			b.WriteString("          " + kv[0] + ": " + scalar(kv[1]) + "\n")
		}
	}
	if s.Run == "" {
		return
	}
	if workDir != "" && workDir != "." {
		b.WriteString("        working-directory: " + workDir + "\n")
	}
	b.WriteString("        run: |\n" + indent(s.Run, 10))
}

// marshal renders a value decoded from the project's extension, which YAML can always encode again.
func marshal(v any) string {
	out, _ := yaml.Marshal(v)
	return string(out)
}

func writeJob(b *strings.Builder, id, name string, steps []Step) {
	b.WriteString("  " + id + ":\n    name: " + scalar(name) + "\n    runs-on: ubuntu-latest\n    steps:\n")
	b.WriteString("      - name: Checkout\n        uses: actions/checkout@v4\n")
	for _, s := range steps {
		writeStep(b, s, ".")
	}
}

// gateSteps are the steps of the quality job: install loom-gate, then run its checks.
func gateSteps(comps []Component, ext Extension) []Step {
	var steps []Step
	cmd := "loom-gate"
	if ext.LoomGate != "" {
		cmd = ext.LoomGate
		steps = append(steps,
			Step{Name: "Set up Rust", Uses: "dtolnay/rust-toolchain@stable"},
			Step{Name: "Cache cargo", Uses: "Swatinem/rust-cache@v2"})
	} else {
		steps = append(steps,
			Step{Name: "Set up Rust", Uses: "dtolnay/rust-toolchain@stable"},
			Step{Name: "Cache loom-gate", Uses: "actions/cache@v4", With: [][2]string{
				{"path", "~/.cargo/bin/loom-gate"},
				{"key", "loom-gate-${{ runner.os }}-${{ vars.LOOM_GATE_REF || 'develop' }}"}}},
			run("Install loom-gate", `command -v loom-gate >/dev/null || cargo install --locked --git `+loomGateRepo+` --branch "${{ vars.LOOM_GATE_REF || 'develop' }}" loom-gate`))
	}
	checks := []string{"loc", "clones", "complexity", "structure"}
	if hasKind(comps, Rust) {
		checks = append(checks, "rstest")
	}
	for _, c := range checks {
		steps = append(steps, run("Gate: "+c, cmd+" "+c+" --format github"))
	}
	return steps
}

func hasKind(comps []Component, k Kind) bool {
	for _, c := range comps {
		if c.Kind == k {
			return true
		}
	}
	return false
}

func jobTitle(c Component) string {
	title := map[Kind]string{Go: "Go", Rust: "Rust", Bun: "Bun", UV: "Python (uv)"}[c.Kind]
	if c.Dir != "." {
		title += " (" + c.Dir + ")"
	}
	return title
}

// writeComponentJob writes the job of one component: setup, lint, the project's pre-test steps, then tests.
func writeComponentJob(b *strings.Builder, c Component, o Options, ext Extension) {
	s := stacks[c.Kind]
	b.WriteString("  " + c.ID() + ":\n    name: " + scalar(jobTitle(c)) + "\n    runs-on: ubuntu-latest\n    steps:\n")
	b.WriteString("      - name: Checkout\n        uses: actions/checkout@v4\n")
	for _, st := range append(s.setup(c), s.lint(c, o)...) {
		writeStep(b, st, c.Dir)
	}
	if len(ext.PreTest) > 0 {
		b.WriteString(indent(marshal(ext.PreTest), 6))
	}
	for _, st := range s.test(c, o) {
		writeStep(b, st, c.Dir)
	}
}

// Branches are the branches CI runs for: the project's integration branch, main, and its feature branches.
type Branches struct{ Integration, FeaturePrefix string }

// BranchesOf reads the branches of the project at root (see pkg/branchpolicy).
func BranchesOf(root string) Branches {
	t := branchpolicy.For(root)
	return Branches{Integration: t.Integration, FeaturePrefix: t.FeaturePrefix}
}

// triggers is the flow list of branches a push runs CI for, without repeats. Every pull request runs CI,
// whatever its target: the generated workflow puts no branch or path filter on pull_request.
func (b Branches) triggers() string {
	list := []string{b.Integration}
	if b.Integration != "main" {
		list = append(list, "main")
	}
	if b.FeaturePrefix != "" {
		list = append(list, strconv.Quote(b.FeaturePrefix+"**"))
	}
	return "[ " + strings.Join(list, ", ") + " ]"
}

// Render writes the workflow for the components, the directives, the project's extension and its branches.
func Render(comps []Component, set directive.Set, ext Extension, br Branches) []byte {
	o := Options{Set: set, SkipCoverageFloor: ext.SkipCoverageFloor}
	var b strings.Builder
	b.WriteString(ManagedHeader + "\nname: Dendrite CI/CD\n\non:\n  push:\n    branches: " + br.triggers() + "\n  pull_request:\n\n")
	if len(ext.Env) > 0 {
		b.WriteString("env:\n" + indent(marshal(ext.Env), 2) + "\n")
	}
	b.WriteString("jobs:\n")
	var jobs strings.Builder
	writeJob(&jobs, "quality", "Quality gates (loc, clones, complexity, structure)", gateSteps(comps, ext))
	writeReleaseRefsJob(&jobs)
	if hasKind(comps, Rust) {
		writeDBAccessJob(&jobs)
	}
	for _, c := range comps {
		writeComponentJob(&jobs, c, o, ext)
	}
	b.WriteString(jobs.String())
	b.WriteString(extraJobs(ext))
	return []byte(b.String())
}

func extraJobs(ext Extension) string {
	if len(ext.Jobs) == 0 {
		return ""
	}
	names := make([]string, 0, len(ext.Jobs))
	for n := range ext.Jobs {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		b.WriteString(indent(marshal(map[string]any{n: ext.Jobs[n]}), 2))
	}
	return b.String()
}
