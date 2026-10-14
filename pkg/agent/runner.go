package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BuildAgentEnv constructs the environment variable mappings required by the selected agent tool.
func BuildAgentEnv(cfg AgentConfig) (map[string]string, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	env := make(map[string]string)

	switch cfg.Type {
	case AgentAider:
		env["OPENAI_API_BASE"] = cfg.LiteLLMBaseURL
		env["OPENAI_API_KEY"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["AIDER_MODEL"] = cfg.Model
		}
		if cfg.MapTokens > 0 {
			env["AIDER_MAP_TOKENS"] = strconv.Itoa(cfg.MapTokens)
		}

	case AgentOpenHands:
		env["LLM_BASE_URL"] = cfg.LiteLLMBaseURL
		env["LLM_API_KEY"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["LLM_MODEL"] = cfg.Model
		}
		env["WORKSPACE_MOUNT_PATH"] = cfg.WorkspaceDir
		if cfg.Sandbox {
			env["SANDBOX_CONTAINER_IMAGE"] = "docker.all-hands.dev/all-hands-ai/runtime:0.14-nikolaik"
		}

	case AgentOpenCode:
		env["OPENCODE_GATEWAY_URL"] = cfg.LiteLLMBaseURL
		env["OPENCODE_API_KEY"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["OPENCODE_MODEL"] = cfg.Model
		}
		if cfg.SpendLimitUSD > 0 {
			env["OPENCODE_SPEND_LIMIT"] = fmt.Sprintf("%.2f", cfg.SpendLimitUSD)
		}

	case AgentAGY, AgentAntigravity:
		env["GEMINI_API_KEY"] = cfg.VirtualAPIKey
		env["GOOGLE_GEMINI_BASE_URL"] = cfg.LiteLLMBaseURL
		env["OPENAI_API_BASE"] = cfg.LiteLLMBaseURL
		env["OPENAI_API_KEY"] = cfg.VirtualAPIKey
		if cfg.Model != "" {
			env["AGY_MODEL"] = cfg.Model
		}
	}

	return env, nil
}

// BuildAgentCommand creates the process argument list for launching the agent.
func BuildAgentCommand(cfg AgentConfig) ([]string, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	var cmd []string

	switch cfg.Type {
	case AgentAider:
		cmd = []string{"aider", "--openai-api-base", cfg.LiteLLMBaseURL}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}
		if cfg.TicketID != "" {
			cmd = append(cmd, "--message-template", fmt.Sprintf("[%s] {msg}", cfg.TicketID))
		}
		if cfg.MapTokens > 0 {
			cmd = append(cmd, "--map-tokens", strconv.Itoa(cfg.MapTokens))
		}
		if cfg.TaskDescription != "" {
			cmd = append(cmd, "--message", cfg.TaskDescription)
		}

	case AgentOpenHands:
		cmd = []string{"openhands", "--workspace", cfg.WorkspaceDir}
		if cfg.TaskDescription != "" {
			cmd = append(cmd, "--task", cfg.TaskDescription)
		}
		if cfg.Sandbox {
			cmd = append(cmd, "--sandbox")
		}

	case AgentOpenCode:
		cmd = []string{"opencode", "exec", "--workspace", cfg.WorkspaceDir}
		if cfg.TaskDescription != "" {
			cmd = append(cmd, "--prompt", cfg.TaskDescription)
		}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}

	case AgentAGY, AgentAntigravity:
		cmd = []string{"agy", "--add-dir", cfg.WorkspaceDir}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}
		if cfg.Sandbox {
			cmd = append(cmd, "--sandbox")
		}
		if cfg.TaskDescription != "" {
			cmd = append(cmd, "--dangerously-skip-permissions", "-p")
			if cfg.TicketID != "" {
				cmd = append(cmd, fmt.Sprintf("[%s] %s", cfg.TicketID, cfg.TaskDescription))
			} else {
				cmd = append(cmd, cfg.TaskDescription)
			}
		}
	}

	if len(cfg.ExtraArgs) > 0 {
		cmd = append(cmd, cfg.ExtraArgs...)
	}

	return cmd, nil
}

// FormatCommitMessage creates a standard Dendrite-compliant commit message with ticket ID and agent provenance.
func FormatCommitMessage(ticketID string, agent AgentType, summary string) string {
	trimmedSummary := strings.TrimSpace(summary)
	trimmedTicket := strings.TrimSpace(ticketID)

	if trimmedTicket == "" {
		return fmt.Sprintf("%s (Agent: %s)", trimmedSummary, agent)
	}
	return fmt.Sprintf("[%s] %s (Agent: %s)", trimmedTicket, trimmedSummary, agent)
}

// Runner defines the interface for orchestrating agent lifecycles.
type Runner interface {
	Execute(ctx context.Context, cfg AgentConfig) (*SessionSummary, error)
}

// MockRunner provides a deterministic in-memory agent runner for test suites and dry-runs.
type MockRunner struct {
	ExecuteFunc func(ctx context.Context, cfg AgentConfig) (*SessionSummary, error)
}

// Execute runs the simulated or injected agent lifecycle.
func (m *MockRunner) Execute(ctx context.Context, cfg AgentConfig) (*SessionSummary, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, cfg)
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
