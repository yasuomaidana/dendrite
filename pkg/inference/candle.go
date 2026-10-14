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

// CandleEngine implements InferenceEngine for Hugging Face Candle (Minimalist Rust ML framework).
type CandleEngine struct {
	cfg        EngineConfig
	httpClient *http.Client
}

// NewCandleEngine creates a Candle pure-Rust inference engine adapter.
func NewCandleEngine(cfg EngineConfig, client *http.Client) *CandleEngine {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://127.0.0.1:8083"
	}
	cfg.Type = EngineCandle
	cfg.SelfHosted = true
	return &CandleEngine{
		cfg:        cfg,
		httpClient: client,
	}
}

func (e *CandleEngine) ID() string           { return e.cfg.ID }
func (e *CandleEngine) Type() EngineType     { return EngineCandle }
func (e *CandleEngine) Config() EngineConfig { return e.cfg }
func (e *CandleEngine) IsSelfHosted() bool   { return true }

func (e *CandleEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}
	if model == "" {
		model = "candle-llama"
	}

	messages := []map[string]string{}
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.Prompt})

	payload := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"stream":      false,
		"temperature": req.Temperature,
		"top_p":       req.TopP,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	url := fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(e.cfg.Endpoint, "/"))
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
		return nil, fmt.Errorf("candle engine generate error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("candle engine returned error status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode candle response: %w", err)
	}

	text := ""
	if len(res.Choices) > 0 {
		text = res.Choices[0].Message.Content
	}

	return &CompletionResponse{
		ID:           res.ID,
		Model:        model,
		EngineID:     e.cfg.ID,
		EngineType:   EngineCandle,
		Text:         text,
		PromptTokens: res.Usage.PromptTokens,
		OutputTokens: res.Usage.CompletionTokens,
		Duration:     time.Since(start),
		CreatedAt:    time.Now(),
		ZeroCost:     true,
	}, nil
}

func (e *CandleEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)
	res, err := e.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	ch <- StreamChunk{Text: res.Text, Done: true, PromptTokens: res.PromptTokens, OutputTokens: res.OutputTokens}
	close(ch)
	return ch, nil
}

func (e *CandleEngine) Health(ctx context.Context) EngineHealth {
	start := time.Now()
	h := EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineCandle,
		Endpoint:   e.cfg.Endpoint,
		SelfHosted: true,
		ZeroCost:   true,
		LastCheck:  time.Now(),
	}

	url := fmt.Sprintf("%s/v1/models", strings.TrimRight(e.cfg.Endpoint, "/"))
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

func (e *CandleEngine) SupportedModels(ctx context.Context) ([]string, error) {
	if len(e.cfg.Models) > 0 {
		return append([]string(nil), e.cfg.Models...), nil
	}
	return []string{"candle-quant-llama", "candle-mistral"}, nil
}
