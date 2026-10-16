package gateway_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Dendrite/pkg/gateway"
	"Dendrite/pkg/topology"
)

func TestDefaultLiteLLMConfig(t *testing.T) {
	cfg := gateway.DefaultLiteLLMConfig("http://100.84.12.25:11434")
	if cfg == nil {
		t.Fatalf("expected non-nil config")
	}

	if len(cfg.ModelList) < 4 {
		t.Fatalf("expected 4 models in default model list, got %d", len(cfg.ModelList))
	}

	modelsFound := make(map[string]gateway.ModelListEntry)
	for _, m := range cfg.ModelList {
		modelsFound[m.ModelName] = m
	}

	// 1. Verify gemini-2.5-pro
	pro, ok := modelsFound["gemini-2.5-pro"]
	if !ok {
		t.Fatalf("missing gemini-2.5-pro")
	}
	if pro.ModelInfo.MaxInputTokens != 1048576 {
		t.Errorf("expected 1M input tokens for pro, got %d", pro.ModelInfo.MaxInputTokens)
	}

	// 2. Verify copilot-sonnet
	sonnet, ok := modelsFound["copilot-sonnet"]
	if !ok {
		t.Fatalf("missing copilot-sonnet")
	}
	if !sonnet.LitellmParams.DropParams {
		t.Errorf("expected copilot-sonnet to drop unsupported client params")
	}
	if sonnet.LitellmParams.ExtraHeaders["Editor-Version"] == "" {
		t.Errorf("expected Editor-Version header in copilot-sonnet")
	}

	// 3. Verify local-qwen-coder
	qwen, ok := modelsFound["local-qwen-coder"]
	if !ok {
		t.Fatalf("missing local-qwen-coder")
	}
	if qwen.ModelInfo.InputCostPerToken != 0.0 || qwen.ModelInfo.OutputCostPerToken != 0.0 {
		t.Errorf("expected zero token cost for local-qwen-coder")
	}
	if !qwen.ModelInfo.SkipBudgetChecks {
		t.Errorf("expected skip_budget_checks to be true for local-qwen-coder")
	}
	if qwen.LitellmParams.APIBase != "http://100.84.12.25:11434" {
		t.Errorf("expected custom APIBase http://100.84.12.25:11434, got %s", qwen.LitellmParams.APIBase)
	}

	// Check router settings
	if cfg.RouterSettings == nil {
		t.Fatalf("expected non-nil router settings")
	}
	if len(cfg.RouterSettings.Fallbacks) == 0 {
		t.Errorf("expected fallback cascades configured")
	}
	if len(cfg.RouterSettings.ContextWindowFallbacks) == 0 {
		t.Errorf("expected context window fallbacks configured")
	}
}

func TestValidationErrors(t *testing.T) {
	// Nil config
	var nilCfg *gateway.LiteLLMConfig
	if err := nilCfg.Validate(); err == nil {
		t.Errorf("expected error validating nil config")
	}

	// Empty model list
	emptyList := &gateway.LiteLLMConfig{ModelList: []gateway.ModelListEntry{}}
	if err := emptyList.Validate(); err == nil {
		t.Errorf("expected error for empty model list")
	}

	// Empty model name
	emptyName := &gateway.LiteLLMConfig{
		ModelList: []gateway.ModelListEntry{
			{
				ModelName: "",
				LitellmParams: gateway.ModelParams{
					Model: "gemini/gemini-2.5-pro",
				},
			},
		},
	}
	if err := emptyName.Validate(); err == nil {
		t.Errorf("expected error for empty model name")
	}

	// Empty upstream provider model
	emptyProvider := &gateway.LiteLLMConfig{
		ModelList: []gateway.ModelListEntry{
			{
				ModelName: "test-model",
				LitellmParams: gateway.ModelParams{
					Model: "",
				},
			},
		},
	}
	if err := emptyProvider.Validate(); err == nil {
		t.Errorf("expected error for empty provider model")
	}
}

func TestRenderYAML(t *testing.T) {
	cfg := gateway.DefaultLiteLLMConfig("")
	data, err := gateway.RenderYAML(cfg)
	if err != nil {
		t.Fatalf("unexpected error rendering YAML: %v", err)
	}

	yamlStr := string(data)
	if !strings.Contains(yamlStr, "gemini-2.5-pro") {
		t.Errorf("expected YAML to contain gemini-2.5-pro")
	}
	if !strings.Contains(yamlStr, "local-qwen-coder") {
		t.Errorf("expected YAML to contain local-qwen-coder")
	}
	if !strings.Contains(yamlStr, "fallbacks:") {
		t.Errorf("expected YAML to contain fallbacks")
	}

	// Render invalid config
	badCfg := &gateway.LiteLLMConfig{}
	if _, err := gateway.RenderYAML(badCfg); err == nil {
		t.Errorf("expected error rendering invalid YAML config")
	}
}

func TestRenderForTopology(t *testing.T) {
	top, err := topology.NewTopologyConfig(topology.ScenarioLocalHybrid, topology.TailscaleEndpoints{
		ControlPlane:   "100.84.12.10",
		ExecutionPlane: "100.84.12.25",
	})
	if err != nil {
		t.Fatalf("failed to create topology: %v", err)
	}

	data, err := gateway.RenderForTopology(top)
	if err != nil {
		t.Fatalf("unexpected error rendering for topology: %v", err)
	}

	yamlStr := string(data)
	if !strings.Contains(yamlStr, "http://100.84.12.25:11434") {
		t.Errorf("expected YAML to inject execution plane IP for Ollama")
	}

	// Test nil topology
	if _, err := gateway.RenderForTopology(nil); err == nil {
		t.Errorf("expected error rendering for nil topology")
	}

	// Test invalid topology
	invalidTop := &topology.TopologyConfig{Scenario: "unknown"}
	if _, err := gateway.RenderForTopology(invalidTop); err == nil {
		t.Errorf("expected error rendering for invalid topology")
	}

	// Test empty execution plane IP fallback to 127.0.0.1
	topNoIP, _ := topology.NewTopologyConfig(topology.ScenarioLocalHybrid, topology.TailscaleEndpoints{})
	dataFallback, err := gateway.RenderForTopology(topNoIP)
	if err != nil {
		t.Fatalf("failed rendering for topology with empty endpoints: %v", err)
	}
	if !strings.Contains(string(dataFallback), "http://127.0.0.1:11434") {
		t.Errorf("expected default 127.0.0.1 fallback for Ollama URL")
	}
}

func TestGatewayValidationError(t *testing.T) {
	vErr := &gateway.GatewayValidationError{
		Field:   "model_list",
		Message: "cannot be empty",
		Err:     gateway.ErrEmptyModelList,
	}

	if vErr.Unwrap() != gateway.ErrEmptyModelList {
		t.Errorf("expected unwrap to match ErrEmptyModelList")
	}
	if !strings.Contains(vErr.Error(), "model_list") {
		t.Errorf("expected error string to contain field name")
	}

	vErrNoCause := &gateway.GatewayValidationError{
		Field:   "field_only",
		Message: "some message",
	}
	if !strings.Contains(vErrNoCause.Error(), "field_only") {
		t.Errorf("expected error string to contain field name")
	}
}

func TestDefaultLiteLLMConfig_OxiLLaMaRoutingWhenPresent(t *testing.T) {
	oxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"qwen3.8:27b"},{"id":"qwen3.5:9b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer oxServer.Close()

	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", oxServer.URL)
	t.Setenv("DENDRITE_GATEWAY_OXILLAMA_ROUTING", "true")

	cfg := gateway.DefaultLiteLLMConfig("http://127.0.0.1:11434")
	if cfg == nil {
		t.Fatalf("expected non-nil config")
	}

	found27b := false
	found9b := false
	for _, m := range cfg.ModelList {
		if m.ModelName == "local-qwen3-27b" {
			found27b = true
			if m.LitellmParams.CustomLLMProvider != "openai" {
				t.Errorf("expected openai provider for local-qwen3-27b, got %s", m.LitellmParams.CustomLLMProvider)
			}
			if m.LitellmParams.Model != "openai/qwen3.8:27b" {
				t.Errorf("expected openai/qwen3.8:27b, got %s", m.LitellmParams.Model)
			}
			if !strings.Contains(m.LitellmParams.APIBase, oxServer.URL) {
				t.Errorf("expected APIBase to point to oxServer, got %s", m.LitellmParams.APIBase)
			}
		}
		if m.ModelName == "local-qwen3-9b" {
			found9b = true
			if m.LitellmParams.CustomLLMProvider != "openai" {
				t.Errorf("expected openai provider for local-qwen3-9b, got %s", m.LitellmParams.CustomLLMProvider)
			}
			if m.LitellmParams.Model != "openai/qwen3.5:9b" {
				t.Errorf("expected openai/qwen3.5:9b, got %s", m.LitellmParams.Model)
			}
		}
	}

	if !found27b || !found9b {
		t.Errorf("missing local-qwen3 models in rendered config")
	}
}
