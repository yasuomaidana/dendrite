package cli

import (
	"Dendrite/pkg/env/bootstrap"
	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/tracker/hitl"
	"Dendrite/pkg/workcopy"
	"Dendrite/pkg/workspace"
	"context"
	"io"
	"os/exec"
)

// agyRegister and agyMCPStatus are the seams to Antigravity's own MCP registry; the tests replace them
// so no test changes the developer's real AGY configuration.
var (
	AgyRegister  = bootstrap.RegisterAGY
	AgyMCPStatus = bootstrap.AGYMCPStatus
)

// dockerCmd runs the docker CLI and returns its output. It is a variable so tests never touch the
// developer's real containers.
var DockerCmd = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", args...).Output()
}

// newHITLSyncer is a seam so tests can point the commands at a local Plane and gh.
var NewHITLSyncer = func(ctx context.Context, repo string) (*hitl.Syncer, error) {
	return hitl.NewFromEnv(ctx, repo, ".")
}

// newProjectRegistry is a seam so tests can point the CLI at a temporary registry.
var NewProjectRegistry = func() (*workspace.Registry, error) { return workspace.NewRegistry("", "") }

// newWorkManager is a seam so tests can point the CLI at a temporary repository.
var NewWorkManager = func() *workcopy.Manager { return workcopy.New(".") }

// openSegmentedStore opens the persistent store on use; tests replace it with a temp store.
var OpenSegmentedStore = func() (*segmented.SegmentedStore, error) { return segmented.NewSegmentedStore("") }

// restartRunningLiteLLM reloads or restarts the LiteLLM proxy container if running. It is a variable
// so tests never restart the developer's real gateway.
var RestartRunningLiteLLM = func(ctx context.Context) error {
	return exec.CommandContext(ctx, "docker", "restart", "dendrite-litellm").Run()
}

// runCompose is a seam so tests can run the service commands without Docker.
var RunCompose = func(ctx context.Context, out io.Writer, file string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose", "-f", file}, args...)...)
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}
