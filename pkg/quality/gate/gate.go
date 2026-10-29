// Package gate runs the quality gates of a project's directives as one check: production lines per file,
// repeated code, the Rust test structure and test coverage. It is what CI, `dendrite allostasis gate` and
// `dendrite chronos finish` all run, so the same rule holds everywhere.
package gate

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/directive"
)

// Status is how one check ended.
type Status string

const (
	Pass    Status = "pass"
	Fail    Status = "fail"
	Skipped Status = "skipped"
	// Error means the check could not run (no sidecar, a bad directives file): it fails the gate, because a
	// check that did not run proves nothing.
	Error Status = "error"
)

// Check is the outcome of one gate.
type Check struct {
	Name     string   `json:"name"`
	Status   Status   `json:"status"`
	Summary  string   `json:"summary"`
	Findings []string `json:"findings,omitempty"`
	// Advice says how the project wants a failure of this check fixed.
	Advice string `json:"advice,omitempty"`
}

// Report is the outcome of the whole gate.
type Report struct {
	Schema int `json:"schema"`
	// Scope is "repository" (everything) or "changes" (what a change touched).
	Scope  string  `json:"scope"`
	Passed bool    `json:"passed"`
	Checks []Check `json:"checks"`
}

// Sidecar runs the dendrite-rag quality methods; *rag.Retriever is the real one.
type Sidecar interface {
	Quality(ctx context.Context, root, method string, params map[string]interface{}, out interface{}) error
}

// CommandRunner runs a command and returns its combined output (see coverage.Options).
type CommandRunner func(ctx context.Context, dir, name string, args ...string) ([]byte, error)

// Options say what to check and against what.
type Options struct {
	// Dir is the project root.
	Dir string
	// Set are the project's directives.
	Set directive.Set
	// Sidecar runs the loc, clones and rstest gates.
	Sidecar Sidecar
	// Scope limits the gate to a change; nil checks the whole repository.
	Scope *Scope
	// Only and Skip pick checks by name (loc, clones, rstest, coverage). Only wins when both are set.
	Only, Skip []string
	// CoverageRunner replaces the real `go test` of the coverage check (tests only).
	CoverageRunner CommandRunner
}

// Names are the checks, in the order they run.
var Names = []string{"loc", "clones", "rstest", "coverage"}

// Run runs the selected checks and collects their outcomes.
func Run(ctx context.Context, o Options) Report {
	r := Report{Schema: 1, Scope: "repository", Passed: true}
	if o.Scope != nil {
		r.Scope = "changes"
	}
	for _, name := range Names {
		if !selected(name, o.Only, o.Skip) {
			continue
		}
		var c Check
		switch name {
		case "loc":
			c = checkLoc(ctx, o)
		case "clones":
			c = checkClones(ctx, o)
		case "rstest":
			c = checkRstest(ctx, o)
		default:
			c = checkCoverage(ctx, o)
		}
		if c.Status == Fail || c.Status == Error {
			r.Passed = false
		}
		r.Checks = append(r.Checks, c)
	}
	return r
}

func selected(name string, only, skip []string) bool {
	if len(only) > 0 {
		return contains(only, name)
	}
	return !contains(skip, name)
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), want) {
			return true
		}
	}
	return false
}

// Validate checks that a list of check names only holds known ones.
func Validate(names []string) error {
	for _, n := range names {
		if !contains(Names, n) {
			return fmt.Errorf("unknown check %q (known: %s)", n, strings.Join(Names, ", "))
		}
	}
	return nil
}
