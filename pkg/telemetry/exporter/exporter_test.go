package exporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/telemetry"
)

type mockSensor struct {
	temp float64
	err  error
}

func (m *mockSensor) GetSoCTemperature(ctx context.Context) (float64, error) {
	return m.temp, m.err
}

type mockSpend struct {
	spend24h float64
	spend7d  float64
	budget   float64
}

func (m *mockSpend) GetTokenSpend(ctx context.Context) (float64, float64, float64, error) {
	return m.spend24h, m.spend7d, m.budget, nil
}

type mockPinger struct {
	nodes map[string]string
}

func (m *mockPinger) PingNodes(ctx context.Context) map[string]string {
	return m.nodes
}

func TestPrometheusExporter_FormatAndServe(t *testing.T) {
	sensor := &mockSensor{temp: 82.5}
	spend := &mockSpend{spend24h: 12.45, spend7d: 68.20, budget: 50.0}
	pinger := &mockPinger{nodes: map[string]string{
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
}
