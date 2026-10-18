package switcher

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// goModuleVersion matches a Go module version usable in go.mod (semver or pseudo-version).
var goModuleVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+incompatible)?$`)

// goGet resolves dep@ref (a branch, tag or commit) into a go.mod requirement. A branch
// name is not a valid go.mod version, so only the go command can pin it (to a
// pseudo-version). It is a variable so tests can run without network access.
var goGet = func(dir, depAtRef string) error {
	cmd := exec.Command("go", "get", depAtRef)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go get %s: %w: %s", depAtRef, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// goModulePath turns a fork URL (https://github.com/o/r.git) into a module path (github.com/o/r).
func goModulePath(url string) string {
	for _, p := range []string{"https://", "http://", "ssh://", "git@"} {
		url = strings.TrimPrefix(url, p)
	}
	url = strings.Replace(url, ":", "/", 1)
	return strings.TrimSuffix(url, ".git")
}

// DependencySwitcher executes transactional mode transitions and updates IDE scopes.
type DependencySwitcher struct{}

// NewDependencySwitcher creates a new DependencySwitcher instance.
func NewDependencySwitcher() *DependencySwitcher {
	return &DependencySwitcher{}
}

// Switch coordinates the transactional mode transition across manifests and IDE scopes.
func (s *DependencySwitcher) Switch(ctx context.Context, req SwitchRequest) (*SwitchResult, error) {
	// 1. Validate typestate invariants
	state, err := validateAndBuildState(req)
	if err != nil {
		return nil, err
	}

	absProjectDir, err := filepath.Abs(req.TargetProjectDir)
	if err != nil {
		return nil, fmt.Errorf("resolving project directory: %w", err)
	}

	// 2. Discover manifest files
	goMod := filepath.Join(absProjectDir, "go.mod")
	cargoToml := filepath.Join(absProjectDir, "Cargo.toml")
	packageJSON := filepath.Join(absProjectDir, "package.json")
	pyprojectToml := filepath.Join(absProjectDir, "pyproject.toml")

	hasGo := fileExists(goMod)
	hasRust := fileExists(cargoToml)
	hasTS := fileExists(packageJSON)
	hasPython := fileExists(pyprojectToml)

	if !hasGo && !hasRust && !hasTS && !hasPython {
		return nil, ErrManifestNotFound
	}

	tx := NewTransaction()
	var modifiedFiles []string
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	prevMode := ModePublished

	// 3. Mutate Go manifests
	if hasGo {
		if err := tx.Snapshot(goMod); err != nil {
			return nil, err
		}
		pm, err := mutateGoMod(goMod, req)
		if err != nil {
			return nil, fmt.Errorf("%w: mutating go.mod: %v", ErrTransactionalRollback, err)
		}
		prevMode = pm
		modifiedFiles = append(modifiedFiles, goMod)

		// Optionally update or create go.work if in local mode
		if req.Mode == ModeLocal && req.LocalPath != "" {
			goWork := filepath.Join(absProjectDir, "go.work")
			if err := tx.Snapshot(goWork); err == nil {
				if err := updateGoWork(goWork, absProjectDir, req.LocalPath); err == nil {
					modifiedFiles = append(modifiedFiles, goWork)
				}
			}
		}
	}

	// 4. Mutate Rust manifests
	if hasRust {
		if err := tx.Snapshot(cargoToml); err != nil {
			return nil, err
		}
		pm, err := mutateCargoToml(cargoToml, req)
		if err != nil {
			return nil, fmt.Errorf("%w: mutating Cargo.toml: %v", ErrTransactionalRollback, err)
		}
		prevMode = pm
		modifiedFiles = append(modifiedFiles, cargoToml)
	}

	// 5. Mutate TS/Node manifests
	if hasTS {
		if err := tx.Snapshot(packageJSON); err != nil {
			return nil, err
		}
		pm, err := mutatePackageJSON(packageJSON, req)
		if err != nil {
			return nil, fmt.Errorf("%w: mutating package.json: %v", ErrTransactionalRollback, err)
		}
		prevMode = pm
		modifiedFiles = append(modifiedFiles, packageJSON)
	}

	// 6. Mutate Python manifests
	if hasPython {
		if err := tx.Snapshot(pyprojectToml); err != nil {
			return nil, err
		}
		pm, err := mutatePyprojectToml(pyprojectToml, req)
		if err != nil {
			return nil, fmt.Errorf("%w: mutating pyproject.toml: %v", ErrTransactionalRollback, err)
		}
		prevMode = pm
		modifiedFiles = append(modifiedFiles, pyprojectToml)
	}

	// 7. Synchronize JetBrains IDE Scopes
	ideUpdated := false
	if req.SyncIDE || req.Mode == ModeLocal {
		scopePath := filepath.Join(absProjectDir, ".idea", "scopes", "Dendrite_Local_Pool.xml")
		_ = tx.Snapshot(scopePath)
		localRef := req.LocalPath
		if localRef == "" {
			localRef = absProjectDir
		}
		if err := syncJetBrainsScope(scopePath, req.DependencyName, localRef); err == nil {
			ideUpdated = true
			modifiedFiles = append(modifiedFiles, scopePath)
		}
	}

	// All mutations succeeded; commit transaction
	rollback = false
	tx.Commit()

	return &SwitchResult{
		DependencyName:   req.DependencyName,
		TargetProject:    absProjectDir,
		PreviousMode:     prevMode,
		NewMode:          state.Mode(),
		ModifiedFiles:    modifiedFiles,
		IDEScopesUpdated: ideUpdated,
		Timestamp:        time.Now().UTC(),
	}, nil
}

func validateAndBuildState(req SwitchRequest) (DependencyState, error) {
	if req.DependencyName == "" {
		return nil, fmt.Errorf("%w: dependency_name is required", ErrMissingParameter)
	}
	var state DependencyState
	switch req.Mode {
	case ModePublished:
		state = PublishedState{Version: req.Version}
	case ModeBranch:
		state = BranchState{BranchName: req.BranchName, CommitSHA: req.CommitSHA}
	case ModeLocal:
		state = LocalState{LocalPath: req.LocalPath}
	case ModeFork:
		state = ForkState{ForkURL: req.ForkURL, ForkBranch: req.ForkBranch}
	default:
		return nil, fmt.Errorf("%w: '%s'", ErrInvalidMode, req.Mode)
	}

	if err := state.Validate(); err != nil {
		return nil, err
	}
	return state, nil
}

func mutateGoMod(path string, req SwitchRequest) (DependencyMode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	var newLines []string
	prevMode := ModePublished

	replacePrefix := fmt.Sprintf("replace %s =>", req.DependencyName)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, replacePrefix) {
			if strings.Contains(trimmed, "=>") && strings.Contains(trimmed, "/") {
				if strings.Contains(trimmed, "git") {
					prevMode = ModeFork
				} else {
					prevMode = ModeLocal
				}
			}
			continue // Strip existing replace directive
		}
		newLines = append(newLines, line)
	}

	// Inject new mode directive
	var branchRef string
	switch req.Mode {
	case ModePublished:
		// Pin required version if version provided
		if req.Version != "" {
			newLines = setGoRequire(newLines, req.DependencyName, req.Version)
		}
	case ModeBranch:
		branchRef = req.BranchName
		if req.CommitSHA != "" {
			branchRef = req.CommitSHA
		}
		if goModuleVersion.MatchString(branchRef) {
			newLines = setGoRequire(newLines, req.DependencyName, branchRef)
			branchRef = ""
		}
	case ModeLocal:
		newLines = append(newLines, fmt.Sprintf("replace %s => %s", req.DependencyName, req.LocalPath))
	case ModeFork:
		// A replacement module needs a version; a branch must first be resolved to a
		// pseudo-version (e.g. with `go list -m <fork>@<branch>`).
		if !goModuleVersion.MatchString(req.ForkBranch) {
			return "", fmt.Errorf("go fork replace needs a module version (vX.Y.Z or pseudo-version), got %q", req.ForkBranch)
		}
		newLines = append(newLines, fmt.Sprintf("replace %s => %s %s", req.DependencyName, goModulePath(req.ForkURL), req.ForkBranch))
	}

	if err := os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return "", err
	}
	if branchRef != "" {
		if err := goGet(filepath.Dir(path), req.DependencyName+"@"+branchRef); err != nil {
			return "", err
		}
	}
	return prevMode, nil
}

func setGoRequire(lines []string, dep, version string) []string {
	var result []string
	found := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, dep+" ") {
			result = append(result, fmt.Sprintf("\t%s %s", dep, version))
			found = true
			continue
		}
		result = append(result, l)
	}
	if !found {
		result = append(result, fmt.Sprintf("require %s %s", dep, version))
	}
	return result
}

func updateGoWork(goWorkPath, projectDir, localPath string) error {
	var lines []string
	if fileExists(goWorkPath) {
		data, _ := os.ReadFile(goWorkPath)
		lines = strings.Split(string(data), "\n")
	} else {
		goVersion := goDirective(filepath.Join(projectDir, "go.mod"))
		if goVersion == "" {
			goVersion = "1.24"
		}
		lines = []string{"go " + goVersion, "", "use ("}
	}

	useEntry := fmt.Sprintf("\t%s", localPath)
	found := false
	for _, l := range lines {
		if strings.Contains(l, localPath) {
			found = true
			break
		}
	}
	if !found {
		if len(lines) >= 3 && lines[2] == "use (" {
			lines = append(lines[:len(lines)-1], useEntry, ")")
		} else {
			lines = append(lines, fmt.Sprintf("use %s", localPath))
		}
	}
	return os.WriteFile(goWorkPath, []byte(strings.Join(lines, "\n")), 0644)
}

func mutateCargoToml(path string, req SwitchRequest) (DependencyMode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	reDep := regexp.MustCompile(fmt.Sprintf(`(?m)^%s\s*=.*$`, regexp.QuoteMeta(req.DependencyName)))
	prevMode := ModePublished
	if loc := reDep.FindString(string(data)); loc != "" {
		if strings.Contains(loc, "path =") {
			prevMode = ModeLocal
		} else if strings.Contains(loc, "branch =") || strings.Contains(loc, "rev =") {
			prevMode = ModeBranch
		} else if strings.Contains(loc, "git =") {
			prevMode = ModeFork
		}
	}

	var depReplacement string
	switch req.Mode {
	case ModePublished:
		depReplacement = fmt.Sprintf(`%s = "%s"`, req.DependencyName, req.Version)
	case ModeBranch:
		// Cargo only accepts `branch` on a git dependency.
		if req.ForkURL == "" {
			return "", errors.New("cargo branch mode needs the git repository URL (ForkURL)")
		}
		depReplacement = fmt.Sprintf(`%s = { git = "%s", branch = "%s" }`, req.DependencyName, req.ForkURL, req.BranchName)
	case ModeLocal:
		depReplacement = fmt.Sprintf(`%s = { path = "%s" }`, req.DependencyName, req.LocalPath)
	case ModeFork:
		depReplacement = fmt.Sprintf(`%s = { git = "%s", branch = "%s" }`, req.DependencyName, req.ForkURL, req.ForkBranch)
	}

	content := string(data)
	if reDep.MatchString(content) {
		content = reDep.ReplaceAllString(content, depReplacement)
	} else {
		// Append under [dependencies]
		if strings.Contains(content, "[dependencies]") {
			content = strings.Replace(content, "[dependencies]", "[dependencies]\n"+depReplacement, 1)
		} else {
			content += "\n[dependencies]\n" + depReplacement + "\n"
		}
	}

	return prevMode, os.WriteFile(path, []byte(content), 0644)
}

func mutatePackageJSON(path string, req SwitchRequest) (DependencyMode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var pkg map[string]interface{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", err
	}

	deps, ok := pkg["dependencies"].(map[string]interface{})
	if !ok {
		deps = make(map[string]interface{})
		pkg["dependencies"] = deps
	}

	prevMode := ModePublished
	if currentVal, exists := deps[req.DependencyName].(string); exists {
		if strings.HasPrefix(currentVal, "link:") || strings.HasPrefix(currentVal, "file:") {
			prevMode = ModeLocal
		} else if strings.Contains(currentVal, "#") {
			prevMode = ModeBranch
		}
	}

	switch req.Mode {
	case ModePublished:
		deps[req.DependencyName] = req.Version
	case ModeBranch:
		target := req.BranchName
		if req.CommitSHA != "" {
			target = req.CommitSHA
		}
		if req.ForkURL == "" {
			return "", errors.New("npm branch mode needs the git repository URL (ForkURL)")
		}
		deps[req.DependencyName] = fmt.Sprintf("git+%s#%s", strings.TrimPrefix(req.ForkURL, "git+"), target)
	case ModeLocal:
		deps[req.DependencyName] = fmt.Sprintf("link:%s", req.LocalPath)
	case ModeFork:
		deps[req.DependencyName] = fmt.Sprintf("git+%s#%s", req.ForkURL, req.ForkBranch)
	}

	out, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return "", err
	}
	return prevMode, os.WriteFile(path, out, 0644)
}

func mutatePyprojectToml(path string, req SwitchRequest) (DependencyMode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content := string(data)
	prevMode := ModePublished
	reDep := regexp.MustCompile(fmt.Sprintf(`(?m)^%s\s*=.*$`, regexp.QuoteMeta(req.DependencyName)))

	if loc := reDep.FindString(content); loc != "" {
		if strings.Contains(loc, "editable = true") || strings.Contains(loc, "path =") {
			prevMode = ModeLocal
		} else if strings.Contains(loc, "branch =") {
			prevMode = ModeBranch
		} else if strings.Contains(loc, "git =") {
			prevMode = ModeFork
		}
	}

	var depReplacement string
	switch req.Mode {
	case ModePublished:
		depReplacement = fmt.Sprintf(`%s = "==%s"`, req.DependencyName, req.Version)
	case ModeBranch:
		depReplacement = fmt.Sprintf(`%s = { git = "%s", branch = "%s" }`, req.DependencyName, req.ForkURL, req.BranchName)
	case ModeLocal:
		depReplacement = fmt.Sprintf(`%s = { path = "%s", editable = true }`, req.DependencyName, req.LocalPath)
	case ModeFork:
		depReplacement = fmt.Sprintf(`%s = { git = "%s", branch = "%s" }`, req.DependencyName, req.ForkURL, req.ForkBranch)
	}

	if reDep.MatchString(content) {
		content = reDep.ReplaceAllString(content, depReplacement)
	} else {
		if strings.Contains(content, "[tool.poetry.dependencies]") {
			content = strings.Replace(content, "[tool.poetry.dependencies]", "[tool.poetry.dependencies]\n"+depReplacement, 1)
		} else {
			content += "\n[tool.poetry.dependencies]\n" + depReplacement + "\n"
		}
	}

	return prevMode, os.WriteFile(path, []byte(content), 0644)
}

func syncJetBrainsScope(scopeFile, depName, localPath string) error {
	dir := filepath.Dir(scopeFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	scopeXML := fmt.Sprintf(`<component name="DependencyValidationManager">
  <scope name="Dendrite_Local_Pool" pattern="file[%s]:*//*||file[%s]:*//*" />
</component>
`, depName, localPath)

	return os.WriteFile(scopeFile, []byte(scopeXML), 0644)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// goDirective returns the version on the `go` line of a go.mod, or "".
func goDirective(goMod string) string {
	f, err := os.Open(goMod)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	return ""
}
