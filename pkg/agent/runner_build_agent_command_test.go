package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// agentBase fills the fields every agent configuration shares.
func agentBase(typ AgentType, mutate func(*AgentConfig)) AgentConfig {
	cfg := AgentConfig{
		Type:           typ,
		LiteLLMBaseURL: "http://localhost:4000/v1",
		VirtualAPIKey:  "sk-test",
		WorkspaceDir:   "/workspace",
	}
	mutate(&cfg)
	return cfg
}

func TestBuildAgentCommand(t *testing.T) {
	cases := []struct {
		name string
		cfg  AgentConfig
		want []string
	}{
		{"aider command with ticket and map tokens", agentBase(AgentAider, func(c *AgentConfig) {
			c.Model, c.TicketID, c.MapTokens = "copilot-sonnet", "DEN-006", 1024
			c.TaskDescription, c.ExtraArgs = "Refactor pkg/ast", []string{"--auto-commits"}
		}), []string{
			"aider", "--openai-api-base", "http://localhost:4000/v1", "--model", "copilot-sonnet",
			"--message-template", "[DEN-006] {msg}", "--map-tokens", "1024",
			"--message", "Refactor pkg/ast", "--auto-commits",
		}},
		{"openhands command with sandbox flag", agentBase(AgentOpenHands, func(c *AgentConfig) {
			c.TaskDescription, c.Sandbox = "Fix issue 42", true
		}), []string{"openhands", "--workspace", "/workspace", "--task", "Fix issue 42", "--sandbox"}},
		{"opencode command with model", agentBase(AgentOpenCode, func(c *AgentConfig) {
			c.Model, c.TaskDescription = "local-qwen-coder", "Run unit tests"
		}), []string{"opencode", "exec", "--workspace", "/workspace", "--prompt", "Run unit tests", "--model", "local-qwen-coder"}},
		{"agy command with prompt ticket and sandbox", agentBase(AgentAGY, func(c *AgentConfig) {
			c.Model, c.TicketID, c.TaskDescription = "gemini-2.5-pro", "DEN-028", "Integrate AGY CLI"
			c.Sandbox, c.ExtraArgs = true, []string{"--effort", "high"}
		}), []string{
			"agy", "--add-dir", "/workspace", "--model", "gemini-2.5-pro", "--sandbox",
			"--dangerously-skip-permissions", "-p", "[DEN-028] Integrate AGY CLI", "--effort", "high",
		}},
		{"copilot command with prompt and ticket", agentBase(AgentCopilot, func(c *AgentConfig) {
			c.Model, c.TicketID, c.TaskDescription = "claude-sonnet-4.5", "DEN-072", "Develop copilot runtime adapter"
		}), []string{
			"copilot", "-C", "/workspace", "--model", "claude-sonnet-4.5", "--allow-all",
			"-p", "[DEN-072] Develop copilot runtime adapter",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd, err := BuildAgentCommand(c.cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(cmd, c.want) {
				t.Fatalf("command = %v, want %v", cmd, c.want)
			}
		})
	}

	t.Run("copilot environment", func(t *testing.T) {
		cfg := agentBase(AgentCopilot, func(c *AgentConfig) {
			c.VirtualAPIKey, c.Model = "ghp_mock_token", "claude-sonnet-4.5"
		})
		env, err := BuildAgentEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for key, want := range map[string]string{
			"COPILOT_API_KEY": "ghp_mock_token", "GITHUB_TOKEN": "ghp_mock_token", "COPILOT_MODEL": "claude-sonnet-4.5",
		} {
			if env[key] != want {
				t.Errorf("%s = %q, want %q", key, env[key], want)
			}
		}
	})

	t.Run("invalid config returns error", func(t *testing.T) {
		if _, err := BuildAgentCommand(AgentConfig{}); err == nil {
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

	for _, c := range []struct {
		name   string
		typ    AgentType
		ticket string
	}{{"default scripted execution", AgentAider, "DEN-006"}, {"scripted execution with agy", AgentAGY, "DEN-028"}} {
		t.Run(c.name, func(t *testing.T) {
			cfg := agentBase(c.typ, func(cfg *AgentConfig) { cfg.TicketID = c.ticket })
			summary, err := (&scriptedRunner{}).Execute(ctx, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if summary.AgentType != c.typ || summary.TicketID != c.ticket || summary.Status != "completed" || len(summary.CommitsGenerated) == 0 {
				t.Errorf("unexpected summary: %+v", summary)
			}
		})
	}

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
