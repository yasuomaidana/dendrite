// Package coretest gives Dendrite's tests a dendrite-core sidecar without a Rust build: the standin command
// answers the protocol with scripted replies (see standin/main.go for what each input does).
package coretest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/testmain"
)

// BinaryName is the sidecar the stand-in replaces; it is the locator name and the file name of the build.
const BinaryName = "dendrite-core"

// Binary is the path of the stand-in built by Install; empty before Install.
var Binary string

// Build compiles the stand-in sidecar into dir and returns its path.
func Build(dir string) (string, error) {
	bin := filepath.Join(dir, BinaryName)
	out, err := exec.Command("go", "build", "-o", bin, "Dendrite/pkg/coreclient/coretest/standin").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("building the dendrite-core stand-in: %v\n%s", err, out)
	}
	return bin, nil
}

// Install builds the stand-in and points the dendrite-core locator at it, so an installed dendrite-core is never
// used. Call the returned function to remove the build.
func Install() (func(), error) {
	dir, err := os.MkdirTemp("", "coretest")
	if err != nil {
		return nil, err
	}
	bin, err := Build(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	Binary = bin
	_ = os.Setenv(sidecar.EnvVar(BinaryName), bin)
	return func() { _ = os.RemoveAll(dir) }, nil
}

// Main installs the stand-in, runs the tests and exits.
func Main(m *testing.M) { testmain.Run(m, Install) }
