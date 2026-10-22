package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHostRunner(t *testing.T) {
	ctx := context.Background()

	t.Run("NewHostRunner constructor", func(t *testing.T) {
		hr := NewHostRunner(true)
		if !hr.Interactive || hr.LookPath == nil || hr.CommandFunc == nil {
			t.Fatalf("unexpected NewHostRunner fields: %+v", hr)
		}
	})

	t.Run("invalid config returns error", func(t *testing.T) {
		runner := NewHostRunner(false)
		_, err := runner.Execute(ctx, AgentConfig{})
		if err == nil {
			t.Fatal("expected error for invalid config")
		}
	})

	t.Run("executable not found in PATH returns actionable error", func(t *testing.T) {
		runner := &HostRunner{
			LookPath: func(file string) (string, error) {
				return "", errors.New("not found")
			},
		}
		cfg := AgentConfig{
			Type:           AgentOpenCode,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   "/tmp",
		}
		_, err := runner.Execute(ctx, cfg)
		if err == nil {
			t.Fatal("expected error for missing executable, got nil")
		}
	})

	t.Run("successful host execution with temp-dir executable", func(t *testing.T) {
		tmpDir := t.TempDir()
		binPath := filepath.Join(tmpDir, "agy")
		script := "#!/bin/sh\necho \"executed agy\"\nexit 0\n"
		if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
			t.Fatalf("failed to write executable: %v", err)
		}

		var stdoutBuf, stderrBuf bytes.Buffer
		runner := &HostRunner{
			Interactive: false,
			Stdout:      &stdoutBuf,
			Stderr:      &stderrBuf,
			Stdin:       bytes.NewBufferString(""),
			LookPath: func(file string) (string, error) {
				return binPath, nil
			},
		}
		cfg := AgentConfig{
			Type:            AgentAGY,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			Model:           "local-qwen-coder",
			WorkspaceDir:    tmpDir,
			TaskDescription: "test task",
			TicketID:        "DEN-024",
		}
		summary, err := runner.Execute(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary == nil || summary.Status != "completed" {
			t.Fatalf("expected completed status, got %v", summary)
		}
		if summary.EstimatedCostUSD != 0.0 {
			t.Errorf("expected 0 cost for local-qwen-coder, got %f", summary.EstimatedCostUSD)
		}
	})

	t.Run("interactive host execution and failed command", func(t *testing.T) {
		tmpDir := t.TempDir()
		var stdoutBuf, stderrBuf bytes.Buffer
		runner := &HostRunner{
			Interactive: true,
			Stdout:      &stdoutBuf,
			Stderr:      &stderrBuf,
			Stdin:       bytes.NewBufferString(""),
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
			WorkspaceDir:   tmpDir,
		}
		summary, err := runner.Execute(ctx, cfg)
		if err == nil {
			t.Fatal("expected error from false command")
		}
		if summary == nil || summary.Status != "failed" {
			t.Fatalf("expected failed status, got %+v", summary)
		}
	})

	t.Run("curfew interrupted context cancellation", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		runner := &HostRunner{
			LookPath: func(file string) (string, error) {
				return "/bin/sleep", nil
			},
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "sleep", "5")
			},
		}
		cfg := AgentConfig{
			Type:           AgentAider,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   t.TempDir(),
		}
		summary, err := runner.Execute(cctx, cfg)
		if err == nil {
			t.Fatal("expected context canceled error")
		}
		if summary == nil || summary.Status != "curfew_interrupted" {
			t.Fatalf("expected curfew_interrupted status, got %+v", summary)
		}
	})
}
