package inference

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Manager orchestrates multi-provider inference engines, health monitoring, and fallback routing.
type Manager struct {
	mu           sync.RWMutex
	engines      map[string]InferenceEngine
	cascadeOrder []string
	healthCache  map[string]EngineHealth
	stopPoll     chan struct{}
}

// NewManager creates an empty inference engine manager.
func NewManager() *Manager {
	return &Manager{
		engines:      make(map[string]InferenceEngine),
		cascadeOrder: make([]string, 0),
		healthCache:  make(map[string]EngineHealth),
		stopPoll:     make(chan struct{}),
	}
}

// NewEngineFromConfig creates an InferenceEngine matching the provided config.
func NewEngineFromConfig(cfg EngineConfig, client *http.Client) (InferenceEngine, error) {
	if cfg.ID == "" {
		cfg.ID = string(cfg.Type)
	}

	switch cfg.Type {
	case EngineOllama:
		return NewOllamaEngine(cfg, client), nil
	case EngineOxiLLaMa:
		return NewOxiLLaMaEngine(cfg, client), nil
	case EngineCandle:
		return NewCandleEngine(cfg, client), nil
	case EngineHuggingFaceTGI:
		return NewHuggingFaceTGIEngine(cfg, client), nil
	case EngineHuggingFaceTEI:
		return NewHuggingFaceTEIEngine(cfg, client), nil
	case EngineHuggingFaceEndpoint:
		return NewHuggingFaceEndpointEngine(cfg, client), nil
	case EngineVLLM:
		return NewVLLMEngine(cfg, client), nil
	case EngineLlamaCpp:
		return NewLlamaCppEngine(cfg, client), nil
	default:
		return nil, fmt.Errorf("unsupported engine type: %s", cfg.Type)
	}
}

// RegisterEngine registers an inference engine in the manager.
func (m *Manager) RegisterEngine(engine InferenceEngine) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := engine.ID()
	m.engines[id] = engine

	// Add to cascade order if not already present
	exists := false
	for _, existing := range m.cascadeOrder {
		if existing == id {
			exists = true
			break
		}
	}
	if !exists {
		m.cascadeOrder = append(m.cascadeOrder, id)
	}

	return nil
}

// GetEngine retrieves an engine by its ID.
func (m *Manager) GetEngine(id string) (InferenceEngine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	eng, ok := m.engines[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrEngineNotFound, id)
	}
	return eng, nil
}

// ListEngines returns all registered inference engines.
func (m *Manager) ListEngines() []InferenceEngine {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]InferenceEngine, 0, len(m.engines))
	for _, id := range m.cascadeOrder {
		if eng, ok := m.engines[id]; ok {
			res = append(res, eng)
		}
	}
	// append any remaining
	for id, eng := range m.engines {
		found := false
		for _, orderedID := range m.cascadeOrder {
			if orderedID == id {
				found = true
				break
			}
		}
		if !found {
			res = append(res, eng)
		}
	}
	return res
}

// SetCascadeOrder configures the fallback failover priority sequence.
func (m *Manager) SetCascadeOrder(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, id := range ids {
		if _, ok := m.engines[id]; !ok {
			return fmt.Errorf("cannot set cascade order: engine %s not registered", id)
		}
	}
	m.cascadeOrder = append([]string(nil), ids...)
	return nil
}

// GetCascadeOrder returns the current failover cascade sequence.
func (m *Manager) GetCascadeOrder() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.cascadeOrder...)
}

// PollHealthAll queries health of all registered engines in parallel.
func (m *Manager) PollHealthAll(ctx context.Context) map[string]EngineHealth {
	m.mu.RLock()
	engines := make(map[string]InferenceEngine, len(m.engines))
	for k, v := range m.engines {
		engines[k] = v
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	results := sync.Map{}

	for id, eng := range engines {
		wg.Add(1)
		go func(engineID string, e InferenceEngine) {
			defer wg.Done()
			pollCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			h := e.Health(pollCtx)
			results.Store(engineID, h)
		}(id, eng)
	}

	wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]EngineHealth, len(engines))
	results.Range(func(key, value interface{}) bool {
		id := key.(string)
		h := value.(EngineHealth)
		m.healthCache[id] = h
		out[id] = h
		return true
	})
	return out
}

// ExecuteWithCascade executes a prompt against the primary engine, dynamically falling back
// to subsequent tiers in the cascade upon errors or unreachability.
func (m *Manager) ExecuteWithCascade(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	m.mu.RLock()
	cascade := append([]string(nil), m.cascadeOrder...)
	m.mu.RUnlock()

	if len(cascade) == 0 {
		return nil, ErrNoAvailableEngine
	}

	var lastErr error
	for _, id := range cascade {
		eng, err := m.GetEngine(id)
		if err != nil {
			continue
		}

		resp, err := eng.Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("%w: last error: %v", ErrNoAvailableEngine, lastErr)
}

// DefaultManager initializes a standard multi-provider manager with default local and self-hosted endpoints.
func DefaultManager() *Manager {
	mgr := NewManager()

	// 1. Ollama (Metal local primary)
	_ = mgr.RegisterEngine(NewOllamaEngine(EngineConfig{
		ID:       "ollama-metal",
		Type:     EngineOllama,
		Endpoint: "http://127.0.0.1:11434",
		Models:   []string{"qwen2.5-coder:14b", "deepseek-coder:6.7b", "llama3.2:3b"},
	}, nil))

	// 2. OxiLLaMa (Pure-Rust GGUF zero-FFI runner)
	_ = mgr.RegisterEngine(NewOxiLLaMaEngine(EngineConfig{
		ID:       "oxillama-rust",
		Type:     EngineOxiLLaMa,
		Endpoint: "http://127.0.0.1:8080",
		Models:   []string{"qwen2.5-coder-7b", "mistral-7b-instruct"},
	}, nil))

	// 3. vLLM (High-throughput PagedAttention cluster)
	_ = mgr.RegisterEngine(NewVLLMEngine(EngineConfig{
		ID:         "vllm-cluster",
		Type:       EngineVLLM,
		Endpoint:   "http://vllm-node:8000",
		Models:     []string{"Qwen/Qwen2.5-Coder-32B-Instruct"},
		SelfHosted: true,
	}, nil))

	// 4. Hugging Face TGI (Remote GPU)
	_ = mgr.RegisterEngine(NewHuggingFaceTGIEngine(EngineConfig{
		ID:         "hf-tgi-remote",
		Type:       EngineHuggingFaceTGI,
		Endpoint:   "http://tgi-node:8080",
		Models:     []string{"deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct"},
		SelfHosted: true,
	}, nil))

	// 5. Hugging Face TEI (Text Embeddings)
	_ = mgr.RegisterEngine(NewHuggingFaceTEIEngine(EngineConfig{
		ID:         "hf-tei-embed",
		Type:       EngineHuggingFaceTEI,
		Endpoint:   "http://localhost:8082",
		Models:     []string{"BAAI/bge-large-en-v1.5"},
		SelfHosted: true,
	}, nil))

	// 7. Candle (Hugging Face Minimalist Rust ML Framework)
	_ = mgr.RegisterEngine(NewCandleEngine(EngineConfig{
		ID:       "candle-rust",
		Type:     EngineCandle,
		Endpoint: "http://127.0.0.1:8083",
		Models:   []string{"candle-quant-llama", "candle-mistral"},
	}, nil))

	// 6. llama.cpp (Lightweight C++ GGUF server)
	_ = mgr.RegisterEngine(NewLlamaCppEngine(EngineConfig{
		ID:       "llamacpp-local",
		Type:     EngineLlamaCpp,
		Endpoint: "http://localhost:8081",
		Models:   []string{"mistral-7b-instruct-q4"},
	}, nil))

	_ = mgr.SetCascadeOrder([]string{
		"ollama-metal",
		"oxillama-rust",
		"candle-rust",
		"vllm-cluster",
		"hf-tgi-remote",
		"llamacpp-local",
	})

	return mgr
}
