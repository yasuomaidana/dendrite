package bootstrap

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DetectStack inspects the target directory manifests to infer primary and secondary stacks.
func DetectStack(projectPath string) (*ProjectFingerprint, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, err
	}

	projectName := filepath.Base(absPath)
	fp := &ProjectFingerprint{
		RootPath:        absPath,
		Name:            projectName,
		PrimaryStack:    StackGeneric,
		SecondaryStacks: []LanguageStack{},
		Linters:         []string{},
		TestFrameworks:  []string{},
	}

	// 1. Inspect Go (go.mod)
	goModPath := filepath.Join(absPath, "go.mod")
	if fileExists(goModPath) {
		fp.HasGoMod = true
		fp.PrimaryStack = StackGo
		fp.TestFrameworks = append(fp.TestFrameworks, "go test", "testify")
		if fileExists(filepath.Join(absPath, ".golangci.yml")) || fileExists(filepath.Join(absPath, ".golangci.yaml")) {
			fp.Linters = append(fp.Linters, "golangci-lint")
		}

		// Extract Go version
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

	// 2. Inspect Rust (Cargo.toml)
	cargoTomlPath := filepath.Join(absPath, "Cargo.toml")
	if fileExists(cargoTomlPath) {
		fp.HasCargoToml = true
		if fp.PrimaryStack == StackGeneric {
			fp.PrimaryStack = StackRust
		} else {
			fp.SecondaryStacks = append(fp.SecondaryStacks, StackRust)
		}
		fp.TestFrameworks = append(fp.TestFrameworks, "cargo test")
		fp.Linters = append(fp.Linters, "clippy")
	}

	// 3. Inspect TypeScript / JavaScript (package.json)
	packageJSONPath := filepath.Join(absPath, "package.json")
	websitePkgPath := filepath.Join(absPath, "website", "package.json")
	if fileExists(packageJSONPath) || fileExists(websitePkgPath) {
		fp.HasPackageJSON = true
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
		fp.TestFrameworks = append(fp.TestFrameworks, "jest", "vitest")
		fp.Linters = append(fp.Linters, "eslint", "prettier")
	}

	// 4. Inspect Python (pyproject.toml, requirements.txt)
	pyprojectPath := filepath.Join(absPath, "pyproject.toml")
	reqTxtPath := filepath.Join(absPath, "requirements.txt")
	if fileExists(pyprojectPath) || fileExists(reqTxtPath) {
		fp.HasPyProject = true
		if fp.PrimaryStack == StackGeneric {
			fp.PrimaryStack = StackPython
		} else {
			fp.SecondaryStacks = append(fp.SecondaryStacks, StackPython)
		}
		fp.TestFrameworks = append(fp.TestFrameworks, "pytest")
		fp.Linters = append(fp.Linters, "ruff", "flake8")
	}

	return fp, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
