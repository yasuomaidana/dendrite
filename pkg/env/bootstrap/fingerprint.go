package bootstrap

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/env/pipeline"
)

// DetectStack inspects the target directory manifests to infer primary and secondary stacks and canonical toolchains.
func DetectStack(projectPath string) (*ProjectFingerprint, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, err
	}

	projectName := filepath.Base(absPath)
	fp := &ProjectFingerprint{
		RootPath:         absPath,
		Name:             projectName,
		PrimaryStack:     StackGeneric,
		SecondaryStacks:  []LanguageStack{},
		Toolchains:       []string{},
		ToolchainDetails: make(map[string]ToolchainInfo),
		Linters:          []string{},
		TestFrameworks:   []string{},
	}

	// Bun and uv count only for a component with something to verify (a lockfile and scripts, tests); a bare
	// manifest is reported by pipeline.Detect as a note and is not a stack.
	comps, _ := pipeline.Detect(absPath)

	// 1. Inspect Go (go.mod)
	detectGoStack(absPath, fp)

	// 2. Inspect Rust & Cargo (Cargo.toml)
	detectCargoStack(absPath, fp)

	// 3. Inspect TypeScript / JavaScript & Bun (package.json, website/package.json)
	detectBunStack(absPath, fp, comps)

	// 4. Inspect Python & uv (pyproject.toml, requirements.txt, scripts/pyproject.toml)
	detectUVStack(absPath, fp, comps)

	return fp, nil
}

func detectGoStack(absPath string, fp *ProjectFingerprint) {
	goModPath := filepath.Join(absPath, "go.mod")
	if !fileExists(goModPath) {
		return
	}

	fp.HasGoMod = true
	fp.PrimaryStack = StackGo
	fp.Toolchains = append(fp.Toolchains, "go")
	fp.TestFrameworks = append(fp.TestFrameworks, "go test", "testify")
	if anyFileExists(filepath.Join(absPath, ".golangci.yml"), filepath.Join(absPath, ".golangci.yaml")) {
		fp.Linters = append(fp.Linters, "golangci-lint")
	}

	goLock := "go.sum"
	if !fileExists(filepath.Join(absPath, goLock)) {
		goLock = ""
	}

	fp.ToolchainDetails["go"] = ToolchainInfo{
		Name:           "go",
		Binary:         "go",
		ManifestFile:   "go.mod",
		LockFile:       goLock,
		TestCommand:    "go test -v -race -cover ./pkg/...",
		BuildCommand:   "go build -v ./...",
		InstallCommand: "curl -fsSL https://go.dev/dl/ || brew install go",
	}

	// Extract Go version & Module name
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "go ") {
			fp.GoVersion = strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
		if strings.HasPrefix(line, "module ") {
			modName := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if modName != "" {
				fp.Name = filepath.Base(modName)
			}
		}
	}
}

func detectCargoStack(absPath string, fp *ProjectFingerprint) {
	cargoTomlPath := filepath.Join(absPath, "Cargo.toml")
	if !fileExists(cargoTomlPath) {
		return
	}

	fp.HasCargoToml = true
	fp.HasCargo = true
	fp.Toolchains = append(fp.Toolchains, "cargo")
	appendStack(fp, StackRust)
	fp.TestFrameworks = append(fp.TestFrameworks, "cargo test")
	fp.Linters = append(fp.Linters, "clippy")

	cargoLock := "Cargo.lock"
	if !fileExists(filepath.Join(absPath, cargoLock)) {
		cargoLock = ""
	}

	fp.ToolchainDetails["cargo"] = ToolchainInfo{
		Name:           "cargo",
		Binary:         "cargo",
		ManifestFile:   "Cargo.toml",
		LockFile:       cargoLock,
		TestCommand:    "cargo test --verbose",
		BuildCommand:   "cargo build --release",
		InstallCommand: "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh",
	}
}

func detectBunStack(absPath string, fp *ProjectFingerprint, comps []pipeline.Component) {
	if !hasComponent(comps, pipeline.Bun) {
		return
	}

	fp.HasPackageJSON = true
	fp.HasBun = true
	fp.Toolchains = append(fp.Toolchains, "bun")

	targetStack := StackJavaScript
	if anyFileExists(filepath.Join(absPath, "tsconfig.json"), filepath.Join(absPath, "website", "tsconfig.json")) {
		targetStack = StackTypeScript
	}
	appendStack(fp, targetStack)
	fp.TestFrameworks = append(fp.TestFrameworks, "bun test", "vitest")
	fp.Linters = append(fp.Linters, "eslint", "prettier")

	manifestRel := "package.json"
	packageJSONPath := filepath.Join(absPath, "package.json")
	websitePkgPath := filepath.Join(absPath, "website", "package.json")
	if !fileExists(packageJSONPath) && fileExists(websitePkgPath) {
		manifestRel = filepath.Join("website", "package.json")
	}

	bunLock := ""
	if anyFileExists(filepath.Join(absPath, "bun.lockb"), filepath.Join(absPath, "website", "bun.lockb")) {
		bunLock = "bun.lockb"
	} else if anyFileExists(filepath.Join(absPath, "bun.lock"), filepath.Join(absPath, "website", "bun.lock")) {
		bunLock = "bun.lock"
	}

	fp.ToolchainDetails["bun"] = ToolchainInfo{
		Name:           "bun",
		Binary:         "bun",
		ManifestFile:   manifestRel,
		LockFile:       bunLock,
		TestCommand:    "bun test",
		BuildCommand:   "bun run build",
		InstallCommand: "curl -fsSL https://bun.sh/install | bash",
	}
}

func detectUVStack(absPath string, fp *ProjectFingerprint, comps []pipeline.Component) {
	if !hasComponent(comps, pipeline.UV) {
		return
	}

	fp.HasPyProject = true
	fp.HasUV = true
	fp.Toolchains = append(fp.Toolchains, "uv")
	appendStack(fp, StackPython)
	fp.TestFrameworks = append(fp.TestFrameworks, "uv run pytest", "pytest")
	fp.Linters = append(fp.Linters, "ruff", "flake8")

	manifestRel := "pyproject.toml"
	pyprojectPath := filepath.Join(absPath, "pyproject.toml")
	scriptsPyproject := filepath.Join(absPath, "scripts", "pyproject.toml")
	reqTxtPath := filepath.Join(absPath, "requirements.txt")
	if !fileExists(pyprojectPath) && fileExists(scriptsPyproject) {
		manifestRel = filepath.Join("scripts", "pyproject.toml")
	} else if !fileExists(pyprojectPath) && fileExists(reqTxtPath) {
		manifestRel = "requirements.txt"
	}

	uvLock := ""
	if anyFileExists(filepath.Join(absPath, "uv.lock"), filepath.Join(absPath, "scripts", "uv.lock")) {
		uvLock = "uv.lock"
	}

	fp.ToolchainDetails["uv"] = ToolchainInfo{
		Name:           "uv",
		Binary:         "uv",
		ManifestFile:   manifestRel,
		LockFile:       uvLock,
		TestCommand:    "uv run pytest",
		BuildCommand:   "uv build",
		InstallCommand: "curl -LsSf https://astral.sh/uv/install.sh | sh",
	}
}

func appendStack(fp *ProjectFingerprint, stack LanguageStack) {
	if fp.PrimaryStack == StackGeneric {
		fp.PrimaryStack = stack
	} else {
		fp.SecondaryStacks = append(fp.SecondaryStacks, stack)
	}
}

func anyFileExists(paths ...string) bool {
	for _, p := range paths {
		if fileExists(p) {
			return true
		}
	}
	return false
}

func hasComponent(comps []pipeline.Component, k pipeline.Kind) bool {
	for _, c := range comps {
		if c.Kind == k {
			return true
		}
	}
	return false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
