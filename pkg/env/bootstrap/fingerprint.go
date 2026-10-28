package bootstrap

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
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

	// 1. Inspect Go (go.mod)
	goModPath := filepath.Join(absPath, "go.mod")
	if fileExists(goModPath) {
		fp.HasGoMod = true
		fp.PrimaryStack = StackGo
		fp.Toolchains = append(fp.Toolchains, "go")
		fp.TestFrameworks = append(fp.TestFrameworks, "go test", "testify")
		if fileExists(filepath.Join(absPath, ".golangci.yml")) || fileExists(filepath.Join(absPath, ".golangci.yaml")) {
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
		if data, err := os.ReadFile(goModPath); err == nil {
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
	}

	// 2. Inspect Rust & Cargo (Cargo.toml)
	cargoTomlPath := filepath.Join(absPath, "Cargo.toml")
	if fileExists(cargoTomlPath) {
		fp.HasCargoToml = true
		fp.HasCargo = true
		fp.Toolchains = append(fp.Toolchains, "cargo")
		if fp.PrimaryStack == StackGeneric {
			fp.PrimaryStack = StackRust
		} else {
			fp.SecondaryStacks = append(fp.SecondaryStacks, StackRust)
		}
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

	// 3. Inspect TypeScript / JavaScript & Bun (package.json, website/package.json)
	packageJSONPath := filepath.Join(absPath, "package.json")
	websitePkgPath := filepath.Join(absPath, "website", "package.json")
	if fileExists(packageJSONPath) || fileExists(websitePkgPath) {
		fp.HasPackageJSON = true
		fp.HasBun = true
		fp.Toolchains = append(fp.Toolchains, "bun")

		isTS := fileExists(filepath.Join(absPath, "tsconfig.json")) || fileExists(filepath.Join(absPath, "website", "tsconfig.json"))
		targetStack := StackJavaScript
		if isTS {
			targetStack = StackTypeScript
		}
		if fp.PrimaryStack == StackGeneric {
			fp.PrimaryStack = targetStack
		} else {
			fp.SecondaryStacks = append(fp.SecondaryStacks, targetStack)
		}
		fp.TestFrameworks = append(fp.TestFrameworks, "bun test", "vitest")
		fp.Linters = append(fp.Linters, "eslint", "prettier")

		manifestRel := "package.json"
		if !fileExists(packageJSONPath) && fileExists(websitePkgPath) {
			manifestRel = filepath.Join("website", "package.json")
		}

		bunLock := "bun.lockb"
		if !fileExists(filepath.Join(absPath, bunLock)) && !fileExists(filepath.Join(absPath, "website", bunLock)) {
			if fileExists(filepath.Join(absPath, "bun.lock")) || fileExists(filepath.Join(absPath, "website", "bun.lock")) {
				bunLock = "bun.lock"
			} else {
				bunLock = ""
			}
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

	// 4. Inspect Python & uv (pyproject.toml, requirements.txt, scripts/pyproject.toml)
	pyprojectPath := filepath.Join(absPath, "pyproject.toml")
	scriptsPyproject := filepath.Join(absPath, "scripts", "pyproject.toml")
	reqTxtPath := filepath.Join(absPath, "requirements.txt")
	if fileExists(pyprojectPath) || fileExists(scriptsPyproject) || fileExists(reqTxtPath) {
		fp.HasPyProject = true
		fp.HasUV = true
		fp.Toolchains = append(fp.Toolchains, "uv")

		if fp.PrimaryStack == StackGeneric {
			fp.PrimaryStack = StackPython
		} else {
			fp.SecondaryStacks = append(fp.SecondaryStacks, StackPython)
		}
		fp.TestFrameworks = append(fp.TestFrameworks, "uv run pytest", "pytest")
		fp.Linters = append(fp.Linters, "ruff", "flake8")

		manifestRel := "pyproject.toml"
		if !fileExists(pyprojectPath) && fileExists(scriptsPyproject) {
			manifestRel = filepath.Join("scripts", "pyproject.toml")
		} else if !fileExists(pyprojectPath) && fileExists(reqTxtPath) {
			manifestRel = "requirements.txt"
		}

		uvLock := "uv.lock"
		if !fileExists(filepath.Join(absPath, uvLock)) && !fileExists(filepath.Join(absPath, "scripts", uvLock)) {
			uvLock = ""
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

	return fp, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
