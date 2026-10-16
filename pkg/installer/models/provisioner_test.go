package models

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestNewProvisioner(t *testing.T) {
	t.Run("default hosts", func(t *testing.T) {
		p := NewProvisioner("", "", "")
		if p.OllamaHost != "http://127.0.0.1:11434" {
			t.Errorf("unexpected default ollama host: %s", p.OllamaHost)
		}
		if p.OxiLLaMaHost != "http://127.0.0.1:8080" {
			t.Errorf("unexpected default oxillama host: %s", p.OxiLLaMaHost)
		}
		if p.LiteLLMHost != "http://127.0.0.1:4000" {
			t.Errorf("unexpected default litellm host: %s", p.LiteLLMHost)
		}
	})

	t.Run("custom hosts with trailing slashes", func(t *testing.T) {
		p := NewProvisioner("http://localhost:11435/", "http://localhost:8081/", "http://localhost:4001/")
		if p.OllamaHost != "http://localhost:11435" {
			t.Errorf("expected trimmed ollama host: %s", p.OllamaHost)
		}
	})
}

func TestProvisioner_ProvisionOllamaModels(t *testing.T) {
	ctx := context.Background()

	t.Run("already present and pull missing model", func(t *testing.T) {
		pulledMap := make(map[string]bool)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/tags" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2:latest"}]}`))
				return
			}
			if r.URL.Path == "/api/pull" {
				pulledMap["nomic-embed-text:latest"] = true
				w.WriteHeader(http.StatusOK)
				return
			}
			http.NotFound(w, r)
		}))
		defer ts.Close()

		p := NewProvisioner(ts.URL, "", "")
		rep, err := p.ProvisionOllamaModels(ctx, nil) // tests default models
		if err != nil {
			t.Fatalf("unexpected provision error: %v", err)
		}

		if rep.Models["llama3.2:latest"] != StatusAlreadyPresent {
			t.Errorf("expected llama3.2 to be already present, got %s", rep.Models["llama3.2:latest"])
		}
		if rep.Models["nomic-embed-text:latest"] != StatusPulled {
			t.Errorf("expected nomic-embed to be pulled, got %s", rep.Models["nomic-embed-text:latest"])
		}
		if !pulledMap["nomic-embed-text:latest"] {
			t.Errorf("expected pull request to be executed")
		}
	})

	t.Run("engine unreachable", func(t *testing.T) {
		p := NewProvisioner("http://127.0.0.1:59999", "", "")
		_, err := p.ProvisionOllamaModels(ctx, []string{"llama3.2"})
		if !errors.Is(err, ErrEngineUnreachable) {
			t.Errorf("expected ErrEngineUnreachable, got %v", err)
		}
	})

	t.Run("tags returns non-200", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}))
		defer ts.Close()

		p := NewProvisioner(ts.URL, "", "")
		_, err := p.ProvisionOllamaModels(ctx, []string{"llama3.2"})
		if !errors.Is(err, ErrEngineUnreachable) {
			t.Errorf("expected ErrEngineUnreachable on non-200, got %v", err)
		}
	})

	t.Run("tags malformed json", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{not valid json`))
		}))
		defer ts.Close()

		p := NewProvisioner(ts.URL, "", "")
		_, err := p.ProvisionOllamaModels(ctx, []string{"llama3.2"})
		if !errors.Is(err, ErrInvalidResponse) {
			t.Errorf("expected ErrInvalidResponse on bad json, got %v", err)
		}
	})

	t.Run("pull fails with error status", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/tags" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"models":[]}`))
				return
			}
			if r.URL.Path == "/api/pull" {
				http.Error(w, "disk full", http.StatusInsufficientStorage)
				return
			}
		}))
		defer ts.Close()

		p := NewProvisioner(ts.URL, "", "")
		rep, err := p.ProvisionOllamaModels(ctx, []string{"model-to-fail"})
		if !errors.Is(err, ErrPullFailed) {
			t.Errorf("expected ErrPullFailed, got %v", err)
		}
		if rep.Models["model-to-fail"] != StatusFailed {
			t.Errorf("expected status FAILED, got %s", rep.Models["model-to-fail"])
		}
	})

	t.Run("pull fails with network error", func(t *testing.T) {
		hitCount := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hitCount++
			if hitCount == 1 {
				// tags succeeds
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"models":[]}`))
				return
			}
		}))
		// Close server immediately so pull fails
		p := NewProvisioner(ts.URL, "", "")
		ts.Close()

		err := p.pullModel(ctx, "any-model")
		if err == nil {
			t.Errorf("expected network error on pull")
		}
	})

	t.Run("invalid request URL", func(t *testing.T) {
		p := NewProvisioner("http://\x7f", "http://\x7f", "http://\x7f")
		_, err := p.ProvisionOllamaModels(ctx, []string{"m"})
		if err == nil {
			t.Errorf("expected URL creation error on tags")
		}
		err = p.pullModel(ctx, "m")
		if err == nil {
			t.Errorf("expected URL creation error on pull")
		}
		_, err = p.ValidateLiteLLMRouting(ctx)
		if err == nil {
			t.Errorf("expected URL creation error on litellm")
		}
	})
}

func TestProvisioner_ProbeOxiLLaMa(t *testing.T) {
	ctx := context.Background()

	t.Run("active daemon with models", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/models" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":[{"id":"qwen3.8:27b"},{"id":"qwen3.5:9b"}]}`))
				return
			}
			http.NotFound(w, r)
		}))
		defer ts.Close()

		p := NewProvisioner("", ts.URL, "")
		st, err := p.ProbeOxiLLaMa(ctx)
		if err != nil {
			t.Fatalf("unexpected probe error: %v", err)
		}
		if st.State != OxiLLaMaActive {
			t.Errorf("expected OxiLLaMaActive, got %s", st.State)
		}
		if len(st.LoadedModels) != 2 {
			t.Errorf("expected 2 loaded models, got %d", len(st.LoadedModels))
		}
	})

	t.Run("daemon offline, binary found in PATH", func(t *testing.T) {
		p := NewProvisioner("", "http://127.0.0.1:59998", "")
		p.BinaryLookupFn = func(file string) (string, error) {
			if file == "oxillama" {
				return "/opt/homebrew/bin/oxillama", nil
			}
			return "", exec.ErrNotFound
		}

		st, err := p.ProbeOxiLLaMa(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if st.State != OxiLLaMaBinaryFound {
			t.Errorf("expected OxiLLaMaBinaryFound, got %s", st.State)
		}
		if st.BinaryPath != "/opt/homebrew/bin/oxillama" {
			t.Errorf("unexpected binary path: %s", st.BinaryPath)
		}
	})

	t.Run("daemon offline, binary not found", func(t *testing.T) {
		p := NewProvisioner("", "http://127.0.0.1:59998", "")
		p.BinaryLookupFn = func(file string) (string, error) {
			return "", exec.ErrNotFound
		}

		st, err := p.ProbeOxiLLaMa(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if st.State != OxiLLaMaStandby {
			t.Errorf("expected OxiLLaMaStandby, got %s", st.State)
		}
		if !strings.Contains(st.Message, "Ready for on-demand launch") {
			t.Errorf("expected standby message, got: %s", st.Message)
		}
	})
}

func TestProvisioner_ValidateLiteLLMRouting(t *testing.T) {
	ctx := context.Background()

	t.Run("healthy gateway with zero cost models", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/models" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":[{"id":"llama3.2:latest"},{"id":"claude-3-opus"}]}`))
				return
			}
			http.NotFound(w, r)
		}))
		defer ts.Close()

		p := NewProvisioner("", "", ts.URL)
		rep, err := p.ValidateLiteLLMRouting(ctx)
		if err != nil {
			t.Fatalf("unexpected routing error: %v", err)
		}
		if !rep.Healthy || !rep.HasZeroCost {
			t.Errorf("expected healthy with zero-cost, got %+v", rep)
		}
		if len(rep.RoutedModels) != 2 {
			t.Errorf("expected 2 routed models, got %d", len(rep.RoutedModels))
		}
	})

	t.Run("offline gateway", func(t *testing.T) {
		p := NewProvisioner("", "", "http://127.0.0.1:59997")
		rep, err := p.ValidateLiteLLMRouting(ctx)
		if !errors.Is(err, ErrGatewayOffline) {
			t.Errorf("expected ErrGatewayOffline, got %v", err)
		}
		if rep.Healthy {
			t.Errorf("expected Healthy = false")
		}
	})

	t.Run("gateway non-200", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}))
		defer ts.Close()

		p := NewProvisioner("", "", ts.URL)
		_, err := p.ValidateLiteLLMRouting(ctx)
		if !errors.Is(err, ErrGatewayOffline) {
			t.Errorf("expected ErrGatewayOffline on 502, got %v", err)
		}
	})

	t.Run("gateway malformed json", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`invalid json`))
		}))
		defer ts.Close()

		p := NewProvisioner("", "", ts.URL)
		_, err := p.ValidateLiteLLMRouting(ctx)
		if !errors.Is(err, ErrInvalidResponse) {
			t.Errorf("expected ErrInvalidResponse on bad json, got %v", err)
		}
	})
}
