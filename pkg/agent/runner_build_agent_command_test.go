package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

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

type scriptedRunner struct {
	executeFunc func(ctx context.Context, cfg AgentConfig) (*SessionSummary, error)
}

func (s *scriptedRunner) Execute(ctx context.Context, cfg AgentConfig) (*SessionSummary, error) {
	if s.executeFunc != nil {
		return s.executeFunc(ctx, cfg)
	}
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	now := time.Now()
	commitMsg := FormatCommitMessage(cfg.TicketID, cfg.Type, "Automated change via agent")
	return &SessionSummary{
		SessionID:        fmt.Sprintf("sess-%d", now.UnixNano()),
		AgentType:        cfg.Type,
		TicketID:         cfg.TicketID,
		StartTime:        now,
		EndTime:          now.Add(2 * time.Second),
		Duration:         2 * time.Second,
		TokensConsumed:   1250,
		EstimatedCostUSD: 0.0025,
		CommitsGenerated: []string{commitMsg},
		Status:           "completed",
	}, nil
}

func TestScriptedRunner(t *testing.T) {
	ctx := context.Background()

	t.Run("default scripted execution", func(t *testing.T) {
		runner := &scriptedRunner{}
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

	t.Run("scripted execution with agy", func(t *testing.T) {
		runner := &scriptedRunner{}
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

	t.Run("invalid config in scripted execution", func(t *testing.T) {
		runner := &scriptedRunner{}
		_, err := runner.Execute(ctx, AgentConfig{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("custom execute func", func(t *testing.T) {
		expectedErr := errors.New("custom failure")
		runner := &scriptedRunner{
			executeFunc: func(ctx context.Context, cfg AgentConfig) (*SessionSummary, error) {
				return nil, expectedErr
			},
		}

		_, err := runner.Execute(ctx, AgentConfig{})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected custom failure, got %v", err)
		}
	})
}
