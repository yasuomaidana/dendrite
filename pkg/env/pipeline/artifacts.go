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

func golangciContent(dir string, s directive.Set) []byte {
	if !present(dir, Go) {
		return nil
	}
	return []byte(strings.Join([]string{
		directive.Header,
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
	}, "\n"))
}

func clippyContent(dir string, s directive.Set) []byte {
	if !present(dir, Rust) {
		return nil
	}
	return []byte(fmt.Sprintf("%s\ncognitive-complexity-threshold = %d\n", directive.Header, s.Complexity.MaxCognitive))
}

func ruffContent(dir string, s directive.Set) []byte {
	if !present(dir, UV) {
		return nil
	}
	return []byte(fmt.Sprintf("%s\n[lint]\nselect = [\"E\", \"F\", \"C90\"]\n\n[lint.mccabe]\nmax-complexity = %d\n", directive.Header, s.Complexity.MaxCyclomatic))
}
