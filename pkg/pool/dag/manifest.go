package dag

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ManifestScanner inspects project directories to extract names, ecosystems, and declared dependencies.
type ManifestScanner struct{}

// NewManifestScanner creates a new default manifest scanner.
func NewManifestScanner() *ManifestScanner {
	return &ManifestScanner{}
}

// ScanProject scans a directory for recognized package manifests and returns its ProjectInfo.
func (s *ManifestScanner) ScanProject(dir string) (*ProjectInfo, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving absolute path: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("project path does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project path is not a directory: %s", absDir)
	}

	projectName := filepath.Base(absDir)
	var ecosystems []Ecosystem
	var manifests []string
	depSet := make(map[string]struct{})

	// 1. Check Go: go.mod
	goModPath := filepath.Join(absDir, "go.mod")
	if exists(goModPath) {
		modName, deps, err := parseGoMod(goModPath)
		if err == nil {
			if modName != "" {
				projectName = modName
			}
			ecosystems = append(ecosystems, EcosystemGo)
			manifests = append(manifests, "go.mod")
			for _, d := range deps {
				depSet[d] = struct{}{}
			}
		}
	}

	// 2. Check Rust: Cargo.toml
	cargoPath := filepath.Join(absDir, "Cargo.toml")
	if exists(cargoPath) {
		crateName, deps, err := parseCargoToml(cargoPath)
		if err == nil {
			if crateName != "" {
				projectName = crateName
			}
			ecosystems = append(ecosystems, EcosystemRust)
			manifests = append(manifests, "Cargo.toml")
			for _, d := range deps {
				depSet[d] = struct{}{}
			}
		}
	}

	// 3. Check Node/TypeScript: package.json
	pkgJsonPath := filepath.Join(absDir, "package.json")
	if exists(pkgJsonPath) {
		pkgName, deps, err := parsePackageJSON(pkgJsonPath)
		if err == nil {
			if pkgName != "" {
				projectName = pkgName
			}
			ecosystems = append(ecosystems, EcosystemTypeScript)
			manifests = append(manifests, "package.json")
			for _, d := range deps {
				depSet[d] = struct{}{}
			}
		}
	}

	// 4. Check Python: pyproject.toml
	pyprojectPath := filepath.Join(absDir, "pyproject.toml")
	if exists(pyprojectPath) {
		pyName, deps, err := parsePyprojectToml(pyprojectPath)
		if err == nil {
			if pyName != "" {
				projectName = pyName
			}
			ecosystems = append(ecosystems, EcosystemPython)
			manifests = append(manifests, "pyproject.toml")
			for _, d := range deps {
				depSet[d] = struct{}{}
			}
		}
	}

	if len(ecosystems) == 0 {
		ecosystems = append(ecosystems, EcosystemUnknown)
	}

	var allDeps []string
	for dep := range depSet {
		allDeps = append(allDeps, dep)
	}

	return &ProjectInfo{
		Name:         projectName,
		Path:         absDir,
		Ecosystems:   ecosystems,
		Manifests:    manifests,
		Dependencies: allDeps,
	}, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// parseGoMod parses go.mod files for module name and required dependencies.
func parseGoMod(path string) (string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	var moduleName string
	var deps []string
	inRequireBlock := false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}

		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				moduleName = parts[1]
			}
			continue
		}

		if line == "require (" {
			inRequireBlock = true
			continue
		}
		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				dep := fields[0]
				deps = append(deps, dep)
				// Also include base name for local matching
				deps = append(deps, filepath.Base(dep))
			}
			continue
		}

		if strings.HasPrefix(line, "require ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				dep := fields[1]
				deps = append(deps, dep)
				deps = append(deps, filepath.Base(dep))
			}
		}
	}

	return moduleName, uniqueStrings(deps), nil
}

// parseCargoToml extracts crate name and dependencies from Cargo.toml.
func parseCargoToml(path string) (string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	var crateName string
	var deps []string
	currentSection := ""

	reName := regexp.MustCompile(`^name\s*=\s*["']([^"']+)["']`)
	reDepKey := regexp.MustCompile(`^([a-zA-Z0-9_\-]+)\s*=`)
	reSectionDep := regexp.MustCompile(`^\[dependencies\.([a-zA-Z0-9_\-]+)\]`)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if matches := reSectionDep.FindStringSubmatch(line); len(matches) > 1 {
				deps = append(deps, matches[1])
				currentSection = "dependencies"
				continue
			}
			currentSection = strings.Trim(line, "[] \t")
			continue
		}

		if currentSection == "package" {
			if matches := reName.FindStringSubmatch(line); len(matches) > 1 {
				crateName = matches[1]
			}
			continue
		}

		if strings.HasPrefix(currentSection, "dependencies") ||
			strings.HasPrefix(currentSection, "dev-dependencies") {
			if matches := reDepKey.FindStringSubmatch(line); len(matches) > 1 {
				dep := matches[1]
				if dep != "version" && dep != "path" {
					deps = append(deps, dep)
				}
			}
		}
	}

	return crateName, uniqueStrings(deps), nil
}

// parsePackageJSON extracts package name and dependencies from package.json.
func parsePackageJSON(path string) (string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	var raw struct {
		Name             string            `json:"name"`
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrManifestMalformed, err)
	}

	var deps []string
	for k := range raw.Dependencies {
		deps = append(deps, k)
	}
	for k := range raw.DevDependencies {
		deps = append(deps, k)
	}
	for k := range raw.PeerDependencies {
		deps = append(deps, k)
	}

	return raw.Name, uniqueStrings(deps), nil
}

// parsePyprojectToml extracts package name and dependencies from pyproject.toml.
func parsePyprojectToml(path string) (string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	var name string
	var deps []string
	currentSection := ""

	reName := regexp.MustCompile(`^name\s*=\s*["']([^"']+)["']`)
	reDepArray := regexp.MustCompile(`["']([a-zA-Z0-9_\-]+)[^"']*["']`)
	reDepKey := regexp.MustCompile(`^([a-zA-Z0-9_\-]+)\s*=`)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	inDepArray := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[] \t")
			inDepArray = false
			continue
		}

		if (currentSection == "project" || currentSection == "tool.poetry") && name == "" {
			if matches := reName.FindStringSubmatch(line); len(matches) > 1 {
				name = matches[1]
			}
		}

		if currentSection == "project" && strings.HasPrefix(line, "dependencies") {
			if strings.Contains(line, "[") {
				inDepArray = true
			}
		}

		if inDepArray {
			matches := reDepArray.FindAllStringSubmatch(line, -1)
			for _, m := range matches {
				if len(m) > 1 && m[1] != "dependencies" {
					deps = append(deps, m[1])
				}
			}
			if strings.Contains(line, "]") {
				inDepArray = false
			}
			continue
		}

		if currentSection == "tool.poetry.dependencies" {
			if matches := reDepKey.FindStringSubmatch(line); len(matches) > 1 {
				dep := matches[1]
				if dep != "python" {
					deps = append(deps, dep)
				}
			}
		}
	}

	return name, uniqueStrings(deps), nil
}

func uniqueStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slice {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
