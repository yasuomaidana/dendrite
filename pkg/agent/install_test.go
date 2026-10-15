package agent

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestGetAgentInstallSpec_Fallbacks(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	tests := []struct {
		name          string
		agentType     AgentType
		mockLookPaths map[string]error
		wantBin       string
		wantCmd       string
		wantErr       bool
	}{
		{
			name:      "Aider with brew available",
			agentType: AgentAider,
			mockLookPaths: map[string]error{
				"brew": nil,
			},
			wantBin: "aider",
			wantCmd: "brew",
		},
		{
			name:      "Aider without brew fallback to pip3",
			agentType: AgentAider,
			mockLookPaths: map[string]error{
				"brew": exec.ErrNotFound,
			},
			wantBin: "aider",
			wantCmd: "pip3",
		},
		{
			name:      "Copilot with brew available",
			agentType: AgentCopilot,
			mockLookPaths: map[string]error{
				"brew": nil,
			},
			wantBin: "copilot",
			wantCmd: "brew",
		},
		{
			name:      "GitHub Copilot without brew fallback to npm",
			agentType: AgentGitHubCopilot,
			mockLookPaths: map[string]error{
				"brew": exec.ErrNotFound,
			},
			wantBin: "copilot",
			wantCmd: "npm",
		},
		{
			name:      "OpenHands with pip3 available",
			agentType: AgentOpenHands,
			mockLookPaths: map[string]error{
				"pip3": nil,
			},
			wantBin: "openhands",
			wantCmd: "pip3",
		},
		{
			name:      "OpenHands without pip3 fallback to docker",
			agentType: AgentOpenHands,
			mockLookPaths: map[string]error{
				"pip3": exec.ErrNotFound,
			},
			wantBin: "openhands",
			wantCmd: "docker",
		},
		{
			name:      "OpenCode standard npm spec",
			agentType: AgentOpenCode,
			wantBin:   "opencode",
			wantCmd:   "npm",
		},
		{
			name:      "AGY standard curl spec",
			agentType: AgentAGY,
			wantBin:   "agy",
			wantCmd:   "curl",
		},
		{
			name:      "Antigravity alias standard curl spec",
			agentType: AgentAntigravity,
			wantBin:   "agy",
			wantCmd:   "curl",
		},
		{
			name:      "Unsupported agent type returns error",
			agentType: "unknown-agent",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookPath = func(file string) (string, error) {
				if err, ok := tt.mockLookPaths[file]; ok {
					return file, err
				}
				return "/usr/bin/" + file, nil
			}

			spec, err := GetAgentInstallSpec(tt.agentType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetAgentInstallSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if spec.Binary != tt.wantBin {
					t.Errorf("spec.Binary = %s, want %s", spec.Binary, tt.wantBin)
				}
				if spec.Command != tt.wantCmd {
					t.Errorf("spec.Command = %s, want %s", spec.Command, tt.wantCmd)
				}
			}
		})
	}
}

func TestPromptAndInstallAgent_Workflows(t *testing.T) {
	origLookPath := lookPath
	origCmdContext := commandContext
	defer func() {
		lookPath = origLookPath
		commandContext = origCmdContext
	}()

	ctx := context.Background()

	t.Run("Already installed binary skips installation", func(t *testing.T) {
		lookPath = func(file string) (string, error) {
			return "/usr/local/bin/" + file, nil
		}
		var out bytes.Buffer
		err := PromptAndInstallAgent(ctx, AgentOpenCode, false, nil, &out)
		if err != nil {
			t.Fatalf("expected nil when binary is already installed, got: %v", err)
		}
		if out.Len() > 0 {
			t.Errorf("expected no output when already installed, got: %s", out.String())
		}
	})

	t.Run("Unsupported agent type returns error", func(t *testing.T) {
		err := PromptAndInstallAgent(ctx, "invalid-agent", false, nil, nil)
		if err == nil {
			t.Fatal("expected error for unsupported agent")
		}
	})

	t.Run("Interactive user approval with successful install and path discovery", func(t *testing.T) {
		lookPathCalls := 0
		lookPath = func(file string) (string, error) {
			lookPathCalls++
			if lookPathCalls == 1 {
				// Initially not found
				return "", exec.ErrNotFound
			}
			// Found after install
			return "/usr/local/bin/" + file, nil
		}
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.Command("true")
		}

		in := bytes.NewBufferString("y\n")
		var out bytes.Buffer
		err := PromptAndInstallAgent(ctx, AgentOpenCode, false, in, &out)
		if err != nil {
			t.Fatalf("expected successful install on 'y', got: %v", err)
		}
		if !strings.Contains(out.String(), "successfully installed") {
			t.Errorf("expected success message, got: %s", out.String())
		}
	})

	t.Run("Interactive user approval with 'yes'", func(t *testing.T) {
		first := true
		lookPath = func(file string) (string, error) {
			if first {
				first = false
				return "", exec.ErrNotFound
			}
			return "/usr/local/bin/" + file, nil
		}
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.Command("echo", "installed")
		}

		in := bytes.NewBufferString("yes\n")
		var out bytes.Buffer
		err := PromptAndInstallAgent(ctx, AgentOpenCode, false, in, &out)
		if err != nil {
			t.Fatalf("expected success on 'yes', got: %v", err)
		}
	})

	t.Run("Interactive user decline with EOF/empty input", func(t *testing.T) {
		lookPath = func(file string) (string, error) {
			return "", exec.ErrNotFound
		}
		in := bytes.NewBufferString("")
		var out bytes.Buffer
		err := PromptAndInstallAgent(ctx, AgentOpenCode, false, in, &out)
		if err == nil {
			t.Fatal("expected error when user input is empty")
		}
		if !strings.Contains(out.String(), "No response received") {
			t.Errorf("expected 'No response received' in output, got: %s", out.String())
		}
	})

	t.Run("Interactive user decline with 'no'", func(t *testing.T) {
		lookPath = func(file string) (string, error) {
			return "", exec.ErrNotFound
		}
		in := bytes.NewBufferString("no\n")
		var out bytes.Buffer
		err := PromptAndInstallAgent(ctx, AgentOpenCode, false, in, &out)
		if err == nil {
			t.Fatal("expected error on 'no'")
		}
		if !strings.Contains(out.String(), "Installation cancelled by user") {
			t.Errorf("expected cancellation message, got: %s", out.String())
		}
	})

	t.Run("AutoApprove true runs installer directly", func(t *testing.T) {
		lookPathCalls := 0
		lookPath = func(file string) (string, error) {
			lookPathCalls++
			if lookPathCalls == 1 {
				return "", exec.ErrNotFound
			}
			return "/usr/local/bin/" + file, nil
		}
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.Command("true")
		}

		var out bytes.Buffer
		err := PromptAndInstallAgent(ctx, AgentOpenCode, true, nil, &out)
		if err != nil {
			t.Fatalf("unexpected error with autoApprove: %v", err)
		}
		if !strings.Contains(out.String(), "Auto-approving installation") {
			t.Errorf("expected auto-approve message, got: %s", out.String())
		}
	})

	t.Run("AutoApprove true with nil in and out", func(t *testing.T) {
		lookPathCalls := 0
		lookPath = func(file string) (string, error) {
			lookPathCalls++
			if lookPathCalls == 1 {
				return "", exec.ErrNotFound
			}
			return "/usr/local/bin/" + file, nil
		}
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.Command("true")
		}

		err := PromptAndInstallAgent(ctx, AgentOpenCode, true, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error with nil in/out: %v", err)
		}
	})

	t.Run("Installer command execution failure", func(t *testing.T) {
		lookPath = func(file string) (string, error) {
			return "", exec.ErrNotFound
		}
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.Command("false")
		}

		var out bytes.Buffer
		err := PromptAndInstallAgent(ctx, AgentOpenCode, true, nil, &out)
		if err == nil {
			t.Fatal("expected error when install command fails")
		}
		if !strings.Contains(err.Error(), "failed to install") {
			t.Errorf("expected failed to install error, got: %v", err)
		}
	})

	t.Run("Installer succeeds but binary not found in PATH", func(t *testing.T) {
		lookPath = func(file string) (string, error) {
			// Always return not found
			return "", errors.New("not found in path")
		}
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.Command("true")
		}

		var out bytes.Buffer
		err := PromptAndInstallAgent(ctx, AgentOpenCode, true, nil, &out)
		if err != nil {
			t.Fatalf("expected nil even if binary not in path yet, got: %v", err)
		}
		if !strings.Contains(out.String(), "not yet in current PATH") {
			t.Errorf("expected PATH warning, got: %s", out.String())
		}
	})
}
