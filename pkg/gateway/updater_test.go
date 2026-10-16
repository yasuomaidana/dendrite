package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/inference"
)

func TestDefaultUpdaterOptions_EnvOverrides(t *testing.T) {
	os.Setenv("DENDRITE_LITELLM_CONFIG_PATH", "/tmp/custom_litellm.yaml")
	os.Setenv("DENDRITE_ROUTING_UPDATE_INTERVAL", "12h")
	os.Setenv("DENDRITE_ROUTING_CONTAINER_HOSTS", "false")
	os.Setenv("DENDRITE_OLLAMA_ENDPOINT", "http://ollama-custom:11434")
	os.Setenv("DENDRITE_OXILLAMA_ENDPOINT", "http://oxillama-custom:8080")
	defer func() {
		os.Unsetenv("DENDRITE_LITELLM_CONFIG_PATH")
		os.Unsetenv("DENDRITE_ROUTING_UPDATE_INTERVAL")
		os.Unsetenv("DENDRITE_ROUTING_CONTAINER_HOSTS")
		os.Unsetenv("DENDRITE_OLLAMA_ENDPOINT")
		os.Unsetenv("DENDRITE_OXILLAMA_ENDPOINT")
	}()

	opts := DefaultUpdaterOptions()
	if opts.ConfigPath != "/tmp/custom_litellm.yaml" {
		t.Errorf("expected custom config path, got %s", opts.ConfigPath)
	}
	if opts.Interval != 12*time.Hour {
		t.Errorf("expected 12h interval, got %v", opts.Interval)
	}
	if opts.UseContainerHost != false {
		t.Errorf("expected UseContainerHost=false, got %v", opts.UseContainerHost)
	}
	if opts.OllamaEndpoint != "http://ollama-custom:11434" {
		t.Errorf("expected custom ollama endpoint, got %s", opts.OllamaEndpoint)
	}
	if opts.OxillamaEndpoint != "http://oxillama-custom:8080" {
		t.Errorf("expected custom oxillama endpoint, got %s", opts.OxillamaEndpoint)
	}
}

func TestRoutingTableUpdater_DryRun_And_Precedence(t *testing.T) {
	// Create mock OxiLLaMa server with smart 27B model
	oxiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen3.8:27b"},{"id":"qwen3.5:9b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer oxiServer.Close()

	// Create mock Ollama server with smaller 3B model
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"0.5.4"}`))
			return
		}
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2:3b"},{"name":"tinyllama:latest"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ollamaServer.Close()

	mgr := inference.NewManager()
	oxiEng := inference.NewOxiLLaMaEngine(inference.EngineConfig{
		ID:       "oxillama-rust",
		Type:     inference.EngineOxiLLaMa,
		Endpoint: oxiServer.URL,
	}, oxiServer.Client())
	_ = mgr.RegisterEngine(oxiEng)

	ollamaEng := inference.NewOllamaEngine(inference.EngineConfig{
		ID:       "ollama-metal",
		Type:     inference.EngineOllama,
		Endpoint: ollamaServer.URL,
	}, ollamaServer.Client())
	_ = mgr.RegisterEngine(ollamaEng)

	updater := NewRoutingTableUpdater(UpdaterOptions{
		ConfigPath:       "/tmp/test_litellm_routing.yaml",
		DryRun:           true,
		UseContainerHost: true,
		Manager:          mgr,
	})

	ctx := context.Background()
	report, err := updater.UpdateNow(ctx)
	if err != nil {
		t.Fatalf("UpdateNow failed: %v", err)
	}

	if !report.DryRun {
		t.Errorf("expected report to be dry run")
	}
	if report.ActiveLocalPrimary != "qwen3.8:27b" {
		t.Errorf("expected smartest model 'qwen3.8:27b' to be active primary, got %s", report.ActiveLocalPrimary)
	}
	if report.ActiveLocalPrimaryEngine != "oxillama-rust" {
		t.Errorf("expected active primary engine to be 'oxillama-rust', got %s", report.ActiveLocalPrimaryEngine)
	}
	if report.ActiveLocalPrimaryScore < 85 {
		t.Errorf("expected score >= 85 for 27B model, got %d", report.ActiveLocalPrimaryScore)
	}

	// Verify rendered YAML content
	yamlStr := report.YAMLContent
	if !strings.Contains(yamlStr, "local-qwen3-27b") {
		t.Errorf("expected YAML to define local-qwen3-27b")
	}
	if !strings.Contains(yamlStr, "openai/qwen3.8:27b") {
		t.Errorf("expected YAML to map local-qwen3-27b to OxiLLaMa openai/qwen3.8:27b")
	}
	if !strings.Contains(yamlStr, "host.docker.internal") {
		t.Errorf("expected container host rewrite to host.docker.internal")
	}
	if !strings.Contains(yamlStr, "skip_budget_checks: true") {
		t.Errorf("expected zero-cost models to skip budget checks")
	}
}

func TestRoutingTableUpdater_ActualWriteAndDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "litellm_actual.yaml")

	mgr := inference.NewManager()
	// Register self-hosted engine
	_ = mgr.RegisterEngine(inference.NewOxiLLaMaEngine(inference.EngineConfig{
		ID:       "oxillama-rust",
		Type:     inference.EngineOxiLLaMa,
		Endpoint: "http://127.0.0.1:8080",
		Models:   []string{"qwen3.8:27b"},
	}, nil))

	updater := NewRoutingTableUpdater(UpdaterOptions{
		ConfigPath:       targetFile,
		DryRun:           false,
		Interval:         50 * time.Millisecond,
		UseContainerHost: false,
		Manager:          mgr,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Test manual trigger UpdateNow with actual write
	report, err := updater.UpdateNow(ctx)
	if err != nil {
		t.Fatalf("manual UpdateNow failed: %v", err)
	}
	if report.BytesWritten == 0 {
		t.Errorf("expected non-zero bytes written")
	}

	content, rErr := os.ReadFile(targetFile)
	if rErr != nil {
		t.Fatalf("failed to read generated config file: %v", rErr)
	}
	if !strings.Contains(string(content), "gemini-2.5-pro") {
		t.Errorf("expected written file to contain gemini-2.5-pro")
	}

	// 2. Test Daily Daemon with short ticker
	repCh, stopDaemon := updater.StartDailyDaemon(ctx)
	defer stopDaemon()

	select {
	case daemonRep, ok := <-repCh:
		if !ok || daemonRep == nil {
			t.Errorf("expected report from daily daemon")
		}
	case <-time.After(500 * time.Millisecond):
		t.Errorf("timed out waiting for daily daemon report")
	}

	// Verify Stop
	updater.Stop()
	if updater.running {
		t.Errorf("expected updater.running to be false after Stop()")
	}
}

func TestRoutingTableUpdater_DirectoryCreationAndErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	nestedTarget := filepath.Join(tmpDir, "nested", "sub", "litellm.yaml")

	updater := NewRoutingTableUpdater(UpdaterOptions{
		ConfigPath: nestedTarget,
		DryRun:     false,
	})

	report, err := updater.UpdateNow(context.Background())
	if err != nil {
		t.Fatalf("expected successful directory creation and write, got %v", err)
	}
	if _, statErr := os.Stat(nestedTarget); statErr != nil {
		t.Errorf("expected file %s to be created: %v", nestedTarget, statErr)
	}
	if report.TotalModelsConfigured == 0 {
		t.Errorf("expected non-zero models configured")
	}

	// Test Options and SetOptions
	opts := updater.Options()
	opts.PrimaryCloudModel = "custom-pro-model"
	updater.SetOptions(opts)
	if updater.Options().PrimaryCloudModel != "custom-pro-model" {
		t.Errorf("expected SetOptions to update PrimaryCloudModel")
	}
}
