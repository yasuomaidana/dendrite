// Package ragtest gives Dendrite's tests a dendrite-rag sidecar without a Rust build: the
// standin command speaks the sidecar protocol over the project's files.
package ragtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"Dendrite/pkg/sidecar"
)

// Build compiles the stand-in sidecar into dir and returns its path.
func Build(dir string) (string, error) {
	bin := filepath.Join(dir, "dendrite-rag")
	out, err := exec.Command("go", "build", "-o", bin, "Dendrite/pkg/rag/ragtest/standin").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("building the dendrite-rag stand-in: %v\n%s", err, out)
	}
	return bin, nil
}

// Install builds the stand-in and points the dendrite-rag locator at it, so an installed
// dendrite-rag is never used. Call the returned function to remove the build.
func Install() (func(), error) {
	dir, err := os.MkdirTemp("", "ragtest")
	if err != nil {
		return nil, err
	}
	bin, err := Build(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	_ = os.Setenv(sidecar.EnvVar("dendrite-rag"), bin)
	return func() { _ = os.RemoveAll(dir) }, nil
}

// Main installs the stand-in, runs the tests and exits.
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
