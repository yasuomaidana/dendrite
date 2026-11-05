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

func writeTestExecutableScript(t *testing.T, dir, name, script string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write executable: %v", err)
	}
	return p
}

func assertHostRunnerExecute(t *testing.T, ctx context.Context, runner *HostRunner, cfg AgentConfig, wantErr bool, wantStatus string) *SessionSummary {
	t.Helper()
	summary, err := runner.Execute(ctx, cfg)
	if wantErr {
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wantStatus != "" && (summary == nil || summary.Status != wantStatus) {
		t.Fatalf("expected status %q, got %+v", wantStatus, summary)
	}
	return summary
}

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
		assertHostRunnerExecute(t, ctx, runner, AgentConfig{}, true, "")
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
		assertHostRunnerExecute(t, ctx, runner, cfg, true, "")
	})

	t.Run("successful host execution with temp-dir executable", func(t *testing.T) {
		tmpDir := t.TempDir()
		binPath := writeTestExecutableScript(t, tmpDir, "agy", "#!/bin/sh\necho \"executed agy\"\nexit 0\n")

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
		summary := assertHostRunnerExecute(t, ctx, runner, cfg, false, "completed")
		if summary.EstimatedCostUSD != 0.0 {
			t.Errorf("expected 0 cost for local-qwen-coder, got %f", summary.EstimatedCostUSD)
		}
	})

	errorCases := []struct {
		name       string
		makeCtx    func() (context.Context, context.CancelFunc)
		cmdName    string
		wantStatus string
	}{
		{
			name:       "interactive host execution and failed command",
			makeCtx:    func() (context.Context, context.CancelFunc) { return context.WithCancel(ctx) },
			cmdName:    "false",
			wantStatus: "failed",
		},
		{
			name: "curfew interrupted context cancellation",
			makeCtx: func() (context.Context, context.CancelFunc) {
				c, cancel := context.WithCancel(ctx)
				cancel()
				return c, cancel
			},
			cmdName:    "sleep",
			wantStatus: "curfew_interrupted",
		},
	}
	for _, ec := range errorCases {
		t.Run(ec.name, func(t *testing.T) {
			c, cancel := ec.makeCtx()
			defer cancel()
			var stdoutBuf, stderrBuf bytes.Buffer
			runner := &HostRunner{
				Interactive: true,
				Stdout:      &stdoutBuf,
				Stderr:      &stderrBuf,
				Stdin:       bytes.NewBufferString(""),
				LookPath: func(file string) (string, error) {
					return "/bin/" + ec.cmdName, nil
				},
				CommandFunc: func(cmdCtx context.Context, name string, arg ...string) *exec.Cmd {
					return exec.CommandContext(c, ec.cmdName)
				},
			}
			cfg := AgentConfig{
				Type:           AgentAider,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-test",
				WorkspaceDir:   t.TempDir(),
			}
			assertHostRunnerExecute(t, c, runner, cfg, true, ec.wantStatus)
		})
	}
}
