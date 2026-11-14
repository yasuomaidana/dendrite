package pipeline

import (
	"fmt"
	"strconv"

	"Dendrite/pkg/directive"
)

// Step is one step of a job.
type Step struct {
	Name string
	Uses string
	Run  string
	With [][2]string
}

// Options say what the quality directives and the project extension ask of a component's steps.
type Options struct {
	Set directive.Set
	// SkipCoverageFloor leaves the coverage threshold to the project (its own ratchet, in the extension).
	SkipCoverageFloor bool
}

// stack holds the rules of one toolchain: how it is set up, linted and tested.
type stack interface {
	setup(c Component) []Step
	lint(c Component, o Options) []Step
	test(c Component, o Options) []Step
}

var stacks = map[Kind]stack{Go: goStack{}, Rust: rustStack{}, Bun: bunStack{}, UV: uvStack{}}

func percent(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func run(name, cmd string) Step { return Step{Name: name, Run: cmd} }

type goStack struct{}

func (goStack) setup(c Component) []Step {
	return []Step{{Name: "Set up Go", Uses: "actions/setup-go@v5", With: [][2]string{{"go-version-file", join(c.Dir, "go.mod")}, {"cache", "true"}}}}
}

func (goStack) lint(c Component, _ Options) []Step {
	with := [][2]string{{"version", "latest"}}
	if c.Dir != "." {
		with = append(with, [2]string{"working-directory", c.Dir})
	}
	return []Step{{Name: "Lint (golangci-lint, cyclomatic and cognitive limits from the directives)", Uses: "golangci/golangci-lint-action@v8", With: with}}
}

const goCoverageFloor = `total=$(go tool cover -func=coverage.out | awk '/^total:/ {sub("%%","",$3); print $3}')
echo "Total statement coverage: ${total}%%"
awk -v cov="$total" -v min=%s 'BEGIN { if (cov+0 < min+0) { print "coverage " cov "%% is below the required " min "%%"; exit 1 } }'`

func (goStack) test(_ Component, o Options) []Step {
	steps := []Step{run("Test (race detector, coverage profile)", "go test -race -coverprofile=coverage.out -covermode=atomic ./...")}
	if o.SkipCoverageFloor {
		return steps
	}
	min := percent(o.Set.Coverage.MinPercent)
	return append(steps, run(fmt.Sprintf("Coverage floor (>= %s%%)", min), fmt.Sprintf(goCoverageFloor, min)))
}

type rustStack struct{}

func (rustStack) setup(c Component) []Step {
	return []Step{
		{Name: "Set up Rust", Uses: "dtolnay/rust-toolchain@stable", With: [][2]string{{"components", "rustfmt, clippy, llvm-tools-preview"}}},
		{Name: "Cache cargo", Uses: "Swatinem/rust-cache@v2", With: [][2]string{{"workspaces", c.Dir}}},
	}
}

func cargoScope(c Component) string {
	if c.Workspace {
		return " --workspace"
	}
	return ""
}

func (rustStack) lint(c Component, _ Options) []Step {
	return []Step{
		run("Format", "cargo fmt --all --check"),
		run("Lint (clippy, cognitive complexity limit from clippy.toml)", "cargo clippy"+cargoScope(c)+" --all-targets -- -D warnings -W clippy::cognitive_complexity"),
	}
}

func (rustStack) test(c Component, o Options) []Step {
	if o.SkipCoverageFloor {
		return []Step{run("Test", "cargo test"+cargoScope(c))}
	}
	min := percent(o.Set.Coverage.MinPercent)
	return []Step{
		{Name: "Install cargo-llvm-cov", Uses: "taiki-e/install-action@cargo-llvm-cov"},
		run(fmt.Sprintf("Test with coverage floor (>= %s%%)", min), "cargo llvm-cov"+cargoScope(c)+" --fail-under-lines "+min),
	}
}

type bunStack struct{}

func (bunStack) setup(Component) []Step {
	return []Step{{Name: "Set up Bun", Uses: "oven-sh/setup-bun@v2", With: [][2]string{{"bun-version", "latest"}}}}
}

func (bunStack) lint(c Component, _ Options) []Step {
	steps := []Step{run("Install (frozen lockfile)", "bun install --frozen-lockfile")}
	if c.Has("lint") {
		steps = append(steps, run("Lint", "bun run lint"))
	}
	return steps
}

func (bunStack) test(c Component, _ Options) []Step {
	var steps []Step
	if c.Has("test") {
		steps = append(steps, run("Test", "bun run test"))
	}
	if c.Has("build") {
		steps = append(steps, run("Build", "bun run build"))
	}
	return steps
}

type uvStack struct{}

func (uvStack) setup(Component) []Step {
	return []Step{{Name: "Set up uv", Uses: "astral-sh/setup-uv@v5", With: [][2]string{{"enable-cache", "true"}}}}
}

func (uvStack) lint(Component, Options) []Step {
	return []Step{run("Lint (ruff, cyclomatic limit from ruff.toml)", "uvx ruff check .")}
}

func (uvStack) test(_ Component, o Options) []Step {
	if o.SkipCoverageFloor {
		return []Step{run("Test", "uv run pytest")}
	}
	min := percent(o.Set.Coverage.MinPercent)
	return []Step{run(fmt.Sprintf("Test with coverage floor (>= %s%%)", min), "uv run --with pytest-cov pytest --cov --cov-fail-under="+min)}
}
