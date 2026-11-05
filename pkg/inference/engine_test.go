package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEngineType_IsValid(t *testing.T) {
	validTypes := []EngineType{
		EngineOllama, EngineOxiLLaMa, EngineHuggingFaceTGI,
		EngineHuggingFaceTEI, EngineHuggingFaceEndpoint, EngineVLLM, EngineLlamaCpp,
	}
	for _, vt := range validTypes {
		if !vt.IsValid() {
			t.Errorf("expected %s to be valid", vt)
		}
	}
	if EngineType("invalid").IsValid() {
		t.Errorf("expected invalid type to be invalid")
	}
}

func TestOllamaEngine_GenerateAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/generate":
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if stream, ok := req["stream"].(bool); ok && stream {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"response":"token1","done":false,"eval_count":1}` + "\n"))
				_, _ = w.Write([]byte(`{"response":"token2","done":true,"eval_count":2}` + "\n"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"response":"hello world","prompt_eval_count":5,"eval_count":10}`))
		case "/api/version":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"0.5.1"}`))
		case "/api/tags":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:14b"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	engine := NewOllamaEngine(EngineConfig{
		ID:       "test-ollama",
		Endpoint: server.URL,
	}, server.Client())

	if engine.ID() != "test-ollama" || engine.Type() != EngineOllama || !engine.IsSelfHosted() {
		t.Errorf("unexpected engine metadata: %+v", engine.Config())
	}
	_ = engine.Config()

	ctx := context.Background()

	// 1. Generate empty prompt error
	_, err := engine.Generate(ctx, CompletionRequest{Prompt: "   "})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}

	// 2. Generate success
	resp, err := engine.Generate(ctx, CompletionRequest{
		Prompt:    "print hi",
		MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("unexpected generate error: %v", err)
	}
	if resp.Text != "hello world" || resp.PromptTokens != 5 || resp.OutputTokens != 10 || !resp.ZeroCost {
		t.Errorf("unexpected completion response: %+v", resp)
	}

	// 3. Stream success
	ch, err := engine.Stream(ctx, CompletionRequest{Prompt: "stream hi"})
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}
	var streamed []string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream chunk error: %v", chunk.Error)
		}
		streamed = append(streamed, chunk.Text)
	}
	if len(streamed) != 2 || streamed[0] != "token1" || streamed[1] != "token2" {
		t.Errorf("unexpected streamed chunks: %v", streamed)
	}

	// Stream empty prompt error
	_, err = engine.Stream(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt on stream, got %v", err)
	}

	// 4. Health & models
	h := engine.Health(ctx)
	if !h.Reachable || len(h.Models) != 1 || h.Models[0] != "qwen2.5-coder:14b" {
		t.Errorf("unexpected health status: %+v", h)
	}

	// Unhealthy check
	badEngine := NewOllamaEngine(EngineConfig{Endpoint: "http://127.0.0.1:99999"}, nil)
	badH := badEngine.Health(ctx)
	if badH.Reachable {
		t.Errorf("expected unreachable")
	}
}

func TestOxiLLaMaEngine_GenerateAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if stream, ok := req["stream"].(bool); ok && stream {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"rust \"}}]}\n\n"))
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"gguf\"}}]}\n\n"))
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id":"ox-1",
				"choices":[{"message":{"content":"pure rust response"}}],
				"usage":{"prompt_tokens":12,"completion_tokens":24}
			}`))
		case "/v1/models":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen2.5-coder-7b"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	engine := NewOxiLLaMaEngine(EngineConfig{
		ID:       "test-oxillama",
		Endpoint: server.URL,
	}, server.Client())

	if engine.ID() != "test-oxillama" || engine.Type() != EngineOxiLLaMa || !engine.IsSelfHosted() {
		t.Errorf("unexpected oxillama metadata")
	}
	_ = engine.Config()

	ctx := context.Background()

	// 1. Generate
	resp, err := engine.Generate(ctx, CompletionRequest{
		Prompt:    "hello rust",
		System:    "be precise",
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("oxillama generate failed: %v", err)
	}
	if resp.Text != "pure rust response" || resp.PromptTokens != 12 || resp.OutputTokens != 24 {
		t.Errorf("unexpected oxillama response: %+v", resp)
	}

	// Empty prompt
	_, err = engine.Generate(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}

	// 2. Stream
	ch, err := engine.Stream(ctx, CompletionRequest{Prompt: "stream rust"})
	if err != nil {
		t.Fatalf("oxillama stream error: %v", err)
	}
	var chunks []string
	for c := range ch {
		if c.Text != "" {
			chunks = append(chunks, c.Text)
		}
	}
	if len(chunks) != 2 || chunks[0] != "rust " || chunks[1] != "gguf" {
		t.Errorf("unexpected stream chunks: %v", chunks)
	}

	// Stream empty prompt
	_, err = engine.Stream(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt on stream, got %v", err)
	}

	// 3. Health & Models
	h := engine.Health(ctx)
	if !h.Reachable || len(h.Models) != 1 || h.Models[0] != "qwen2.5-coder-7b" {
		t.Errorf("unexpected health status: %+v", h)
	}
}

func TestHuggingFaceTGIEngine_GenerateAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/generate":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"generated_text":"tgi result",
				"details":{"generated_tokens":15,"prefill":[{"id":1},{"id":2}]}
			}`))
		case "/generate_stream":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"token\":{\"text\":\"tgi \"}}\n\n"))
			_, _ = w.Write([]byte("data: {\"token\":{\"text\":\"stream\"},\"generated_text\":\"tgi stream\"}\n\n"))
		case "/info":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"model_id":"deepseek-ai/DeepSeek-Coder-V2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	engine := NewHuggingFaceTGIEngine(EngineConfig{
		ID:         "test-tgi",
		Endpoint:   server.URL,
		SelfHosted: true,
	}, server.Client())

	if engine.ID() != "test-tgi" || engine.Type() != EngineHuggingFaceTGI || !engine.IsSelfHosted() {
		t.Errorf("unexpected tgi metadata")
	}
	_ = engine.Config()

	ctx := context.Background()

	// 1. Generate
	resp, err := engine.Generate(ctx, CompletionRequest{Prompt: "generate tgi"})
	if err != nil {
		t.Fatalf("tgi generate failed: %v", err)
	}
	if resp.Text != "tgi result" || resp.PromptTokens != 2 || resp.OutputTokens != 15 {
		t.Errorf("unexpected tgi response: %+v", resp)
	}

	// Empty prompt
	_, err = engine.Generate(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}

	// 2. Stream
	ch, err := engine.Stream(ctx, CompletionRequest{Prompt: "stream tgi"})
	if err != nil {
		t.Fatalf("tgi stream error: %v", err)
	}
	var textParts []string
	for c := range ch {
		if c.Text != "" {
			textParts = append(textParts, c.Text)
		}
	}
	if len(textParts) != 2 {
		t.Errorf("unexpected stream parts: %v", textParts)
	}

	// Stream empty prompt
	_, err = engine.Stream(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt on stream, got %v", err)
	}

	// 3. Health & Models
	h := engine.Health(ctx)
	if !h.Reachable || len(h.Models) != 1 || h.Models[0] != "deepseek-ai/DeepSeek-Coder-V2" {
		t.Errorf("unexpected health status: %+v", h)
	}
}

func TestHuggingFaceTEIEngine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/embed":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[[0.12, 0.34, 0.56, 0.78]]`))
		case "/info":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"model_id":"BAAI/bge-large-en-v1.5"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	engine := NewHuggingFaceTEIEngine(EngineConfig{
		ID:         "test-tei",
		Endpoint:   server.URL,
		SelfHosted: true,
	}, server.Client())

	if engine.ID() != "test-tei" || engine.Type() != EngineHuggingFaceTEI || !engine.IsSelfHosted() {
		t.Errorf("unexpected tei metadata")
	}
	_ = engine.Config()

	ctx := context.Background()
	resp, err := engine.Generate(ctx, CompletionRequest{Prompt: "embed semantic chunk"})
	if err != nil {
		t.Fatalf("tei generate error: %v", err)
	}
	if resp.OutputTokens != 4 {
		t.Errorf("expected 4 embedding dimensions, got %d", resp.OutputTokens)
	}

	// Empty prompt
	_, err = engine.Generate(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}

	ch, err := engine.Stream(ctx, CompletionRequest{Prompt: "embed stream"})
	if err != nil {
		t.Fatalf("tei stream error: %v", err)
	}
	chunk := <-ch
	if !chunk.Done || chunk.OutputTokens != 4 {
		t.Errorf("unexpected chunk: %+v", chunk)
	}

	h := engine.Health(ctx)
	if !h.Reachable || len(h.Models) != 1 || h.Models[0] != "BAAI/bge-large-en-v1.5" {
		t.Errorf("unexpected health: %+v", h)
	}
}

func TestHuggingFaceEndpointEngine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"generated_text":"hf hosted completion"}]`))
	}))
	defer server.Close()

	engine := NewHuggingFaceEndpointEngine(EngineConfig{
		ID:       "test-hfe",
		Endpoint: server.URL,
		Models:   []string{"meta-llama/Llama-3-8B"},
	}, server.Client())

	if engine.ID() != "test-hfe" || engine.Type() != EngineHuggingFaceEndpoint || engine.IsSelfHosted() {
		t.Errorf("unexpected hfe metadata")
	}
	_ = engine.Config()

	ctx := context.Background()
	resp, err := engine.Generate(ctx, CompletionRequest{Prompt: "prompt"})
	if err != nil {
		t.Fatalf("hf endpoint generate error: %v", err)
	}
	if resp.Text != "hf hosted completion" || resp.ZeroCost {
		t.Errorf("unexpected hfe response: %+v", resp)
	}

	// Empty prompt
	_, err = engine.Generate(ctx, CompletionRequest{Prompt: ""})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}

	ch, err := engine.Stream(ctx, CompletionRequest{Prompt: "prompt stream"})
	if err != nil {
		t.Fatalf("hf endpoint stream error: %v", err)
	}
	c := <-ch
	if !c.Done || c.Text != "hf hosted completion" {
		t.Errorf("unexpected stream chunk: %+v", c)
	}

	h := engine.Health(ctx)
	if !h.Reachable {
		t.Errorf("expected reachable health: %+v", h)
	}

	models, _ := engine.SupportedModels(ctx)
	if len(models) != 1 || models[0] != "meta-llama/Llama-3-8B" {
		t.Errorf("unexpected models: %v", models)
	}
}
