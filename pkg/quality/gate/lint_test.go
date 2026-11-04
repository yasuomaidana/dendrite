package gate

import (
	"Dendrite/pkg/env/pipeline"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/testkit"
)

// toolBox is a directory of real executables put first on PATH, so the real runner (sh -c) finds them. Each
// script appends its name and arguments to calls.log, prints out and exits with code.
type toolBox struct {
	dir string
}

func newToolBox(t *testing.T) *toolBox {
	t.Helper()
	b := &toolBox{dir: t.TempDir()}
	t.Setenv("PATH", b.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return b
}

func (b *toolBox) add(t *testing.T, name, out string, code int) {
	t.Helper()
	script := "#!/bin/sh\necho \"" + name + " $*\" >> \"" + filepath.Join(b.dir, "calls.log") + "\"\nprintf '%s\\n' '" + out + "'\nexit " + string(rune('0'+code)) + "\n"
	testkit.WriteFile(t, filepath.Join(b.dir, name), script, 0o755)
}

func (b *toolBox) calls() string {
	data, _ := os.ReadFile(filepath.Join(b.dir, "calls.log"))
	return string(data)
}

// rustPeer is a Cargo project whose workflow runs fmt, clippy and coverage with its own floor.
func rustPeer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[package]\nname = \"peer\"\n")
	write(t, dir, ".github/workflows/ci.yml", `name: CI
on: [push]
jobs:
  rust:
    runs-on: ubuntu-latest
    steps:
      - name: Format
        run: |
          cargo fmt --all --check
      - name: Lint
        run: |
          cargo clippy --all-targets -- -D warnings
      - name: Test with coverage floor
        run: cargo llvm-cov --fail-under-lines 90
`)
	return dir
}

func TestLintRunsTheWorkflowsFormatAndLintStepsAsWritten(t *testing.T) {
	box := newToolBox(t)
	box.add(t, "cargo", "ok", 0)
	dir := rustPeer(t)
	c := checkLint(context.Background(), Options{Dir: dir, Set: directive.Defaults()})
	if c.Status != Pass || !strings.Contains(c.Summary, "Format") || !strings.Contains(c.Summary, "Lint") {
		t.Fatalf("%+v", c)
	}
	calls := box.calls()
	if !strings.Contains(calls, "cargo fmt --all --check") || !strings.Contains(calls, "cargo clippy --all-targets -- -D warnings") {
		t.Errorf("the exact workflow commands ran: %s", calls)
	}
	if strings.Contains(calls, "llvm-cov") {
		t.Errorf("the coverage step belongs to the coverage check: %s", calls)
	}
}

func TestLintFailsWithTheEndOfTheToolsOutput(t *testing.T) {
	box := newToolBox(t)
	box.add(t, "cargo", "Diff in src/lib.rs:3", 1)
	c := checkLint(context.Background(), Options{Dir: rustPeer(t), Set: directive.Defaults()})
	if c.Status != Fail || len(c.Findings) != 2 || !strings.Contains(c.Findings[0], "Diff in src/lib.rs:3") {
		t.Fatalf("%+v", c)
	}
}

func TestLintIsAnErrorWhenTheToolIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := checkLint(context.Background(), Options{Dir: rustPeer(t), Set: directive.Defaults(), CoverageRunner: pathlessRunner})
	if c.Status != Error || !strings.Contains(c.Advice, "rustup component add") {
		t.Fatalf("a lint that cannot run is an error: %+v", c)
	}
}

// pathlessRunner runs the command with sh by absolute path and an empty PATH, as on a machine without the tool.
func pathlessRunner(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	return cleanRunner(ctx, dir, "/bin/sh", args...)
}

func TestLintOfAProjectWithoutWorkflowsOrWithOnlyForeignSteps(t *testing.T) {
	if c := checkLint(context.Background(), Options{Dir: t.TempDir(), Set: directive.Defaults()}); c.Status != Skipped {
		t.Errorf("%+v", c)
	}
	dir := t.TempDir()
	write(t, dir, ".github/workflows/ci.yml", "jobs:\n  x:\n    steps:\n      - run: ./deploy.sh\n")
	if c := checkLint(context.Background(), Options{Dir: dir, Set: directive.Defaults()}); c.Status != Skipped || !strings.Contains(c.Summary, "no format or lint step") {
		t.Errorf("%+v", c)
	}
	write(t, dir, ".github/workflows/bad.yml", "jobs: [")
	if c := checkLint(context.Background(), Options{Dir: dir, Set: directive.Defaults()}); c.Status != Error {
		t.Errorf("a workflow that is not YAML is an error: %+v", c)
	}
}

func TestLintOnlyRunsTheStepsOfDirectoriesAChangeTouched(t *testing.T) {
	box := newToolBox(t)
	box.add(t, "cargo", "ok", 0)
	dir := t.TempDir()
	write(t, dir, ".github/workflows/ci.yml", "jobs:\n  a:\n    defaults:\n      run:\n        working-directory: crate-a\n    steps:\n      - run: cargo fmt --all --check\n  b:\n    defaults:\n      run:\n        working-directory: crate-b\n    steps:\n      - run: cargo clippy\n")
	write(t, dir, "crate-a/x.rs", "")
	write(t, dir, "crate-b/x.rs", "")
	scope := &Scope{lines: map[string][][2]int{"crate-b/x.rs": nil}}
	c := checkLint(context.Background(), Options{Dir: dir, Set: directive.Defaults(), Scope: scope})
	if c.Status != Pass || strings.Contains(box.calls(), "fmt") || !strings.Contains(box.calls(), "clippy") {
		t.Errorf("only crate-b was touched: %+v calls %s", c, box.calls())
	}
}

func TestLintRunsGolangciLintForTheActionAndSkipsMissingLoomGate(t *testing.T) {
	box := newToolBox(t)
	box.add(t, "golangci-lint", "clean", 0)
	dir := t.TempDir()
	write(t, dir, ".github/workflows/ci.yml", "jobs:\n  go:\n    steps:\n      - uses: golangci/golangci-lint-action@v8\n      - run: |\n          loom-gate loc --format github\n")
	c := checkLint(context.Background(), Options{Dir: dir, Set: directive.Defaults()})
	if !strings.Contains(box.calls(), "golangci-lint run") {
		t.Errorf("the action runs golangci-lint: %s", box.calls())
	}
	if c.Status != Pass {
		t.Errorf("a missing loom-gate is no failure, the gate runs the same rules itself: %+v", c)
	}
}

func TestInstallAdviceNamesTheToolsInstall(t *testing.T) {
	for script, want := range map[string]string{
		"cargo fmt": "rustfmt", "cargo clippy": "clippy", "uvx ruff check .": "uv", "bun run lint": "bun",
		"golangci-lint run": "golangci-lint", "loom-gate loc": "loom-gate", "other": "the tool",
	} {
		if got := installAdvice(script); !strings.Contains(got, want) {
			t.Errorf("%q: %q", script, got)
		}
	}
}

func TestACoverageStepsOwnFloorIsTheOneTheCoverageCheckHolds(t *testing.T) {
	box := newToolBox(t)
	box.add(t, "cargo", "ok", 0)
	dir := rustPeer(t)
	c := checkCoverage(context.Background(), Options{Dir: dir, Set: directive.Defaults()})
	if c.Status != Pass || !strings.Contains(c.Summary, "coverage floor of its workflow") {
		t.Fatalf("%+v", c)
	}
	if calls := box.calls(); !strings.Contains(calls, "cargo llvm-cov --fail-under-lines 90") || strings.Contains(calls, "100") {
		t.Errorf("the floor is the workflow's 90, not the directive's 100: %s", calls)
	}
}

// loomSource is a git repository holding a loom-gate crate (or not) and the revision of its one commit.
func loomSource(t *testing.T, withCrate bool) (repo, sha string) {
	t.Helper()
	repo = t.TempDir()
	testkit.GitInit(t, repo, "develop")
	if withCrate {
		write(t, repo, "loom-gate/Cargo.toml", "[package]\nname = \"loom-gate\"\n")
	} else {
		write(t, repo, "README.md", "no crate yet\n")
	}
	testkit.Git(t, repo, "add", "-A")
	testkit.Git(t, repo, "commit", "-q", "-m", "init")
	return repo, testkit.Git(t, repo, "rev-parse", "HEAD")
}

// loomWorkflow is a workflow installing loom-gate from repo with the given flag and ref text.
func loomWorkflow(t *testing.T, repo, flag, ref string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, ".github/workflows/ci.yml", "jobs:\n  gate:\n    steps:\n      - name: Install loom-gate\n        run: |\n          command -v loom-gate >/dev/null || cargo install --locked --git "+repo+" "+flag+" "+ref+" loom-gate\n")
	return dir
}

func TestLoomGatePinThatExistsAndHoldsTheCratePasses(t *testing.T) {
	repo, sha := loomSource(t, true)
	for name, tc := range map[string]struct{ flag, ref string }{
		"revision":          {"--rev", sha},
		"branch":            {"--branch", "develop"},
		"quoted expression": {"--rev", `"${{ vars.LOOM_GATE_REF || '` + sha + `' }}"`},
	} {
		c := checkLint(context.Background(), Options{Dir: loomWorkflow(t, repo, tc.flag, tc.ref), Set: directive.Defaults()})
		if c.Status != Pass || !strings.Contains(c.Summary, "exists and holds the crate") {
			t.Errorf("%s: %+v", name, c)
		}
	}
}

func TestLoomGatePinToARevisionThatDoesNotExistFails(t *testing.T) {
	repo, _ := loomSource(t, true)
	c := checkLint(context.Background(), Options{Dir: loomWorkflow(t, repo, "--rev", strings.Repeat("a", 40)), Set: directive.Defaults()})
	if c.Status != Fail || !strings.Contains(c.Summary, "does not exist") || !strings.Contains(c.Advice, "revision that exists") {
		t.Fatalf("%+v", c)
	}
	c = checkLint(context.Background(), Options{Dir: loomWorkflow(t, repo, "--branch", "nope"), Set: directive.Defaults()})
	if c.Status != Fail || !strings.Contains(c.Summary, "does not exist") {
		t.Fatalf("a branch that does not exist: %+v", c)
	}
}

func TestLoomGatePinToARevisionWithoutTheCrateFails(t *testing.T) {
	repo, sha := loomSource(t, false)
	c := checkLint(context.Background(), Options{Dir: loomWorkflow(t, repo, "--rev", sha), Set: directive.Defaults()})
	if c.Status != Fail || !strings.Contains(c.Summary, "has no loom-gate crate") {
		t.Fatalf("%+v", c)
	}
}

func TestLoomGatePinOfARepositoryThatCannotBeReachedIsAnError(t *testing.T) {
	c := checkLint(context.Background(), Options{Dir: loomWorkflow(t, filepath.Join(t.TempDir(), "gone.git"), "--branch", "develop"), Set: directive.Defaults()})
	if c.Status != Error || !strings.Contains(c.Summary, "could not be checked") {
		t.Fatalf("%+v", c)
	}
}

func TestLoomGatePinReadingIgnoresInstallsItCannotResolve(t *testing.T) {
	for run, ok := range map[string]bool{
		"cargo install --locked --git https://x/y --rev abc loom-gate":             true,
		"cargo install --locked --git https://x/y --tag v1 loom-gate":              true,
		"cargo install --locked --git https://x/y --rev ${{ vars.PIN }} loom-gate": false,
		"cargo install ripgrep": false,
		"command -v loom-gate >/dev/null || cargo install --git https://x/y --branch b loom-gate": true,
	} {
		if _, got := loomGatePin(ciStep{Run: run}); got != ok {
			t.Errorf("%q: %v, want %v", run, got, ok)
		}
	}
}

func TestMissingRefRecognisesGitsMessages(t *testing.T) {
	for out, want := range map[string]bool{
		"fatal: couldn't find remote ref nope":              true,
		"fatal: remote error: upload-pack: not our ref abc": true,
		"fatal: unable to access":                           false,
	} {
		if got := missingRef(out); got != want {
			t.Errorf("%q: %v", out, got)
		}
	}
	if hasLoomGateCrate("README.md\nsrc/lib.rs") || !hasLoomGateCrate("a\ncrates/loom-gate/Cargo.toml") {
		t.Error("the crate manifest is found at the root or nested")
	}
}

func TestLoomGatePinChecksSurfaceScratchAndTreeFailures(t *testing.T) {
	repo, sha := loomSource(t, true)
	dir := loomWorkflow(t, repo, "--rev", sha)
	failTree := func(ctx context.Context, d, name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "ls-tree" {
			return []byte("boom"), os.ErrInvalid
		}
		return cleanRunner(ctx, d, name, args...)
	}
	if c := checkLint(context.Background(), Options{Dir: dir, Set: directive.Defaults(), CoverageRunner: failTree}); c.Status != Error {
		t.Errorf("a tree that cannot be listed is an error: %+v", c)
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if c := checkLint(context.Background(), Options{Dir: dir, Set: directive.Defaults()}); c.Status != Error {
		t.Errorf("no scratch directory is an error: %+v", c)
	}
}

func TestCoverageIgnoresAWorkflowItCannotRead(t *testing.T) {
	dir := rustPeer(t)
	write(t, dir, ".github/workflows/bad.yml", "jobs: [")
	if script, found := workflowCoverage(dir, pipelineRust()); found || script != "" {
		t.Errorf("an unreadable workflow gives no command: %q %v", script, found)
	}
}

func pipelineRust() pipeline.Component { return pipeline.Component{Kind: pipeline.Rust, Dir: "."} }
