package inference

import (
	"Dendrite/pkg/oxilease"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OxiLLaMaEngine implements InferenceEngine for cool-japan/oxillama (Pure-Rust zero-FFI GGUF runner).
type OxiLLaMaEngine struct {
	cfg        EngineConfig
	httpClient *http.Client
}

// NewOxiLLaMaEngine creates an OxiLLaMa pure-Rust inference engine adapter.
func NewOxiLLaMaEngine(cfg EngineConfig, client *http.Client) *OxiLLaMaEngine {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://127.0.0.1:8080"
	}
	cfg.Type = EngineOxiLLaMa
	cfg.SelfHosted = true
	return &OxiLLaMaEngine{
		cfg:        cfg,
		httpClient: client,
	}
}

func (e *OxiLLaMaEngine) ID() string           { return e.cfg.ID }
func (e *OxiLLaMaEngine) Type() EngineType     { return EngineOxiLLaMa }
func (e *OxiLLaMaEngine) Config() EngineConfig { return e.cfg }
func (e *OxiLLaMaEngine) IsSelfHosted() bool   { return true }

func (e *OxiLLaMaEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrEmptyPrompt
	}

	release, err := e.acquireLocalServer(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	model := req.Model
	if model == "" && len(e.cfg.Models) > 0 {
		model = e.cfg.Models[0]
	}
	if model == "" {
		model = "oxillama-default"
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
		return nil, fmt.Errorf("oxillama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("oxillama error HTTP %d: %s", resp.StatusCode, string(body))
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
		EngineType:   EngineOxiLLaMa,
		Text:         text,
		PromptTokens: res.Usage.PromptTokens,
		OutputTokens: res.Usage.CompletionTokens,
		Duration:     time.Since(start),
		CreatedAt:    time.Now(),
		ZeroCost:     true,
	}, nil
}

func (e *OxiLLaMaEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
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

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(e.cfg.Endpoint, "/"))
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
				if err != io.EOF {
					ch <- StreamChunk{Error: err}
				}
				return
			}
		}
	}()

	return ch, nil
}

func (e *OxiLLaMaEngine) Health(ctx context.Context) EngineHealth {
	start := time.Now()
	h := EngineHealth{
		EngineID:   e.cfg.ID,
		Type:       EngineOxiLLaMa,
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
		models, _ := e.SupportedModels(ctx)
		h.Models = models
		return h
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		h.Reachable = true
		models, _ := e.SupportedModels(ctx)
		h.Models = models
	} else {
		h.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
		models, _ := e.SupportedModels(ctx)
		h.Models = models
	}

	return h
}

func (e *OxiLLaMaEngine) SupportedModels(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/v1/models", strings.TrimRight(e.cfg.Endpoint, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, doErr := e.httpClient.Do(req)
	if doErr == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var res struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&res) == nil {
				var names []string
				for _, m := range res.Data {
					names = append(names, m.ID)
				}
				return names, nil
			}
		} else {
			return nil, fmt.Errorf("oxillama models failed with HTTP %d", resp.StatusCode)
		}
	}

	// Local discovery fallback from ~/.oxillama/models if daemon unreachable
	home, _ := os.UserHomeDir()
	oxillamaDir := filepath.Join(home, ".oxillama", "models")
	entries, readErr := os.ReadDir(oxillamaDir)
	if readErr == nil && len(entries) > 0 {
		var localModels []string
		for _, entry := range entries {
			if entry.IsDir() {
				mfPath := filepath.Join(oxillamaDir, entry.Name(), "oxillama-manifest.json")
				if data, mfErr := os.ReadFile(mfPath); mfErr == nil {
					var mf struct {
						Name string `json:"name"`
					}
					if json.Unmarshal(data, &mf) == nil && mf.Name != "" {
						localModels = append(localModels, mf.Name)
						continue
					}
				}
				localModels = append(localModels, entry.Name())
			}
		}
		if len(localModels) > 0 {
			return localModels, nil
		}
	}

	if doErr != nil {
		return nil, doErr
	}
	return e.cfg.Models, nil
}

// acquireLocalServer holds a lease on the local OxiLLaMa server for the duration of one call, so the
// server is started for the call and stopped once no session needs it. It does nothing for a remote
// or non-default endpoint, or when DENDRITE_OXILLAMA_AUTOSTART=off.
func (e *OxiLLaMaEngine) acquireLocalServer(ctx context.Context) (func(), error) {
	noop := func() {}
	if strings.EqualFold(os.Getenv("DENDRITE_OXILLAMA_AUTOSTART"), "off") {
		return noop, nil
	}
	want := fmt.Sprintf("127.0.0.1:%d", oxilease.DefaultPort)
	ep := strings.TrimPrefix(strings.TrimPrefix(strings.TrimRight(e.cfg.Endpoint, "/"), "http://"), "https://")
	if ep != want && ep != fmt.Sprintf("localhost:%d", oxilease.DefaultPort) {
		return noop, nil
	}
	mgr := oxilease.Default()
	req := oxilease.Request{Holder: "engine:" + e.cfg.ID}
	if !mgr.Running() { // only choose a model when one has to be started
		m, err := oxilease.BestFitting(ctx)
		if err != nil {
			return noop, err
		}
		req.ModelPath, req.ModelID = m.Path, m.ID
	}
	lease, err := mgr.Acquire(ctx, req)
	if err != nil {
		return noop, err
	}
	return func() { _ = lease.Release() }, nil
}
