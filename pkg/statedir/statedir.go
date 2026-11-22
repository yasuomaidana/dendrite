package statedir

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	gitOnce sync.Once
	gitRoot string

	testOnce sync.Once
	testRoot string

	// isTestBinary and gitTopLevel are seams so every resolution branch is testable.
	isTestBinary = testing.Testing
	gitTopLevel  = mainCheckout
)

// mainCheckout prints the checkout that owns the git directory: for a linked worktree that is
// the main checkout, so every working copy shares one untracked .dendrite/. Outside a
// standard layout (bare repository) it falls back to the current top level.
func mainCheckout() ([]byte, error) { return mainCheckoutIn("") }

// mainCheckoutIn is mainCheckout for the repository containing dir ("" is the current directory).
func mainCheckoutIn(dir string) ([]byte, error) {
	git := func(args ...string) ([]byte, error) {
		if dir != "" {
			args = append([]string{"-C", dir}, args...)
		}
		return exec.Command("git", args...).Output()
	}
	out, err := git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err == nil {
		if common := strings.TrimSpace(string(out)); filepath.Base(common) == ".git" {
			return []byte(filepath.Dir(common) + "\n"), nil
		}
	}
	return git("rev-parse", "--show-toplevel")
}

// testStateDir isolates state per test binary so `go test` never mutates the
// repository's tracked .dendrite/ files or leaves state in package directories.
func testStateDir() string {
	testOnce.Do(func() {
		if dir, err := os.MkdirTemp("", "dendrite-test-state-"); err == nil {
			testRoot = dir
		}
	})
	return testRoot
}

func gitStateDir() string {
	gitOnce.Do(func() {
		out, err := gitTopLevel()
		if err == nil {
			top := strings.TrimSpace(string(out))
			if top != "" {
				gitRoot = filepath.Join(top, ".dendrite")
			}
		}
	})
	return gitRoot
}

// Resolve maps a ".dendrite/..." relative path to the canonical state directory.
// Absolute paths and paths not starting with ".dendrite/" are returned unchanged.
// Resolution order for the ".dendrite" root:
//  1. $DENDRITE_STATE_DIR (used as the .dendrite directory itself)
//  2. a per-process temp dir when running inside a `go test` binary
//  3. <git rev-parse --show-toplevel>/.dendrite (computed once, cached with sync.Once)
//  4. the path unchanged (CWD-relative) when not inside a git repository
func Resolve(p string) string { return resolve(p, gitStateDir) }

// ResolveIn is Resolve for the repository that contains dir instead of the current directory: a guard that
// judges a file in another repository must read that repository's state, not its own. $DENDRITE_STATE_DIR
// and the per-test directory still win, as in Resolve.
func ResolveIn(dir, p string) string {
	return resolve(p, func() string {
		out, err := mainCheckoutIn(dir)
		if top := strings.TrimSpace(string(out)); err == nil && top != "" {
			return filepath.Join(top, ".dendrite")
		}
		return ""
	})
}

func resolve(p string, gitDir func() string) string {
	if filepath.IsAbs(p) {
		return p
	}

	slash := filepath.ToSlash(p)
	if !strings.HasPrefix(slash, ".dendrite/") && slash != ".dendrite" {
		return p
	}

	var root string
	if envDir := os.Getenv("DENDRITE_STATE_DIR"); envDir != "" {
		root = envDir
	} else if isTestBinary() && testStateDir() != "" {
		root = testStateDir()
	} else if dir := gitDir(); dir != "" {
		root = dir
	} else {
		return p
	}

	if slash == ".dendrite" {
		return root
	}

	rel := strings.TrimPrefix(slash, ".dendrite/")
	return filepath.Join(root, filepath.FromSlash(rel))
}
