package perf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/telemetry"
)

func TestPreFlightCascadeToFallbackWithZeroPromptLoss(t *testing.T) {
	// Setup a healthy fallback server simulating Ollama Metal
	var receivedPrompt string
	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &payload)
		if len(payload.Messages) > 0 {
			receivedPrompt = payload.Messages[0].Content
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello from fallback!\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer fallbackServer.Close()

	// Target endpoint points to a non-existent port (offline engine)
	offlineEndpoint := "http://127.0.0.1:54321/v1/chat/completions"
	testPrompt := "Strict zero prompt loss invariant test for DEN-048"

	cfg := BenchmarkConfig{
		RunID:             "run-cascade-048",
		Mode:              ModeInference,
		TargetEndpoint:    offlineEndpoint,
		EngineID:          "oxillama-offline",
		Model:             "qwen2.5-coder-7b",
		VirtualUsers:      2,
		Duration:          200 * time.Millisecond,
		Prompt:            testPrompt,
		MaxTokens:         64,
		PreFlightCascade:  true,
		CascadeFallbackID: "ollama-metal",
		FailoverEndpoint:  fallbackServer.URL,
	}

	sensor := telemetry.NewMockThermalSensor(45.0)
	runner := NewSyntheticRunner(sensor)

	summary, err := runner.Execute(context.Background(), cfg)
	if err != nil {
		t.Fatalf("runner.Execute failed: %v", err)
	}

	// 1. Verify pre-flight cascade triggered
	if summary.FailoverMetrics == nil {
		t.Fatalf("expected non-nil FailoverMetrics")
	}
	if !summary.FailoverMetrics.FailoverTriggered {
		t.Errorf("expected FailoverTriggered to be true")
	}
	if !summary.FailoverMetrics.PreFlightSwitched {
		t.Errorf("expected PreFlightSwitched to be true")
	}
	if summary.FailoverMetrics.PrimaryEngineID != "oxillama-offline" {
		t.Errorf("expected PrimaryEngineID 'oxillama-offline', got %s", summary.FailoverMetrics.PrimaryEngineID)
	}
	if summary.FailoverMetrics.CascadedEngineID != "ollama-metal" {
		t.Errorf("expected CascadedEngineID 'ollama-metal', got %s", summary.FailoverMetrics.CascadedEngineID)
	}
	if len(summary.FailoverMetrics.SwitchEvents) != 1 {
		t.Fatalf("expected 1 switch event, got %d", len(summary.FailoverMetrics.SwitchEvents))
	}
	if summary.FailoverMetrics.SwitchEvents[0].FromEngine != "oxillama-offline" {
		t.Errorf("unexpected switch event from: %s", summary.FailoverMetrics.SwitchEvents[0].FromEngine)
	}

	// 2. Verify Zero Prompt Loss: Fallback server received identical prompt
	if receivedPrompt != testPrompt {
		t.Errorf("prompt loss detected! expected %q, received %q", testPrompt, receivedPrompt)
	}

	// 3. Verify Markdown Scorecard contains cascade details
	scorecard := FormatMarkdownScorecard(summary)
	if !strings.Contains(scorecard, "Pre-Flight Cascade") {
		t.Errorf("expected 'Pre-Flight Cascade' in scorecard, got:\n%s", scorecard)
	}
	if !strings.Contains(scorecard, "Zero Prompt Loss") {
		t.Errorf("expected 'Zero Prompt Loss' in scorecard")
	}
	if !strings.Contains(scorecard, "Failover Switch Events") {
		t.Errorf("expected 'Failover Switch Events' in scorecard")
	}
}
