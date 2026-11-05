// Package pipeline renders a project's CI workflow from what the project is: the components it holds (Go
// module, Rust crate or workspace, Bun package, uv project), the quality directives, and an optional
// project-owned extension (.github/dendrite/pipeline.yml). A manifest with nothing to verify is not a
// component; it becomes a note instead of a CI step.
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
)

// Kind is the toolchain a component is built and tested with.
type Kind string

// The toolchains Dendrite generates CI for.
const (
	Go   Kind = "go"
	Rust Kind = "rust"
	Bun  Kind = "bun"
	UV   Kind = "uv"
)

// Component is one buildable unit of a repository: a manifest, in a directory, with work to run.
type Component struct {
	Kind Kind
	// Dir is relative to the project root; "." for the root.
	Dir string
	// Workspace is true for a Cargo workspace.
	Workspace bool
	// Scripts are the package.json scripts of a Bun component that CI runs (lint, test, build).
	Scripts []string
}

// ID is the component's job id: "go", "bun-website".
func (c Component) ID() string {
	if c.Dir == "." {
		return string(c.Kind)
	}
	return string(c.Kind) + "-" + strings.NewReplacer("/", "-", "_", "-", ".", "-").Replace(c.Dir)
}

// Has reports whether a Bun component has the script.
func (c Component) Has(script string) bool {
	for _, s := range c.Scripts {
		if s == script {
			return true
		}
	}
	return false
}

// skipDirs are directories that never hold a component of the project itself.
var skipDirs = map[string]bool{
	"node_modules": true, "target": true, "vendor": true, "dist": true, "build": true,
	"testdata": true, "dendrite-work": true, "docs": true,
}

// Detect finds the components of the project at root: the root and its direct subdirectories. The notes name
// every manifest that was found and left out, and why.
func Detect(root string) (components []Component, notes []string) {
	for _, rel := range scanDirs(root) {
		found, note := detectDir(root, rel)
		components = append(components, found...)
		notes = append(notes, note...)
	}
	return components, notes
}

func scanDirs(root string) []string {
	dirs := []string{"."}
	entries, err := os.ReadDir(root)
	if err != nil {
		return dirs
	}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !skipDirs[e.Name()] {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

// detector reports a component for a directory, or a note when a manifest has nothing to run.
type detector func(abs, rel string) (*Component, string)

var detectors = []detector{detectGo, detectRust, detectBun, detectUV}

func detectDir(root, rel string) (found []Component, notes []string) {
	abs := filepath.Join(root, rel)
	for _, d := range detectors {
		c, note := d(abs, rel)
		if c != nil {
			found = append(found, *c)
		}
		if note != "" {
			notes = append(notes, note)
		}
	}
	return found, notes
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func join(rel, name string) string { return filepath.ToSlash(filepath.Join(rel, name)) }
