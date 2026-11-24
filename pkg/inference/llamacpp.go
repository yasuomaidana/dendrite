package inference

import (
	"Dendrite/pkg/llamalease"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// LlamaCppEngine implements InferenceEngine for native llama.cpp (llama-server).
type LlamaCppEngine struct {
	cfg        EngineConfig
	httpClient *http.Client
}

// NewLlamaCppEngine creates a llama.cpp engine adapter.
func NewLlamaCppEngine(cfg EngineConfig, client *http.Client) *LlamaCppEngine {
	cfg, client = initSelfHostedEngine(cfg, client, EngineLlamaCpp, "http://127.0.0.1:8080")
	return &LlamaCppEngine{
		cfg:        cfg,
		httpClient: client,
	}
}

func (e *LlamaCppEngine) ID() string           { return e.cfg.ID }
func (e *LlamaCppEngine) Type() EngineType     { return EngineLlamaCpp }
func (e *LlamaCppEngine) Config() EngineConfig { return e.cfg }
func (e *LlamaCppEngine) IsSelfHosted() bool   { return true }

func (e *LlamaCppEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	release, err := e.acquireLocalServer(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	model := e.modelFor(req)

	payload := map[string]interface{}{
		"prompt":      req.Prompt,
		"temperature": req.Temperature,
		"top_p":       req.TopP,
		"stream":      false,
	}
	if req.MaxTokens > 0 {
		payload["n_predict"] = req.MaxTokens
	}
	if len(req.Stop) > 0 {
		payload["stop"] = req.Stop
	}

	data, _ := json.Marshal(payload) // a map of strings and numbers always marshals

	start := time.Now()
	url := fmt.Sprintf("%s/completion", strings.TrimRight(e.cfg.Endpoint, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llamacpp request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llamacpp error HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Content         string `json:"content"`
		TokensEvaluated int    `json:"tokens_evaluated"`
		TokensPredicted int    `json:"tokens_predicted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return &CompletionResponse{
		ID:           fmt.Sprintf("llama-%d", time.Now().UnixNano()),
		Model:        model,
		EngineID:     e.cfg.ID,
		EngineType:   EngineLlamaCpp,
		Text:         res.Content,
		PromptTokens: res.TokensEvaluated,
		OutputTokens: res.TokensPredicted,
		Duration:     time.Since(start),
		CreatedAt:    time.Now(),
		ZeroCost:     true,
	}, nil
}

// modelFor is the model a request names, else the engine's first configured model.
func (e *LlamaCppEngine) modelFor(req CompletionRequest) string {
	switch {
	case req.Model != "":
		return req.Model
	case len(e.cfg.Models) > 0:
		return e.cfg.Models[0]
	}
	return "llamacpp-gguf"
}

// llamaChunk is one data line of a llama.cpp stream.
type llamaChunk struct {
	Content string `json:"content"`
	Stop    bool   `json:"stop"`
}

var llamaStream = streamSpec[llamaChunk]{
	path: "/completion",
	payload: func(req CompletionRequest) map[string]interface{} {
		return map[string]interface{}{"prompt": req.Prompt, "stream": true}
	},
	decode: func(c llamaChunk) (StreamChunk, bool) { return StreamChunk{Text: c.Content, Done: c.Stop}, c.Stop },
}

func (e *LlamaCppEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	return llamaStream.run(ctx, e.httpClient, e.cfg.Endpoint, req)
}

func (e *LlamaCppEngine) Health(ctx context.Context) EngineHealth {
	return checkHTTPHealth(ctx, e.httpClient, EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineLlamaCpp,
		Endpoint:   e.cfg.Endpoint,
		SelfHosted: true,
		ZeroCost:   true,
	}, "/health", e.SupportedModels)
}

func (e *LlamaCppEngine) SupportedModels(ctx context.Context) ([]string, error) {
	return fetchV1Models(ctx, e.httpClient, e.cfg.Endpoint, e.cfg.Models)
}

// The lease manager and the model choice behind acquireLocalServer; tests replace them so no server starts.
var (
	llamaManager     = llamalease.Default
	llamaBestFitting = llamalease.BestFitting
)

// acquireLocalServer holds a lease on the local llama-server for the duration of one call, so the
// server is started for the call and stopped once no session needs it. It does nothing for a remote
// or non-default endpoint, or when DENDRITE_LLAMA_AUTOSTART=off.
func (e *LlamaCppEngine) acquireLocalServer(ctx context.Context) (func(), error) {
	noop := func() {}
	if strings.EqualFold(os.Getenv("DENDRITE_LLAMA_AUTOSTART"), "off") {
		return noop, nil
	}
	want := fmt.Sprintf("127.0.0.1:%d", llamalease.DefaultPort)
	ep := strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(e.cfg.Endpoint, "/"), "http://"), "https://")
	if ep != want && ep != fmt.Sprintf("localhost:%d", llamalease.DefaultPort) {
		return noop, nil
	}
	mgr := llamaManager()
	req := llamalease.Request{Holder: "engine:" + e.cfg.ID}
	if !mgr.Running() { // only choose a model when one has to be started
		m, err := llamaBestFitting(ctx)
		if err != nil {
			return noop, err
		}
		req.ModelPath, req.ModelID = m.Path, m.ID
	}
	lease, err := mgr.Acquire(ctx, req)
	if err != nil {
		return noop, err
	}
	return func() { _ = lease.Release() }, nil
}
