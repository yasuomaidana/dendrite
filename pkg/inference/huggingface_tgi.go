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

// HuggingFaceTGIEngine implements InferenceEngine for Hugging Face Text Generation Inference (TGI).
type HuggingFaceTGIEngine struct {
	cfg        EngineConfig
	httpClient *http.Client
}

// NewHuggingFaceTGIEngine creates an HF TGI engine adapter.
func NewHuggingFaceTGIEngine(cfg EngineConfig, client *http.Client) *HuggingFaceTGIEngine {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:8080"
	}
	cfg.Type = EngineHuggingFaceTGI
	return &HuggingFaceTGIEngine{
		cfg:        cfg,
		httpClient: client,
	}
}

func (e *HuggingFaceTGIEngine) ID() string           { return e.cfg.ID }
func (e *HuggingFaceTGIEngine) Type() EngineType     { return EngineHuggingFaceTGI }
func (e *HuggingFaceTGIEngine) Config() EngineConfig { return e.cfg }
func (e *HuggingFaceTGIEngine) IsSelfHosted() bool   { return e.cfg.SelfHosted }

func (e *HuggingFaceTGIEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}

	payload := map[string]interface{}{
		"inputs": req.Prompt,
		"parameters": map[string]interface{}{
			"max_new_tokens": req.MaxTokens,
			"temperature":    req.Temperature,
			"top_p":          req.TopP,
			"stop":           req.Stop,
		},
	}
	if req.MaxTokens == 0 {
		payload["parameters"].(map[string]interface{})["max_new_tokens"] = 1024
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	url := fmt.Sprintf("%s/generate", strings.TrimRight(e.cfg.Endpoint, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tgi request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tgi error HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		GeneratedText string `json:"generated_text"`
		Details       struct {
			GeneratedTokens int `json:"generated_tokens"`
			Prefill         []struct {
				ID int `json:"id"`
			} `json:"prefill"`
		} `json:"details"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	promptToks := len(res.Details.Prefill)
	if promptToks == 0 {
		promptToks = len(strings.Fields(req.Prompt))
	}

	return &CompletionResponse{
		ID:           fmt.Sprintf("tgi-%d", time.Now().UnixNano()),
		Model:        model,
		EngineID:     e.cfg.ID,
		EngineType:   EngineHuggingFaceTGI,
		Text:         res.GeneratedText,
		PromptTokens: promptToks,
		OutputTokens: res.Details.GeneratedTokens,
		Duration:     time.Since(start),
		CreatedAt:    time.Now(),
		ZeroCost:     e.cfg.SelfHosted,
	}, nil
}

func (e *HuggingFaceTGIEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	payload := map[string]interface{}{
		"inputs": req.Prompt,
		"parameters": map[string]interface{}{
			"max_new_tokens": 512,
		},
	}
	url := fmt.Sprintf("%s/generate_stream", strings.TrimRight(e.cfg.Endpoint, "/"))
	resp, err := postJSONRequest(ctx, e.httpClient, url, payload)
	if err != nil {
		return nil, err
	}

	return streamSSEChunks(resp, func(payload string) (StreamChunk, bool, bool) {
		var chunkRes struct {
			Token struct {
				Text string `json:"text"`
			} `json:"token"`
			GeneratedText *string `json:"generated_text"`
		}
		if err := json.Unmarshal([]byte(payload), &chunkRes); err == nil {
			done := chunkRes.GeneratedText != nil
			return StreamChunk{Text: chunkRes.Token.Text, Done: done}, true, done
		}
		return StreamChunk{}, false, false
	}), nil
}

func (e *HuggingFaceTGIEngine) Health(ctx context.Context) EngineHealth {
	return checkHTTPHealth(ctx, e.httpClient, EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineHuggingFaceTGI,
		Endpoint:   e.cfg.Endpoint,
		SelfHosted: e.cfg.SelfHosted,
		ZeroCost:   e.cfg.SelfHosted,
	}, "/info", e.SupportedModels)
}

func (e *HuggingFaceTGIEngine) SupportedModels(ctx context.Context) ([]string, error) {
	return fetchHFInfoModelID(ctx, e.httpClient, e.cfg.Endpoint, e.cfg.Models)
}
