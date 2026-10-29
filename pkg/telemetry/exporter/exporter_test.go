package exporter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/telemetry"
)

type stubSensor struct {
	temp float64
	err  error
}

func (s *stubSensor) GetSoCTemperature(ctx context.Context) (float64, error) {
	return s.temp, s.err
}

type stubSpend struct {
	spend24h float64
	spend7d  float64
	budget   float64
}

func (s *stubSpend) GetTokenSpend(ctx context.Context) (float64, float64, float64, error) {
	return s.spend24h, s.spend7d, s.budget, nil
}

type stubPinger struct {
	nodes map[string]string
}

func (s *stubPinger) PingNodes(ctx context.Context) map[string]string {
	return s.nodes
}

type errorWriter struct{}

func (errorWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write failed")
}

func TestPrometheusExporter_FormatAndServe(t *testing.T) {
	sensor := &stubSensor{temp: 82.5}
	spend := &stubSpend{spend24h: 12.45, spend7d: 68.20, budget: 50.0}
	pinger := &stubPinger{nodes: map[string]string{
		"macbook-m3": "online",
		"cloud-node": "offline",
	}}

	daemonCfg := telemetry.DaemonConfig{
		Interval:      100 * time.Millisecond,
		WarningTempC:  80.0,
		CriticalTempC: 85.0,
	}
	daemon := telemetry.NewDaemon(daemonCfg, sensor, spend, pinger, nil)

	exp := NewPrometheusExporter(daemon)
	exp.SetActiveSessions(3)
	exp.RecordGatewayRequest("ollama", "200")
	exp.RecordGatewayRequest("ollama", "200")
	exp.RecordGatewayRequest("oxillama", "429")
	exp.RecordFallbackEvent("oxillama", "ollama")
	exp.SetCustomGauge("cache_hit_ratio", 0.94)

	// Verify WriteMetrics directly
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	exp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got %s", contentType)
	}

	body := rr.Body.String()

	// Assert metric definitions and values
	expectedSnippets := []string{
		"# HELP dendrite_soc_temperature_celsius",
		"dendrite_soc_temperature_celsius 82.50",
		"dendrite_thermal_alert_status 1",
		"dendrite_thermal_critical_status 0",
		"dendrite_spend_24h_usd 12.4500",
		"dendrite_spend_7d_usd 68.2000",
		"dendrite_daily_budget_usd 50.00",
		"dendrite_active_agent_sessions 3",
		`dendrite_node_reachable{node="cloud-node",status="offline"} 0`,
		`dendrite_node_reachable{node="macbook-m3",status="online"} 1`,
		`dendrite_gateway_requests_total{engine="ollama",status="200"} 2`,
		`dendrite_gateway_requests_total{engine="oxillama",status="429"} 1`,
		`dendrite_gateway_fallback_events_total{from="oxillama",to="ollama"} 1`,
		`dendrite_custom_metric{name="cache_hit_ratio"} 0.9400`,
		"dendrite_exporter_scrape_duration_seconds",
	}

	for _, snip := range expectedSnippets {
		if !strings.Contains(body, snip) {
			t.Errorf("expected metric snippet %q not found in body:\n%s", snip, body)
		}
	}

	if exp.Handler() == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestPrometheusExporter_ThermalCriticalAndReachableNodes(t *testing.T) {
	sensor := &stubSensor{temp: 89.5} // > CriticalTempC (85.0)
	spend := &stubSpend{spend24h: 1.0, spend7d: 2.0, budget: 10.0}
	pinger := &stubPinger{nodes: map[string]string{
		"node-a": "reachable",
		"node-b": "healthy",
		"node-c": "online",
		"node-d": "degraded",
	}}

	daemonCfg := telemetry.DaemonConfig{
		Interval:      100 * time.Millisecond,
		WarningTempC:  80.0,
		CriticalTempC: 85.0,
	}
	daemon := telemetry.NewDaemon(daemonCfg, sensor, spend, pinger, nil)

	exp := NewPrometheusExporter(daemon)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	exp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()
	expectedSnippets := []string{
		"dendrite_thermal_alert_status 1",
		"dendrite_thermal_critical_status 1",
		`dendrite_node_reachable{node="node-a",status="reachable"} 1`,
		`dendrite_node_reachable{node="node-b",status="healthy"} 1`,
		`dendrite_node_reachable{node="node-c",status="online"} 1`,
		`dendrite_node_reachable{node="node-d",status="degraded"} 0`,
	}

	for _, snip := range expectedSnippets {
		if !strings.Contains(body, snip) {
			t.Errorf("expected snippet %q not found in body:\n%s", snip, body)
		}
	}
}

func TestPrometheusExporter_NilDaemonDefaults(t *testing.T) {
	exp := NewPrometheusExporter(nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	exp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "dendrite_active_agent_sessions 0") {
		t.Errorf("expected zero active sessions, got:\n%s", body)
	}
	if !strings.Contains(body, `dendrite_node_reachable{node="localhost",status="healthy"} 1`) {
		t.Errorf("expected default localhost node status, got:\n%s", body)
	}
	if !strings.Contains(body, `dendrite_gateway_requests_total{engine="default",status="200"} 0`) {
		t.Errorf("expected default gateway request counter, got:\n%s", body)
	}
	if !strings.Contains(body, `dendrite_gateway_fallback_events_total{from="none",to="none"} 0`) {
		t.Errorf("expected default fallback event counter, got:\n%s", body)
	}
}

func TestPrometheusExporter_WriteMetrics_Error(t *testing.T) {
	exp := NewPrometheusExporter(nil)
	err := exp.WriteMetrics(context.Background(), errorWriter{})
	if err == nil {
		t.Fatal("expected error from WriteMetrics with errorWriter, got nil")
	}
}
