package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func detectGo(abs, rel string) (*Component, string) {
	if !exists(filepath.Join(abs, "go.mod")) {
		return nil, ""
	}
	return &Component{Kind: Go, Dir: rel}, ""
}

func detectRust(abs, rel string) (*Component, string) {
	data, err := os.ReadFile(filepath.Join(abs, "Cargo.toml"))
	if err != nil {
		return nil, ""
	}
	return &Component{Kind: Rust, Dir: rel, Workspace: strings.Contains(string(data), "[workspace]")}, ""
}

// ciScripts are the package.json scripts CI runs, in order.
var ciScripts = []string{"lint", "test", "build"}

func hasBunLock(abs string) bool {
	return exists(filepath.Join(abs, "bun.lock")) || exists(filepath.Join(abs, "bun.lockb"))
}

func packageScripts(data []byte) []string {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	var found []string
	for _, name := range ciScripts {
		if _, ok := pkg.Scripts[name]; ok {
			found = append(found, name)
		}
	}
	return found
}

// detectBun makes a component of a package with a Bun lockfile and a lint, test or build script. A package
// with scripts but another package manager's lockfile is a note: CI does not guess a migration.
func detectBun(abs, rel string) (*Component, string) {
	data, err := os.ReadFile(filepath.Join(abs, "package.json"))
	if err != nil {
		return nil, ""
	}
	scripts := packageScripts(data)
	switch {
	case len(scripts) == 0:
		return nil, join(rel, "package.json") + " has no lint, test or build script; it is not part of the CI workflow"
	case !hasBunLock(abs):
		return nil, join(rel, "package.json") + " has no bun.lock; run `bun install` there to adopt Bun, then `dendrite soma prepare` adds its CI job"
	}
	return &Component{Kind: Bun, Dir: rel, Scripts: scripts}, ""
}

func hasPythonTests(abs string) bool {
	if isDir(filepath.Join(abs, "tests")) {
		return true
	}
	data, err := os.ReadFile(filepath.Join(abs, "pyproject.toml"))
	return err == nil && strings.Contains(string(data), "[tool.pytest")
}

// detectUV makes a component of a Python project that has tests to run. A pyproject with none is a note.
func detectUV(abs, rel string) (*Component, string) {
	if !exists(filepath.Join(abs, "pyproject.toml")) {
		return nil, ""
	}
	if !hasPythonTests(abs) {
		return nil, join(rel, "pyproject.toml") + " has no tests (no tests/ directory, no [tool.pytest]); it is not part of the CI workflow"
	}
	return &Component{Kind: UV, Dir: rel}, ""
}
