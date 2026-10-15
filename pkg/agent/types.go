package agent

import (
	"fmt"
	"time"
)

// AgentType defines the supported autonomous coding agent engines.
type AgentType string

const (
	// AgentAider is the terminal-based Git pair programming agent.
	AgentAider AgentType = "aider"
	// AgentOpenHands is the autonomous containerized software engineering agent (formerly OpenDevin).
	AgentOpenHands AgentType = "openhands"
	// AgentOpenCode is the lightweight headless batch refactoring and code generation agent.
	AgentOpenCode AgentType = "opencode"
	// AgentAGY is Google's Antigravity autonomous coding agent CLI.
	AgentAGY AgentType = "agy"
	// AgentAntigravity is the canonical alias for Google Antigravity.
	AgentAntigravity AgentType = "antigravity"
	// AgentCopilot is GitHub Copilot CLI autonomous coding agent.
	AgentCopilot AgentType = "copilot"
	// AgentGitHubCopilot is the canonical alias for GitHub Copilot.
	AgentGitHubCopilot AgentType = "github-copilot"
)

// IsValid validates whether the AgentType is recognized.
func (a AgentType) IsValid() bool {
	switch a {
	case AgentAider, AgentOpenHands, AgentOpenCode, AgentAGY, AgentAntigravity, AgentCopilot, AgentGitHubCopilot:
		return true
	default:
		return false
	}
}

// AgentConfig represents the configuration payload required to launch and bound an agent.
type AgentConfig struct {
	Type            AgentType     `json:"type"`
	LiteLLMBaseURL  string        `json:"litellm_base_url"`
	VirtualAPIKey   string        `json:"virtual_api_key"`
	Model           string        `json:"model"`
	FallbackModel   string        `json:"fallback_model,omitempty"`
	TicketID        string        `json:"ticket_id,omitempty"`
	WorkspaceDir    string        `json:"workspace_dir"`
	Sandbox         bool          `json:"sandbox"`
	Ephemeral       bool          `json:"ephemeral"`
	SpendLimitUSD   float64       `json:"spend_limit_usd,omitempty"`
	MapTokens       int           `json:"map_tokens,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
	TaskDescription string        `json:"task_description,omitempty"`
	ExtraArgs       []string      `json:"extra_args,omitempty"`
}

// SessionSummary captures metrics and status after an agent execution run.
type SessionSummary struct {
	SessionID        string        `json:"session_id"`
	AgentType        AgentType     `json:"agent_type"`
	TicketID         string        `json:"ticket_id,omitempty"`
	StartTime        time.Time     `json:"start_time"`
	EndTime          time.Time     `json:"end_time"`
	Duration         time.Duration `json:"duration"`
	TokensConsumed   int           `json:"tokens_consumed"`
	EstimatedCostUSD float64       `json:"estimated_cost_usd"`
	CommitsGenerated []string      `json:"commits_generated,omitempty"`
	Status           string        `json:"status"`
}

// ValidateConfig verifies that required configuration values are populated and structurally sound.
func ValidateConfig(cfg AgentConfig) error {
	if !cfg.Type.IsValid() {
		return fmt.Errorf("unsupported agent type: %q (must be aider, openhands, opencode, agy, or copilot)", cfg.Type)
	}
	if cfg.LiteLLMBaseURL == "" {
		return fmt.Errorf("litellm_base_url is required")
	}
	if cfg.VirtualAPIKey == "" {
		return fmt.Errorf("virtual_api_key is required")
	}
	if cfg.WorkspaceDir == "" {
		return fmt.Errorf("workspace_dir is required")
	}
	if cfg.SpendLimitUSD < 0 {
		return fmt.Errorf("spend_limit_usd cannot be negative")
	}
	return nil
}
