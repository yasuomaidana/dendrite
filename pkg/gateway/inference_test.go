package gateway

import (
	"strings"
	"testing"

	"Dendrite/pkg/inference"
)

func TestMultiEngineLiteLLMConfig_Rendering(t *testing.T) {
	mgr := inference.DefaultManager()
	engines := mgr.ListEngines()

	cfg := MultiEngineLiteLLMConfig(engines)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("multi-engine config validation failed: %v", err)
	}

	// Verify all engines are mapped into model_list
	modelNames := make(map[string]bool)
	for _, m := range cfg.ModelList {
		modelNames[m.ModelName] = true
		if m.ModelName == "oxillama-rust" || m.ModelName == "local-coder" || m.ModelName == "llamacpp-local" {
			if !m.ModelInfo.SkipBudgetChecks {
				t.Errorf("expected %s to have SkipBudgetChecks=true", m.ModelName)
			}
			if m.ModelInfo.InputCostPerToken != 0.0 {
				t.Errorf("expected %s to have zero input cost", m.ModelName)
			}
		}
	}

	expected := []string{"local-coder", "oxillama-rust", "vllm-cluster", "hf-deepseek", "llamacpp-local"}
	for _, exp := range expected {
		if !modelNames[exp] {
			t.Errorf("expected model %s to be registered in model list", exp)
		}
	}

	// Render YAML
	yamlBytes, err := RenderForInferenceManager(mgr)
	if err != nil {
		t.Fatalf("failed to render YAML for inference manager: %v", err)
	}
	yamlStr := string(yamlBytes)

	if !strings.Contains(yamlStr, "oxillama-rust") {
		t.Errorf("expected YAML to contain oxillama-rust")
	}
	if !strings.Contains(yamlStr, "skip_budget_checks: true") {
		t.Errorf("expected YAML to contain skip_budget_checks: true")
	}

	// Test with nil manager (defaults to DefaultManager)
	yamlBytesDefault, err := RenderForInferenceManager(nil)
	if err != nil || len(yamlBytesDefault) == 0 {
		t.Errorf("expected default render with nil manager")
	}
}

func TestIsZeroCostSelfHostedModel(t *testing.T) {
	tests := []struct {
		model    string
		zeroCost bool
	}{
		{"local-qwen-coder", true},
		{"oxillama-rust", true},
		{"vllm-cluster", true},
		{"hf-tgi-remote", true},
		{"llamacpp-local", true},
		{"ollama/qwen2.5-coder", true},
		{"oxillama/custom", true},
		{"vllm/qwen", true},
		{"llamacpp/mistral", true},
		{"huggingface-tei/bge", true},
		{"gemini-2.5-pro", false},
		{"copilot-sonnet", false},
		{"openai/gpt-4o", false},
	}

	for _, tt := range tests {
		got := IsZeroCostSelfHostedModel(tt.model)
		if got != tt.zeroCost {
			t.Errorf("IsZeroCostSelfHostedModel(%q) = %v, expected %v", tt.model, got, tt.zeroCost)
		}
	}
}

func TestBudgetLimiter_ExemptSelfHostedEngines(t *testing.T) {
	limiter := NewBudgetLimiter()
	limiter.RegisterKey(VirtualKeyConfig{
		KeyAlias: "agent-key",
		BudgetLimits: []BudgetLimit{
			{Duration: Window24h, MaxBudgetUSD: 0.001},
		},
	})

	// Record spend to exceed budget
	limiter.RecordSpend("agent-key", "gemini-2.5-pro", 1000, 0.05)

	// Commercial model should be blocked
	allowed, fallback, err := limiter.CheckBudget("agent-key", "gemini-2.5-pro")
	if err != nil || allowed {
		t.Errorf("expected commercial model to be disallowed when budget exceeded")
	}
	if fallback != "local-qwen-coder" {
		t.Errorf("expected fallback to local-qwen-coder, got %s", fallback)
	}

	// Self-hosted models should ALWAYS be allowed
	selfHostedModels := []string{"local-qwen-coder", "oxillama-rust", "vllm-cluster", "llamacpp-local"}
	for _, m := range selfHostedModels {
		allowed, selected, err := limiter.CheckBudget("agent-key", m)
		if err != nil || !allowed || selected != m {
			t.Errorf("expected self-hosted model %s to be allowed, got allowed=%v, selected=%s, err=%v", m, allowed, selected, err)
		}
	}
}
