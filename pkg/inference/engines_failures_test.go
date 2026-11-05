package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// engineMakers builds every HTTP engine against an endpoint.
var engineMakers = map[string]func(cfg EngineConfig) InferenceEngine{
	"vllm":        func(c EngineConfig) InferenceEngine { return NewVLLMEngine(c, nil) },
	"ollama":      func(c EngineConfig) InferenceEngine { return NewOllamaEngine(c, nil) },
	"oxillama":    func(c EngineConfig) InferenceEngine { return NewOxiLLaMaEngine(c, nil) },
	"candle":      func(c EngineConfig) InferenceEngine { return NewCandleEngine(c, nil) },
	"hf-tgi":      func(c EngineConfig) InferenceEngine { return NewHuggingFaceTGIEngine(c, nil) },
	"hf-tei":      func(c EngineConfig) InferenceEngine { return NewHuggingFaceTEIEngine(c, nil) },
	"hf-endpoint": func(c EngineConfig) InferenceEngine { return NewHuggingFaceEndpointEngine(c, nil) },
	"llamacpp":    func(c EngineConfig) InferenceEngine { return NewLlamaCppEngine(c, nil) },
}

func answering(code int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
}

func closedEndpoint() string {
	s := httptest.NewServer(http.NotFoundHandler())
	s.Close()
	return s.URL
}

func TestEveryEngineReportsAnEndpointThatIsDown(t *testing.T) {
	ctx := context.Background()
	for name, make := range engineMakers {
		t.Run(name, func(t *testing.T) {
			e := make(EngineConfig{ID: name, Endpoint: closedEndpoint(), APIKey: "key", Models: []string{"m"}})
			if _, err := e.Generate(ctx, CompletionRequest{Prompt: "hi", System: "be brief", MaxTokens: 5}); err == nil {
				t.Error("Generate must fail")
			}
			if _, err := e.Stream(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
				t.Error("Stream must fail")
			}
			if h := e.Health(ctx); h.Reachable {
				t.Errorf("Health = %+v, want unreachable", h)
			}
			expectFallbackOrError(t, e, []string{"m"})
		})
	}
}

// expectFallbackOrError checks SupportedModels against an unusable server: it either fails or
// answers with the models from the engine's configuration, never with invented ones.
func expectFallbackOrError(t *testing.T, e InferenceEngine, configured []string) {
	t.Helper()
	models, err := e.SupportedModels(context.Background())
	if err != nil || e.ID() == "candle" || e.ID() == "oxillama" { // these two list what they ship with
		return
	}
	if len(models) != len(configured) {
		t.Errorf("SupportedModels = %v, want the configured %v", models, configured)
	}
}

func TestEveryEngineReportsAnEndpointThatIsNotAURL(t *testing.T) {
	ctx := context.Background()
	for name, make := range engineMakers {
		t.Run(name, func(t *testing.T) {
			e := make(EngineConfig{ID: name, Endpoint: "http://%zz"})
			if _, err := e.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
				t.Error("Generate must fail")
			}
			if _, err := e.Stream(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
				t.Error("Stream must fail")
			}
			if h := e.Health(ctx); h.Reachable {
				t.Errorf("Health = %+v", h)
			}
			expectFallbackOrError(t, e, nil)
		})
	}
}

func TestEveryEngineReportsServerErrorsAndBrokenAnswers(t *testing.T) {
	ctx := context.Background()
	for name, make := range engineMakers {
		t.Run(name+" 500", func(t *testing.T) {
			s := answering(500, "boom")
			defer s.Close()
			e := make(EngineConfig{ID: name, Endpoint: s.URL})
			if _, err := e.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
				t.Error("Generate must fail on 500")
			}
			if h := e.Health(ctx); h.Reachable {
				t.Errorf("Health on 500 = %+v", h)
			}
			expectFallbackOrError(t, e, nil)
		})
		t.Run(name+" not json", func(t *testing.T) {
			s := answering(200, "not json")
			defer s.Close()
			e := make(EngineConfig{ID: name, Endpoint: s.URL})
			if _, err := e.Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
				t.Error("Generate must fail on a broken body")
			}
			expectFallbackOrError(t, e, nil)
		})
		t.Run(name+" blank prompt", func(t *testing.T) {
			e := make(EngineConfig{ID: name, Endpoint: "http://127.0.0.1:1"})
			if _, err := e.Generate(ctx, CompletionRequest{Prompt: "  "}); err != ErrEmptyPrompt {
				t.Errorf("Generate = %v, want ErrEmptyPrompt", err)
			}
			if _, err := e.Stream(ctx, CompletionRequest{}); err != ErrEmptyPrompt {
				t.Errorf("Stream = %v, want ErrEmptyPrompt", err)
			}
		})
	}
}
