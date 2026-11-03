package pipeline

import (
	"bytes"
	"fmt"
	"strings"

	"Dendrite/pkg/directive"
)

func init() {
	directive.Register(directive.Artifact{Path: ".github/workflows/ci.yml", Content: ciContent, Owns: ownsWorkflow})
	directive.Register(directive.Artifact{Path: ".golangci.yml", Content: golangciContent, Owns: ownsConfig})
	directive.Register(directive.Artifact{Path: "clippy.toml", Content: clippyContent, Owns: ownsConfig})
	directive.Register(directive.Artifact{Path: "ruff.toml", Content: ruffContent, Owns: ownsConfig})
}

func ownsWorkflow(b []byte) bool { return bytes.HasPrefix(b, []byte(ManagedHeader)) }
func ownsConfig(b []byte) bool   { return bytes.HasPrefix(b, []byte(directive.Header)) }

// ciContent renders the workflow; a project whose extension cannot be read keeps its workflow.
func ciContent(dir string, s directive.Set) []byte {
	ext, err := LoadExtension(dir)
	if err != nil {
		return nil
	}
	comps, _ := Detect(dir)
	return Render(comps, s, ext)
}

func present(dir string, k Kind) bool {
	comps, _ := Detect(dir)
	return hasKind(comps, k)
}

// forKind renders a configuration file only for a project that has a component of the kind.
func forKind(k Kind, body func(directive.Set) string) func(string, directive.Set) []byte {
	return func(dir string, s directive.Set) []byte {
		if !present(dir, k) {
			return nil
		}
		return []byte(directive.Header + "\n" + body(s))
	}
}

var (
	golangciContent = forKind(Go, func(s directive.Set) string {
		return strings.Join([]string{
			`version: "2"`,
			"linters:",
			"  enable:",
			"    - gocyclo",
			"    - gocognit",
			"  settings:",
			"    gocyclo:",
			fmt.Sprintf("      min-complexity: %d", s.Complexity.MaxCyclomatic),
			"    gocognit:",
			fmt.Sprintf("      min-complexity: %d", s.Complexity.MaxCognitive),
			"",
		}, "\n")
	})
	clippyContent = forKind(Rust, func(s directive.Set) string {
		return fmt.Sprintf("cognitive-complexity-threshold = %d\n", s.Complexity.MaxCognitive)
	})
	ruffContent = forKind(UV, func(s directive.Set) string {
		return fmt.Sprintf("[lint]\nselect = [\"E\", \"F\", \"C90\"]\n\n[lint.mccabe]\nmax-complexity = %d\n", s.Complexity.MaxCyclomatic)
	})
)
