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

// HuggingFaceEndpointEngine implements InferenceEngine for Hugging Face Hosted Inference Endpoints.
type HuggingFaceEndpointEngine struct {
	cfg        EngineConfig
	httpClient *http.Client
}

// NewHuggingFaceEndpointEngine creates an HF hosted endpoint engine adapter.
func NewHuggingFaceEndpointEngine(cfg EngineConfig, client *http.Client) *HuggingFaceEndpointEngine {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api-inference.huggingface.co/models"
	}
	cfg.Type = EngineHuggingFaceEndpoint
	cfg.SelfHosted = false
	return &HuggingFaceEndpointEngine{
		cfg:        cfg,
		httpClient: client,
	}
}

func (e *HuggingFaceEndpointEngine) ID() string           { return e.cfg.ID }
func (e *HuggingFaceEndpointEngine) Type() EngineType     { return EngineHuggingFaceEndpoint }
func (e *HuggingFaceEndpointEngine) Config() EngineConfig { return e.cfg }
func (e *HuggingFaceEndpointEngine) IsSelfHosted() bool   { return false }

func (e *HuggingFaceEndpointEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}
	if model == "" {
		model = "meta-llama/Meta-Llama-3-8B-Instruct"
	}

	payload := map[string]interface{}{
		"inputs": req.Prompt,
		"parameters": map[string]interface{}{
			"max_new_tokens": req.MaxTokens,
			"temperature":    req.Temperature,
			"top_p":          req.TopP,
		},
	}
	if req.MaxTokens == 0 {
		payload["parameters"].(map[string]interface{})["max_new_tokens"] = 512
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	url := fmt.Sprintf("%s/%s", strings.TrimRight(e.cfg.Endpoint, "/"), model)
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
		return nil, fmt.Errorf("hf endpoint request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hf endpoint error HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res []struct {
		GeneratedText string `json:"generated_text"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &res); err != nil {
		var singleRes struct {
			GeneratedText string `json:"generated_text"`
		}
		if err2 := json.Unmarshal(body, &singleRes); err2 == nil {
			res = append(res, singleRes)
		} else {
			return nil, fmt.Errorf("failed to parse hf endpoint response: %s", string(body))
		}
	}

	text := ""
	if len(res) > 0 {
		text = res[0].GeneratedText
	}

	return &CompletionResponse{
		ID:           fmt.Sprintf("hfe-%d", time.Now().UnixNano()),
		Model:        model,
		EngineID:     e.cfg.ID,
		EngineType:   EngineHuggingFaceEndpoint,
		Text:         text,
		PromptTokens: len(strings.Fields(req.Prompt)),
		OutputTokens: len(strings.Fields(text)),
		Duration:     time.Since(start),
		CreatedAt:    time.Now(),
		ZeroCost:     false,
	}, nil
}

func (e *HuggingFaceEndpointEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	resp, err := e.Generate(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{
		Text:         resp.Text,
		Done:         true,
		OutputTokens: resp.OutputTokens,
	}
	close(ch)
	return ch, nil
}

func (e *HuggingFaceEndpointEngine) Health(ctx context.Context) EngineHealth {
	start := time.Now()
	h := EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineHuggingFaceEndpoint,
		Endpoint:   e.cfg.Endpoint,
		SelfHosted: false,
		ZeroCost:   false,
		LastCheck:  time.Now(),
	}

	url := e.cfg.Endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	if e.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}

	resp, err := e.httpClient.Do(req)
	h.Latency = time.Since(start)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	defer resp.Body.Close()

	if resp.StatusCode < 500 {
		h.Reachable = true
		h.Models = e.cfg.Models
	} else {
		h.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
	}

	return h
}

func (e *HuggingFaceEndpointEngine) SupportedModels(ctx context.Context) ([]string, error) {
	return e.cfg.Models, nil
}
