package assert

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnknownToolchain is returned when no supported toolchain marker is found in the repository.
var ErrUnknownToolchain = errors.New("no supported toolchain detected")

// ToolchainAdapter resolves execution commands for a specific runtime toolchain.
type ToolchainAdapter interface {
	Name() string
	TestCommand(ticketID string, changedPkgs []string) []string
	CoverageCommand(covProfile string) []string
}

// GoAdapter implements ToolchainAdapter for Go repositories (go.mod).
type GoAdapter struct{}

func (g *GoAdapter) Name() string {
	return "go"
}

func (g *GoAdapter) TestCommand(ticketID string, changedPkgs []string) []string {
	covFile := ".dendrite/cov/coverage.out"
	if ticketID != "" {
		covFile = fmt.Sprintf(".dendrite/cov/%s.out", ticketID)
	}

	cmd := []string{"go", "test", "-race", "-coverprofile=" + covFile}
	if len(changedPkgs) == 0 {
		return append(cmd, "./...")
	}

	for _, p := range changedPkgs {
		target := filepath.ToSlash(strings.TrimSpace(p))
		if target == "" {
			continue
		}
		if !strings.HasPrefix(target, "./") {
			target = "./" + target
		}
		if !strings.HasSuffix(target, "/...") {
			target = strings.TrimSuffix(target, "/") + "/..."
		}
		cmd = append(cmd, target)
	}
	return cmd
}

func (g *GoAdapter) CoverageCommand(covProfile string) []string {
	if covProfile == "" {
		covProfile = ".dendrite/cov/coverage.out"
	}
	return []string{"go", "tool", "cover", "-func=" + covProfile}
}

// CargoAdapter implements ToolchainAdapter for Rust repositories (Cargo.toml).
type CargoAdapter struct{}

func (c *CargoAdapter) Name() string {
	return "cargo"
}

func (c *CargoAdapter) TestCommand(ticketID string, changedPkgs []string) []string {
	return []string{"cargo", "test"}
}

func (c *CargoAdapter) CoverageCommand(covProfile string) []string {
	return []string{"cargo", "llvm-cov", "--json"}
}

// BunAdapter implements ToolchainAdapter for JavaScript/TypeScript repositories (package.json + bun.lockb).
type BunAdapter struct{}

func (b *BunAdapter) Name() string {
	return "bun"
}

func (b *BunAdapter) TestCommand(ticketID string, changedPkgs []string) []string {
	return []string{"bun", "test"}
}

func (b *BunAdapter) CoverageCommand(covProfile string) []string {
	return []string{"bun", "test", "--coverage"}
}

// UVAdapter implements ToolchainAdapter for Python repositories (pyproject.toml + uv.lock).
type UVAdapter struct{}

func (u *UVAdapter) Name() string {
	return "uv"
}

func (u *UVAdapter) TestCommand(ticketID string, changedPkgs []string) []string {
	return []string{"uv", "run", "pytest"}
}

func (u *UVAdapter) CoverageCommand(covProfile string) []string {
	return []string{"uv", "run", "pytest", "--cov"}
}

// DetectToolchain identifies the repository toolchain adapter based on marker files.
func DetectToolchain(repoDir string) (ToolchainAdapter, error) {
	if repoDir == "" {
		repoDir = "."
	}
	// Walk up to the nearest directory carrying a toolchain marker (e.g. from a package dir to the module root).
	if abs, err := filepath.Abs(repoDir); err == nil {
		for dir := abs; ; dir = filepath.Dir(dir) {
			if hasToolchainMarker(dir) {
				repoDir = dir
				break
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}

	if fileExists(filepath.Join(repoDir, "go.mod")) {
		return &GoAdapter{}, nil
	}

	if fileExists(filepath.Join(repoDir, "Cargo.toml")) {
		return &CargoAdapter{}, nil
	}

	if isBunRepo(repoDir) {
		return &BunAdapter{}, nil
	}

	if isUVRepo(repoDir) {
		return &UVAdapter{}, nil
	}

	return nil, fmt.Errorf("%w in %s", ErrUnknownToolchain, repoDir)
}

func isBunRepo(repoDir string) bool {
	if !fileExists(filepath.Join(repoDir, "package.json")) {
		return false
	}
	return fileExists(filepath.Join(repoDir, "bun.lockb")) || fileExists(filepath.Join(repoDir, "bun.lock"))
}

func isUVRepo(repoDir string) bool {
	if !fileExists(filepath.Join(repoDir, "pyproject.toml")) {
		return false
	}
	return fileExists(filepath.Join(repoDir, "uv.lock"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func hasToolchainMarker(dir string) bool {
	return fileExists(filepath.Join(dir, "go.mod")) || fileExists(filepath.Join(dir, "Cargo.toml")) ||
		fileExists(filepath.Join(dir, "package.json")) || fileExists(filepath.Join(dir, "pyproject.toml"))
}
