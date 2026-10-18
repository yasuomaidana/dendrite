package assert

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectToolchain_Go(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0644); err != nil {
		t.Fatalf("failed creating go.mod: %v", err)
	}

	adapter, err := DetectToolchain(dir)
	if err != nil {
		t.Fatalf("expected successful detection, got err: %v", err)
	}
	if adapter.Name() != "go" {
		t.Fatalf("expected toolchain 'go', got '%s'", adapter.Name())
	}

	testCmd := adapter.TestCommand("DEN-912", []string{"pkg/assert", "pkg/ast"})
	expectedTestCmd := []string{
		"go", "test", "-race", "-coverprofile=.dendrite/cov/DEN-912.out",
		"./pkg/assert/...", "./pkg/ast/...",
	}
	if !reflect.DeepEqual(testCmd, expectedTestCmd) {
		t.Fatalf("expected test command %v, got %v", expectedTestCmd, testCmd)
	}

	defaultCmd := adapter.TestCommand("", []string{})
	expectedDefaultCmd := []string{
		"go", "test", "-race", "-coverprofile=.dendrite/cov/coverage.out", "./...",
	}
	if !reflect.DeepEqual(defaultCmd, expectedDefaultCmd) {
		t.Fatalf("expected default test command %v, got %v", expectedDefaultCmd, defaultCmd)
	}

	covCmd := adapter.CoverageCommand(".dendrite/cov/DEN-912.out")
	expectedCovCmd := []string{"go", "tool", "cover", "-func=.dendrite/cov/DEN-912.out"}
	if !reflect.DeepEqual(covCmd, expectedCovCmd) {
		t.Fatalf("expected coverage command %v, got %v", expectedCovCmd, covCmd)
	}

	defaultCovCmd := adapter.CoverageCommand("")
	expectedDefaultCovCmd := []string{"go", "tool", "cover", "-func=.dendrite/cov/coverage.out"}
	if !reflect.DeepEqual(defaultCovCmd, expectedDefaultCovCmd) {
		t.Fatalf("expected default coverage command %v, got %v", expectedDefaultCovCmd, defaultCovCmd)
	}
}

func TestDetectToolchain_Cargo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("failed creating Cargo.toml: %v", err)
	}

	adapter, err := DetectToolchain(dir)
	if err != nil {
		t.Fatalf("expected successful detection, got err: %v", err)
	}
	if adapter.Name() != "cargo" {
		t.Fatalf("expected toolchain 'cargo', got '%s'", adapter.Name())
	}

	testCmd := adapter.TestCommand("DEN-912", []string{"crate_a"})
	if !reflect.DeepEqual(testCmd, []string{"cargo", "test"}) {
		t.Fatalf("unexpected test command: %v", testCmd)
	}

	covCmd := adapter.CoverageCommand("target/cov.json")
	if !reflect.DeepEqual(covCmd, []string{"cargo", "llvm-cov", "--json"}) {
		t.Fatalf("unexpected coverage command: %v", covCmd)
	}
}

func TestDetectToolchain_Bun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{\"name\": \"bun-demo\"}"), 0644); err != nil {
		t.Fatalf("failed creating package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bun.lockb"), []byte("binary-lock-content"), 0644); err != nil {
		t.Fatalf("failed creating bun.lockb: %v", err)
	}

	adapter, err := DetectToolchain(dir)
	if err != nil {
		t.Fatalf("expected successful detection, got err: %v", err)
	}
	if adapter.Name() != "bun" {
		t.Fatalf("expected toolchain 'bun', got '%s'", adapter.Name())
	}

	testCmd := adapter.TestCommand("DEN-912", []string{"src"})
	if !reflect.DeepEqual(testCmd, []string{"bun", "test"}) {
		t.Fatalf("unexpected test command: %v", testCmd)
	}

	covCmd := adapter.CoverageCommand("")
	if !reflect.DeepEqual(covCmd, []string{"bun", "test", "--coverage"}) {
		t.Fatalf("unexpected coverage command: %v", covCmd)
	}

	// Test with text bun.lock format
	dirTextLock := t.TempDir()
	_ = os.WriteFile(filepath.Join(dirTextLock, "package.json"), []byte("{}"), 0644)
	_ = os.WriteFile(filepath.Join(dirTextLock, "bun.lock"), []byte("# bun lockfile"), 0644)
	adapter2, err2 := DetectToolchain(dirTextLock)
	if err2 != nil || adapter2.Name() != "bun" {
		t.Fatalf("expected bun detection with bun.lock, got: %v", adapter2)
	}
}

func TestDetectToolchain_UV(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"uv-demo\"\n"), 0644); err != nil {
		t.Fatalf("failed creating pyproject.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "uv.lock"), []byte("version = 1\n"), 0644); err != nil {
		t.Fatalf("failed creating uv.lock: %v", err)
	}

	adapter, err := DetectToolchain(dir)
	if err != nil {
		t.Fatalf("expected successful detection, got err: %v", err)
	}
	if adapter.Name() != "uv" {
		t.Fatalf("expected toolchain 'uv', got '%s'", adapter.Name())
	}

	testCmd := adapter.TestCommand("DEN-912", []string{"tests"})
	if !reflect.DeepEqual(testCmd, []string{"uv", "run", "pytest"}) {
		t.Fatalf("unexpected test command: %v", testCmd)
	}

	covCmd := adapter.CoverageCommand("")
	if !reflect.DeepEqual(covCmd, []string{"uv", "run", "pytest", "--cov"}) {
		t.Fatalf("unexpected coverage command: %v", covCmd)
	}
}

func TestDetectToolchain_UnknownAndIncomplete(t *testing.T) {
	emptyDir := t.TempDir()
	adapter, err := DetectToolchain(emptyDir)
	if err == nil || adapter != nil {
		t.Fatalf("expected error for empty directory, got adapter: %v", adapter)
	}
	if !errors.Is(err, ErrUnknownToolchain) {
		t.Fatalf("expected ErrUnknownToolchain, got: %v", err)
	}

	// package.json without bun lockfile
	nodeOnlyDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(nodeOnlyDir, "package.json"), []byte("{}"), 0644)
	_, errNode := DetectToolchain(nodeOnlyDir)
	if !errors.Is(errNode, ErrUnknownToolchain) {
		t.Fatalf("expected ErrUnknownToolchain for package.json without bun lockfile, got: %v", errNode)
	}

	// pyproject.toml without uv.lock
	pyOnlyDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(pyOnlyDir, "pyproject.toml"), []byte(""), 0644)
	_, errPy := DetectToolchain(pyOnlyDir)
	if !errors.Is(errPy, ErrUnknownToolchain) {
		t.Fatalf("expected ErrUnknownToolchain for pyproject.toml without uv.lock, got: %v", errPy)
	}
}

func TestDetectToolchain_DefaultCurrentDir(t *testing.T) {
	adapter, err := DetectToolchain("")
	if err != nil {
		t.Fatalf("expected detecting workspace toolchain, got err: %v", err)
	}
	if adapter.Name() != "go" {
		t.Fatalf("expected current workspace to be 'go', got: %s", adapter.Name())
	}
}

func TestGoAdapter_TargetFormatting(t *testing.T) {
	adapter := &GoAdapter{}
	cmd := adapter.TestCommand("DEN-912", []string{"", "  ", "pkg/assert/...", "./pkg/ast", "cmd/dendrite/"})
	expected := []string{
		"go", "test", "-race", "-coverprofile=.dendrite/cov/DEN-912.out",
		"./pkg/assert/...", "./pkg/ast/...", "./cmd/dendrite/...",
	}
	if !reflect.DeepEqual(cmd, expected) {
		t.Fatalf("unexpected formatted command:\nexpected: %v\ngot:      %v", expected, cmd)
	}
}
