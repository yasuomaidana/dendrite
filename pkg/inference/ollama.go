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

// OllamaEngine implements InferenceEngine for Apple Silicon Metal & local Ollama daemon.
type OllamaEngine struct {
	cfg        EngineConfig
	httpClient *http.Client
}

// NewOllamaEngine creates an Ollama inference engine adapter.
func NewOllamaEngine(cfg EngineConfig, client *http.Client) *OllamaEngine {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://127.0.0.1:11434"
	}
	cfg.Type = EngineOllama
	cfg.SelfHosted = true
	return &OllamaEngine{
		cfg:        cfg,
		httpClient: client,
	}
}

func (e *OllamaEngine) ID() string           { return e.cfg.ID }
func (e *OllamaEngine) Type() EngineType     { return EngineOllama }
func (e *OllamaEngine) Config() EngineConfig { return e.cfg }
func (e *OllamaEngine) IsSelfHosted() bool   { return true }

func (e *OllamaEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}
	if model == "" {
		model = "qwen3.8:27b"
	}

	payload := map[string]interface{}{
		"model":  model,
		"prompt": req.Prompt,
		"system": req.System,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": req.Temperature,
			"top_p":       req.TopP,
		},
	}
	if req.MaxTokens > 0 {
		payload["options"].(map[string]interface{})["num_predict"] = req.MaxTokens
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	start := time.Now()
	url := fmt.Sprintf("%s/api/generate", strings.TrimRight(e.cfg.Endpoint, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Response        string `json:"response"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &CompletionResponse{
		ID:           fmt.Sprintf("ollama-%d", time.Now().UnixNano()),
		Model:        model,
		EngineID:     e.cfg.ID,
		EngineType:   EngineOllama,
		Text:         res.Response,
		PromptTokens: res.PromptEvalCount,
		OutputTokens: res.EvalCount,
		Duration:     time.Since(start),
		CreatedAt:    time.Now(),
		ZeroCost:     IsZeroCostSelfHostedModel(model) || e.IsSelfHosted(),
	}, nil
}

func (e *OllamaEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}
	if model == "" {
		model = "qwen3.8:27b"
	}

	payload := map[string]interface{}{
		"model":  model,
		"prompt": req.Prompt,
		"stream": true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/generate", strings.TrimRight(e.cfg.Endpoint, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamChunk)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		dec := json.NewDecoder(resp.Body)
		for dec.More() {
			var chunk struct {
				Response  string `json:"response"`
				Done      bool   `json:"done"`
				EvalCount int    `json:"eval_count"`
			}
			if err := dec.Decode(&chunk); err != nil {
				ch <- StreamChunk{Error: err}
				return
			}
			ch <- StreamChunk{
				Text:         chunk.Response,
				Done:         chunk.Done,
				OutputTokens: chunk.EvalCount,
			}
			if chunk.Done {
				return
			}
		}
	}()

	return ch, nil
}

func (e *OllamaEngine) Health(ctx context.Context) EngineHealth {
	start := time.Now()
	h := EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineOllama,
		Endpoint:   e.cfg.Endpoint,
		SelfHosted: true,
		ZeroCost:   true,
		LastCheck:  time.Now(),
	}

	url := fmt.Sprintf("%s/api/version", strings.TrimRight(e.cfg.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		h.Error = err.Error()
		return h
	}

	resp, err := e.httpClient.Do(req)
	h.Latency = time.Since(start)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		h.Reachable = true
		models, _ := e.SupportedModels(ctx)
		h.Models = models
	} else {
		h.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
	}

	return h
}

func (e *OllamaEngine) SupportedModels(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/api/tags", strings.TrimRight(e.cfg.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tags failed with HTTP %d", resp.StatusCode)
	}

	var res struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var names []string
	for _, m := range res.Models {
		names = append(names, m.Name)
	}
	return names, nil
}
