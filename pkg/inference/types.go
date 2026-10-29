package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Standard error definitions for inference engines.
var (
	ErrEngineNotFound    = errors.New("inference engine not found")
	ErrEngineUnhealthy   = errors.New("inference engine is unhealthy")
	ErrModelNotSupported = errors.New("model not supported by engine")
	ErrEmptyPrompt       = errors.New("prompt cannot be empty")
	ErrNoAvailableEngine = errors.New("no healthy inference engine available in fallback cascade")
)

// EngineType defines supported inference backends.
type EngineType string

const (
	EngineOllama              EngineType = "ollama"
	EngineOxiLLaMa            EngineType = "oxillama"
	EngineCandle              EngineType = "candle"
	EngineHuggingFaceTGI      EngineType = "huggingface_tgi"
	EngineHuggingFaceTEI      EngineType = "huggingface_tei"
	EngineHuggingFaceEndpoint EngineType = "huggingface_endpoint"
	EngineVLLM                EngineType = "vllm"
	EngineLlamaCpp            EngineType = "llamacpp"
)

// IsValid checks if the engine type is recognized.
func (t EngineType) IsValid() bool {
	switch t {
	case EngineOllama, EngineOxiLLaMa, EngineCandle, EngineHuggingFaceTGI, EngineHuggingFaceTEI,
		EngineHuggingFaceEndpoint, EngineVLLM, EngineLlamaCpp:
		return true
	default:
		return false
	}
}

// EngineConfig holds configuration parameters for initializing an InferenceEngine.
type EngineConfig struct {
	ID          string            `json:"id"`
	Type        EngineType        `json:"type"`
	Endpoint    string            `json:"endpoint"`
	APIKey      string            `json:"api_key,omitempty"`
	Models      []string          `json:"models"`
	SelfHosted  bool              `json:"self_hosted"`
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Concurrency int               `json:"concurrency,omitempty"`
}

// CompletionRequest defines a unified LLM prompt completion or chat request.
type CompletionRequest struct {
	Model       string                 `json:"model"`
	Prompt      string                 `json:"prompt"`
	System      string                 `json:"system,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	TopP        float64                `json:"top_p,omitempty"`
	Stop        []string               `json:"stop,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// CompletionResponse represents the synthesized text response.
type CompletionResponse struct {
	ID           string        `json:"id"`
	Model        string        `json:"model"`
	EngineID     string        `json:"engine_id"`
	EngineType   EngineType    `json:"engine_type"`
	Text         string        `json:"text"`
	PromptTokens int           `json:"prompt_tokens"`
	OutputTokens int           `json:"output_tokens"`
	Duration     time.Duration `json:"duration"`
	CreatedAt    time.Time     `json:"created_at"`
	ZeroCost     bool          `json:"zero_cost"`
}

// StreamChunk represents an incremental SSE token streaming chunk.
type StreamChunk struct {
	Text         string `json:"text"`
	Done         bool   `json:"done"`
	PromptTokens int    `json:"prompt_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Error        error  `json:"error,omitempty"`
}

// EngineHealth captures real-time reachability, latency, and status.
type EngineHealth struct {
	EngineID   string        `json:"engine_id"`
	Type       EngineType    `json:"type"`
	Endpoint   string        `json:"endpoint"`
	Reachable  bool          `json:"reachable"`
	Latency    time.Duration `json:"latency"`
	Models     []string      `json:"models"`
	LastCheck  time.Time     `json:"last_check"`
	SelfHosted bool          `json:"self_hosted"`
	ZeroCost   bool          `json:"zero_cost"`
	Error      string        `json:"error,omitempty"`
}

// InferenceEngine is the unified interface decoupling execution from backend runtimes.
type InferenceEngine interface {
	ID() string
	Type() EngineType
	Config() EngineConfig
	Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
	Health(ctx context.Context) EngineHealth
	SupportedModels(ctx context.Context) ([]string, error)
	IsSelfHosted() bool
}

// initSelfHostedEngine applies common defaults for self-hosted inference engine configs.
func initSelfHostedEngine(cfg EngineConfig, client *http.Client, engineType EngineType, defaultEndpoint string) (EngineConfig, *http.Client) {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultEndpoint
	}
	cfg.Type = engineType
	cfg.SelfHosted = true
	return cfg, client
}

// checkHTTPHealth executes a GET request to the engine's health endpoint and records latency and availability.
func checkHTTPHealth(ctx context.Context, client *http.Client, base EngineHealth, path string, supportedModels func(context.Context) ([]string, error)) EngineHealth {
	start := time.Now()
	base.LastCheck = start

	url := fmt.Sprintf("%s%s", strings.TrimRight(base.Endpoint, "/"), path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		base.Error = err.Error()
		return base
	}

	resp, err := client.Do(req)
	base.Latency = time.Since(start)
	if err != nil {
		base.Error = err.Error()
		return base
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		base.Reachable = true
		if supportedModels != nil {
			models, _ := supportedModels(ctx)
			base.Models = models
		}
	} else {
		base.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
	}

	return base
}

// fetchV1Models queries standard /v1/models endpoints and parses data[].id names.
func fetchV1Models(ctx context.Context, client *http.Client, endpoint string, fallback []string) ([]string, error) {
	url := fmt.Sprintf("%s/v1/models", strings.TrimRight(endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fallback, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return fallback, nil
	}
	defer resp.Body.Close()

	var res struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && len(res.Data) > 0 {
		var names []string
		for _, m := range res.Data {
			names = append(names, m.ID)
		}
		return names, nil
	}

	return fallback, nil
}

// fetchHFInfoModelID queries HuggingFace /info endpoints to extract model_id.
func fetchHFInfoModelID(ctx context.Context, client *http.Client, endpoint string, fallback []string) ([]string, error) {
	url := fmt.Sprintf("%s/info", strings.TrimRight(endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fallback, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return fallback, nil
	}
	defer resp.Body.Close()

	var res struct {
		ModelID string `json:"model_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.ModelID != "" {
		return []string{res.ModelID}, nil
	}

	return fallback, nil
}

// streamUnaryGenerate executes a unary Generate call and converts the single response into a stream channel.
func streamUnaryGenerate(ctx context.Context, req CompletionRequest, generate func(context.Context, CompletionRequest) (*CompletionResponse, error)) (<-chan StreamChunk, error) {
	resp, err := generate(ctx, req)
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

// postJSONRequest marshals payload and sends a POST request with application/json Content-Type.
func postJSONRequest(ctx context.Context, client *http.Client, url string, payload interface{}) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	return client.Do(httpReq)
}

// streamSSEChunks streams tokens from an SSE HTTP response, dispatching parsed payloads to caller.
func streamSSEChunks(resp *http.Response, parsePayload func(payload string) (StreamChunk, bool, bool)) <-chan StreamChunk {
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
					if !strings.HasPrefix(line, "data:") {
						continue
					}
					dataPayload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
					if chunk, emit, stop := parsePayload(dataPayload); emit {
						ch <- chunk
						if stop {
							return
						}
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}
