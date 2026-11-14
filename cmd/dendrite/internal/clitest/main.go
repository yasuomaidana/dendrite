package clitest

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/env/bootstrap"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/workcopy"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
)

// Main is the TestMain of every command package. It keeps the tests away from the developer's real
// environment: session commands run working-copy cleanup as a side effect, and an unstubbed
// `docker compose down` once removed the live Plane containers.
func Main(m *testing.M) {
	dir, err := os.MkdirTemp("", "dendrite-work-test-")
	if err == nil {
		_ = os.Setenv(workcopy.EnvWorkDir, dir)
	}
	// Commands must not pick up the developer's real keys from .env files.
	_ = os.Setenv(dotenv.EnvDisable, "off")
	// Nothing here may reach the developer's real Docker or write into the real ~/.dendrite/stacks:
	// an unstubbed `docker compose down` once removed the live Plane containers.
	cli.RunCompose = func(context.Context, io.Writer, string, ...string) error { return nil }
	cli.RestartRunningLiteLLM = func(context.Context) error { return nil }
	cli.DockerCmd = func(context.Context, ...string) ([]byte, error) { return nil, nil }
	// Nor may a test change the developer's real Antigravity MCP registry.
	cli.AgyRegister = func(string) ([]string, string) { return nil, "" }
	cli.AgyMCPStatus = func(string) (bootstrap.AGYMCPState, error) { return bootstrap.AGYAbsent, nil }
	stacksDir, stacksErr := os.MkdirTemp("", "dendrite-stacks-test-")
	if stacksErr == nil {
		_ = os.Setenv("DENDRITE_STACKS_DIR", stacksDir)
	}
	// The dendrite-plane sidecar is required: tests that reach Plane run the real binary.
	stopPlane, planeErr := planetest.Install()
	if planeErr != nil {
		fmt.Fprintln(os.Stderr, planeErr)
		os.Exit(1)
	}
	code := m.Run()
	stopPlane()
	if err == nil {
		_ = os.RemoveAll(dir)
	}
	if stacksErr == nil {
		_ = os.RemoveAll(stacksDir)
	}
	os.Exit(code)
}
