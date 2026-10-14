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
	data, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/generate_stream", strings.TrimRight(e.cfg.Endpoint, "/"))
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

		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				lines := strings.Split(string(buf[:n]), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "data:") {
						dataPayload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
						var chunkRes struct {
							Token struct {
								Text string `json:"text"`
							} `json:"token"`
							GeneratedText *string `json:"generated_text"`
						}
						if err := json.Unmarshal([]byte(dataPayload), &chunkRes); err == nil {
							done := chunkRes.GeneratedText != nil
							ch <- StreamChunk{Text: chunkRes.Token.Text, Done: done}
							if done {
								return
							}
						}
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	return ch, nil
}

func (e *HuggingFaceTGIEngine) Health(ctx context.Context) EngineHealth {
	start := time.Now()
	h := EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineHuggingFaceTGI,
		Endpoint:   e.cfg.Endpoint,
		SelfHosted: e.cfg.SelfHosted,
		ZeroCost:   e.cfg.SelfHosted,
		LastCheck:  time.Now(),
	}

	url := fmt.Sprintf("%s/info", strings.TrimRight(e.cfg.Endpoint, "/"))
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

func (e *HuggingFaceTGIEngine) SupportedModels(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/info", strings.TrimRight(e.cfg.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return e.cfg.Models, nil
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return e.cfg.Models, nil
	}
	defer resp.Body.Close()

	var res struct {
		ModelID string `json:"model_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.ModelID != "" {
		return []string{res.ModelID}, nil
	}

	return e.cfg.Models, nil
}
