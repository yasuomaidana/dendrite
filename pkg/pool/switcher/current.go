package switcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Entry is how one manifest of a project names a dependency right now. It is read before a temporary switch so
// the manifest can be put back exactly as it was.
type Entry struct {
	// Manifest is the manifest's file name: go.mod, Cargo.toml, package.json or pyproject.toml.
	Manifest string `json:"manifest"`
	// Mode is how the dependency is consumed.
	Mode DependencyMode `json:"mode"`
	// Raw is the manifest's own text for the dependency: the require line of go.mod, the `name = ...` line of
	// Cargo.toml and pyproject.toml, the value of package.json.
	Raw string `json:"raw"`
	// Replace is the replace line of go.mod, when there is one.
	Replace string `json:"replace,omitempty"`
}

var manifestReaders = []struct {
	name string
	read func(content, dep string) (Entry, bool)
}{
	{"go.mod", currentGoMod},
	{"Cargo.toml", currentTomlLine},
	{"package.json", currentPackageJSON},
	{"pyproject.toml", currentTomlLine},
}

// Current reads how the project at dir names dep, one Entry per manifest that names it. It changes nothing.
func Current(dir, dep string) ([]Entry, error) {
	var entries []Entry
	for _, m := range manifestReaders {
		data, err := os.ReadFile(filepath.Join(dir, m.name))
		if err != nil {
			continue
		}
		if e, ok := m.read(string(data), dep); ok {
			e.Manifest = m.name
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: %s in %s", ErrDependencyNotFound, dep, dir)
	}
	return entries, nil
}

func currentGoMod(content, dep string) (Entry, bool) {
	var e Entry
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "replace "+dep+" =>"):
			e.Replace = trimmed
		case strings.HasPrefix(trimmed, dep+" "), strings.HasPrefix(trimmed, "require "+dep+" "):
			e.Raw = strings.TrimPrefix(trimmed, "require ")
		}
	}
	e.Mode = ModePublished
	if e.Replace != "" {
		e.Mode = ModeLocal
		if fields := strings.Fields(e.Replace); len(fields) == 5 {
			e.Mode = ModeFork // a replacement with a version is another module, not a directory
		}
	}
	return e, e.Raw != "" || e.Replace != ""
}

// currentTomlLine reads the `dep = ...` line of a Cargo.toml or pyproject.toml.
func currentTomlLine(content, dep string) (Entry, bool) {
	line := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(dep) + `\s*=.*$`).FindString(content)
	if line == "" {
		return Entry{}, false
	}
	e := Entry{Raw: line, Mode: ModePublished}
	switch {
	case strings.Contains(line, "path ="):
		e.Mode = ModeLocal
	case strings.Contains(line, "branch ="), strings.Contains(line, "rev ="):
		e.Mode = ModeBranch
	case strings.Contains(line, "git ="):
		e.Mode = ModeFork
	}
	return e, true
}

func currentPackageJSON(content, dep string) (Entry, bool) {
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if json.Unmarshal([]byte(content), &pkg) != nil {
		return Entry{}, false
	}
	value, ok := pkg.Dependencies[dep]
	if !ok {
		return Entry{}, false
	}
	e := Entry{Raw: value, Mode: ModePublished}
	switch {
	case strings.HasPrefix(value, "link:"), strings.HasPrefix(value, "file:"):
		e.Mode = ModeLocal
	case strings.Contains(value, "#"):
		e.Mode = ModeBranch
	}
	return e, true
}

// Restore writes e back into its manifest in dir, undoing whatever a switch put there for dep.
func Restore(dir, dep string, e Entry) error {
	path := filepath.Join(dir, e.Manifest)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var content string
	switch e.Manifest {
	case "go.mod":
		content = restoreGoMod(string(data), dep, e)
	case "package.json":
		if content, err = restorePackageJSON(data, dep, e); err != nil {
			return err
		}
	default:
		re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(dep) + `\s*=.*$`)
		if !re.Match(data) {
			return fmt.Errorf("%w: %s in %s", ErrDependencyNotFound, dep, path)
		}
		content = re.ReplaceAllLiteralString(string(data), e.Raw)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func restoreGoMod(content, dep string, e Entry) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "replace "+dep+" =>"):
			continue
		case e.Raw != "" && strings.HasPrefix(trimmed, "require "+dep+" "):
			line = "require " + e.Raw
		case e.Raw != "" && strings.HasPrefix(trimmed, dep+" "):
			line = "\t" + e.Raw
		}
		lines = append(lines, line)
	}
	if e.Replace != "" {
		lines = append(lines, e.Replace)
	}
	return strings.Join(lines, "\n")
}

func restorePackageJSON(data []byte, dep string, e Entry) (string, error) {
	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", err
	}
	deps, ok := pkg["dependencies"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("%w: %s in package.json", ErrDependencyNotFound, dep)
	}
	deps[dep] = e.Raw
	out, err := json.MarshalIndent(pkg, "", "  ")
	return string(out), err
}
