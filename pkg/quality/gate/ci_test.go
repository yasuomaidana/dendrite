package gate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// peerWorkflow is a Rust peer's CI in the shapes the real ones use: block scalars, a job default directory, a
// step with its own env, a GitHub-only step and a coverage step.
const peerWorkflow = `name: CI
on: [push]
jobs:
  rust:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: crates
    env:
      CARGO_TERM_COLOR: always
    steps:
      - uses: actions/checkout@v4
      - name: Format
        run: |
          cargo fmt --all --check
      - name: Lint
        env:
          RUSTFLAGS: "-D warnings"
        run: |
          cargo clippy --all-targets -- -D warnings -W clippy::cognitive_complexity
      - name: Report only
        run: |
          out=$(cargo clippy 2>&1) && echo "::warning title=x::$out"
      - name: Coverage
        run: cargo llvm-cov --workspace --fail-under-lines 90
      - name: Deploy
        run: ./deploy.sh
  go:
    runs-on: ubuntu-latest
    steps:
      - uses: golangci/golangci-lint-action@v8
        with:
          version: latest
          working-directory: svc
          fetch-depth: 0
`

func TestCiStepsReadsJobsStepsDirectoriesAndEnv(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".github/workflows/ci.yml", peerWorkflow)
	steps, err := ciSteps(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 7 {
		t.Fatalf("%d steps: %+v", len(steps), steps)
	}
	if steps[0].Job != "go" || steps[1].Job != "rust" {
		t.Errorf("jobs are read in name order: %s then %s", steps[0].Job, steps[1].Job)
	}
	var format, lint ciStep
	for _, s := range steps {
		switch s.Name {
		case "Format":
			format = s
		case "Lint":
			lint = s
		}
	}
	if format.Dir != "crates" || format.File != ".github/workflows/ci.yml" || format.Env["CARGO_TERM_COLOR"] != "always" {
		t.Errorf("format = %+v", format)
	}
	if lint.Env["RUSTFLAGS"] != "-D warnings" || lint.Env["CARGO_TERM_COLOR"] != "always" {
		t.Errorf("a step's env joins its job's: %+v", lint.Env)
	}
}

func TestCiStepsOfAProjectWithoutWorkflowsAndWithABrokenOne(t *testing.T) {
	dir := t.TempDir()
	if steps, err := ciSteps(dir); err != nil || len(steps) != 0 {
		t.Fatalf("no workflows: %v, %v", steps, err)
	}
	write(t, dir, ".github/workflows/bad.yaml", "jobs: [")
	_, err := ciSteps(dir)
	var we *workflowError
	if err == nil || !strings.Contains(err.Error(), "bad.yaml") || !asWorkflowError(err, &we) {
		t.Fatalf("a workflow that is not YAML names its file: %v", err)
	}
	if we.Unwrap() == nil {
		t.Error("the YAML error is kept")
	}
}

func asWorkflowError(err error, target **workflowError) bool {
	we, ok := err.(*workflowError)
	*target = we
	return ok
}

func TestRunnableLinesAcceptsOnlyWholeBlocksOfKnownCommands(t *testing.T) {
	for name, tc := range map[string]struct {
		run  string
		want []string
	}{
		"one":             {"cargo fmt --all --check", []string{"cargo fmt --all --check"}},
		"block":           {"# format\ncargo fmt --all --check\n\ncargo clippy -- -D warnings\n", []string{"cargo fmt --all --check", "cargo clippy -- -D warnings"}},
		"an unknown line": {"cargo fmt --all --check\n./deploy.sh", nil},
		"github only":     {"cargo clippy -- -D warnings ${{ matrix.flags }}", nil},
		"warning command": {`echo "::warning title=x::y"`, nil},
		"empty":           {"  \n", nil},
		"runner temp":     {"cargo clippy --target-dir $RUNNER_TEMP/t", nil},
		"loom-gate":       {"loom-gate loc --format github", []string{"loom-gate loc --format github"}},
	} {
		if got := runnableLines(tc.run); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

func TestScriptExportsTheEnvAndRunsTheGolangciActionInItsDirectory(t *testing.T) {
	s := ciStep{Run: "cargo clippy", Env: map[string]string{"B": "it's", "A": "1"}}
	script, ok := s.script()
	if !ok || script != "export A='1'\nexport B='it'\\''s'\ncargo clippy" {
		t.Errorf("script = %q, %v", script, ok)
	}
	if _, ok := (ciStep{Run: "cargo clippy", Env: map[string]string{"X": "${{ secrets.K }}"}}).script(); ok {
		t.Error("an env that needs GitHub cannot run locally")
	}
	if _, ok := (ciStep{Run: "./deploy.sh"}).script(); ok {
		t.Error("an unknown command is left to CI")
	}
	action := ciStep{Uses: "golangci/golangci-lint-action@v8", Dir: ".", With: map[string]string{"working-directory": "svc/"}}
	script, ok = action.script()
	if !ok || script != "golangci-lint run" {
		t.Errorf("action script = %q, %v", script, ok)
	}
}

func TestIsCoverageRecognisesLlvmCovAndPytestCov(t *testing.T) {
	for run, want := range map[string]bool{"cargo llvm-cov --workspace": true, "uv run pytest --cov": true, "cargo fmt": false} {
		if got := (ciStep{Run: run}).isCoverage(); got != want {
			t.Errorf("%q: %v", run, got)
		}
	}
}

func TestCiStepsKeepsNonTextWithValuesAndReportsAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".github/workflows/ci.yml", peerWorkflow)
	steps, err := ciSteps(dir)
	if err != nil || steps[0].With["fetch-depth"] != "0" {
		t.Fatalf("a number in with: is kept as text: %+v, %v", steps[0].With, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows", "dir.yml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ciSteps(dir); err == nil {
		t.Error("a workflow that cannot be read is an error")
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty of nothing = %q", got)
	}
}
