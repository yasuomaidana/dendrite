package perf_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/perf"
)

type fixedSensor struct {
	temp float64
}

func (f *fixedSensor) GetSoCTemperature(_ context.Context) (float64, error) {
	return f.temp, nil
}

func TestEndToEndInferenceBenchmark(t *testing.T) {
	// Setup test inference endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		time.Sleep(5 * time.Millisecond) // Simulated TTFT
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Chunk 1 \"}}]}\n\n")
		flusher.Flush()

		time.Sleep(5 * time.Millisecond)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Chunk 2\"}}]}\n\n")
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	sensor := &fixedSensor{temp: 55.0}
	runner := perf.NewSyntheticRunner(sensor)

	cfg := perf.BenchmarkConfig{
		TargetEndpoint:    server.URL,
		EngineID:          "oxillama",
		Model:             "qwen2.5-coder-7b",
		VirtualUsers:      5,
		Duration:          300 * time.Millisecond,
		Prompt:            "Benchmark test prompt",
		MaxTokens:         128,
		ThermalMonitoring: true,
	}

	session := perf.NewSession(cfg)
	prov, err := session.Provision(runner)
	if err != nil {
		t.Fatalf("failed to provision session: %v", err)
	}

	ctx := context.Background()
	completed, err := prov.Run(ctx)
	if err != nil {
		t.Fatalf("benchmark run failed: %v", err)
	}

	summary := completed.Summary()
	if summary.SuccessfulReqs == 0 {
		t.Errorf("expected successful requests, got 0")
	}
	if summary.TTFTPercentiles.P50 <= 0 {
		t.Errorf("expected valid TTFT p50")
	}
	if summary.AvgTPS <= 0 {
		t.Errorf("expected valid generation TPS")
	}

	scorecard := perf.FormatMarkdownScorecard(summary)
	if !strings.Contains(scorecard, "Benchmark Scorecard") {
		t.Errorf("scorecard format invalid: %s", scorecard)
	}
}

func TestEndToEndGatewayFailoverCascade(t *testing.T) {
	// Fallback secondary tier
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"recovered via cascade\"}}]}\n\n")
			f.Flush()
			fmt.Fprintf(w, "data: [DONE]\n\n")
			f.Flush()
		}
	}))
	defer secondary.Close()

	// Primary tier that triggers 429
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"error":"rate_limit_exceeded"}`)
	}))
	defer primary.Close()

	runner := perf.NewSyntheticRunner(nil)
	cfg := perf.BenchmarkConfig{
		TargetEndpoint:   primary.URL,
		FailoverEndpoint: secondary.URL,
		EngineID:         "primary-tier",
		Model:            "gpt-4o",
		VirtualUsers:     2,
		Duration:         200 * time.Millisecond,
		FaultInjection:   perf.FaultRateLimit429,
	}

	session := perf.NewSession(cfg)
	prov, err := session.Provision(runner)
	if err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	completed, err := prov.Run(context.Background())
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	summary := completed.Summary()
	if summary.SuccessfulReqs == 0 {
		t.Errorf("expected successful requests after cascade failover")
	}
	if summary.FailoverMetrics == nil || !summary.FailoverMetrics.FailoverTriggered {
		t.Errorf("expected failover to be triggered")
	}
}
