package gate

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// workflowGlobs are where a project keeps the GitHub workflows the gate mirrors.
var workflowGlobs = []string{".github/workflows/*.yml", ".github/workflows/*.yaml"}

// ciStep is one step of a workflow job, with the directory it runs in (relative to the project root, "." for
// the root) and the environment it sets.
type ciStep struct {
	File string
	Job  string
	Name string
	Dir  string
	Run  string
	Uses string
	With map[string]string
	Env  map[string]string
}

// workflowFile is the part of a GitHub workflow the gate reads.
type workflowFile struct {
	Jobs map[string]struct {
		Defaults struct {
			Run struct {
				WorkingDirectory string `yaml:"working-directory"`
			} `yaml:"run"`
		} `yaml:"defaults"`
		Env   map[string]string `yaml:"env"`
		Steps []struct {
			Name             string            `yaml:"name"`
			Run              string            `yaml:"run"`
			Uses             string            `yaml:"uses"`
			With             map[string]any    `yaml:"with"`
			Env              map[string]string `yaml:"env"`
			WorkingDirectory string            `yaml:"working-directory"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// ciSteps lists every step of the project's workflows, in file and job order. A workflow that is not valid YAML
// is an error: the gate cannot say what CI would run.
func ciSteps(root string) ([]ciStep, error) {
	var files []string
	for _, g := range workflowGlobs {
		found, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(g)))
		files = append(files, found...)
	}
	sort.Strings(files)
	var steps []ciStep
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var wf workflowFile
		if err := yaml.Unmarshal(data, &wf); err != nil {
			return nil, &workflowError{File: f, Err: err}
		}
		rel, _ := filepath.Rel(root, f)
		steps = append(steps, stepsOf(filepath.ToSlash(rel), wf)...)
	}
	return steps, nil
}

// workflowError names the workflow file that could not be read.
type workflowError struct {
	File string
	Err  error
}

func (e *workflowError) Error() string { return e.File + ": " + e.Err.Error() }
func (e *workflowError) Unwrap() error { return e.Err }

func stepsOf(file string, wf workflowFile) []ciStep {
	jobs := make([]string, 0, len(wf.Jobs))
	for id := range wf.Jobs {
		jobs = append(jobs, id)
	}
	sort.Strings(jobs)
	var out []ciStep
	for _, id := range jobs {
		job := wf.Jobs[id]
		for _, s := range job.Steps {
			env := map[string]string{}
			for k, v := range job.Env {
				env[k] = v
			}
			for k, v := range s.Env {
				env[k] = v
			}
			with := map[string]string{}
			for k, v := range s.With {
				with[k] = stringOf(v)
			}
			dir := firstNonEmpty(s.WorkingDirectory, job.Defaults.Run.WorkingDirectory, ".")
			out = append(out, ciStep{File: file, Job: id, Name: s.Name, Dir: filepath.ToSlash(filepath.Clean(dir)), Run: s.Run, Uses: s.Uses, With: with, Env: env})
		}
	}
	return out
}

func stringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	out, _ := yaml.Marshal(v)
	return strings.TrimSpace(string(out))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// localRunnable are the command prefixes whose steps the gate runs as the workflow wrote them: formatting,
// lint and coverage of the toolchains CI is generated for. Anything else (deploys, caches, report-only steps)
// is left to CI.
var localRunnable = []string{
	"cargo fmt", "cargo clippy", "cargo llvm-cov",
	"ruff ", "uvx ruff", "uv run ruff", "uv run --with pytest-cov pytest",
	"bun run lint", "golangci-lint", "loom-gate ",
}

// githubOnly marks a command that needs the GitHub Actions runtime: an expression or one of its variables.
var githubOnly = regexp.MustCompile(`\$\{\{|\$GITHUB_|\$RUNNER_|::(error|warning|notice)`)

// runnableLines are the commands of a step's run block when every line is one the gate can run locally and none
// needs GitHub; otherwise nil.
func runnableLines(run string) []string {
	if strings.TrimSpace(run) == "" || githubOnly.MatchString(run) {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(run, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if !hasAnyPrefix(l, localRunnable) {
			return nil
		}
		lines = append(lines, l)
	}
	return lines
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// isGolangciAction reports whether the step is the golangci-lint GitHub action.
func (s ciStep) isGolangciAction() bool {
	return strings.HasPrefix(s.Uses, "golangci/golangci-lint-action")
}

// script is the shell the step runs locally: its environment exported, then its commands. ok is false for a step
// the gate does not run.
func (s ciStep) script() (script string, ok bool) {
	var cmds []string
	switch {
	case s.isGolangciAction():
		cmds = []string{"golangci-lint run"}
		if dir := s.With["working-directory"]; dir != "" {
			s.Dir = filepath.ToSlash(filepath.Clean(dir))
		}
	default:
		cmds = runnableLines(s.Run)
	}
	if len(cmds) == 0 {
		return "", false
	}
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		if githubOnly.MatchString(s.Env[k]) {
			return "", false
		}
		b.WriteString("export " + k + "=" + shellQuote(s.Env[k]) + "\n")
	}
	b.WriteString(strings.Join(cmds, "\n"))
	return b.String(), true
}

func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

// isCoverage reports whether the step measures coverage (the coverage check owns those).
func (s ciStep) isCoverage() bool {
	return strings.Contains(s.Run, "cargo llvm-cov") || strings.Contains(s.Run, "--cov")
}
