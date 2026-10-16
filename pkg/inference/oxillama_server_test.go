package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOxiLLaMaServer_LifecycleAndAPI(t *testing.T) {
	tmpDir := t.TempDir()
	modelsDir := filepath.Join(tmpDir, "models")
	_ = os.MkdirAll(filepath.Join(modelsDir, "qwen3.5_9b"), 0755)

	manifestData := `{"name": "qwen3.5:9b", "engine": "oxillama"}`
	_ = os.WriteFile(filepath.Join(modelsDir, "qwen3.5_9b", "oxillama-manifest.json"), []byte(manifestData), 0644)

	srv := NewOxiLLaMaServer(OxiLLaMaServerConfig{
		Host:      "127.0.0.1",
		Port:      18080,
		ModelsDir: modelsDir,
	})

	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() {
		_ = srv.Stop(context.Background())
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /health
	resp, err := client.Get("http://127.0.0.1:18080/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 2. GET /v1/models
	resp, err = client.Get("http://127.0.0.1:18080/v1/models")
	if err != nil {
		t.Fatalf("models request failed: %v", err)
	}
	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&modelsResp)
	_ = resp.Body.Close()
	if len(modelsResp.Data) == 0 || modelsResp.Data[0].ID != "qwen3.5:9b" {
		t.Errorf("unexpected models resp: %+v", modelsResp)
	}

	// 3. POST /v1/chat/completions (Non-streaming)
	reqPayload := map[string]interface{}{
		"model": "qwen3.5:9b",
		"messages": []map[string]string{
			{"role": "user", "content": "Explain pure-Rust zero-FFI inference."},
		},
		"stream": false,
	}
	bodyBytes, _ := json.Marshal(reqPayload)
	resp, err = client.Post("http://127.0.0.1:18080/v1/chat/completions", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("chat completion failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&chatResp)
	_ = resp.Body.Close()

	if len(chatResp.Choices) == 0 || !strings.Contains(chatResp.Choices[0].Message.Content, "pure-Rust") {
		t.Errorf("unexpected chat response: %+v", chatResp)
	}

	// 4. POST /v1/chat/completions (Streaming)
	reqPayload["stream"] = true
	bodyBytes, _ = json.Marshal(reqPayload)
	resp, err = client.Post("http://127.0.0.1:18080/v1/chat/completions", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("streaming request failed: %v", err)
	}
	streamBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(streamBody), "[DONE]") {
		t.Errorf("expected [DONE] in stream, got %s", string(streamBody))
	}

	// 5. POST /v1/chat/completions (Missing model)
	reqPayload["model"] = "nonexistent-model"
	reqPayload["stream"] = false
	bodyBytes, _ = json.Marshal(reqPayload)
	resp, err = client.Post("http://127.0.0.1:18080/v1/chat/completions", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for missing model, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}
