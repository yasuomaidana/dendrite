package models

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Typed error hierarchy for Model Provisioner and Gateway Bridge.
var (
	ErrEngineUnreachable = errors.New("inference engine is unreachable")
	ErrPullFailed        = errors.New("model pull operation failed")
	ErrInvalidResponse   = errors.New("invalid or malformed response from inference backend")
	ErrGatewayOffline    = errors.New("litellm token governance gateway is offline")
)

// ModelProvisionStatus tracks the provisioning state of a specific model.
type ModelProvisionStatus string

const (
	StatusAlreadyPresent ModelProvisionStatus = "ALREADY_PRESENT"
	StatusPulled         ModelProvisionStatus = "PULLED_SUCCESS"
	StatusFailed         ModelProvisionStatus = "FAILED"
)

// CanonicalZeroCostModels lists standard zero-cost local models to provision.
var CanonicalZeroCostModels = []string{
	"llama3.2:latest",
	"nomic-embed-text:latest",
}

// ProvisionReport summarizes the result of model provisioning on Ollama.
type ProvisionReport struct {
	Engine    string                          `json:"engine"`
	Host      string                          `json:"host"`
	Models    map[string]ModelProvisionStatus `json:"models"`
	Duration  time.Duration                   `json:"duration"`
	Timestamp time.Time                       `json:"timestamp"`
}

// LlamaCppState reflects the lifecycle status of the llama.cpp llama-server runtime.
type LlamaCppState string

const (
	LlamaCppActive      LlamaCppState = "ACTIVE"
	LlamaCppBinaryFound LlamaCppState = "BINARY_FOUND"
	LlamaCppStandby     LlamaCppState = "STANDBY"
)

// LlamaCppStatus details the state of the llama.cpp inference engine.
type LlamaCppStatus struct {
	State        LlamaCppState `json:"state"`
	Host         string        `json:"host"`
	BinaryPath   string        `json:"binary_path,omitempty"`
	LoadedModels []string      `json:"loaded_models,omitempty"`
	Message      string        `json:"message"`
}

// GatewayRoutingReport details the LiteLLM proxy status and available models.
type GatewayRoutingReport struct {
	GatewayHost  string        `json:"gateway_host"`
	Healthy      bool          `json:"healthy"`
	HasZeroCost  bool          `json:"has_zero_cost"`
	RoutedModels []string      `json:"routed_models"`
	Latency      time.Duration `json:"latency"`
	Timestamp    time.Time     `json:"timestamp"`
}

// Provisioner orchestrates local model pulling and gateway routing checks.
type Provisioner struct {
	OllamaHost     string
	LlamaCppHost   string
	LiteLLMHost    string
	HTTPClient     *http.Client
	BinaryLookupFn func(file string) (string, error)
}

// NewProvisioner constructs a Provisioner with sensible production defaults.
func NewProvisioner(ollamaHost, llamaCppHost, litellmHost string) *Provisioner {
	if ollamaHost == "" {
		ollamaHost = "http://127.0.0.1:11434"
	}
	if llamaCppHost == "" {
		llamaCppHost = "http://127.0.0.1:8080"
	}
	if litellmHost == "" {
		litellmHost = "http://127.0.0.1:4000"
	}

	return &Provisioner{
		OllamaHost:     strings.TrimRight(ollamaHost, "/"),
		LlamaCppHost:   strings.TrimRight(llamaCppHost, "/"),
		LiteLLMHost:    strings.TrimRight(litellmHost, "/"),
		HTTPClient:     &http.Client{Timeout: 30 * time.Second},
		BinaryLookupFn: exec.LookPath,
	}
}

// ProvisionOllamaModels probes local Ollama and pulls missing zero-cost models.
func (p *Provisioner) ProvisionOllamaModels(ctx context.Context, models []string) (*ProvisionReport, error) {
	if len(models) == 0 {
		models = CanonicalZeroCostModels
	}

	start := time.Now()

	// 1. Probe /api/tags
	tagsURL := fmt.Sprintf("%s/api/tags", p.OllamaHost)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to probe Ollama at %s: %v", ErrEngineUnreachable, p.OllamaHost, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d from %s", ErrEngineUnreachable, resp.StatusCode, tagsURL)
	}

	var tagsData struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tagsData); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	installedMap := make(map[string]bool)
	for _, m := range tagsData.Models {
		installedMap[strings.ToLower(m.Name)] = true
	}

	// 2. Provision models
	results := make(map[string]ModelProvisionStatus)
	for _, target := range models {
		cleanName := strings.ToLower(target)
		// Check both exact name and base name
		baseName := cleanName
		if idx := strings.Index(cleanName, ":"); idx != -1 {
			baseName = cleanName[:idx]
		}

		alreadyPresent := installedMap[cleanName] || installedMap[baseName] || installedMap[baseName+":latest"]
		if alreadyPresent {
			results[target] = StatusAlreadyPresent
			continue
		}

		// Pull missing model
		pullErr := p.pullModel(ctx, target)
		if pullErr != nil {
			results[target] = StatusFailed
			return &ProvisionReport{
				Engine:    "ollama",
				Host:      p.OllamaHost,
				Models:    results,
				Duration:  time.Since(start),
				Timestamp: time.Now(),
			}, fmt.Errorf("%w: %s (%v)", ErrPullFailed, target, pullErr)
		}
		results[target] = StatusPulled
	}

	return &ProvisionReport{
		Engine:    "ollama",
		Host:      p.OllamaHost,
		Models:    results,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}, nil
}

func (p *Provisioner) pullModel(ctx context.Context, modelName string) error {
	pullURL := fmt.Sprintf("%s/api/pull", p.OllamaHost)
	payload := map[string]interface{}{
		"name":   modelName,
		"stream": false,
	}

	data, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pullURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ProbeLlamaCpp checks the llama-server daemon and discovers the local binary.
func (p *Provisioner) ProbeLlamaCpp(ctx context.Context) (*LlamaCppStatus, error) {
	// 1. Probe /v1/models endpoint
	modelsURL := fmt.Sprintf("%s/v1/models", p.LlamaCppHost)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err == nil {
		resp, err := p.HTTPClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var data struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				if json.NewDecoder(resp.Body).Decode(&data) == nil && len(data.Data) > 0 {
					var names []string
					for _, item := range data.Data {
						names = append(names, item.ID)
					}
					return &LlamaCppStatus{
						State:        LlamaCppActive,
						Host:         p.LlamaCppHost,
						LoadedModels: names,
						Message:      fmt.Sprintf("llama-server is active on %s (%d models)", p.LlamaCppHost, len(names)),
					}, nil
				}
			}
		}
	}

	// 2. If daemon not running, check for binary in PATH
	if path, err := p.BinaryLookupFn("llama-server"); err == nil && path != "" {
		return &LlamaCppStatus{
			State:      LlamaCppBinaryFound,
			Host:       p.LlamaCppHost,
			BinaryPath: path,
			Message:    fmt.Sprintf("llama-server binary discovered at %s (standby mode)", path),
		}, nil
	}

	return &LlamaCppStatus{
		State:   LlamaCppStandby,
		Host:    p.LlamaCppHost,
		Message: "llama-server not running on port 8080 and binary not discovered in PATH. Install llama.cpp (brew install llama.cpp) to launch it on demand.",
	}, nil
}

// ValidateLiteLLMRouting verifies that the LiteLLM gateway is online and exposing models.
func (p *Provisioner) ValidateLiteLLMRouting(ctx context.Context) (*GatewayRoutingReport, error) {
	start := time.Now()
	modelsURL := fmt.Sprintf("%s/v1/models", p.LiteLLMHost)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return &GatewayRoutingReport{
			GatewayHost: p.LiteLLMHost,
			Healthy:     false,
			Timestamp:   time.Now(),
		}, fmt.Errorf("%w: %v", ErrGatewayOffline, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &GatewayRoutingReport{
			GatewayHost: p.LiteLLMHost,
			Healthy:     false,
			Timestamp:   time.Now(),
		}, fmt.Errorf("%w: status %d from %s", ErrGatewayOffline, resp.StatusCode, modelsURL)
	}

	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	var models []string
	hasZeroCost := false
	for _, item := range data.Data {
		models = append(models, item.ID)
		lower := strings.ToLower(item.ID)
		if strings.Contains(lower, "llama") || strings.Contains(lower, "qwen") || strings.Contains(lower, "local") {
			hasZeroCost = true
		}
	}

	return &GatewayRoutingReport{
		GatewayHost:  p.LiteLLMHost,
		Healthy:      true,
		HasZeroCost:  hasZeroCost,
		RoutedModels: models,
		Latency:      time.Since(start),
		Timestamp:    time.Now(),
	}, nil
}
