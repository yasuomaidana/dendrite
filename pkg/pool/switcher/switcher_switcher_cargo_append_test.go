package switcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwitcher_CargoAppendDependencies(t *testing.T) {
	tempDir := t.TempDir()
	cargoContent := `[package]
name = "bare-crate"
version = "0.1.0"
`
	_ = os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte(cargoContent), 0644)
	sw := NewDependencySwitcher()
	ctx := context.Background()

	// Branch with empty fork URL is not a valid cargo dependency.
	_, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "serde_json",
		Mode:             ModeBranch,
		BranchName:       "master",
	})
	if err == nil {
		t.Fatal("expected cargo branch mode without a git URL to fail")
	}
	_, err = sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "serde_json",
		Mode:             ModeBranch,
		BranchName:       "master",
		ForkURL:          "https://github.com/serde-rs/json",
	})
	if err != nil {
		t.Fatalf("cargo branch append failed: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(tempDir, "Cargo.toml")); !strings.Contains(string(data), `serde_json = { git = "https://github.com/serde-rs/json", branch = "master" }`) {
		t.Errorf("Cargo.toml = %s", data)
	}
}

func TestSwitcher_GoWorkExisting(t *testing.T) {
	tempDir := t.TempDir()
	goModContent := "module testmod\ngo 1.24\n"
	goWorkContent := "go 1.24\n\nuse (\n\t./\n)\n"
	_ = os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "go.work"), []byte(goWorkContent), 0644)

	sw := NewDependencySwitcher()
	ctx := context.Background()
	_, err := sw.Switch(ctx, SwitchRequest{
		TargetProjectDir: tempDir,
		DependencyName:   "github.com/foo/bar",
		Mode:             ModeLocal,
		LocalPath:        "../bar",
	})
	if err != nil {
		t.Fatalf("go.work update failed: %v", err)
	}
}

func TestSwitcher_TypestateValidationFailures(t *testing.T) {
	// Published without version
	p := PublishedState{}
	if err := p.Validate(); err == nil {
		t.Error("expected error for empty version")
	}

	// Branch without branch or commit
	b := BranchState{}
	if err := b.Validate(); err == nil {
		t.Error("expected error for empty branch")
	}

	// Local without path
	l := LocalState{}
	if err := l.Validate(); err == nil {
		t.Error("expected error for empty local path")
	}

	// Fork without URL
	f := ForkState{}
	if err := f.Validate(); err == nil {
		t.Error("expected error for empty fork url")
	}
}

func TestSwitcher_GoWorkUsesModuleGoVersion(t *testing.T) {
	tempDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module testmod\n\ngo 1.25\n"), 0644)
	if _, err := NewDependencySwitcher().Switch(context.Background(), SwitchRequest{
		TargetProjectDir: tempDir, DependencyName: "github.com/foo/bar", Mode: ModeLocal, LocalPath: "../bar",
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(tempDir, "go.work"))
	if !strings.HasPrefix(string(data), "go 1.25\n") {
		t.Fatalf("go.work = %q", data)
	}
	if goDirective(filepath.Join(tempDir, "missing.mod")) != "" {
		t.Fatal("goDirective on missing file")
	}
}

func TestGoModulePathAndGoGet(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r.git": "github.com/o/r",
		"git@github.com:o/r.git":     "github.com/o/r",
		"github.com/o/r":             "github.com/o/r",
	} {
		if got := goModulePath(in); got != want {
			t.Errorf("goModulePath(%q) = %q, want %q", in, got, want)
		}
	}
	// The real go get fails outside a module with a clear error.
	if err := goGet(t.TempDir(), "example.invalid/nope@main"); err == nil || !strings.Contains(err.Error(), "go get") {
		t.Fatalf("goGet err = %v", err)
	}
}
