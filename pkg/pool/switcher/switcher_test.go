package switcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwitcher_GoModTransitions(t *testing.T) {
	tempDir := t.TempDir()
	goModContent := `module github.com/test/myproject

go 1.24

require (
	github.com/gin-gonic/gin v1.9.1
	github.com/test/dep-lib v1.0.0
)
`
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	sw := NewDependencySwitcher()
	ctx := context.Background()

	// 1. Switch to Local mode
	localRes, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "github.com/test/dep-lib",
		Mode:             ModeLocal,
		LocalPath:        "../dep-lib-local",
		SyncIDE:          true,
	})
	if err != nil {
		t.Fatalf("switch to local failed: %v", err)
	}
	if localRes.NewMode != ModeLocal {
		t.Errorf("expected ModeLocal, got %s", localRes.NewMode)
	}

	content, _ := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	if !strings.Contains(string(content), "replace github.com/test/dep-lib => ../dep-lib-local") {
		t.Errorf("go.mod missing replace directive: %s", string(content))
	}

	// Verify JetBrains scope XML created
	scopeContent, err := os.ReadFile(filepath.Join(tempDir, ".idea", "scopes", "Dendrite_Local_Pool.xml"))
	if err != nil {
		t.Errorf("expected JetBrains scope file, got error: %v", err)
	} else if !strings.Contains(string(scopeContent), "Dendrite_Local_Pool") {
		t.Errorf("invalid scope XML: %s", string(scopeContent))
	}

	// 2. Switch to Branch mode: a branch is not a go.mod version, so it is pinned via `go get`.
	var gotGet string
	origGoGet := goGet
	goGet = func(dir, depAtRef string) error { gotGet = depAtRef; return nil }
	defer func() { goGet = origGoGet }()
	branchRes, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "github.com/test/dep-lib",
		Mode:             ModeBranch,
		BranchName:       "feat/stream-v2",
	})
	if err != nil {
		t.Fatalf("switch to branch failed: %v", err)
	}
	if branchRes.NewMode != ModeBranch {
		t.Errorf("expected ModeBranch, got %s", branchRes.NewMode)
	}
	content, _ = os.ReadFile(filepath.Join(tempDir, "go.mod"))
	if strings.Contains(string(content), "replace github.com/test/dep-lib =>") {
		t.Errorf("replace directive should have been stripped in branch mode")
	}
	if gotGet != "github.com/test/dep-lib@feat/stream-v2" {
		t.Errorf("go get = %q", gotGet)
	}

	// A version-shaped ref is written directly; a failing go get rolls back.
	if _, err := sw.Switch(ctx, SwitchRequest{TargetProjectDir: tempDir, DependencyName: "github.com/test/dep-lib", Mode: ModeBranch, BranchName: "v1.3.0"}); err != nil {
		t.Fatalf("switch to version ref failed: %v", err)
	}
	if content, _ = os.ReadFile(filepath.Join(tempDir, "go.mod")); !strings.Contains(string(content), "github.com/test/dep-lib v1.3.0") {
		t.Errorf("expected pinned require: %s", content)
	}
	goGet = func(dir, depAtRef string) error { return errors.New("offline") }
	if _, err := sw.Switch(ctx, SwitchRequest{TargetProjectDir: tempDir, DependencyName: "github.com/test/dep-lib", Mode: ModeBranch, BranchName: "main"}); err == nil {
		t.Fatal("expected go get failure to fail the switch")
	}
	if after, _ := os.ReadFile(filepath.Join(tempDir, "go.mod")); string(after) != string(content) {
		t.Errorf("go.mod not rolled back after go get failure")
	}

	// 3. Switch to Fork mode: a branch is rejected, a module version is required.
	if _, err := sw.Switch(ctx, SwitchRequest{TargetProjectDir: tempDir, DependencyName: "github.com/test/dep-lib", Mode: ModeFork, ForkURL: "github.com/myfork/dep-lib", ForkBranch: "fix-patch"}); err == nil {
		t.Fatal("expected fork with a branch name to be rejected")
	}
	forkRes, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "github.com/test/dep-lib",
		Mode:             ModeFork,
		ForkURL:          "https://github.com/myfork/dep-lib.git",
		ForkBranch:       "v0.0.0-20261018000000-abcdef123456",
	})
	if err != nil {
		t.Fatalf("switch to fork failed: %v", err)
	}
	if forkRes.NewMode != ModeFork {
		t.Errorf("expected ModeFork, got %s", forkRes.NewMode)
	}
	content, _ = os.ReadFile(filepath.Join(tempDir, "go.mod"))
	if !strings.Contains(string(content), "replace github.com/test/dep-lib => github.com/myfork/dep-lib v0.0.0-20261018000000-abcdef123456") {
		t.Errorf("expected fork replace directive: %s", string(content))
	}

	// 4. Switch back to Published mode
	pubRes, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "github.com/test/dep-lib",
		Mode:             ModePublished,
		Version:          "v1.2.0",
	})
	if err != nil {
		t.Fatalf("switch to published failed: %v", err)
	}
	if pubRes.NewMode != ModePublished {
		t.Errorf("expected ModePublished, got %s", pubRes.NewMode)
	}
	content, _ = os.ReadFile(filepath.Join(tempDir, "go.mod"))
	if strings.Contains(string(content), "replace github.com/test/dep-lib =>") {
		t.Errorf("replace directive should be stripped in published mode: %s", string(content))
	}
	if !strings.Contains(string(content), "github.com/test/dep-lib v1.2.0") {
		t.Errorf("expected required version v1.2.0: %s", string(content))
	}
}

func TestSwitcher_CargoTomlTransitions(t *testing.T) {
	tempDir := t.TempDir()
	cargoContent := `[package]
name = "rust-service"
version = "0.1.0"
edition = "2024"

[dependencies]
tokio = "1.0"
shared-core = "0.5.0"
`
	if err := os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte(cargoContent), 0644); err != nil {
		t.Fatalf("write Cargo.toml: %v", err)
	}

	sw := NewDependencySwitcher()
	ctx := context.Background()

	// Switch to Local
	_, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "shared-core",
		Mode:             ModeLocal,
		LocalPath:        "../shared-core-rs",
	})
	if err != nil {
		t.Fatalf("switch to local cargo failed: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(tempDir, "Cargo.toml"))
	if !strings.Contains(string(content), `shared-core = { path = "../shared-core-rs" }`) {
		t.Errorf("expected path dependency: %s", string(content))
	}

	// Switch to Fork
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "shared-core",
		Mode:             ModeFork,
		ForkURL:          "https://github.com/myfork/shared-core.git",
		ForkBranch:       "custom-feat",
	})
	if err != nil {
		t.Fatalf("switch to fork cargo failed: %v", err)
	}
	content, _ = os.ReadFile(filepath.Join(tempDir, "Cargo.toml"))
	if !strings.Contains(string(content), `git = "https://github.com/myfork/shared-core.git"`) {
		t.Errorf("expected git fork dependency: %s", string(content))
	}

	// Switch back to Published
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "shared-core",
		Mode:             ModePublished,
		Version:          "0.6.0",
	})
	if err != nil {
		t.Fatalf("switch to published cargo failed: %v", err)
	}
	content, _ = os.ReadFile(filepath.Join(tempDir, "Cargo.toml"))
	if !strings.Contains(string(content), `shared-core = "0.6.0"`) {
		t.Errorf("expected published dependency: %s", string(content))
	}
}

func TestSwitcher_PackageJSONTransitions(t *testing.T) {
	tempDir := t.TempDir()
	pkgContent := `{
  "name": "web-frontend",
  "version": "1.0.0",
  "dependencies": {
    "shared-ui": "^1.0.0"
  }
}`
	if err := os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(pkgContent), 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	sw := NewDependencySwitcher()
	ctx := context.Background()

	// Switch to Local (link:)
	_, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "shared-ui",
		Mode:             ModeLocal,
		LocalPath:        "../shared-ui-pkg",
	})
	if err != nil {
		t.Fatalf("switch to local ts failed: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(tempDir, "package.json"))
	if !strings.Contains(string(content), `"link:../shared-ui-pkg"`) {
		t.Errorf("expected link: dependency: %s", string(content))
	}

	// Switch to Branch
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "shared-ui",
		Mode:             ModeBranch,
		BranchName:       "develop",
	})
	if err == nil {
		t.Fatal("expected npm branch mode without a repository URL to fail")
	}
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "shared-ui",
		Mode:             ModeBranch,
		BranchName:       "develop",
		ForkURL:          "https://github.com/o/shared-ui.git",
	})
	if err != nil {
		t.Fatalf("switch to branch ts failed: %v", err)
	}
	content, _ = os.ReadFile(filepath.Join(tempDir, "package.json"))
	if !strings.Contains(string(content), `"git+https://github.com/o/shared-ui.git#develop"`) {
		t.Errorf("expected branch git url in package.json: %s", string(content))
	}
}

func TestSwitcher_PyprojectTransitions(t *testing.T) {
	tempDir := t.TempDir()
	pyContent := `[project]
name = "py-service"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.12"
common-lib = "^1.0.0"
`
	if err := os.WriteFile(filepath.Join(tempDir, "pyproject.toml"), []byte(pyContent), 0644); err != nil {
		t.Fatalf("write pyproject.toml: %v", err)
	}

	sw := NewDependencySwitcher()
	ctx := context.Background()

	// Switch to Local
	_, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "common-lib",
		Mode:             ModeLocal,
		LocalPath:        "../common-lib",
	})
	if err != nil {
		t.Fatalf("switch to local python failed: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(tempDir, "pyproject.toml"))
	if !strings.Contains(string(content), `editable = true`) {
		t.Errorf("expected editable local python dependency: %s", string(content))
	}
}

func TestSwitcher_ValidationAndRollback(t *testing.T) {
	tempDir := t.TempDir()
	sw := NewDependencySwitcher()
	ctx := context.Background()

	// Missing dependency name
	_, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		Mode:             ModePublished,
		Version:          "1.0.0",
	})
	if !errors.Is(err, ErrMissingParameter) {
		t.Errorf("expected ErrMissingParameter, got %v", err)
	}

	// Invalid mode
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "foo",
		Mode:             "unsupported_mode",
	})
	if !errors.Is(err, ErrInvalidMode) {
		t.Errorf("expected ErrInvalidMode, got %v", err)
	}

	// Manifest not found
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "foo",
		Mode:             ModePublished,
		Version:          "1.0.0",
	})
	if !errors.Is(err, ErrManifestNotFound) {
		t.Errorf("expected ErrManifestNotFound, got %v", err)
	}

	// Transaction rollback test
	badJSONPath := filepath.Join(tempDir, "package.json")
	originalBadJSON := []byte(`{ invalid-json }`)
	_ = os.WriteFile(badJSONPath, originalBadJSON, 0644)

	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "foo",
		Mode:             ModePublished,
		Version:          "1.0.0",
	})
	if err == nil {
		t.Fatal("expected error on malformed package.json, got nil")
	}

	// Ensure original file was restored
	restored, _ := os.ReadFile(badJSONPath)
	if string(restored) != string(originalBadJSON) {
		t.Errorf("file was not properly rolled back on failure: %s", string(restored))
	}
}

func TestSwitcher_PyprojectAllModes(t *testing.T) {
	tempDir := t.TempDir()
	pyContent := `[project]
name = "py-app"
version = "0.1.0"
dependencies = []

[tool.poetry.dependencies]
python = "^3.12"
`
	_ = os.WriteFile(filepath.Join(tempDir, "pyproject.toml"), []byte(pyContent), 0644)
	sw := NewDependencySwitcher()
	ctx := context.Background()

	// 1. Published
	_, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "numpy",
		Mode:             ModePublished,
		Version:          "1.26.0",
	})
	if err != nil {
		t.Fatalf("pyproject published failed: %v", err)
	}

	// 2. Branch
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "numpy",
		Mode:             ModeBranch,
		BranchName:       "main",
		ForkURL:          "https://github.com/numpy/numpy.git",
	})
	if err != nil {
		t.Fatalf("pyproject branch failed: %v", err)
	}

	// 3. Fork
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "numpy",
		Mode:             ModeFork,
		ForkURL:          "https://github.com/myfork/numpy.git",
		ForkBranch:       "custom",
	})
	if err != nil {
		t.Fatalf("pyproject fork failed: %v", err)
	}
}

func TestSwitcher_PackageJSONForkAndPublished(t *testing.T) {
	tempDir := t.TempDir()
	pkgContent := `{"name": "test-pkg", "dependencies": {"my-dep": "^1.0.0"}}`
	_ = os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(pkgContent), 0644)
	sw := NewDependencySwitcher()
	ctx := context.Background()

	// Fork
	_, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "my-dep",
		Mode:             ModeFork,
		ForkURL:          "https://github.com/fork/my-dep.git",
		ForkBranch:       "patch-1",
	})
	if err != nil {
		t.Fatalf("package.json fork failed: %v", err)
	}

	// Published
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "my-dep",
		Mode:             ModePublished,
		Version:          "2.0.0",
	})
	if err != nil {
		t.Fatalf("package.json published failed: %v", err)
	}
}
