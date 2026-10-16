package inference

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelIntelligenceScore(t *testing.T) {
	cases := []struct {
		model    string
		minScore int
		maxScore int
	}{
		{"", 5, 15},
		{"qwen3.8:27b", 90, 100},
		{"Qwen/Qwen2.5-Coder-32B-Instruct", 90, 100},
		{"qwen2.5-coder:14b", 75, 88},
		{"qwen3.5:9b", 65, 80},
		{"qwen2.5-coder-7b", 60, 75},
		{"mistral-7b-instruct", 55, 70},
		{"llama3.2:3b", 30, 45},
		{"qwen2.5:1.5b", 20, 35},
		{"unknown-model", 40, 65},
		{"custom-coder", 50, 70},
		{"ollama/deepseek-32b", 85, 100},
		{"oxillama/6.7b", 55, 70},
		{"openai/8b", 60, 75},
		{"local-1b", 20, 35},
	}

	for _, c := range cases {
		score := ModelIntelligenceScore(c.model)
		if score < c.minScore || score > c.maxScore {
			t.Errorf("ModelIntelligenceScore(%q) = %d; want between %d and %d", c.model, score, c.minScore, c.maxScore)
		}
	}

	// Verify relative ordering: 27b > 14b > 7b > 3b
	if ModelIntelligenceScore("qwen3.8:27b") <= ModelIntelligenceScore("qwen2.5-coder:14b") {
		t.Errorf("expected 27b to be smarter than 14b")
	}
	if ModelIntelligenceScore("qwen2.5-coder:14b") <= ModelIntelligenceScore("qwen2.5-coder-7b") {
		t.Errorf("expected 14b to be smarter than 7b")
	}
	if ModelIntelligenceScore("qwen2.5-coder-7b") <= ModelIntelligenceScore("llama3.2:3b") {
		t.Errorf("expected 7b to be smarter than 3b")
	}
}

// Mock engine for cascade tests
type mockEngineWithModel struct {
	id         string
	selfHosted bool
	models     []string
	failModels map[string]error
}

func (m *mockEngineWithModel) ID() string             { return m.id }
func (m *mockEngineWithModel) Type() EngineType       { return EngineOxiLLaMa }
func (m *mockEngineWithModel) Config() EngineConfig   { return EngineConfig{ID: m.id, Models: m.models, SelfHosted: m.selfHosted} }
func (m *mockEngineWithModel) IsSelfHosted() bool     { return m.selfHosted }
func (m *mockEngineWithModel) Health(ctx context.Context) EngineHealth {
	return EngineHealth{EngineID: m.id, Reachable: true, Models: m.models}
}
func (m *mockEngineWithModel) SupportedModels(ctx context.Context) ([]string, error) {
	return m.models, nil
}
func (m *mockEngineWithModel) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	return nil, nil
}
func (m *mockEngineWithModel) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if err, fail := m.failModels[req.Model]; fail && err != nil {
		return nil, err
	}
	return &CompletionResponse{
		Model:    req.Model,
		EngineID: m.id,
		Text:     "success from " + m.id + " with " + req.Model,
		ZeroCost: m.selfHosted,
	}, nil
}

func TestReorderEnginesByIntelligence_OxiLLaMaPrecedence(t *testing.T) {
	ctx := context.Background()

	// Case: Ollama has only a 3B model, while OxiLLaMa has a 7B coder model.
	// OxiLLaMa must be given precedence over Ollama!
	ollamaEng := &mockEngineWithModel{
		id:         "ollama-metal",
		selfHosted: true,
		models:     []string{"llama3.2:3b"},
	}
	oxillamaEng := &mockEngineWithModel{
		id:         "oxillama-rust",
		selfHosted: true,
		models:     []string{"qwen2.5-coder-7b"},
	}

	ordered := ReorderEnginesByIntelligence(ctx, []InferenceEngine{ollamaEng, oxillamaEng})
	if len(ordered) != 2 {
		t.Fatalf("expected 2 engines, got %d", len(ordered))
	}
	if ordered[0].ID() != "oxillama-rust" {
		t.Errorf("expected oxillama-rust first due to smarter 7B model vs 3B model on ollama, got %s", ordered[0].ID())
	}
}

func TestExecuteSmartCascade_TrialFailuresNotCountedAndPaidLeverage(t *testing.T) {
	ctx := context.Background()

	// Engine 1: Ollama has 3b model (score ~38) and 14b model (score ~80)
	// Engine 2: OxiLLaMa has 7b model (score ~65)
	// Both 3b and 7b fail.
	// Smartest is 14b. 14b fails twice (meeting threshold 2).
	// It must then leverage to paid model!
	mockOllama := &mockEngineWithModel{
		id:         "ollama-metal",
		selfHosted: true,
		models:     []string{"llama3.2:3b", "qwen2.5-coder:14b"},
		failModels: map[string]error{
			"llama3.2:3b":       errors.New("3b out of memory / hallucination"),
			"qwen2.5-coder:14b": errors.New("14b context error"),
		},
	}
	mockOxi := &mockEngineWithModel{
		id:         "oxillama-rust",
		selfHosted: true,
		models:     []string{"qwen2.5-coder-7b"},
		failModels: map[string]error{
			"qwen2.5-coder-7b": errors.New("7b syntax failure"),
		},
	}

	paidCalled := false
	paidCaller := func(ctx context.Context, req CompletionRequest, model string) (*CompletionResponse, error) {
		paidCalled = true
		if model != "gemini-2.5-pro" {
			t.Errorf("expected paid model gemini-2.5-pro, got %s", model)
		}
		return &CompletionResponse{
			Model:    model,
			EngineID: "paid-cloud",
			Text:     "paid response",
			ZeroCost: false,
		}, nil
	}

	cfg := SmartCascadeConfig{
		MaxSmartFailures:  2,
		PaidFallbackModel: "gemini-2.5-pro",
		SmartScoreFloor:   75, // Only 14B qualifies as smart
	}

	resp, report, err := ExecuteSmartCascade(ctx, []InferenceEngine{mockOllama, mockOxi}, CompletionRequest{Prompt: "write code"}, paidCaller, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !paidCalled {
		t.Errorf("expected paid model to be invoked")
	}
	if !report.LeveragedToPaid {
		t.Errorf("expected report.LeveragedToPaid to be true")
	}
	if report.TrialFailuresIgnored == 0 {
		t.Errorf("expected trial failures on smaller models (3b/7b) to be ignored and counted separately")
	}
	if report.SmartFailuresCounted != 1 { // 14b was the only smart model and failed once in candidate loop
		t.Logf("Smart failures counted: %d, ignored trials: %d", report.SmartFailuresCounted, report.TrialFailuresIgnored)
	}
	if resp.Model != "gemini-2.5-pro" {
		t.Errorf("expected response from gemini-2.5-pro, got %s", resp.Model)
	}
}

func TestDefaultSmartCascadeConfig_And_EngineMaxIntelligence(t *testing.T) {
	cfg := DefaultSmartCascadeConfig()
	if cfg.MaxSmartFailures != 2 || cfg.PaidFallbackModel != "gemini-2.5-pro" {
		t.Errorf("unexpected default config: %+v", cfg)
	}

	score, model := EngineMaxIntelligence(context.Background(), nil)
	if score != 0 || model != "" {
		t.Errorf("expected 0, empty for nil engine, got %d, %s", score, model)
	}
}

func TestManager_SmartCascade_And_ZeroCost(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager()

	mockEng := &mockEngineWithModel{
		id:         "local-mock",
		selfHosted: true,
		models:     []string{"qwen3.8:27b"},
	}
	mgr.RegisterEngine(mockEng)

	order := mgr.DynamicCascadeOrderByIntelligence(ctx)
	if len(order) != 1 || order[0] != "local-mock" {
		t.Errorf("unexpected dynamic cascade order: %v", order)
	}

	resp, rep, err := mgr.ExecuteWithSmartPrecedence(ctx, CompletionRequest{Prompt: "test"}, nil, SmartCascadeConfig{MaxSmartFailures: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || rep == nil || rep.SelectedModel != "qwen3.8:27b" {
		t.Errorf("unexpected response/report: %+v, %+v", resp, rep)
	}

	if !IsEngineZeroCost(mockEng) {
		t.Errorf("expected self-hosted engine to be zero-cost")
	}
}

func TestCandleEngine_Methods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"candle-test-model"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[{"message":{"content":"candle response"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	eng := NewCandleEngine(EngineConfig{
		ID:         "candle-engine",
		Endpoint:   server.URL,
		Models:     []string{"candle-test-model"},
		SelfHosted: true,
	}, server.Client())

	if eng.ID() != "candle-engine" || eng.Type() != EngineCandle || !eng.IsSelfHosted() {
		t.Errorf("unexpected engine properties: ID=%s Type=%s SelfHosted=%v", eng.ID(), eng.Type(), eng.IsSelfHosted())
	}
	if eng.Config().Endpoint != server.URL {
		t.Errorf("unexpected endpoint: %s", eng.Config().Endpoint)
	}

	ctx := context.Background()
	resp, err := eng.Generate(ctx, CompletionRequest{Prompt: "test prompt"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Text != "candle response" {
		t.Errorf("unexpected response text: %s", resp.Text)
	}

	health := eng.Health(ctx)
	if !health.Reachable {
		t.Errorf("expected reachable health: %+v", health)
	}

	models, err := eng.SupportedModels(ctx)
	if err != nil || len(models) == 0 {
		t.Errorf("expected supported models, got %v (err: %v)", models, err)
	}

	stream, err := eng.Stream(ctx, CompletionRequest{Prompt: "stream"})
	if err != nil || stream == nil {
		t.Errorf("expected stream channel")
	}
}
