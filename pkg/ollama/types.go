package ollama

import (
	"fmt"
	"time"
)

// DefaultBaseURL is the default endpoint for local Ollama instances.
const DefaultBaseURL = "http://localhost:11434"

// ModelName represents a typed model identifier (e.g., "qwen2.5-coder:14b").
type ModelName string

// APIError represents an error returned by the Ollama HTTP API.
type APIError struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"error"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ollama api error (status %d): %s", e.StatusCode, e.Message)
}

// GenerateRequest defines the payload for text generation via /api/generate.
type GenerateRequest struct {
	Model     ModelName              `json:"model"`
	Prompt    string                 `json:"prompt"`
	System    string                 `json:"system,omitempty"`
	Template  string                 `json:"template,omitempty"`
	Context   []int                  `json:"context,omitempty"`
	Stream    bool                   `json:"stream"`
	Raw       bool                   `json:"raw,omitempty"`
	Format    string                 `json:"format,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

// GenerateResponse represents the response object from /api/generate.
type GenerateResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Response           string    `json:"response"`
	Done               bool      `json:"done"`
	DoneReason         string    `json:"done_reason,omitempty"`
	Context            []int     `json:"context,omitempty"`
	TotalDuration      int64     `json:"total_duration,omitempty"`
	LoadDuration       int64     `json:"load_duration,omitempty"`
	PromptEvalCount    int       `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64     `json:"prompt_eval_duration,omitempty"`
	EvalCount          int       `json:"eval_count,omitempty"`
	EvalDuration       int64     `json:"eval_duration,omitempty"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string   `json:"role"` // "system", "user", "assistant"
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

// ChatRequest defines the payload for multi-turn conversations via /api/chat.
type ChatRequest struct {
	Model     ModelName              `json:"model"`
	Messages  []ChatMessage          `json:"messages"`
	Stream    bool                   `json:"stream"`
	Format    string                 `json:"format,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

// ChatResponse represents the response object from /api/chat.
type ChatResponse struct {
	Model              string      `json:"model"`
	CreatedAt          time.Time   `json:"created_at"`
	Message            ChatMessage `json:"message"`
	Done               bool        `json:"done"`
	DoneReason         string      `json:"done_reason,omitempty"`
	TotalDuration      int64       `json:"total_duration,omitempty"`
	LoadDuration       int64       `json:"load_duration,omitempty"`
	PromptEvalCount    int         `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64       `json:"prompt_eval_duration,omitempty"`
	EvalCount          int         `json:"eval_count,omitempty"`
	EvalDuration       int64       `json:"eval_duration,omitempty"`
}

// ModelInfo describes a model installed locally in Ollama.
type ModelInfo struct {
	Name       string    `json:"name"`
	Model      string    `json:"model"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	Details    struct {
		ParentModel       string   `json:"parent_model"`
		Format            string   `json:"format"`
		Family            string   `json:"family"`
		Families          []string `json:"families"`
		ParameterSize     string   `json:"parameter_size"`
		QuantizationLevel string   `json:"quantization_level"`
	} `json:"details"`
}

// ListModelsResponse represents the response from /api/tags.
type ListModelsResponse struct {
	Models []ModelInfo `json:"models"`
}

// HealthStatus represents the health and latency check of the Ollama engine.
type HealthStatus struct {
	Reachable    bool          `json:"reachable"`
	BaseURL      string        `json:"base_url"`
	Latency      time.Duration `json:"latency"`
	ModelsLoaded int           `json:"models_loaded"`
	Error        string        `json:"error,omitempty"`
}
