package bootstrap

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// RstestRequirement is the dependency every Rust crate with tests carries: the tests use the rstest crate
// (#[rstest], #[case], #[values], #[fixture]) instead of functions that imitate it.
const RstestRequirement = "0.28.0"

// cargoAddRstest adds rstest to the dev-dependencies of the crate in dir. A seam for tests.
var cargoAddRstest = func(dir string) error {
	out, err := exec.Command("cargo", "add", "--dev", "--quiet", "--manifest-path", filepath.Join(dir, "Cargo.toml"), "rstest@"+RstestRequirement).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cargo add rstest: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureRstest makes every Rust crate under root that has tests depend on rstest 0.28 or later, and lists
// the manifests it changed (or would change) and what it could not do.
func ensureRstest(root string, dryRun bool) (changed, notes []string) {
	for _, dir := range rustCrates(root) {
		if !hasRustTests(dir) {
			continue
		}
		manifest, _ := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
		if ok, inherited := rstestDeclared(manifest); ok {
			if inherited {
				notes = append(notes, "rstest of "+rel(root, dir)+" comes from the workspace: keep [workspace.dependencies] rstest at "+RstestRequirement)
			}
			continue
		}
		name := filepath.Join(rel(root, dir), "Cargo.toml")
		if dryRun {
			changed = append(changed, name)
			continue
		}
		if err := cargoAddRstest(dir); err != nil {
			notes = append(notes, "could not add rstest to "+name+": "+err.Error()+` (add rstest = "`+RstestRequirement+`" to [dev-dependencies])`)
			continue
		}
		changed = append(changed, name)
	}
	return changed, notes
}

func rel(root, dir string) string {
	if r, err := filepath.Rel(root, dir); err == nil && r != "." {
		return r
	}
	return "."
}

// rustCrates lists the directories under root (three levels deep) whose Cargo.toml has a [package].
func rustCrates(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "target", ".git", "node_modules", ".dendrite", "dendrite-work":
				return filepath.SkipDir
			}
			if r, _ := filepath.Rel(root, path); strings.Count(r, string(filepath.Separator)) >= 3 {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "Cargo.toml" {
			return nil
		}
		var m struct {
			Package map[string]any `toml:"package"`
		}
		if data, err := os.ReadFile(path); err == nil && toml.Unmarshal(data, &m) == nil && m.Package != nil {
			out = append(out, filepath.Dir(path))
		}
		return nil
	})
	return out
}

// hasRustTests reports whether the crate in dir holds a #[test] function or a #[cfg(test)] module.
func hasRustTests(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipAll
		}
		if d.IsDir() && (d.Name() == "target" || (path != dir && fileExists(filepath.Join(path, "Cargo.toml")))) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".rs") {
			if data, err := os.ReadFile(path); err == nil {
				s := string(data)
				found = strings.Contains(s, "#[test]") || strings.Contains(s, "#[cfg(test)]")
			}
		}
		return nil
	})
	return found
}

// rstestDeclared says whether a manifest declares rstest at the required version or later; inherited is true
// when it takes the version from the workspace.
func rstestDeclared(manifest []byte) (ok, inherited bool) {
	var m struct {
		Dev map[string]any `toml:"dev-dependencies"`
	}
	if toml.Unmarshal(manifest, &m) != nil {
		return false, false
	}
	switch dep := m.Dev["rstest"].(type) {
	case nil:
		return false, false
	case string:
		return rstestRecent(dep), false
	case map[string]any:
		if w, _ := dep["workspace"].(bool); w {
			return true, true
		}
		v, has := dep["version"].(string)
		return !has || rstestRecent(v), false // a path or git dependency has no version to compare
	}
	return false, false
}

// rstestRecent reports whether a version requirement starts at 0.28 or later.
func rstestRecent(req string) bool {
	digits := strings.TrimLeftFunc(req, func(r rune) bool { return r < '0' || r > '9' })
	parts := strings.FieldsFunc(digits, func(r rune) bool { return r < '0' || r > '9' })
	if len(parts) == 0 {
		return true
	}
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major > 0 || minor >= 28
}
