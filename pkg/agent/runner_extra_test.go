package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNewHostRunner_Constructors(t *testing.T) {
	h1 := NewHostRunner(true)
	if !h1.Interactive || h1.LookPath == nil || h1.CommandFunc == nil {
		t.Errorf("expected interactive runner with initialized functions, got %+v", h1)
	}
	h2 := NewHostRunner(false)
	if h2.Interactive || h2.LookPath == nil || h2.CommandFunc == nil {
		t.Errorf("expected non-interactive runner with initialized functions, got %+v", h2)
	}
}

func TestEnsureAgentInstalled(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()
	lookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	if err := EnsureAgentInstalled(context.Background(), AgentOpenCode); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHostRunner_ExecutionPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("Missing binary with AllowInstall true triggers auto-install", func(t *testing.T) {
		lookPathCalls := 0
		origLookPath := lookPath
		origCmdContext := commandContext
		defer func() {
			lookPath = origLookPath
			commandContext = origCmdContext
		}()

		lookPath = func(file string) (string, error) {
			return "/usr/bin/" + file, nil
		}
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.Command("true")
		}

		runner := &HostRunner{
			AllowInstall: true,
			LookPath: func(file string) (string, error) {
				lookPathCalls++
				if lookPathCalls == 1 {
					return "", errors.New("binary not found")
				}
				return "/usr/bin/echo", nil
			},
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "echo", "installed-run")
			},
		}

		cfg := AgentConfig{
			Type:           AgentOpenCode,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   "/tmp",
		}
		summary, err := runner.Execute(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary.Status != "completed" {
			t.Errorf("expected completed status, got %s", summary.Status)
		}
	})

	t.Run("Interactive mode with custom and default streams", func(t *testing.T) {
		var stdin bytes.Buffer
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		runnerCustom := &HostRunner{
			Interactive: true,
			Stdin:       &stdin,
			Stdout:      &stdout,
			Stderr:      &stderr,
			LookPath: func(file string) (string, error) {
				return "/usr/bin/echo", nil
			},
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "echo", "interactive")
			},
		}
		cfg := AgentConfig{
			Type:           AgentAGY,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   "/tmp",
		}
		_, err := runnerCustom.Execute(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		runnerDefault := &HostRunner{
			Interactive: true,
			LookPath: func(file string) (string, error) {
				return "/usr/bin/echo", nil
			},
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "echo", "interactive-default")
			},
		}
		_, err = runnerDefault.Execute(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Non-interactive mode with custom streams", func(t *testing.T) {
		var stdin bytes.Buffer
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		runner := &HostRunner{
			Interactive: false,
			Stdin:       &stdin,
			Stdout:      &stdout,
			Stderr:      &stderr,
			LookPath: func(file string) (string, error) {
				return "/usr/bin/echo", nil
			},
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "echo", "piped")
			},
		}
		cfg := AgentConfig{
			Type:           AgentOpenHands,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   "/tmp",
		}
		_, err := runner.Execute(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Process execution error paths (failed vs curfew)", func(t *testing.T) {
		runnerFail := &HostRunner{
			LookPath: func(file string) (string, error) {
				return "/usr/bin/false", nil
			},
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "false")
			},
		}
		cfg := AgentConfig{
			Type:           AgentAider,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   "/tmp",
		}
		summaryFail, err := runnerFail.Execute(ctx, cfg)
		if err == nil {
			t.Fatal("expected error on failed process")
		}
		if summaryFail.Status != "failed" {
			t.Errorf("expected status 'failed', got %s", summaryFail.Status)
		}

		ctxCancel, cancel := context.WithCancel(context.Background())
		cancel()
		runnerCurfew := &HostRunner{
			LookPath: func(file string) (string, error) {
				return "/usr/bin/sleep", nil
			},
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctxCancel, "sleep", "1")
			},
		}
		summaryCurfew, err := runnerCurfew.Execute(ctxCancel, cfg)
		if err == nil {
			t.Fatal("expected error on cancelled process")
		}
		if summaryCurfew.Status != "curfew_interrupted" {
			t.Errorf("expected status 'curfew_interrupted', got %s", summaryCurfew.Status)
		}
	})

	t.Run("Nil LookPath and CommandFunc fall back to exec defaults", func(t *testing.T) {
		tempDir := t.TempDir()
		dummyBin := filepath.Join(tempDir, "aider")
		scriptContent := "#!/bin/sh\nexit 0\n"
		if err := os.WriteFile(dummyBin, []byte(scriptContent), 0755); err != nil {
			t.Fatalf("failed to write dummy script: %v", err)
		}

		t.Setenv("PATH", tempDir)

		ctxDeadline, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		runner := &HostRunner{}
		cfg := AgentConfig{
			Type:           AgentAider,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   tempDir,
		}
		summary, err := runner.Execute(ctxDeadline, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary == nil || summary.Status != "completed" {
			t.Errorf("expected completed status, got %+v", summary)
		}
	})

	t.Run("Invalid config returns error", func(t *testing.T) {
		runner := &HostRunner{}
		_, err := runner.Execute(ctx, AgentConfig{})
		if err == nil {
			t.Fatal("expected error for invalid config")
		}
	})
}

func TestBuildAgentCommand_ExtraBranches(t *testing.T) {
	t.Run("aider with task description without ticket and map tokens", func(t *testing.T) {
		cfg := AgentConfig{
			Type:            AgentAider,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			WorkspaceDir:    "/workspace",
			MapTokens:       2048,
			TaskDescription: "Task without ticket",
			ExtraArgs:       []string{"--dry-run"},
		}
		cmd, err := BuildAgentCommand(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cmd) < 7 {
			t.Fatalf("expected at least 7 args, got %v", cmd)
		}
	})

	t.Run("agy with task description without ticket and sandbox", func(t *testing.T) {
		cfg := AgentConfig{
			Type:            AgentAGY,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			WorkspaceDir:    "/workspace",
			TaskDescription: "Refactor core",
			Sandbox:         true,
		}
		cmd, err := BuildAgentCommand(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cmd) < 5 {
			t.Fatalf("expected args with sandbox and prompt, got %v", cmd)
		}
	})

	t.Run("copilot with prompt without ticket", func(t *testing.T) {
		cfg := AgentConfig{
			Type:            AgentCopilot,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			WorkspaceDir:    "/workspace",
			TaskDescription: "Quick fix",
		}
		cmd, err := BuildAgentCommand(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cmd[len(cmd)-1] != "Quick fix" {
			t.Errorf("expected 'Quick fix', got %s", cmd[len(cmd)-1])
		}
	})
}
