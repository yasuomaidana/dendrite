package agent

import (
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
