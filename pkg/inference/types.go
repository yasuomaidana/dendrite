package inference

import (
	"context"
	"errors"
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
