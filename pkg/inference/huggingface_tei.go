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

// HuggingFaceTEIEngine implements InferenceEngine for Hugging Face Text Embeddings Inference (TEI).
type HuggingFaceTEIEngine struct {
	cfg        EngineConfig
	httpClient *http.Client
}

// NewHuggingFaceTEIEngine creates a new HF TEI vector embedding engine adapter.
func NewHuggingFaceTEIEngine(cfg EngineConfig, client *http.Client) *HuggingFaceTEIEngine {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:8080"
	}
	cfg.Type = EngineHuggingFaceTEI
	return &HuggingFaceTEIEngine{
		cfg:        cfg,
		httpClient: client,
	}
}

func (e *HuggingFaceTEIEngine) ID() string           { return e.cfg.ID }
func (e *HuggingFaceTEIEngine) Type() EngineType     { return EngineHuggingFaceTEI }
func (e *HuggingFaceTEIEngine) Config() EngineConfig { return e.cfg }
func (e *HuggingFaceTEIEngine) IsSelfHosted() bool   { return e.cfg.SelfHosted }

func (e *HuggingFaceTEIEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	payload := map[string]interface{}{
		"inputs": req.Prompt,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	url := fmt.Sprintf("%s/embed", strings.TrimRight(e.cfg.Endpoint, "/"))
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
		return nil, fmt.Errorf("tei request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tei error HTTP %d: %s", resp.StatusCode, string(body))
	}

	var embeddings [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&embeddings); err != nil {
		return nil, fmt.Errorf("failed to parse tei embeddings: %w", err)
	}

	dim := 0
	if len(embeddings) > 0 {
		dim = len(embeddings[0])
	}

	return &CompletionResponse{
		ID:           fmt.Sprintf("tei-%d", time.Now().UnixNano()),
		Model:        "tei-embeddings",
		EngineID:     e.cfg.ID,
		EngineType:   EngineHuggingFaceTEI,
		Text:         fmt.Sprintf("generated %d embeddings of dimension %d", len(embeddings), dim),
		PromptTokens: len(strings.Fields(req.Prompt)),
		OutputTokens: dim,
		Duration:     time.Since(start),
		CreatedAt:    time.Now(),
		ZeroCost:     e.cfg.SelfHosted,
	}, nil
}

func (e *HuggingFaceTEIEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
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

func (e *HuggingFaceTEIEngine) Health(ctx context.Context) EngineHealth {
	start := time.Now()
	h := EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineHuggingFaceTEI,
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

func (e *HuggingFaceTEIEngine) SupportedModels(ctx context.Context) ([]string, error) {
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
