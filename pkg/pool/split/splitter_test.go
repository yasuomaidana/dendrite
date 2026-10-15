package split

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/ui/modal"
)

func TestProjectSplitter_InteractiveGateSuccess(t *testing.T) {
	tempDir := t.TempDir()
	sourceProj := filepath.Join(tempDir, "parent-go")
	moduleDir := filepath.Join(sourceProj, "internal", "matcher")
	_ = os.MkdirAll(moduleDir, 0755)

	// Parent go.mod
	_ = os.WriteFile(filepath.Join(sourceProj, "go.mod"), []byte("module parent-go\ngo 1.24\n"), 0644)

	// Module files and tests
	_ = os.WriteFile(filepath.Join(moduleDir, "matcher.go"), []byte("package matcher\nfunc Match() bool { return true }\n"), 0644)
	_ = os.WriteFile(filepath.Join(moduleDir, "matcher_test.go"), []byte("package matcher\nimport \"testing\"\nfunc TestMatch(t *testing.T) {}\n"), 0644)

	targetDir := filepath.Join(tempDir, "matcher-core")

	splitter := NewProjectSplitter()
	ctx := context.Background()

	promptCalled := false
	req := SplitRequest{
		SourceProjectDir: sourceProj,
		ModuleSubpath:    filepath.Join("internal", "matcher"),
		NewMemberName:    "matcher-core",
		WireLocalMode:    true,
		Prompt: func(m *modal.DirectoryPickerModal) (string, error) {
			promptCalled = true
			if m.MemberName != "matcher-core" {
				t.Errorf("expected member matcher-core, got %s", m.MemberName)
			}
			return targetDir, nil
		},
	}

	res, err := splitter.Split(ctx, req)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if !promptCalled {
		t.Fatal("expected interactive prompt to be called")
	}

	if res.TargetDirectory != targetDir {
		t.Errorf("expected target %s, got %s", targetDir, res.TargetDirectory)
	}

	// Verify files exist in target
	if !fileExists(filepath.Join(targetDir, "matcher.go")) {
		t.Error("matcher.go was not copied to target directory")
	}
	if !fileExists(filepath.Join(targetDir, "matcher_test.go")) {
		t.Error("matcher_test.go was not copied to target directory")
	}
	if !fileExists(filepath.Join(targetDir, "go.mod")) {
		t.Error("go.mod was not scaffolded in target directory")
	}

	// Verify parent go.mod has replace directive
	parentGoMod, _ := os.ReadFile(filepath.Join(sourceProj, "go.mod"))
	if !strings.Contains(string(parentGoMod), "replace matcher-core =>") {
		t.Errorf("parent was not wired to local mode: %s", string(parentGoMod))
	}
}

func TestProjectSplitter_PromptCancellationRollback(t *testing.T) {
	tempDir := t.TempDir()
	sourceProj := filepath.Join(tempDir, "source")
	moduleDir := filepath.Join(sourceProj, "pkg", "sub")
	_ = os.MkdirAll(moduleDir, 0755)
	_ = os.WriteFile(filepath.Join(moduleDir, "lib.go"), []byte("package sub\n"), 0644)

	splitter := NewProjectSplitter()
	ctx := context.Background()

	req := SplitRequest{
		SourceProjectDir: sourceProj,
		ModuleSubpath:    filepath.Join("pkg", "sub"),
		NewMemberName:    "sub-lib",
		Prompt: func(m *modal.DirectoryPickerModal) (string, error) {
			return "", modal.ErrPromptCancelled
		},
	}

	_, err := splitter.Split(ctx, req)
	if err == nil {
		t.Fatal("expected error on cancelled prompt, got nil")
	}
	if !errors.Is(err, ErrSplitAborted) {
		t.Errorf("expected ErrSplitAborted, got %v", err)
	}
}

func TestProjectSplitter_TargetDirNotEmpty(t *testing.T) {
	tempDir := t.TempDir()
	sourceProj := filepath.Join(tempDir, "source")
	moduleDir := filepath.Join(sourceProj, "pkg", "sub")
	_ = os.MkdirAll(moduleDir, 0755)

	targetDir := filepath.Join(tempDir, "occupied-target")
	_ = os.MkdirAll(targetDir, 0755)
	_ = os.WriteFile(filepath.Join(targetDir, "existing.txt"), []byte("pre-existing file"), 0644)

	splitter := NewProjectSplitter()
	ctx := context.Background()

	req := SplitRequest{
		SourceProjectDir: sourceProj,
		ModuleSubpath:    filepath.Join("pkg", "sub"),
		NewMemberName:    "sub-lib",
		TargetDirectory:  targetDir,
	}

	_, err := splitter.Split(ctx, req)
	if !errors.Is(err, ErrTargetDirNotEmpty) {
		t.Errorf("expected ErrTargetDirNotEmpty, got %v", err)
	}
}

func TestProjectSplitter_MultiEcosystemScaffolding(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Rust Parent
	rustSource := filepath.Join(tempDir, "parent-rust")
	_ = os.MkdirAll(filepath.Join(rustSource, "src", "core"), 0755)
	_ = os.WriteFile(filepath.Join(rustSource, "Cargo.toml"), []byte("[package]\nname = \"parent-rust\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(rustSource, "src", "core", "lib.rs"), []byte("pub fn run() {}\n"), 0644)

	splitter := NewProjectSplitter()
	ctx := context.Background()

	rustTarget := filepath.Join(tempDir, "rust-core")
	resRust, err := splitter.Split(ctx, SplitRequest{
		SourceProjectDir: rustSource,
		ModuleSubpath:    filepath.Join("src", "core"),
		NewMemberName:    "rust-core",
		TargetDirectory:  rustTarget,
	})
	if err != nil {
		t.Fatalf("rust split failed: %v", err)
	}
	if resRust.ManifestCreated != "Cargo.toml" {
		t.Errorf("expected Cargo.toml, got %s", resRust.ManifestCreated)
	}

	// 2. TS Parent
	tsSource := filepath.Join(tempDir, "parent-ts")
	_ = os.MkdirAll(filepath.Join(tsSource, "src", "ui"), 0755)
	_ = os.WriteFile(filepath.Join(tsSource, "package.json"), []byte("{\"name\": \"parent-ts\"}\n"), 0644)
	_ = os.WriteFile(filepath.Join(tsSource, "src", "ui", "button.ts"), []byte("export const Button = {};\n"), 0644)

	tsTarget := filepath.Join(tempDir, "ts-ui")
	resTS, err := splitter.Split(ctx, SplitRequest{
		SourceProjectDir: tsSource,
		ModuleSubpath:    filepath.Join("src", "ui"),
		NewMemberName:    "ts-ui",
		TargetDirectory:  tsTarget,
	})
	if err != nil {
		t.Fatalf("ts split failed: %v", err)
	}
	if resTS.ManifestCreated != "package.json" {
		t.Errorf("expected package.json, got %s", resTS.ManifestCreated)
	}

	// 3. Python Parent
	pySource := filepath.Join(tempDir, "parent-py")
	_ = os.MkdirAll(filepath.Join(pySource, "app", "util"), 0755)
	_ = os.WriteFile(filepath.Join(pySource, "pyproject.toml"), []byte("[project]\nname = \"parent-py\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(pySource, "app", "util", "helper.py"), []byte("def help(): pass\n"), 0644)

	pyTarget := filepath.Join(tempDir, "py-util")
	resPy, err := splitter.Split(ctx, SplitRequest{
		SourceProjectDir: pySource,
		ModuleSubpath:    filepath.Join("app", "util"),
		NewMemberName:    "py-util",
		TargetDirectory:  pyTarget,
	})
	if err != nil {
		t.Fatalf("py split failed: %v", err)
	}
	if resPy.ManifestCreated != "pyproject.toml" {
		t.Errorf("expected pyproject.toml, got %s", resPy.ManifestCreated)
	}
}

func TestProjectSplitter_ValidationErrors(t *testing.T) {
	tempDir := t.TempDir()
	splitter := NewProjectSplitter()
	ctx := context.Background()

	// Non-existent source project
	_, err := splitter.Split(ctx, SplitRequest{
		SourceProjectDir: filepath.Join(tempDir, "non-existent"),
		ModuleSubpath:    "mod",
		NewMemberName:    "mem",
	})
	if !errors.Is(err, ErrSourceProjectNotFound) {
		t.Errorf("expected ErrSourceProjectNotFound, got %v", err)
	}

	// Non-existent module subpath
	realSource := filepath.Join(tempDir, "real-source")
	_ = os.MkdirAll(realSource, 0755)
	_, err = splitter.Split(ctx, SplitRequest{
		SourceProjectDir: realSource,
		ModuleSubpath:    "non-existent-module",
		NewMemberName:    "mem",
	})
	if !errors.Is(err, ErrModuleNotFound) {
		t.Errorf("expected ErrModuleNotFound, got %v", err)
	}

	// Missing prompt and missing target dir
	realMod := filepath.Join(realSource, "real-module")
	_ = os.MkdirAll(realMod, 0755)
	_, err = splitter.Split(ctx, SplitRequest{
		SourceProjectDir: realSource,
		ModuleSubpath:    "real-module",
		NewMemberName:    "mem",
	})
	if !errors.Is(err, ErrTargetDirRequired) {
		t.Errorf("expected ErrTargetDirRequired, got %v", err)
	}
}

func TestProjectSplitter_NestedDirsAndPoolRegister(t *testing.T) {
	tempDir := t.TempDir()
	sourceProj := filepath.Join(tempDir, "base-app")
	nestedSub := filepath.Join(sourceProj, "pkg", "sub", "deep")
	_ = os.MkdirAll(nestedSub, 0755)
	_ = os.WriteFile(filepath.Join(nestedSub, "deep.txt"), []byte("nested"), 0644)

	splitter := NewProjectSplitter()
	ctx := context.Background()

	targetDir := filepath.Join(tempDir, "extracted-deep")
	res, err := splitter.Split(ctx, SplitRequest{
		SourceProjectDir: sourceProj,
		ModuleSubpath:    filepath.Join("pkg", "sub"),
		NewMemberName:    "deep-module",
		TargetDirectory:  targetDir,
		RegisterInPool:   true,
		PoolConfigFile:   filepath.Join(tempDir, "pool.json"),
	})
	if err != nil {
		t.Fatalf("Split with nested dirs failed: %v", err)
	}
	if !res.PoolRegistered {
		t.Error("expected pool registration to be true")
	}
	if !fileExists(filepath.Join(targetDir, "deep", "deep.txt")) {
		t.Error("nested deep.txt was not copied")
	}

	// Test empty member name
	_, err = splitter.Split(ctx, SplitRequest{
		SourceProjectDir: sourceProj,
		ModuleSubpath:    filepath.Join("pkg", "sub"),
		NewMemberName:    "   ",
		TargetDirectory:  filepath.Join(tempDir, "fail"),
	})
	if err == nil {
		t.Error("expected error for empty member name")
	}

	// Test prompt returning empty string
	_, err = splitter.Split(ctx, SplitRequest{
		SourceProjectDir: sourceProj,
		ModuleSubpath:    filepath.Join("pkg", "sub"),
		NewMemberName:    "deep-module-2",
		Prompt: func(m *modal.DirectoryPickerModal) (string, error) {
			return "   ", nil
		},
	})
	if !errors.Is(err, ErrTargetDirRequired) {
		t.Errorf("expected ErrTargetDirRequired, got %v", err)
	}
}
