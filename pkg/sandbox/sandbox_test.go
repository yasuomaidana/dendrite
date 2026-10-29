package sandbox_test

import (
	"Dendrite/pkg/sandbox/sandboxtest"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/sandbox"
)

func TestGenerateContainerName(t *testing.T) {
	name1 := sandbox.GenerateContainerName()
	name2 := sandbox.GenerateContainerName()

	if !strings.HasPrefix(name1, "dendrite-sandbox-") {
		t.Errorf("expected prefix dendrite-sandbox-, got %s", name1)
	}
	if name1 == name2 {
		t.Errorf("expected unique container names")
	}
}

func TestSandboxConfig_BuildDockerRunArgs(t *testing.T) {
	cfg := sandbox.NewDefaultSandboxConfig("alpine:latest", []string{"echo", "hello"})
	cfg.HostScratchDir = "/tmp/host-scratch"
	cfg.Environment["DEBUG"] = "1"

	args, err := cfg.BuildDockerRunArgs()
	if err != nil {
		t.Fatalf("unexpected error building run args: %v", err)
	}

	argStr := strings.Join(args, " ")

	expectedTokens := []string{
		"run",
		"--name", cfg.ContainerName,
		"--rm",
		"--cap-drop ALL",
		"--security-opt no-new-privileges:true",
		"--read-only",
		"--network none",
		"--tmpfs /tmp:rw,noexec,nosuid,size=1G",
		"-m 1024m",
		"--cpus 2.0",
		"-v /tmp/host-scratch:/workspace:rw",
		"-w /workspace",
		"-e DEBUG=1",
		"alpine:latest echo hello",
	}

	for _, token := range expectedTokens {
		if !strings.Contains(argStr, token) {
			t.Errorf("expected argStr to contain '%s', got: %s", token, argStr)
		}
	}

	// Test validation errors
	emptyImgCfg := sandbox.NewDefaultSandboxConfig("", []string{"echo"})
	if _, err := emptyImgCfg.BuildDockerRunArgs(); !errors.Is(err, sandbox.ErrEmptyImage) {
		t.Errorf("expected ErrEmptyImage for empty image")
	}

	emptyCmdCfg := sandbox.NewDefaultSandboxConfig("alpine:latest", []string{})
	if _, err := emptyCmdCfg.BuildDockerRunArgs(); !errors.Is(err, sandbox.ErrEmptyCommand) {
		t.Errorf("expected ErrEmptyCommand for empty command")
	}
}

func TestSandboxSession_RAIIDropCleanup(t *testing.T) {
	runner := &sandboxtest.RecordingRunner{Output: []byte("success"), Err: nil}
	cfg := sandbox.NewDefaultSandboxConfig("golang:1.24", []string{"go", "version"})

	session := sandbox.NewSession(cfg, runner)

	ctx := context.Background()
	res, err := session.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error during execution: %v", err)
	}

	if res.Stdout != "success" || res.ExitCode != 0 {
		t.Errorf("expected success exit code 0, got %d, out: %s", res.ExitCode, res.Stdout)
	}

	// Verify RAII Drop guarantee: docker rm -f was invoked
	cleaned := runner.HasCall(func(args []string) bool {
		return len(args) >= 4 && args[0] == "docker" && args[1] == "rm" && args[2] == "-f" && args[3] == cfg.ContainerName
	})

	if !cleaned {
		t.Errorf("expected docker rm -f %s to be called during session defer", cfg.ContainerName)
	}
	if !session.IsCleanedUp() {
		t.Errorf("expected session.IsCleanedUp() to return true")
	}

	// Idempotency: second Close() call should succeed without re-running
	if err := session.Close(); err != nil {
		t.Errorf("expected idempotent Close() without error, got: %v", err)
	}
}

func TestSandboxSession_ContextCancellationCleanup(t *testing.T) {
	blockCh := make(chan struct{})
	runner := &sandboxtest.RecordingRunner{BlockCh: blockCh}
	cfg := sandbox.NewDefaultSandboxConfig("alpine:latest", []string{"sleep", "60"})

	session := sandbox.NewSession(cfg, runner)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel quickly
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_, err := session.Execute(ctx)
	if !errors.Is(err, sandbox.ErrSandboxTerminated) {
		t.Errorf("expected ErrSandboxTerminated on context cancellation, got: %v", err)
	}

	// Verify RAII cleanup was still called
	cleaned := runner.HasCall(func(args []string) bool {
		return len(args) >= 4 && args[0] == "docker" && args[1] == "rm" && args[2] == "-f" && args[3] == cfg.ContainerName
	})

	if !cleaned {
		t.Errorf("expected cleanup trap to run upon context cancellation")
	}
}

func TestOSCommandRunner(t *testing.T) {
	runner := &sandbox.OSCommandRunner{}
	ctx := context.Background()
	out, err := runner.Run(ctx, "echo", "dendrite")
	if err != nil || !strings.Contains(string(out), "dendrite") {
		t.Errorf("expected echo output, got out=%s, err=%v", string(out), err)
	}
}

func TestSandboxSession_ExtraCoverage(t *testing.T) {
	sessionNilRunner := sandbox.NewSession(&sandbox.SandboxConfig{}, nil)
	if sessionNilRunner == nil || sessionNilRunner.Runner == nil {
		t.Fatal("expected default OSCommandRunner")
	}

	sessionEmptyName := sandbox.NewSession(&sandbox.SandboxConfig{ContainerName: ""}, &sandboxtest.RecordingRunner{})
	if err := sessionEmptyName.Close(); err != nil {
		t.Fatalf("unexpected error on Close: %v", err)
	}

	cfgErr := &sandbox.SandboxConfig{Image: "", Command: []string{"echo"}}
	sessionErr := sandbox.NewSession(cfgErr, &sandboxtest.RecordingRunner{})
	_, err := sessionErr.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error on invalid config in Execute")
	}
}

func TestSandboxConfig_MinimalArgs(t *testing.T) {
	cfg := &sandbox.SandboxConfig{
		Image:               "alpine:latest",
		Command:             []string{"sh"},
		ContainerName:       "",
		DropAllCapabilities: false,
		NoNewPrivileges:     false,
		ReadOnlyRoot:        false,
		NetworkMode:         "",
		TmpfsSize:           "",
		MemoryLimitMB:       0,
		CPULimit:            0,
		HostScratchDir:      "",
		Environment:         nil,
	}

	args, err := cfg.BuildDockerRunArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, "--network none") {
		t.Errorf("expected default network none")
	}
	if strings.Contains(argStr, "--cap-drop") {
		t.Errorf("did not expect cap-drop")
	}
	if strings.Contains(argStr, "--security-opt") {
		t.Errorf("did not expect security-opt")
	}
	if strings.Contains(argStr, "--read-only") {
		t.Errorf("did not expect read-only")
	}
}

func TestSandboxSession_ExitErrorAndGenericError(t *testing.T) {
	// Obtain a genuine *exec.ExitError using real local subprocess
	cmd := exec.Command("sh", "-c", "exit 42")
	realExitErr := cmd.Run()
	if realExitErr == nil {
		t.Fatal("expected exit 42 to return error")
	}

	runnerExitErr := &sandboxtest.RecordingRunner{Err: realExitErr}
	cfg := sandbox.NewDefaultSandboxConfig("alpine:latest", []string{"false"})
	session := sandbox.NewSession(cfg, runnerExitErr)

	res, err := session.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error from Execute")
	}
	if res.ExitCode != 42 {
		t.Errorf("expected ExitCode 42, got %d", res.ExitCode)
	}

	// Generic error (not *exec.ExitError, ctx not cancelled)
	genericErr := errors.New("daemon failure")
	runnerGeneric := &sandboxtest.RecordingRunner{Err: genericErr}
	sessionGen := sandbox.NewSession(cfg, runnerGeneric)

	resGen, errGen := sessionGen.Execute(context.Background())
	if !errors.Is(errGen, genericErr) {
		t.Errorf("expected genericErr, got %v", errGen)
	}
	if resGen.ExitCode != 1 {
		t.Errorf("expected ExitCode 1, got %d", resGen.ExitCode)
	}
}

func TestSandboxSession_CloseRunnerError(t *testing.T) {
	closeErr := errors.New("docker rm failed")
	runner := &sandboxtest.RecordingRunner{Err: closeErr}
	cfg := &sandbox.SandboxConfig{ContainerName: "test-container"}
	session := sandbox.NewSession(cfg, runner)

	if err := session.Close(); !errors.Is(err, closeErr) {
		t.Errorf("expected close error %v, got %v", closeErr, err)
	}
}

func TestSandboxConfig_CustomNetworkAndTmpfs(t *testing.T) {
	cfg := sandbox.NewDefaultSandboxConfig("alpine:latest", []string{"echo"})
	cfg.NetworkMode = "bridge"
	cfg.TmpfsSize = "512M"

	args, err := cfg.BuildDockerRunArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, "--network bridge") {
		t.Errorf("expected --network bridge, got: %s", argStr)
	}
	if !strings.Contains(argStr, "size=512M") {
		t.Errorf("expected size=512M, got: %s", argStr)
	}

	// Verify standard error types are initialized
	if sandbox.ErrSessionClosed == nil {
		t.Error("expected ErrSessionClosed to be non-nil")
	}
}
