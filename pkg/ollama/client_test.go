package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Ping(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ollama is running"))
	}))
	defer ts.Close()

	customHTTP := &http.Client{Timeout: 5 * time.Second}
	client := NewClient(
		WithBaseURL(ts.URL),
		WithHTTPClient(customHTTP),
		WithTimeout(2*time.Second),
	)

	if client.BaseURL() != ts.URL {
		t.Errorf("expected BaseURL %s, got %s", ts.URL, client.BaseURL())
	}

	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("expected ping to succeed, got %v", err)
	}

	// Ping failure with non-200
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer badServer.Close()
	badClient := NewClient(WithBaseURL(badServer.URL))
	if err := badClient.Ping(context.Background()); err == nil {
		t.Error("expected error for 503 ping, got nil")
	}
}

func TestClient_ListModels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		resp := ListModelsResponse{
			Models: []ModelInfo{
				{
					Name:  "qwen2.5-coder:14b",
					Model: "qwen2.5-coder:14b",
					Size:  9000000000,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(WithBaseURL(ts.URL))
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 || models[0].Name != "qwen2.5-coder:14b" {
		t.Errorf("unexpected models result: %+v", models)
	}

	// Test malformed response body
	malformedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{invalid-json"))
	}))
	defer malformedServer.Close()
	malClient := NewClient(WithBaseURL(malformedServer.URL))
	if _, err := malClient.ListModels(context.Background()); err == nil {
		t.Error("expected json decode error, got nil")
	}
}

func TestClient_Generate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		var req GenerateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Prompt == "" {
			http.Error(w, "missing prompt", http.StatusBadRequest)
			return
		}

		resp := GenerateResponse{
			Model:    string(req.Model),
			Response: "Hello from local Ollama!",
			Done:     true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(WithBaseURL(ts.URL))
	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:  "qwen2.5-coder:14b",
		Prompt: "Hello",
	})
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if resp.Response != "Hello from local Ollama!" || !resp.Done {
		t.Errorf("unexpected response: %+v", resp)
	}

	// Test malformed generate response
	malServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer malServer.Close()
	malClient := NewClient(WithBaseURL(malServer.URL))
	if _, err := malClient.Generate(context.Background(), GenerateRequest{Model: "test"}); err == nil {
		t.Error("expected decode error, got nil")
	}
}

func TestClient_GenerateStream(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		chunks := []GenerateResponse{
			{Response: "Hello", Done: false},
			{Response: " world", Done: true},
		}
		for _, c := range chunks {
			data, _ := json.Marshal(c)
			_, _ = w.Write(append(data, '\n'))
		}
	}))
	defer ts.Close()

	client := NewClient(WithBaseURL(ts.URL))
	var collected string
	err := client.GenerateStream(context.Background(), GenerateRequest{
		Model:  "qwen2.5-coder:14b",
		Prompt: "stream me",
	}, func(gr GenerateResponse) error {
		collected += gr.Response
		return nil
	})

	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if collected != "Hello world" {
		t.Errorf("expected 'Hello world', got '%s'", collected)
	}
}

func TestClient_Chat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := ChatResponse{
			Model: string(req.Model),
			Message: ChatMessage{
				Role:    "assistant",
				Content: "Code synthesis complete.",
			},
			Done: true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(WithBaseURL(ts.URL))
	resp, err := client.Chat(context.Background(), ChatRequest{
		Model: "qwen2.5-coder:14b",
		Messages: []ChatMessage{
			{Role: "user", Content: "Write an event loop"},
		},
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if resp.Message.Content != "Code synthesis complete." {
		t.Errorf("unexpected content: %s", resp.Message.Content)
	}

	// Test malformed chat response
	malServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer malServer.Close()
	malClient := NewClient(WithBaseURL(malServer.URL))
	if _, err := malClient.Chat(context.Background(), ChatRequest{Model: "test"}); err == nil {
		t.Error("expected decode error, got nil")
	}
}

func TestClient_ChatStream(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		chunks := []ChatResponse{
			{Message: ChatMessage{Role: "assistant", Content: "Part 1; "}, Done: false},
			{Message: ChatMessage{Role: "assistant", Content: "Part 2."}, Done: true},
		}
		for _, c := range chunks {
			data, _ := json.Marshal(c)
			_, _ = w.Write(append(data, '\n'))
		}
	}))
	defer ts.Close()

	client := NewClient(WithBaseURL(ts.URL))
	var result string
	err := client.ChatStream(context.Background(), ChatRequest{
		Model: "qwen2.5-coder:14b",
		Messages: []ChatMessage{
			{Role: "user", Content: "Tell me something"},
		},
	}, func(cr ChatResponse) error {
		result += cr.Message.Content
		return nil
	})

	if err != nil {
		t.Fatalf("chat stream failed: %v", err)
	}
	if result != "Part 1; Part 2." {
		t.Errorf("unexpected stream content: %s", result)
	}
}

func TestClient_CheckHealth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			resp := ListModelsResponse{
				Models: []ModelInfo{{Name: "qwen2.5-coder:14b"}},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := NewClient(WithBaseURL(ts.URL))
	health, err := client.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if !health.Reachable || health.ModelsLoaded != 1 {
		t.Errorf("unexpected health status: %+v", health)
	}

	// Test unreachable health check
	badClient := NewClient(WithBaseURL("http://127.0.0.1:54321"))
	unreachableHealth, err := badClient.CheckHealth(context.Background())
	if err == nil {
		t.Error("expected error for bad client, got nil")
	}
	if unreachableHealth.Reachable {
		t.Error("expected reachable to be false")
	}
}

func TestClient_Errors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "model 'nonexistent' not found"}`))
	}))
	defer ts.Close()

	client := NewClient(WithBaseURL(ts.URL))
	_, err := client.Generate(context.Background(), GenerateRequest{
		Model:  "nonexistent",
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.Error() == "" {
		t.Error("expected non-empty error string")
	}

	// Plain text error from server (e.g. 500)
	plainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
	}))
	defer plainServer.Close()
	plainClient := NewClient(WithBaseURL(plainServer.URL))
	_, err = plainClient.Generate(context.Background(), GenerateRequest{Model: "err"})
	if err == nil {
		t.Error("expected 500 error, got nil")
	}

	// Test stream error handling
	err = client.GenerateStream(context.Background(), GenerateRequest{
		Model:  "nonexistent",
		Prompt: "test",
	}, func(gr GenerateResponse) error { return nil })
	if err == nil {
		t.Error("expected stream error on 404")
	}

	err = client.ChatStream(context.Background(), ChatRequest{
		Model: "nonexistent",
	}, func(cr ChatResponse) error { return nil })
	if err == nil {
		t.Error("expected chat stream error on 404")
	}

	_, err = client.Chat(context.Background(), ChatRequest{Model: "nonexistent"})
	if err == nil {
		t.Error("expected chat error on 404")
	}

	_, err = client.ListModels(context.Background())
	if err == nil {
		t.Error("expected list models error on 404")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(WithBaseURL(ts.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := client.Ping(ctx)
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
}
