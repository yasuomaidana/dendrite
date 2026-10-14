package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// CommandRunner abstracts shell execution for testability.
type CommandRunner interface {
	Run(ctx context.Context, command string, args ...string) ([]byte, error)
}

// OSCommandRunner executes commands using the host operating system.
type OSCommandRunner struct{}

func (r *OSCommandRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	return cmd.CombinedOutput()
}

// SandboxSession coordinates the lifecycle of an ephemeral container with RAII drop cleanup.
type SandboxSession struct {
	Config    *SandboxConfig
	Runner    CommandRunner
	closeOnce sync.Once
	cleanedUp bool
	mu        sync.Mutex
}

// NewSession creates an active sandbox session.
func NewSession(cfg *SandboxConfig, runner CommandRunner) *SandboxSession {
	if runner == nil {
		runner = &OSCommandRunner{}
	}
	return &SandboxSession{
		Config: cfg,
		Runner: runner,
	}
}

// Close implements the RAII Drop cleanup pattern, guaranteeing container destruction.
// Idempotent and thread-safe.
func (s *SandboxSession) Close() error {
	var cleanupErr error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		// Forcefully remove container if still present
		name := s.Config.ContainerName
		if name != "" && s.Runner != nil {
			_, cleanupErr = s.Runner.Run(context.Background(), "docker", "rm", "-f", name)
		}
		s.cleanedUp = true
	})
	return cleanupErr
}

// IsCleanedUp reports whether the container cleanup drop handler has run.
func (s *SandboxSession) IsCleanedUp() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanedUp
}

// Execute runs the sandboxed container within a context, guaranteeing cleanup via defer.
func (s *SandboxSession) Execute(ctx context.Context) (*ExecutionResult, error) {
	// RAII Drop guarantee: Close() always runs upon exit, panic, or cancellation
	defer func() {
		_ = s.Close()
	}()

	args, err := s.Config.BuildDockerRunArgs()
	if err != nil {
		return nil, err
	}

	start := time.Now()
	output, runErr := s.Runner.Run(ctx, "docker", args...)
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	res := &ExecutionResult{
		ContainerName: s.Config.ContainerName,
		ExitCode:      exitCode,
		Stdout:        string(output),
		Duration:      duration,
		CleanedUp:     true,
	}

	if runErr != nil && ctx.Err() != nil {
		return res, fmt.Errorf("%w: %v", ErrSandboxTerminated, ctx.Err())
	}

	return res, runErr
}
