package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVLLMEngine_GenerateAndHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if stream, ok := req["stream"].(bool); ok && stream {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"vllm stream\"}}]}\n\n"))
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id":"vllm-1",
				"choices":[{"message":{"content":"vllm paged attention response"}}],
				"usage":{"prompt_tokens":10,"completion_tokens":20}
			}`))
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/models":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"Qwen/Qwen2.5-Coder-32B"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	engine := NewVLLMEngine(EngineConfig{
		ID:         "test-vllm",
		Endpoint:   server.URL,
		SelfHosted: true,
	}, server.Client())

	if engine.ID() != "test-vllm" || engine.Type() != EngineVLLM || !engine.IsSelfHosted() {
		t.Errorf("unexpected vllm metadata")
	}
	_ = engine.Config()

	ctx := context.Background()
	resp, err := engine.Generate(ctx, CompletionRequest{Prompt: "vllm task"})
	if err != nil {
		t.Fatalf("vllm generate failed: %v", err)
	}
	if resp.Text != "vllm paged attention response" {
		t.Errorf("unexpected vllm response text: %s", resp.Text)
	}

	// Empty prompt
	_, err = engine.Generate(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}

	ch, err := engine.Stream(ctx, CompletionRequest{Prompt: "vllm stream task"})
	if err != nil {
		t.Fatalf("vllm stream failed: %v", err)
	}
	chunk := <-ch
	if chunk.Text != "vllm stream" {
		t.Errorf("unexpected stream text: %s", chunk.Text)
	}

	h := engine.Health(ctx)
	if !h.Reachable || len(h.Models) != 1 || h.Models[0] != "Qwen/Qwen2.5-Coder-32B" {
		t.Errorf("unexpected health: %+v", h)
	}
}

func TestLlamaCppEngine_GenerateAndHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/completion":
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if stream, ok := req["stream"].(bool); ok && stream {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("data: {\"content\":\"streamed llama\",\"stop\":true}\n\n"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"content":"llama.cpp completion",
				"tokens_evaluated":8,
				"tokens_predicted":16
			}`))
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/models":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"mistral-7b-q4"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	engine := NewLlamaCppEngine(EngineConfig{
		ID:       "test-llamacpp",
		Endpoint: server.URL,
	}, server.Client())

	if engine.ID() != "test-llamacpp" || engine.Type() != EngineLlamaCpp || !engine.IsSelfHosted() {
		t.Errorf("unexpected llamacpp metadata")
	}
	_ = engine.Config()

	ctx := context.Background()
	resp, err := engine.Generate(ctx, CompletionRequest{Prompt: "llama prompt"})
	if err != nil {
		t.Fatalf("llamacpp generate failed: %v", err)
	}
	if resp.Text != "llama.cpp completion" || resp.PromptTokens != 8 || resp.OutputTokens != 16 {
		t.Errorf("unexpected llamacpp response: %+v", resp)
	}

	// Empty prompt
	_, err = engine.Generate(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}

	ch, err := engine.Stream(ctx, CompletionRequest{Prompt: "llama stream"})
	if err != nil {
		t.Fatalf("llamacpp stream failed: %v", err)
	}
	chunk := <-ch
	if chunk.Text != "streamed llama" || !chunk.Done {
		t.Errorf("unexpected chunk: %+v", chunk)
	}

	h := engine.Health(ctx)
	if !h.Reachable || len(h.Models) != 1 || h.Models[0] != "mistral-7b-q4" {
		t.Errorf("unexpected health: %+v", h)
	}
}

func TestManager_CascadeAndFailover(t *testing.T) {
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server overloaded", http.StatusInternalServerError)
	}))
	defer primaryServer.Close()

	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"response":"fallback succeeded","prompt_eval_count":3,"eval_count":7}`))
	}))
	defer fallbackServer.Close()

	mgr := NewManager()

	// Empty cascade check
	_, err := mgr.ExecuteWithCascade(context.Background(), CompletionRequest{Prompt: "empty"})
	if err != ErrNoAvailableEngine {
		t.Errorf("expected ErrNoAvailableEngine, got %v", err)
	}

	primary := NewOllamaEngine(EngineConfig{ID: "primary", Endpoint: primaryServer.URL}, primaryServer.Client())
	fallback := NewOllamaEngine(EngineConfig{ID: "fallback", Endpoint: fallbackServer.URL}, fallbackServer.Client())

	_ = mgr.RegisterEngine(primary)
	_ = mgr.RegisterEngine(fallback)
	_ = mgr.SetCascadeOrder([]string{"primary", "fallback"})

	order := mgr.GetCascadeOrder()
	if len(order) != 2 || order[0] != "primary" || order[1] != "fallback" {
		t.Errorf("unexpected cascade order: %v", order)
	}

	// Set invalid cascade order
	if err := mgr.SetCascadeOrder([]string{"nonexistent"}); err == nil {
		t.Errorf("expected error for nonexistent engine in cascade")
	}

	ctx := context.Background()
	resp, err := mgr.ExecuteWithCascade(ctx, CompletionRequest{Prompt: "cascade check"})
	if err != nil {
		t.Fatalf("cascade failed: %v", err)
	}
	if resp.Text != "fallback succeeded" {
		t.Errorf("expected fallback text, got: %s", resp.Text)
	}

	// Test PollHealthAll
	healthMap := mgr.PollHealthAll(ctx)
	if len(healthMap) != 2 {
		t.Errorf("expected 2 health records, got %d", len(healthMap))
	}
	if !healthMap["fallback"].Reachable {
		t.Errorf("expected fallback to be reachable")
	}

	// Test NewEngineFromConfig for all valid types
	types := []EngineType{
		EngineOllama, EngineOxiLLaMa, EngineHuggingFaceTGI,
		EngineHuggingFaceTEI, EngineHuggingFaceEndpoint, EngineVLLM, EngineLlamaCpp,
	}
	for _, tp := range types {
		eng, err := NewEngineFromConfig(EngineConfig{Type: tp, Endpoint: "http://localhost:8080"}, nil)
		if err != nil || eng.Type() != tp {
			t.Errorf("failed to create engine %s: %v", tp, err)
		}
	}

	// Test NewEngineFromConfig error
	_, err = NewEngineFromConfig(EngineConfig{Type: "invalid"}, nil)
	if err == nil {
		t.Errorf("expected error for invalid engine config")
	}

	// Test DefaultManager
	defaultMgr := DefaultManager()
	engines := defaultMgr.ListEngines()
	if len(engines) < 5 {
		t.Errorf("expected at least 5 default engines, got %d", len(engines))
	}
	if _, err := defaultMgr.GetEngine("ollama-metal"); err != nil {
		t.Errorf("expected ollama-metal in default manager")
	}
	if _, err := defaultMgr.GetEngine("non-existent"); err == nil {
		t.Errorf("expected error for non-existent engine")
	}
}

func TestEngines_ErrorHandlingAndDefaults(t *testing.T) {
	// 1. Defaults verification
	ollama := NewOllamaEngine(EngineConfig{}, nil)
	if ollama.Config().Endpoint != "http://127.0.0.1:11434" {
		t.Errorf("unexpected ollama default endpoint")
	}

	oxillama := NewOxiLLaMaEngine(EngineConfig{}, nil)
	if oxillama.Config().Endpoint != "http://127.0.0.1:8080" {
		t.Errorf("unexpected oxillama default endpoint")
	}

	tgi := NewHuggingFaceTGIEngine(EngineConfig{}, nil)
	if tgi.Config().Endpoint != "http://localhost:8080" {
		t.Errorf("unexpected tgi default endpoint")
	}

	tei := NewHuggingFaceTEIEngine(EngineConfig{}, nil)
	if tei.Config().Endpoint != "http://localhost:8080" {
		t.Errorf("unexpected tei default endpoint")
	}

	hfe := NewHuggingFaceEndpointEngine(EngineConfig{}, nil)
	if hfe.Config().Endpoint != "https://api-inference.huggingface.co/models" {
		t.Errorf("unexpected hfe default endpoint")
	}

	vllm := NewVLLMEngine(EngineConfig{}, nil)
	if vllm.Config().Endpoint != "http://localhost:8000" {
		t.Errorf("unexpected vllm default endpoint")
	}

	llamacpp := NewLlamaCppEngine(EngineConfig{}, nil)
	if llamacpp.Config().Endpoint != "http://localhost:8081" {
		t.Errorf("unexpected llamacpp default endpoint")
	}

	// 2. HTTP 500 error tests
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer errServer.Close()

	ctx := context.Background()

	// Ollama 500
	oErr := NewOllamaEngine(EngineConfig{Endpoint: errServer.URL}, errServer.Client())
	if _, err := oErr.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
		t.Errorf("expected error on ollama 500")
	}
	if _, err := oErr.SupportedModels(ctx); err == nil {
		t.Errorf("expected error on ollama tags 500")
	}

	// OxiLLaMa 500
	oxErr := NewOxiLLaMaEngine(EngineConfig{Endpoint: errServer.URL}, errServer.Client())
	if _, err := oxErr.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
		t.Errorf("expected error on oxillama 500")
	}
	if _, err := oxErr.SupportedModels(ctx); err == nil {
		t.Errorf("expected error on oxillama models 500")
	}

	// TGI 500
	tgiErr := NewHuggingFaceTGIEngine(EngineConfig{Endpoint: errServer.URL}, errServer.Client())
	if _, err := tgiErr.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
		t.Errorf("expected error on tgi 500")
	}

	// TEI 500
	teiErr := NewHuggingFaceTEIEngine(EngineConfig{Endpoint: errServer.URL}, errServer.Client())
	if _, err := teiErr.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
		t.Errorf("expected error on tei 500")
	}

	// HFE 500
	hfeErr := NewHuggingFaceEndpointEngine(EngineConfig{Endpoint: errServer.URL}, errServer.Client())
	if _, err := hfeErr.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
		t.Errorf("expected error on hfe 500")
	}
	if h := hfeErr.Health(ctx); h.Reachable {
		t.Errorf("expected hfe 500 health to be unreachable")
	}

	// VLLM 500
	vllmErr := NewVLLMEngine(EngineConfig{Endpoint: errServer.URL}, errServer.Client())
	if _, err := vllmErr.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
		t.Errorf("expected error on vllm 500")
	}

	// LlamaCpp 500
	llamaErr := NewLlamaCppEngine(EngineConfig{Endpoint: errServer.URL}, errServer.Client())
	if _, err := llamaErr.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
		t.Errorf("expected error on llamacpp 500")
	}

	// Dead endpoints
	deadCfg := EngineConfig{Endpoint: "http://127.0.0.1:54321"}
	if h := NewOxiLLaMaEngine(deadCfg, nil).Health(ctx); h.Reachable {
		t.Errorf("expected dead oxillama unreachable")
	}
	if h := NewHuggingFaceTGIEngine(deadCfg, nil).Health(ctx); h.Reachable {
		t.Errorf("expected dead tgi unreachable")
	}
	if h := NewHuggingFaceTEIEngine(deadCfg, nil).Health(ctx); h.Reachable {
		t.Errorf("expected dead tei unreachable")
	}
	if h := NewVLLMEngine(deadCfg, nil).Health(ctx); h.Reachable {
		t.Errorf("expected dead vllm unreachable")
	}
	if h := NewLlamaCppEngine(deadCfg, nil).Health(ctx); h.Reachable {
		t.Errorf("expected dead llamacpp unreachable")
	}

	// Supported models error fallback
	if _, err := NewHuggingFaceTGIEngine(deadCfg, nil).SupportedModels(ctx); err != nil {
		t.Errorf("expected fallback models on error")
	}
	if _, err := NewHuggingFaceTEIEngine(deadCfg, nil).SupportedModels(ctx); err != nil {
		t.Errorf("expected fallback models on error")
	}
	if _, err := NewVLLMEngine(deadCfg, nil).SupportedModels(ctx); err != nil {
		t.Errorf("expected fallback models on error")
	}
	if _, err := NewLlamaCppEngine(deadCfg, nil).SupportedModels(ctx); err != nil {
		t.Errorf("expected fallback models on error")
	}

	// All failing cascade
	mgr := NewManager()
	_ = mgr.RegisterEngine(oErr)
	_ = mgr.RegisterEngine(oxErr)
	_ = mgr.SetCascadeOrder([]string{oErr.ID(), oxErr.ID()})
	if _, err := mgr.ExecuteWithCascade(ctx, CompletionRequest{Prompt: "failing"}); err == nil {
		t.Errorf("expected error when all engines fail in cascade")
	}
}

func TestDefaultManager_DynamicCascadeAndEnvOverrides(t *testing.T) {
	oxServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]string{{"id": "qwen3.8:27b"}, {"id": "qwen3.5:9b"}},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer oxServer.Close()

	t.Setenv("DENDRITE_OLLAMA_ENDPOINT", "http://127.0.0.1:19999")
	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", oxServer.URL)
	t.Setenv("DENDRITE_OXILLAMA_MODELS", "custom-qwen:32b,custom-qwen:14b")

	mgr := DefaultManager()
	if mgr == nil {
		t.Fatalf("expected non-nil manager")
	}

	eng, err := mgr.GetEngine("oxillama-rust")
	if err != nil {
		t.Fatalf("expected oxillama-rust registered: %v", err)
	}
	if eng.Config().Endpoint != oxServer.URL {
		t.Errorf("expected oxillama custom endpoint, got %s", eng.Config().Endpoint)
	}
	if len(eng.Config().Models) != 2 || eng.Config().Models[0] != "custom-qwen:32b" {
		t.Errorf("expected custom models, got %v", eng.Config().Models)
	}

	ollamaEng, err := mgr.GetEngine("ollama-metal")
	if err != nil {
		t.Fatalf("expected ollama-metal registered: %v", err)
	}
	if ollamaEng.Config().Endpoint != "http://127.0.0.1:19999" {
		t.Errorf("expected ollama custom endpoint, got %s", ollamaEng.Config().Endpoint)
	}

	order := mgr.GetCascadeOrder()
	if len(order) == 0 || order[0] != "oxillama-rust" {
		t.Errorf("expected oxillama-rust to be at index 0, got %v", order)
	}
}
