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

// VLLMEngine implements InferenceEngine for vLLM high-throughput PagedAttention server.
type VLLMEngine struct {
	cfg        EngineConfig
	httpClient *http.Client
}

// NewVLLMEngine creates a vLLM engine adapter.
func NewVLLMEngine(cfg EngineConfig, client *http.Client) *VLLMEngine {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:8000"
	}
	cfg.Type = EngineVLLM
	return &VLLMEngine{
		cfg:        cfg,
		httpClient: client,
	}
}

func (e *VLLMEngine) ID() string           { return e.cfg.ID }
func (e *VLLMEngine) Type() EngineType     { return EngineVLLM }
func (e *VLLMEngine) Config() EngineConfig { return e.cfg }
func (e *VLLMEngine) IsSelfHosted() bool   { return e.cfg.SelfHosted }

func (e *VLLMEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}
	if model == "" {
		model = "Qwen/Qwen2.5-Coder-32B-Instruct"
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
		return nil, fmt.Errorf("vllm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vllm error HTTP %d: %s", resp.StatusCode, string(body))
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
		return nil, err
	}

	text := ""
	if len(res.Choices) > 0 {
		text = res.Choices[0].Message.Content
	}

	return &CompletionResponse{
		ID:           res.ID,
		Model:        model,
		EngineID:     e.cfg.ID,
		EngineType:   EngineVLLM,
		Text:         text,
		PromptTokens: res.Usage.PromptTokens,
		OutputTokens: res.Usage.CompletionTokens,
		Duration:     time.Since(start),
		CreatedAt:    time.Now(),
		ZeroCost:     e.cfg.SelfHosted,
	}, nil
}

func (e *VLLMEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}

	messages := []map[string]string{
		{"role": "user", "content": req.Prompt},
	}
	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}

	data, _ := json.Marshal(payload)
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
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					dataPayload := strings.TrimPrefix(line, "data: ")
					if dataPayload == "[DONE]" {
						ch <- StreamChunk{Done: true}
						return
					}
					var chunkRes struct {
						Choices []struct {
							Delta struct {
								Content string `json:"content"`
							} `json:"delta"`
						} `json:"choices"`
					}
					if err := json.Unmarshal([]byte(dataPayload), &chunkRes); err == nil && len(chunkRes.Choices) > 0 {
						ch <- StreamChunk{
							Text: chunkRes.Choices[0].Delta.Content,
							Done: false,
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

func (e *VLLMEngine) Health(ctx context.Context) EngineHealth {
	return checkHTTPHealth(ctx, e.httpClient, EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineVLLM,
		Endpoint:   e.cfg.Endpoint,
		SelfHosted: e.cfg.SelfHosted,
		ZeroCost:   e.cfg.SelfHosted,
	}, "/health", e.SupportedModels)
}

func (e *VLLMEngine) SupportedModels(ctx context.Context) ([]string, error) {
	return fetchV1Models(ctx, e.httpClient, e.cfg.Endpoint, e.cfg.Models)
}
