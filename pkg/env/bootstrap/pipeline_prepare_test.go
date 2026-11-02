package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAGoOnlyProjectGetsNoBunOrUvAnywhere(t *testing.T) {
	root := t.TempDir()
	report := prepareCI(t, root)
	ci := readCI(t, root)
	for _, banned := range []string{"setup-bun", "setup-uv", "bun ", "uv "} {
		if strings.Contains(ci, banned) {
			t.Errorf("a Go project's workflow mentions %q:\n%s", banned, ci)
		}
	}
	if !strings.Contains(ci, "loom-gate complexity") || !strings.Contains(ci, "golangci-lint") {
		t.Errorf("the workflow gates complexity and lints Go:\n%s", ci)
	}
	copilot, err := os.ReadFile(filepath.Join(root, ".github", "copilot-instructions.md"))
	if err != nil || strings.Contains(string(copilot), "Bun") || strings.Contains(string(copilot), "uv`") {
		t.Errorf("the instructions list only the project's toolchains: %v\n%s", err, copilot)
	}
	if report.CICD.CoverageThreshold != 100 {
		t.Errorf("the coverage threshold is the directive's, got %v", report.CICD.CoverageThreshold)
	}
}

func TestComplexityLimitsComeFromTheDirectivesInEveryFile(t *testing.T) {
	root := t.TempDir()
	prepareCI(t, root)
	set, err := directive.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	set.Complexity.MaxCyclomatic, set.Complexity.MaxCognitive = 7, 11
	if err := directive.Save(root, set); err != nil {
		t.Fatal(err)
	}
	prepareCI(t, root)
	golangci, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil || !strings.Contains(string(golangci), "min-complexity: 7") || !strings.Contains(string(golangci), "min-complexity: 11") {
		t.Errorf("golangci limits follow the directives: %v\n%s", err, golangci)
	}
	copilot, _ := os.ReadFile(filepath.Join(root, ".github", "copilot-instructions.md"))
	if !strings.Contains(string(copilot), "cyclomatic complexity <= 7 and cognitive complexity <= 11") {
		t.Errorf("the instructions follow the directives:\n%s", copilot)
	}
}

func TestAdoptCITakesOverAHandWrittenWorkflowAndKeepsItsJobsReachable(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n\ngo 1.25\n")
	write(t, root, ".github/workflows/ci.yml", "name: CI\njobs:\n  sidecar-build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make sidecar\n")
	write(t, root, ".github/workflows/quality.yml", "name: quality\njobs:\n  loc:\n    runs-on: ubuntu-latest\n    steps:\n      - run: bash scripts/check-max-lines.sh\n")
	t.Setenv("DENDRITE_HOME", t.TempDir())
	p := NewPreparer()
	report, err := p.Prepare(t.Context(), PrepareOptions{ProjectPath: root, EnforceCICD: true, AdoptCI: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(readCI(t, root), ManagedHeader) {
		t.Error("the workflow is now generated")
	}
	prev, err := os.ReadFile(filepath.Join(root, ".github", "dendrite", "ci.previous.yml"))
	if err != nil || !strings.Contains(string(prev), "sidecar-build") {
		t.Errorf("the old workflow is archived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "quality.yml")); err == nil {
		t.Error("the hand-written quality workflow is replaced by the quality job")
	}
	joined := strings.Join(report.Notes, "\n")
	if !strings.Contains(joined, "sidecar-build") || !strings.Contains(joined, "pipeline.yml") {
		t.Errorf("the notes name the jobs to move into the extension: %v", report.Notes)
	}
}

func TestTheProjectExtensionAddsEnvPreTestStepsAndJobs(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".github/dendrite/pipeline.yml", "env:\n  FOO: bar\npre_test:\n  - name: Build the sidecar\n    run: make sidecar\njobs:\n  docs:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make docs\nskip_coverage_floor: true\n")
	prepareCI(t, root)
	ci := readCI(t, root)
	for _, want := range []string{"FOO: bar", "Build the sidecar", "make sidecar", "  docs:", "make docs"} {
		if !strings.Contains(ci, want) {
			t.Errorf("the workflow lacks %q:\n%s", want, ci)
		}
	}
	if strings.Contains(ci, "Coverage floor") {
		t.Error("skip_coverage_floor leaves the threshold to the project")
	}
}

func TestAMalformedExtensionIsAnErrorThatNamesTheFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n\ngo 1.25\n")
	write(t, root, ".github/dendrite/pipeline.yml", "unknown_key: 1\n")
	t.Setenv("DENDRITE_HOME", t.TempDir())
	_, err := NewPreparer().Prepare(t.Context(), PrepareOptions{ProjectPath: root, EnforceCICD: true})
	if err == nil || !strings.Contains(err.Error(), "pipeline.yml") {
		t.Errorf("want an error naming pipeline.yml, got %v", err)
	}
}
