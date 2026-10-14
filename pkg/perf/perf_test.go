package perf

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"Dendrite/pkg/telemetry"
)

func TestTypestateLifecycle(t *testing.T) {
	cfg := BenchmarkConfig{
		TargetEndpoint: "http://localhost:8080/v1/chat/completions",
		EngineID:       "oxillama",
		Model:          "qwen2.5-coder-7b",
		VirtualUsers:   2,
		Duration:       50 * time.Millisecond,
	}

	session := NewSession(cfg)
	if session.ID() == "" {
		t.Fatalf("expected generated session ID, got empty")
	}
	if session.Config().EngineID != "oxillama" {
		t.Errorf("expected engine oxillama in config, got %s", session.Config().EngineID)
	}

	// Cannot provision with nil runner
	if _, err := session.Provision(nil); err == nil {
		t.Fatalf("expected error provisioning nil runner")
	}

	// Missing endpoint on non-matrix mode fails validation
	emptyCfg := BenchmarkConfig{Mode: ModeInference}
	emptySess := NewSession(emptyCfg)
	runner := NewSyntheticRunner(nil)
	if _, err := emptySess.Provision(runner); err == nil {
		t.Fatalf("expected error provisioning session with missing endpoint")
	}

	prov, err := session.Provision(runner)
	if err != nil {
		t.Fatalf("failed to provision session: %v", err)
	}

	if prov.Config().EngineID != "oxillama" {
		t.Errorf("expected engine oxillama, got %s", prov.Config().EngineID)
	}
	if prov.ID() != session.ID() {
		t.Errorf("expected matched session ID")
	}
	if prov.Runner().Name() != "synthetic-load-engine" {
		t.Errorf("expected synthetic-load-engine, got %s", prov.Runner().Name())
	}
}

func TestPercentilesCalculation(t *testing.T) {
	samples := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}

	p := CalculatePercentiles(samples)
	if p.Min != 10*time.Millisecond {
		t.Errorf("expected min 10ms, got %v", p.Min)
	}
	if p.Max != 100*time.Millisecond {
		t.Errorf("expected max 100ms, got %v", p.Max)
	}
	if p.P50 != 50*time.Millisecond {
		t.Errorf("expected p50 50ms, got %v", p.P50)
	}
	if p.P90 != 90*time.Millisecond {
		t.Errorf("expected p90 90ms, got %v", p.P90)
	}
	if p.P95 != 100*time.Millisecond {
		t.Errorf("expected p95 100ms, got %v", p.P95)
	}
	if p.P99 != 100*time.Millisecond {
		t.Errorf("expected p99 100ms, got %v", p.P99)
	}

	// Empty samples
	emptyP := CalculatePercentiles(nil)
	if emptyP.Min != 0 || emptyP.Max != 0 {
		t.Errorf("expected zero percentiles for empty slice")
	}
}

func TestSyntheticRunnerWithSSEMockAndRun(t *testing.T) {
	var requestCount uint64

	// Mock SSE Server emitting simulated OpenAI chat completion chunks
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&requestCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		time.Sleep(10 * time.Millisecond) // Simulated TTFT
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()

		time.Sleep(5 * time.Millisecond)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world!\"}}]}\n\n")
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	sensor := telemetry.NewMockThermalSensor(48.5)
	runner := NewSyntheticRunner(sensor)

	cfg := BenchmarkConfig{
		TargetEndpoint:    ts.URL,
		EngineID:          "mock-sse",
		Model:             "mock-model",
		VirtualUsers:      3,
		Duration:          250 * time.Millisecond,
		Prompt:            "test prompt",
		ThermalMonitoring: true,
	}

	session := NewSession(cfg)
	prov, err := session.Provision(runner)
	if err != nil {
		t.Fatalf("failed to provision session: %v", err)
	}

	ctx := context.Background()
	completed, err := prov.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error during prov.Run: %v", err)
	}

	if completed.ID() != session.ID() {
		t.Errorf("expected matching completed ID")
	}
	if completed.Config().Model != "mock-model" {
		t.Errorf("expected mock-model in completed config")
	}

	summary := completed.Summary()
	if summary == nil {
		t.Fatalf("expected non-nil summary")
	}
	if summary.SuccessfulReqs == 0 {
		t.Errorf("expected successful requests > 0, got %d", summary.SuccessfulReqs)
	}
	if summary.TTFTPercentiles.P50 <= 0 {
		t.Errorf("expected positive P50 TTFT, got %v", summary.TTFTPercentiles.P50)
	}
	if summary.AvgTPS <= 0 {
		t.Errorf("expected positive AvgTPS, got %v", summary.AvgTPS)
	}

	// Markdown and JSON output verification
	md := FormatMarkdownScorecard(summary)
	if !strings.Contains(md, "Benchmark Scorecard") || !strings.Contains(md, "TTFT (p50)") {
		t.Errorf("markdown scorecard missing expected headers: %s", md)
	}

	jsonBytes, err := summary.ToJSON()
	if err != nil || len(jsonBytes) == 0 {
		t.Errorf("failed to serialize summary to JSON: %v", err)
	}

	nilMd := FormatMarkdownScorecard(nil)
	if nilMd != "" {
		t.Errorf("expected empty string for nil summary")
	}
}

func TestSyntheticRunnerFailoverCascade(t *testing.T) {
	// Secondary failover server that succeeds
	secondaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"fallback success\"}}]}\n\n")
			f.Flush()
			fmt.Fprintf(w, "data: [DONE]\n\n")
			f.Flush()
		}
	}))
	defer secondaryServer.Close()

	// Primary server that fails with 429
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"error":"rate limit exceeded"}`)
	}))
	defer primaryServer.Close()

	runner := NewSyntheticRunner(nil)
	cfg := BenchmarkConfig{
		TargetEndpoint:   primaryServer.URL,
		FailoverEndpoint: secondaryServer.URL,
		EngineID:         "primary-tier",
		Model:            "test-model",
		VirtualUsers:     1,
		Duration:         150 * time.Millisecond,
		FaultInjection:   FaultRateLimit429,
	}

	ctx := context.Background()
	summary, err := runner.Execute(ctx, cfg)
	if err != nil {
		t.Fatalf("synthetic runner failover execution failed: %v", err)
	}

	if summary.SuccessfulReqs == 0 {
		t.Errorf("expected successful requests through failover, got 0")
	}
	if summary.FailoverMetrics == nil {
		t.Fatalf("expected non-nil failover metrics")
	}
	if !summary.FailoverMetrics.FailoverTriggered {
		t.Errorf("expected FailoverTriggered to be true")
	}
}

func TestCascadeFailoverSimulation(t *testing.T) {
	metrics, err := SimulateCascadeFailover(10, 350*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected failover error: %v", err)
	}
	if !metrics.FailoverTriggered {
		t.Errorf("expected failover to be triggered")
	}
	if metrics.LostRequests != 0 {
		t.Errorf("expected 0 lost requests, got %d", metrics.LostRequests)
	}
	if metrics.FailoverLatency >= 2*time.Second {
		t.Errorf("expected failover latency < 2s, got %v", metrics.FailoverLatency)
	}

	// Failover exceeding 2s threshold must return error
	_, errSlow := SimulateCascadeFailover(10, 2500*time.Millisecond)
	if errSlow == nil {
		t.Errorf("expected error when failover latency > 2s")
	}
}

func TestBudgetLimiterConcurrencyStress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Default config evaluation
	defResult, err := StressBudgetLimiter(ctx, GatewayStressConfig{})
	if err != nil || defResult.TotalOperations == 0 {
		t.Errorf("expected success with default config, got err=%v", err)
	}

	cfg := GatewayStressConfig{
		VirtualUsers:   50,
		TotalSpends:    300,
		BudgetCeiling:  50.0,
		SpendPerReqUSD: 0.10,
		TokensPerReq:   50,
	}

	result, err := StressBudgetLimiter(ctx, cfg)
	if err != nil {
		t.Fatalf("stress testing failed: %v", err)
	}

	if result.TotalOperations == 0 {
		t.Errorf("expected operations > 0, got 0")
	}
	if result.RaceDetected {
		t.Errorf("race condition detected during budget stress")
	}
}

func TestThermalMonitoringAndThrottling(t *testing.T) {
	sensor := telemetry.NewMockThermalSensor(88.0)
	mon := NewThermalMonitor(sensor, 0) // Test zero interval fallback

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	mon.Start(ctx)
	<-ctx.Done()

	// Initial 50 TPS, dropping to 35 TPS (30% drop) at 88°C
	metrics := mon.Evaluate(TokensPerSecond(50.0), TokensPerSecond(35.0))
	if !metrics.ThrottleDetected {
		t.Errorf("expected throttling detected")
	}
	if !strings.Contains(metrics.ThrottleWarning, "> 85°C") {
		t.Errorf("expected warning to mention > 85°C, got: %s", metrics.ThrottleWarning)
	}
	if !strings.Contains(metrics.ThrottleWarning, "dropped by") {
		t.Errorf("expected warning to mention TPS drop, got: %s", metrics.ThrottleWarning)
	}

	// Normal condition
	normalSensor := telemetry.NewMockThermalSensor(50.0)
	normalMon := NewThermalMonitor(normalSensor, 10*time.Millisecond)
	ctxNorm, cancelNorm := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelNorm()
	normalMon.Start(ctxNorm)
	<-ctxNorm.Done()

	normalMetrics := normalMon.Evaluate(TokensPerSecond(50.0), TokensPerSecond(52.0))
	if normalMetrics.ThrottleDetected {
		t.Errorf("did not expect throttle flag under normal conditions")
	}
}

func TestLocalK6RunnerAndDefaultRunner(t *testing.T) {
	runner := DefaultRunner(nil)
	if runner == nil {
		t.Fatalf("expected non-nil default runner")
	}

	local := NewLocalK6Runner(nil)
	if local.Name() != "local-k6-cli" {
		t.Errorf("expected local-k6-cli, got %s", local.Name())
	}

	// Without k6 in PATH, execute returns error
	cfg := BenchmarkConfig{
		TargetEndpoint: "http://localhost:8080",
		Model:          "test",
	}
	_, err := local.Execute(context.Background(), cfg)
	if err == nil {
		t.Errorf("expected error running local k6 without executable in PATH")
	}

	// Test parseK6Output
	summary := parseK6Output(cfg, "")
	if summary == nil || summary.AvgTPS <= 0 {
		t.Errorf("expected valid summary from parseK6Output")
	}
}

func TestGenerateK6Script(t *testing.T) {
	cfg := BenchmarkConfig{
		TargetEndpoint: "http://localhost:4000/v1/chat/completions",
		EngineID:       "litellm-gateway",
		Model:          "claude-3-5-sonnet",
		VirtualUsers:   15,
		Duration:       30 * time.Second,
		Prompt:         "Benchmark prompt with 'quotes' and \n newline",
		MaxTokens:      512,
		FaultInjection: FaultRateLimit429,
	}

	script := GenerateK6Script(cfg)
	if !strings.Contains(script, "vus: 15") {
		t.Errorf("script missing vus 15")
	}
	if !strings.Contains(script, "duration: '30s'") {
		t.Errorf("script missing duration")
	}
	if !strings.Contains(script, "X-Fault-Injection") {
		t.Errorf("script missing fault injection header")
	}
}
