package pipeline

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/fsutil/fsutiltest"
	"Dendrite/pkg/testkit"
)

func ids(comps []Component) []string {
	var out []string
	for _, c := range comps {
		out = append(out, c.ID())
	}
	return out
}

func TestDetect(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		want  []string
		note  string
	}{
		"go only":           {map[string]string{"go.mod": "module x\n"}, []string{"go"}, ""},
		"rust workspace":    {map[string]string{"Cargo.toml": "[workspace]\n"}, []string{"rust"}, ""},
		"bun with lock":     {map[string]string{"website/package.json": `{"scripts":{"build":"x"}}`, "website/bun.lock": "{}"}, []string{"bun-website"}, ""},
		"bun legacy lock":   {map[string]string{"package.json": `{"scripts":{"test":"x"}}`, "bun.lockb": ""}, []string{"bun"}, ""},
		"npm lock only":     {map[string]string{"website/package.json": `{"scripts":{"build":"x"}}`, "website/package-lock.json": "{}"}, nil, "no bun.lock"},
		"package no work":   {map[string]string{"package.json": `{"name":"x"}`}, nil, "no lint, test or build script"},
		"broken package":    {map[string]string{"package.json": `{`}, nil, "no lint, test or build script"},
		"uv with tests":     {map[string]string{"pyproject.toml": "[project]\n", "tests/test_a.py": ""}, []string{"uv"}, ""},
		"uv with pytest":    {map[string]string{"scripts/pyproject.toml": "[tool.pytest.ini_options]\n"}, []string{"uv-scripts"}, ""},
		"pyproject no test": {map[string]string{"scripts/pyproject.toml": "[project]\n"}, nil, "has no tests"},
		"skipped dirs":      {map[string]string{"node_modules/p/go.mod": "", "target/Cargo.toml": "", ".hidden/go.mod": ""}, nil, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range c.files {
				fsutiltest.WriteFile(t, root, rel, content)
			}
			comps, notes := Detect(root)
			if got := ids(comps); strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("components %v, want %v", got, c.want)
			}
			if joined := strings.Join(notes, "|"); (c.note == "") != (joined == "") || !strings.Contains(joined, c.note) {
				t.Errorf("notes %q, want one containing %q", joined, c.note)
			}
		})
	}
	if comps, _ := Detect(filepath.Join(t.TempDir(), "missing")); len(comps) != 0 {
		t.Error("a missing directory has no components")
	}
}

func TestWorkspaceAndScripts(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "Cargo.toml", "[workspace]\nmembers = []\n")
	fsutiltest.WriteFile(t, root, "package.json", `{"scripts":{"lint":"a","build":"b"}}`)
	fsutiltest.WriteFile(t, root, "bun.lock", "{}")
	comps, _ := Detect(root)
	if len(comps) != 2 || !comps[0].Workspace || !comps[1].Has("lint") || comps[1].Has("test") {
		t.Errorf("%+v", comps)
	}
}

type workflow struct {
	Jobs map[string]struct {
		Steps []map[string]any `yaml:"steps"`
	} `yaml:"jobs"`
	Env map[string]string `yaml:"env"`
}

func render(t *testing.T, root string, set directive.Set, ext Extension) (string, workflow) {
	t.Helper()
	comps, _ := Detect(root)
	text := string(Render(comps, set, ext, BranchesOf(root)))
	var wf workflow
	if err := yaml.Unmarshal([]byte(text), &wf); err != nil {
		t.Fatalf("the generated workflow is not YAML: %v\n%s", err, text)
	}
	return text, wf
}

func jobIDs(wf workflow) string {
	var out []string
	for id := range wf.Jobs {
		out = append(out, id)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestRenderAPolyglotProjectIsValidYAMLWithAJobPerComponent(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n")
	fsutiltest.WriteFile(t, root, "crates/Cargo.toml", "[workspace]\n")
	fsutiltest.WriteFile(t, root, "web/package.json", `{"scripts":{"lint":"a","test":"b","build":"c"}}`)
	fsutiltest.WriteFile(t, root, "web/bun.lock", "{}")
	fsutiltest.WriteFile(t, root, "py/pyproject.toml", "[project]\n")
	fsutiltest.WriteFile(t, root, "py/tests/test_a.py", "")
	set := directive.Defaults()
	set.Coverage.MinPercent = 87.5
	text, wf := render(t, root, set, Extension{})
	if got := jobIDs(wf); got != "bun-web,db-access,go,quality,release-refs,rust-crates,uv-py" {
		t.Errorf("jobs %s\n%s", got, text)
	}
	for _, want := range []string{
		"loom-gate complexity --format github", "loom-gate rstest --format github", "golangci/golangci-lint-action@v8",
		"working-directory: crates", "cargo clippy --workspace --all-targets -- -D warnings -W clippy::cognitive_complexity",
		"cargo llvm-cov --workspace --fail-under-lines 87.5", "components: rustfmt, clippy, llvm-tools-preview", "bun install --frozen-lockfile", "bun run build",
		"uvx ruff check .", "--cov-fail-under=87.5", "Coverage floor (>= 87.5%)", "go-version-file: go.mod",
		`--tag "${{ vars.LOOM_GATE_REF || 'v0.1.0' }}" loom-gate`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q\n%s", want, text)
		}
	}
	if !strings.HasPrefix(text, ManagedHeader+"\n") {
		t.Error("a generated workflow carries the managed header")
	}
	if strings.Contains(text, "--branch") {
		t.Errorf("loom-gate is installed from a release tag, not a branch (the rag-rs develop branch is gone)\n%s", text)
	}
}

func TestRenderWithoutRustSkipsRstestAndTheExtensionShapesTheJobs(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "Cargo.toml", "[package]\nname = \"x\"\n")
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n")
	ext := Extension{
		Env:               map[string]string{"A": "1"},
		PreTest:           []map[string]any{{"name": "Build it", "run": "make"}},
		Jobs:              map[string]map[string]any{"zeta": {"runs-on": "ubuntu-latest"}, "alpha": {"runs-on": "ubuntu-latest"}},
		SkipCoverageFloor: true,
		LoomGate:          "cargo run -q -p loom-gate --",
	}
	text, wf := render(t, root, directive.Defaults(), ext)
	if wf.Env["A"] != "1" || !strings.Contains(text, "cargo run -q -p loom-gate -- rstest --format github") {
		t.Errorf("%s", text)
	}
	if strings.Contains(text, "cargo install") || strings.Contains(text, "Coverage floor") || strings.Contains(text, "llvm-cov") {
		t.Errorf("the extension replaces the install and the floor:\n%s", text)
	}
	if strings.Index(text, "  alpha:") > strings.Index(text, "  zeta:") || jobIDs(wf) != "alpha,db-access,go,quality,release-refs,rust,zeta" {
		t.Errorf("extra jobs are added in name order: %s", jobIDs(wf))
	}
	if strings.Count(text, "Build it") != 2 {
		t.Errorf("pre_test steps run in every component job:\n%s", text)
	}
	_, wf = render(t, t.TempDir(), directive.Defaults(), Extension{})
	if jobIDs(wf) != "quality,release-refs" {
		t.Errorf("a project with no component still runs the quality gates: %s", jobIDs(wf))
	}
}

func TestScalarQuotesOnlyWhatNeedsIt(t *testing.T) {
	for in, want := range map[string]string{"Plain name (x)": "Plain name (x)", "a: b": "'a: b'", "it's": "'it''s'", "${{ x }}": "'${{ x }}'"} {
		if got := scalar(in); got != want {
			t.Errorf("scalar(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadExtension(t *testing.T) {
	root := t.TempDir()
	if ext, err := LoadExtension(root); err != nil || ext.LoomGate != "" {
		t.Fatalf("no file is an empty extension: %+v, %v", ext, err)
	}
	fsutiltest.WriteFile(t, root, ExtensionPath, "")
	if _, err := LoadExtension(root); err != nil {
		t.Errorf("an empty file is an empty extension: %v", err)
	}
	fsutiltest.WriteFile(t, root, ExtensionPath, "skip_coverage_floor: true\n")
	if ext, err := LoadExtension(root); err != nil || !ext.SkipCoverageFloor {
		t.Errorf("%+v, %v", ext, err)
	}
	fsutiltest.WriteFile(t, root, ExtensionPath, "nope: 1\n")
	if _, err := LoadExtension(root); err == nil || !strings.Contains(err.Error(), "pipeline.yml") {
		t.Errorf("a bad extension names its file: %v", err)
	}
}

func TestLoadExtensionRefusesADirectoryWhereTheFileBelongs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(ExtensionPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadExtension(root); err == nil {
		t.Error("a directory where the file belongs is an error")
	}
}

func TestLintConfigurationsFollowTheDirectivesAndTheComponents(t *testing.T) {
	set := directive.Defaults()
	set.Complexity.MaxCyclomatic, set.Complexity.MaxCognitive = 6, 9
	root := t.TempDir()
	if golangciContent(root, set) != nil || clippyContent(root, set) != nil || ruffContent(root, set) != nil {
		t.Error("a project with none of the stacks gets none of the configurations")
	}
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n")
	fsutiltest.WriteFile(t, root, "Cargo.toml", "[package]\n")
	fsutiltest.WriteFile(t, root, "pyproject.toml", "[tool.pytest.ini_options]\n")
	for name, want := range map[string]struct {
		got  []byte
		with []string
	}{
		"golangci": {golangciContent(root, set), []string{"min-complexity: 6", "min-complexity: 9"}},
		"clippy":   {clippyContent(root, set), []string{"threshold = 9"}},
		"ruff":     {ruffContent(root, set), []string{"max-complexity = 6"}},
	} {
		if !ownsConfig(want.got) {
			t.Errorf("%s carries the directives header", name)
		}
		for _, text := range want.with {
			if !strings.Contains(string(want.got), text) {
				t.Errorf("%s lacks %q (limits come from the directives):\n%s", name, text, want.got)
			}
		}
	}
}

func TestAWorkflowWhoseExtensionCannotBeReadIsKept(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, ExtensionPath, "bad: 1\n")
	if ciContent(root, directive.Defaults()) != nil {
		t.Error("a workflow whose extension cannot be read is kept")
	}
}

func TestToolchainDocs(t *testing.T) {
	if got := ToolchainSummary([]string{"go", "uv", "other"}); got != "Go (`go`), Python (`uv`)" {
		t.Errorf("%q", got)
	}
	if ToolchainSummary(nil) != "none detected" || !strings.Contains(ToolchainBullets(nil), "No build toolchain") {
		t.Error("no toolchain says so")
	}
	if got := ToolchainBullets([]string{"cargo"}); !strings.HasPrefix(got, "- **Rust**: `cargo`") {
		t.Errorf("%q", got)
	}
}

func TestScaffold(t *testing.T) {
	set := directive.Defaults()
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n")
	dry, err := Scaffold(root, set, false, true)
	if err != nil || len(dry.Created) != 1 {
		t.Fatalf("%+v, %v", dry, err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(WorkflowPath))); err == nil {
		t.Fatal("a dry run writes nothing")
	}
	if res, err := Scaffold(root, set, false, false); err != nil || len(res.Created) != 1 {
		t.Fatalf("%+v, %v", res, err)
	}
	if res, _ := Scaffold(root, set, false, false); len(res.Created)+len(res.Updated) != 0 {
		t.Errorf("a current workflow is left alone: %+v", res)
	}
	set.Coverage.MinPercent = 90
	if res, _ := Scaffold(root, set, false, false); len(res.Updated) != 1 {
		t.Errorf("a changed directive rewrites the workflow: %+v", res)
	}
	fsutiltest.WriteFile(t, root, ".github/dendrite/pipeline.yml", "bad: 1\n")
	if _, err := Scaffold(root, set, false, false); err == nil {
		t.Error("a bad extension is an error")
	}
}

const handWrittenCI = "name: CI\njobs:\n  keep-me:\n    runs-on: ubuntu-latest\n    steps:\n      - run: true\n"

func handWrittenProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n")
	fsutiltest.WriteFile(t, root, WorkflowPath, handWrittenCI)
	fsutiltest.WriteFile(t, root, ".github/workflows/quality.yml", "name: quality\n")
	return root
}

func fileAt(t *testing.T, root, rel string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func TestScaffoldKeepsAHandWrittenWorkflowByDefault(t *testing.T) {
	root := handWrittenProject(t)
	res, err := Scaffold(root, directive.Defaults(), false, false)
	if err != nil || len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "--adopt-ci") {
		t.Fatalf("kept: %+v, %v", res, err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(WorkflowPath))); string(got) != handWrittenCI {
		t.Fatal("a hand-written workflow stays byte for byte")
	}
}

func TestAdoptingInADryRunArchivesNothing(t *testing.T) {
	root := handWrittenProject(t)
	dry, err := Scaffold(root, directive.Defaults(), true, true)
	if err != nil || len(dry.Notes) < 3 || fileAt(t, root, ".github/dendrite/ci.previous.yml") {
		t.Fatalf("dry adopt: %+v, %v", dry, err)
	}
}

func TestAdoptingArchivesTheOldWorkflowsAndNamesTheLostJobs(t *testing.T) {
	root := handWrittenProject(t)
	res, err := Scaffold(root, directive.Defaults(), true, false)
	if err != nil || len(res.Updated) != 1 || !strings.Contains(strings.Join(res.Notes, "|"), "keep-me") {
		t.Fatalf("adopted: %+v, %v", res, err)
	}
	for _, rel := range []string{".github/dendrite/ci.previous.yml", ".github/dendrite/quality.previous.yml"} {
		if !fileAt(t, root, rel) {
			t.Errorf("%s is archived", rel)
		}
	}
	if fileAt(t, root, ".github/workflows/quality.yml") {
		t.Error("the hand-written quality workflow is gone")
	}
}

func TestUnparseableYAMLHasNoJobs(t *testing.T) {
	if names := jobNames([]byte("jobs: [")); len(names) != 0 {
		t.Error("unparseable YAML has no jobs")
	}
}

func TestCIContentRendersAProjectsWorkflow(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n")
	if got := ciContent(root, directive.Defaults()); !ownsWorkflow(got) || !strings.Contains(string(got), "golangci") {
		t.Errorf("%s", got)
	}
}

func TestStepsOfTheNestedAndPartialComponents(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "svc/go.mod", "module x\n")
	fsutiltest.WriteFile(t, root, "web/package.json", `{"scripts":{"test":"a"}}`)
	fsutiltest.WriteFile(t, root, "web/bun.lock", "{}")
	text, _ := render(t, root, directive.Defaults(), Extension{})
	for _, want := range []string{"working-directory: svc", "bun run test"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "bun run build") || strings.Contains(text, "bun run lint") {
		t.Errorf("only the scripts the package has run:\n%s", text)
	}
}

func TestScaffoldReportsWhatItCannotRead(t *testing.T) {
	set := directive.Defaults()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(WorkflowPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, set, false, false); err == nil {
		t.Error("a directory where the workflow belongs is an error")
	}
}

func TestAdoptReportsAnArchiveItCannotWrite(t *testing.T) {
	set := directive.Defaults()
	for _, blocked := range []string{"ci.previous.yml", "quality.previous.yml"} {
		root := t.TempDir()
		fsutiltest.WriteFile(t, root, WorkflowPath, "name: mine\n")
		fsutiltest.WriteFile(t, root, ".github/workflows/quality.yml", "name: q\n")
		if err := os.MkdirAll(filepath.Join(root, ".github", "dendrite", blocked), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Scaffold(root, set, true, false); err == nil {
			t.Errorf("%s cannot be archived over a directory", blocked)
		}
	}
}

func TestScaffoldReportsAWorkflowItCannotWriteOrRemove(t *testing.T) {
	set := directive.Defaults()
	for _, adopt := range []bool{false, true} {
		root := t.TempDir()
		fsutiltest.WriteFile(t, root, "go.mod", "module x\n")
		if adopt {
			fsutiltest.WriteFile(t, root, WorkflowPath, "name: mine\n")
			fsutiltest.WriteFile(t, root, ".github/workflows/quality.yml", "name: q\n")
		}
		dir := filepath.Join(root, ".github", "workflows")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		// Whoever may write into a read-only directory (root) succeeds; everyone else gets the error.
		canWrite := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o644) == nil
		_, err := Scaffold(root, set, adopt, false)
		if (err == nil) != canWrite {
			t.Errorf("adopt=%v canWrite=%v err=%v", adopt, canWrite, err)
		}
		_ = os.Chmod(dir, 0o755)
	}
}

func TestAdoptWithoutAHandWrittenQualityWorkflowArchivesOnlyCI(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, WorkflowPath, "name: mine\n")
	res, err := Scaffold(root, directive.Defaults(), true, false)
	if err != nil || len(res.Updated) != 1 {
		t.Fatalf("%+v, %v", res, err)
	}
	if fileAt(t, root, ".github/dendrite/quality.previous.yml") {
		t.Error("there is no quality workflow to archive")
	}
}

func TestAProjectWithItsOwnCoverageRatchetKeepsPlainTestRuns(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "pyproject.toml", "[tool.pytest.ini_options]\n")
	text, _ := render(t, root, directive.Defaults(), Extension{SkipCoverageFloor: true})
	if !strings.Contains(text, "uv run pytest\n") || strings.Contains(text, "cov-fail-under") {
		t.Errorf("%s", text)
	}
}

func TestTheWorkflowRunsForTheIntegrationMainAndFeatureBranches(t *testing.T) {
	for name, tc := range map[string]struct {
		br   Branches
		want string
	}{
		"default":        {Branches{Integration: "develop", FeaturePrefix: "feature/"}, `[ develop, main, "feature/**" ]`},
		"custom":         {Branches{Integration: "trunk", FeaturePrefix: "feat/"}, `[ trunk, main, "feat/**" ]`},
		"main is shared": {Branches{Integration: "main", FeaturePrefix: "feature/"}, `[ main, "feature/**" ]`},
		"no features":    {Branches{Integration: "develop"}, `[ develop, main ]`},
	} {
		if got := tc.br.triggers(); got != tc.want {
			t.Errorf("%s: triggers %q, want %q", name, got, tc.want)
		}
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testkit.GitInit(t, root, "develop")
	testkit.Git(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	text, _ := render(t, root, directive.Defaults(), Extension{})
	if strings.Count(text, `branches: [ develop, main, "feature/**" ]`) != 1 {
		t.Errorf("push runs for the branches:\n%s", text)
	}
}

func TestEveryPullRequestRunsTheWorkflowWithoutFilters(t *testing.T) {
	root := t.TempDir()
	fsutiltest.WriteFile(t, root, "go.mod", "module x\n")
	text, _ := render(t, root, directive.Defaults(), Extension{})
	on := text[strings.Index(text, "on:"):strings.Index(text, "jobs:")]
	if !strings.Contains(on, "  pull_request:\n") || strings.Count(on, "branches:") != 1 || strings.Contains(on, "paths") {
		t.Errorf("pull_request must carry no branch or path filter, only push does:\n%s", on)
	}
}

func TestRustProjectsGetTheDieselOnlyJobAndScript(t *testing.T) {
	rust, other := t.TempDir(), t.TempDir()
	fsutiltest.WriteFile(t, rust, "Cargo.toml", "[workspace]\n")
	fsutiltest.WriteFile(t, other, "go.mod", "module x\n")
	text, _ := render(t, rust, directive.Defaults(), Extension{})
	for _, want := range []string{"db-access:", "name: db-access (diesel only)", "run: bash scripts/check-db-access.sh"} {
		if !strings.Contains(text, want) {
			t.Errorf("the Rust workflow lacks %q:\n%s", want, text)
		}
	}
	if goText, _ := render(t, other, directive.Defaults(), Extension{}); strings.Contains(goText, "db-access") {
		t.Errorf("a Go project has no Diesel job:\n%s", goText)
	}
	if got := dbAccessContent(rust, directive.Defaults()); !ownsDBAccess(got) || len(got) == 0 {
		t.Error("a Rust project gets the managed script")
	}
	if dbAccessContent(other, directive.Defaults()) != nil {
		t.Error("a Go project gets no script")
	}
	if ownsDBAccess([]byte("#!/bin/sh\necho mine\n")) {
		t.Error("a hand-written script is not owned")
	}
}
