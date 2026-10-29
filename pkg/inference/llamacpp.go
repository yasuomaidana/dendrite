package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	cfg, client = initSelfHostedEngine(cfg, client, EngineLlamaCpp, "http://localhost:8081")
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

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}
	if model == "" {
		model = "llamacpp-gguf"
	}

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

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

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

func (e *LlamaCppEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	payload := map[string]interface{}{
		"prompt": req.Prompt,
		"stream": true,
	}
	url := fmt.Sprintf("%s/completion", strings.TrimRight(e.cfg.Endpoint, "/"))
	resp, err := postJSONRequest(ctx, e.httpClient, url, payload)
	if err != nil {
		return nil, err
	}

	return streamSSEChunks(resp, func(payload string) (StreamChunk, bool, bool) {
		var chunkRes struct {
			Content string `json:"content"`
			Stop    bool   `json:"stop"`
		}
		if err := json.Unmarshal([]byte(payload), &chunkRes); err == nil {
			return StreamChunk{Text: chunkRes.Content, Done: chunkRes.Stop}, true, chunkRes.Stop
		}
		return StreamChunk{}, false, false
	}), nil
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
