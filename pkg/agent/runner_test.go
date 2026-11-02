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
	tests := []struct {
		name     string
		cfg      AgentConfig
		wantEnv  map[string]string
		checkEnv func(t *testing.T, env map[string]string)
		wantErr  bool
	}{
		{
			name: "aider environment",
			cfg: AgentConfig{
				Type:           AgentAider,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-test-aider",
				Model:          "copilot-sonnet",
				WorkspaceDir:   "/workspace",
				MapTokens:      2048,
			},
			wantEnv: map[string]string{
				"OPENAI_API_BASE":  "http://localhost:4000/v1",
				"OPENAI_API_KEY":   "sk-test-aider",
				"AIDER_MODEL":      "copilot-sonnet",
				"AIDER_MAP_TOKENS": "2048",
			},
		},
		{
			name: "openhands environment with sandbox",
			cfg: AgentConfig{
				Type:           AgentOpenHands,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-test-openhands",
				Model:          "gemini-2.5-pro",
				WorkspaceDir:   "/workspace",
				Sandbox:        true,
			},
			wantEnv: map[string]string{
				"LLM_BASE_URL":         "http://localhost:4000/v1",
				"LLM_API_KEY":          "sk-test-openhands",
				"LLM_MODEL":            "gemini-2.5-pro",
				"WORKSPACE_MOUNT_PATH": "/workspace",
			},
			checkEnv: func(t *testing.T, env map[string]string) {
				if env["SANDBOX_CONTAINER_IMAGE"] == "" {
					t.Errorf("expected SANDBOX_CONTAINER_IMAGE to be set")
				}
			},
		},
		{
			name: "opencode environment with spend limit",
			cfg: AgentConfig{
				Type:           AgentOpenCode,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-test-opencode",
				Model:          "local-qwen-coder",
				WorkspaceDir:   "/workspace",
				SpendLimitUSD:  5.50,
			},
			wantEnv: map[string]string{
				"OPENCODE_GATEWAY_URL": "http://localhost:4000/v1",
				"OPENCODE_API_KEY":     "sk-test-opencode",
				"OPENCODE_MODEL":       "local-qwen-coder",
				"OPENCODE_SPEND_LIMIT": "5.50",
			},
		},
		{
			name: "agy environment with gemini and openai bindings",
			cfg: AgentConfig{
				Type:           AgentAGY,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "sk-test-agy",
				Model:          "gemini-2.5-pro",
				WorkspaceDir:   "/workspace",
			},
			wantEnv: map[string]string{
				"GEMINI_API_KEY":         "sk-test-agy",
				"GOOGLE_GEMINI_BASE_URL": "http://localhost:4000/v1",
				"OPENAI_API_BASE":        "http://localhost:4000/v1",
				"OPENAI_API_KEY":         "sk-test-agy",
				"AGY_MODEL":              "gemini-2.5-pro",
			},
		},
		{
			name: "copilot environment",
			cfg: AgentConfig{
				Type:           AgentCopilot,
				LiteLLMBaseURL: "http://localhost:4000/v1",
				VirtualAPIKey:  "ghp_mock_token",
				Model:          "claude-sonnet-4.5",
				WorkspaceDir:   "/workspace",
			},
			wantEnv: map[string]string{
				"COPILOT_API_KEY": "ghp_mock_token",
				"GITHUB_TOKEN":    "ghp_mock_token",
				"COPILOT_MODEL":   "claude-sonnet-4.5",
			},
		},
		{
			name:    "invalid config returns error",
			cfg:     AgentConfig{},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, err := BuildAgentEnv(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error for empty config, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, wantVal := range tc.wantEnv {
				if env[k] != wantVal {
					t.Errorf("expected %s match %s, got %s", k, wantVal, env[k])
				}
			}
			if tc.checkEnv != nil {
				tc.checkEnv(t, env)
			}
		})
	}
}
