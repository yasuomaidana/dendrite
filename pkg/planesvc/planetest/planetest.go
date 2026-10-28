// Package planetest points Dendrite's tests at a real dendrite-plane sidecar. There is no Go
// stand-in: the sidecar is the only implementation of what it serves, so tests run the binary.
package planetest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/sidecar"
)

// binaryName is the sidecar executable (planesvc.BinaryName; not imported so planesvc tests can use this package).
const binaryName = "dendrite-plane"

// Binary finds a built dendrite-plane: DENDRITE_PLANE_TEST_BIN, the already set locator
// variable, a sibling checkout's release build (next to this repository, or under
// ~/RustRoverProjects), then $DENDRITE_HOME/bin.
func Binary() (string, error) {
	home, _ := os.UserHomeDir()
	candidates := []string{os.Getenv("DENDRITE_PLANE_TEST_BIN"), os.Getenv(sidecar.EnvVar(binaryName))}
	if wd, err := os.Getwd(); err == nil {
		for dir := wd; ; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				candidates = append(candidates, filepath.Join(dir, "..", "dendrite-plane-rs", "target", "release", binaryName))
				break
			}
			if dir == filepath.Dir(dir) {
				break
			}
		}
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, "RustRoverProjects", "dendrite-plane-rs", "target", "release", binaryName),
			filepath.Join(home, ".dendrite", "bin", binaryName))
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if info, err := os.Stat(c); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return c, nil
		}
	}
	return "", fmt.Errorf("no dendrite-plane binary for the tests: run `cargo build --release` in dendrite-plane-rs or set DENDRITE_PLANE_TEST_BIN")
}

// Install points the dendrite-plane locator at the binary and gives the sidecar a private
// relation cache. Call the returned function to remove the cache.
func Install() (func(), error) {
	bin, err := Binary()
	if err != nil {
		return nil, err
	}
	cache, err := os.MkdirTemp("", "planetest")
	if err != nil {
		return nil, err
	}
	_ = os.Setenv(sidecar.EnvVar(binaryName), bin)
	_ = os.Setenv("DENDRITE_PLANE_CACHE_DIR", cache)
	if os.Getenv("PLANE_API_KEY") == "" { // the sidecar reads its token from the environment; tests talk to a local fake Plane
		_ = os.Setenv("PLANE_API_KEY", "planetest-token")
	}
	return func() { _ = os.RemoveAll(cache) }, nil
}

// Main installs the sidecar, runs the tests and exits.
func Main(m *testing.M) {
	cleanup, err := Install()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
