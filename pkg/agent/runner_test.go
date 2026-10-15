package agent

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     AgentConfig
		wantErr bool
	}{
		{
			name: "valid aider config",
			cfg: AgentConfig{
				Type:           AgentAider,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-dendrite-virtual",
				WorkspaceDir:   "/tmp/workspace",
			},
			wantErr: false,
		},
		{
			name: "valid agy config",
			cfg: AgentConfig{
				Type:           AgentAGY,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-dendrite-virtual",
				WorkspaceDir:   "/tmp/workspace",
			},
			wantErr: false,
		},
		{
			name: "valid antigravity alias config",
			cfg: AgentConfig{
				Type:           AgentAntigravity,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-dendrite-virtual",
				WorkspaceDir:   "/tmp/workspace",
			},
			wantErr: false,
		},
		{
			name: "invalid agent type",
			cfg: AgentConfig{
				Type:           "unknown",
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-dendrite-virtual",
				WorkspaceDir:   "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing litellm url",
			cfg: AgentConfig{
				Type:          AgentOpenHands,
				VirtualAPIKey: "sk-dendrite-virtual",
				WorkspaceDir:  "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing virtual api key",
			cfg: AgentConfig{
				Type:           AgentOpenCode,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				WorkspaceDir:   "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing workspace dir",
			cfg: AgentConfig{
				Type:           AgentAider,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-dendrite-virtual",
			},
			wantErr: true,
		},
		{
			name: "negative spend limit",
			cfg: AgentConfig{
				Type:           AgentAider,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-dendrite-virtual",
				WorkspaceDir:   "/tmp/workspace",
				SpendLimitUSD:  -10.0,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConfig(tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateConfig() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestBuildAgentEnv(t *testing.T) {
	t.Run("aider environment", func(t *testing.T) {
		cfg := AgentConfig{
			Type:           AgentAider,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test-aider",
			Model:          "copilot-sonnet",
			WorkspaceDir:   "/workspace",
			MapTokens:      2048,
		}

		env, err := BuildAgentEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if env["OPENAI_API_BASE"] != "http://localhost:4000/v1" {
			t.Errorf("expected OPENAI_API_BASE to match, got %s", env["OPENAI_API_BASE"])
		}
		if env["OPENAI_API_KEY"] != "sk-test-aider" {
			t.Errorf("expected OPENAI_API_KEY to match, got %s", env["OPENAI_API_KEY"])
		}
		if env["AIDER_MODEL"] != "copilot-sonnet" {
			t.Errorf("expected AIDER_MODEL copilot-sonnet, got %s", env["AIDER_MODEL"])
		}
		if env["AIDER_MAP_TOKENS"] != "2048" {
			t.Errorf("expected AIDER_MAP_TOKENS 2048, got %s", env["AIDER_MAP_TOKENS"])
		}
	})

	t.Run("openhands environment with sandbox", func(t *testing.T) {
		cfg := AgentConfig{
			Type:           AgentOpenHands,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test-openhands",
			Model:          "gemini-2.5-pro",
			WorkspaceDir:   "/workspace",
			Sandbox:        true,
		}

		env, err := BuildAgentEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if env["LLM_BASE_URL"] != "http://localhost:4000/v1" {
			t.Errorf("expected LLM_BASE_URL match, got %s", env["LLM_BASE_URL"])
		}
		if env["LLM_API_KEY"] != "sk-test-openhands" {
			t.Errorf("expected LLM_API_KEY match, got %s", env["LLM_API_KEY"])
		}
		if env["LLM_MODEL"] != "gemini-2.5-pro" {
			t.Errorf("expected LLM_MODEL gemini-2.5-pro, got %s", env["LLM_MODEL"])
		}
		if env["WORKSPACE_MOUNT_PATH"] != "/workspace" {
			t.Errorf("expected WORKSPACE_MOUNT_PATH /workspace, got %s", env["WORKSPACE_MOUNT_PATH"])
		}
		if env["SANDBOX_CONTAINER_IMAGE"] == "" {
			t.Errorf("expected SANDBOX_CONTAINER_IMAGE to be set")
		}
	})

	t.Run("opencode environment with spend limit", func(t *testing.T) {
		cfg := AgentConfig{
			Type:           AgentOpenCode,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test-opencode",
			Model:          "local-qwen-coder",
			WorkspaceDir:   "/workspace",
			SpendLimitUSD:  5.50,
		}

		env, err := BuildAgentEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if env["OPENCODE_GATEWAY_URL"] != "http://localhost:4000/v1" {
			t.Errorf("expected OPENCODE_GATEWAY_URL match, got %s", env["OPENCODE_GATEWAY_URL"])
		}
		if env["OPENCODE_API_KEY"] != "sk-test-opencode" {
			t.Errorf("expected OPENCODE_API_KEY match, got %s", env["OPENCODE_API_KEY"])
		}
		if env["OPENCODE_MODEL"] != "local-qwen-coder" {
			t.Errorf("expected OPENCODE_MODEL local-qwen-coder, got %s", env["OPENCODE_MODEL"])
		}
		if env["OPENCODE_SPEND_LIMIT"] != "5.50" {
			t.Errorf("expected OPENCODE_SPEND_LIMIT 5.50, got %s", env["OPENCODE_SPEND_LIMIT"])
		}
	})

	t.Run("agy environment with gemini and openai bindings", func(t *testing.T) {
		cfg := AgentConfig{
			Type:           AgentAGY,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test-agy",
			Model:          "gemini-2.5-pro",
			WorkspaceDir:   "/workspace",
		}

		env, err := BuildAgentEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if env["GEMINI_API_KEY"] != "sk-test-agy" {
			t.Errorf("expected GEMINI_API_KEY match, got %s", env["GEMINI_API_KEY"])
		}
		if env["GOOGLE_GEMINI_BASE_URL"] != "http://localhost:4000/v1" {
			t.Errorf("expected GOOGLE_GEMINI_BASE_URL match, got %s", env["GOOGLE_GEMINI_BASE_URL"])
		}
		if env["OPENAI_API_BASE"] != "http://localhost:4000/v1" {
			t.Errorf("expected OPENAI_API_BASE match, got %s", env["OPENAI_API_BASE"])
		}
		if env["OPENAI_API_KEY"] != "sk-test-agy" {
			t.Errorf("expected OPENAI_API_KEY match, got %s", env["OPENAI_API_KEY"])
		}
		if env["AGY_MODEL"] != "gemini-2.5-pro" {
			t.Errorf("expected AGY_MODEL gemini-2.5-pro, got %s", env["AGY_MODEL"])
		}
	})

	t.Run("copilot environment", func(t *testing.T) {
		cfg := AgentConfig{
			Type:           AgentCopilot,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "ghp_mock_token",
			Model:          "claude-sonnet-4.5",
			WorkspaceDir:   "/workspace",
		}

		env, err := BuildAgentEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env["COPILOT_API_KEY"] != "ghp_mock_token" {
			t.Errorf("expected COPILOT_API_KEY match, got %s", env["COPILOT_API_KEY"])
		}
		if env["GITHUB_TOKEN"] != "ghp_mock_token" {
			t.Errorf("expected GITHUB_TOKEN match, got %s", env["GITHUB_TOKEN"])
		}
		if env["COPILOT_MODEL"] != "claude-sonnet-4.5" {
			t.Errorf("expected COPILOT_MODEL claude-sonnet-4.5, got %s", env["COPILOT_MODEL"])
		}
	})

	t.Run("invalid config returns error", func(t *testing.T) {
		_, err := BuildAgentEnv(AgentConfig{})
		if err == nil {
			t.Fatal("expected error for empty config, got nil")
		}
	})
}

func TestBuildAgentCommand(t *testing.T) {
	t.Run("aider command with ticket and map tokens", func(t *testing.T) {
		cfg := AgentConfig{
			Type:            AgentAider,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			Model:           "copilot-sonnet",
			TicketID:        "DEN-006",
			WorkspaceDir:    "/workspace",
			MapTokens:       1024,
			TaskDescription: "Refactor pkg/ast",
			ExtraArgs:       []string{"--auto-commits"},
		}

		cmd, err := BuildAgentCommand(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []string{
			"aider",
			"--openai-api-base", "http://localhost:4000/v1",
			"--model", "copilot-sonnet",
			"--message-template", "[DEN-006] {msg}",
			"--map-tokens", "1024",
			"--message", "Refactor pkg/ast",
			"--auto-commits",
		}

		if len(cmd) != len(expected) {
			t.Fatalf("expected len %d, got %d: %v", len(expected), len(cmd), cmd)
		}
		for i, v := range expected {
			if cmd[i] != v {
				t.Errorf("at index %d: expected %s, got %s", i, v, cmd[i])
			}
		}
	})

	t.Run("openhands command with sandbox flag", func(t *testing.T) {
		cfg := AgentConfig{
			Type:            AgentOpenHands,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			WorkspaceDir:    "/workspace",
			TaskDescription: "Fix issue 42",
			Sandbox:         true,
		}

		cmd, err := BuildAgentCommand(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []string{
			"openhands",
			"--workspace", "/workspace",
			"--task", "Fix issue 42",
			"--sandbox",
		}

		if len(cmd) != len(expected) {
			t.Fatalf("expected len %d, got %d: %v", len(expected), len(cmd), cmd)
		}
		for i, v := range expected {
			if cmd[i] != v {
				t.Errorf("at index %d: expected %s, got %s", i, v, cmd[i])
			}
		}
	})

	t.Run("opencode command with model", func(t *testing.T) {
		cfg := AgentConfig{
			Type:            AgentOpenCode,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			WorkspaceDir:    "/workspace",
			Model:           "local-qwen-coder",
			TaskDescription: "Run unit tests",
		}

		cmd, err := BuildAgentCommand(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []string{
			"opencode", "exec",
			"--workspace", "/workspace",
			"--prompt", "Run unit tests",
			"--model", "local-qwen-coder",
		}

		if len(cmd) != len(expected) {
			t.Fatalf("expected len %d, got %d: %v", len(expected), len(cmd), cmd)
		}
		for i, v := range expected {
			if cmd[i] != v {
				t.Errorf("at index %d: expected %s, got %s", i, v, cmd[i])
			}
		}
	})

	t.Run("agy command with prompt ticket and sandbox", func(t *testing.T) {
		cfg := AgentConfig{
			Type:            AgentAGY,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			WorkspaceDir:    "/workspace",
			Model:           "gemini-2.5-pro",
			TicketID:        "DEN-028",
			TaskDescription: "Integrate AGY CLI",
			Sandbox:         true,
			ExtraArgs:       []string{"--effort", "high"},
		}

		cmd, err := BuildAgentCommand(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []string{
			"agy",
			"--add-dir", "/workspace",
			"--model", "gemini-2.5-pro",
			"--sandbox",
			"--dangerously-skip-permissions",
			"-p", "[DEN-028] Integrate AGY CLI",
			"--effort", "high",
		}

		if len(cmd) != len(expected) {
			t.Fatalf("expected len %d, got %d: %v", len(expected), len(cmd), cmd)
		}
		for i, v := range expected {
			if cmd[i] != v {
				t.Errorf("at index %d: expected %s, got %s", i, v, cmd[i])
			}
		}
	})

	t.Run("copilot environment", func(t *testing.T) {
		cfg := AgentConfig{
			Type:           AgentCopilot,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "ghp_mock_token",
			Model:          "claude-sonnet-4.5",
			WorkspaceDir:   "/workspace",
		}

		env, err := BuildAgentEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env["COPILOT_API_KEY"] != "ghp_mock_token" {
			t.Errorf("expected COPILOT_API_KEY match, got %s", env["COPILOT_API_KEY"])
		}
		if env["GITHUB_TOKEN"] != "ghp_mock_token" {
			t.Errorf("expected GITHUB_TOKEN match, got %s", env["GITHUB_TOKEN"])
		}
		if env["COPILOT_MODEL"] != "claude-sonnet-4.5" {
			t.Errorf("expected COPILOT_MODEL claude-sonnet-4.5, got %s", env["COPILOT_MODEL"])
		}
	})

	t.Run("copilot command with prompt and ticket", func(t *testing.T) {
		cfg := AgentConfig{
			Type:            AgentCopilot,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			WorkspaceDir:    "/workspace",
			Model:           "claude-sonnet-4.5",
			TicketID:        "DEN-072",
			TaskDescription: "Develop copilot runtime adapter",
		}

		cmd, err := BuildAgentCommand(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := []string{
			"copilot",
			"-C", "/workspace",
			"--model", "claude-sonnet-4.5",
			"--allow-all",
			"-p", "[DEN-072] Develop copilot runtime adapter",
		}

		if len(cmd) != len(expected) {
			t.Fatalf("expected len %d, got %d: %v", len(expected), len(cmd), cmd)
		}
		for i, v := range expected {
			if cmd[i] != v {
				t.Errorf("at index %d: expected %s, got %s", i, v, cmd[i])
			}
		}
	})

	t.Run("invalid config returns error", func(t *testing.T) {
		_, err := BuildAgentCommand(AgentConfig{})
		if err == nil {
			t.Fatal("expected error for empty config, got nil")
		}
	})
}

func TestFormatCommitMessage(t *testing.T) {
	t.Run("with ticket id", func(t *testing.T) {
		msg := FormatCommitMessage("DEN-006", AgentAider, "Implement cloud-init runner")
		expected := "[DEN-006] Implement cloud-init runner (Agent: aider)"
		if msg != expected {
			t.Errorf("expected %q, got %q", expected, msg)
		}
	})

	t.Run("with agy agent", func(t *testing.T) {
		msg := FormatCommitMessage("DEN-028", AgentAGY, "Add Antigravity runner")
		expected := "[DEN-028] Add Antigravity runner (Agent: agy)"
		if msg != expected {
			t.Errorf("expected %q, got %q", expected, msg)
		}
	})

	t.Run("without ticket id", func(t *testing.T) {
		msg := FormatCommitMessage("", AgentOpenHands, "Fix syntax error")
		expected := "Fix syntax error (Agent: openhands)"
		if msg != expected {
			t.Errorf("expected %q, got %q", expected, msg)
		}
	})
}

func TestMockRunner(t *testing.T) {
	ctx := context.Background()

	t.Run("default mock execution", func(t *testing.T) {
		runner := &MockRunner{}
		cfg := AgentConfig{
			Type:           AgentAider,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   "/workspace",
			TicketID:       "DEN-006",
		}

		summary, err := runner.Execute(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if summary.AgentType != AgentAider {
			t.Errorf("expected AgentAider, got %s", summary.AgentType)
		}
		if summary.TicketID != "DEN-006" {
			t.Errorf("expected DEN-006, got %s", summary.TicketID)
		}
		if summary.Status != "completed" {
			t.Errorf("expected completed, got %s", summary.Status)
		}
		if len(summary.CommitsGenerated) == 0 {
			t.Errorf("expected at least one commit generated")
		}
	})

	t.Run("mock execution with agy", func(t *testing.T) {
		runner := &MockRunner{}
		cfg := AgentConfig{
			Type:           AgentAGY,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   "/workspace",
			TicketID:       "DEN-028",
		}

		summary, err := runner.Execute(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if summary.AgentType != AgentAGY {
			t.Errorf("expected AgentAGY, got %s", summary.AgentType)
		}
		if summary.TicketID != "DEN-028" {
			t.Errorf("expected DEN-028, got %s", summary.TicketID)
		}
	})

	t.Run("invalid config in mock execution", func(t *testing.T) {
		runner := &MockRunner{}
		_, err := runner.Execute(ctx, AgentConfig{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("custom execute func", func(t *testing.T) {
		expectedErr := errors.New("custom failure")
		runner := &MockRunner{
			ExecuteFunc: func(ctx context.Context, cfg AgentConfig) (*SessionSummary, error) {
				return nil, expectedErr
			},
		}

		_, err := runner.Execute(ctx, AgentConfig{})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected custom failure, got %v", err)
		}
	})
}

func TestHostRunner(t *testing.T) {
	ctx := context.Background()

	t.Run("executable not found in PATH returns actionable error", func(t *testing.T) {
		runner := &HostRunner{
			LookPath: func(file string) (string, error) {
				return "", errors.New("not found")
			},
		}
		cfg := AgentConfig{
			Type:           AgentAider,
			LiteLLMBaseURL: "http://localhost:4000/v1",
			VirtualAPIKey:  "sk-test",
			WorkspaceDir:   "/tmp",
		}
		_, err := runner.Execute(ctx, cfg)
		if err == nil {
			t.Fatal("expected error for missing executable, got nil")
		}
	})

	t.Run("successful host execution with mocked command", func(t *testing.T) {
		runner := &HostRunner{
			LookPath: func(file string) (string, error) {
				return "/usr/bin/echo", nil
			},
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "echo", "mock-run")
			},
		}
		cfg := AgentConfig{
			Type:            AgentAGY,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-test",
			WorkspaceDir:    "/tmp",
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
	})
}
