package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func snapshotOf(t *testing.T, s *Server) TelemetryFrame {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleTelemetrySnapshot(rec, httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/snapshot", nil))
	var f TelemetryFrame
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSnapshotReportsWhatIsUnmeasuredInsteadOfInventingIt(t *testing.T) {
	s := NewServer(ServerConfig{}, nil, nil)
	f := snapshotOf(t, s)
	if f.TokenSpend24h != 0 || f.TokenSpend7d != 0 || f.DailyBudgetUSD != 0 || len(f.NodeHealth) != 0 {
		t.Fatalf("spend, budget and node health have no source and must not carry numbers: %+v", f)
	}
	joined := strings.Join(f.Unavailable, ",")
	for _, k := range []string{"token_spend_24h_usd", "token_spend_7d_usd", "daily_budget_usd", "node_health"} {
		if !strings.Contains(joined, k) {
			t.Fatalf("%s must be listed unavailable, got %v", k, f.Unavailable)
		}
	}
}

func TestSnapshotServesTheLatestBroadcastFrame(t *testing.T) {
	hub := NewTelemetryHub()
	hub.Broadcast(TelemetryFrame{SoCTemperatureC: 61.5, TokenSpend24h: 3.25, DailyBudgetUSD: 10})
	f := snapshotOf(t, NewServer(ServerConfig{}, nil, hub))
	if f.SoCTemperatureC != 61.5 || f.TokenSpend24h != 3.25 || len(f.Unavailable) != 0 {
		t.Fatalf("the measured frame must be served as is: %+v", f)
	}
}

func TestHealthReportsTheConfiguredVersion(t *testing.T) {
	for want, cfg := range map[string]string{"0.1.0": "0.1.0", "dev": ""} {
		rec := httptest.NewRecorder()
		NewServer(ServerConfig{Version: cfg}, nil, nil).handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
		if !strings.Contains(rec.Body.String(), `"version":"`+want+`"`) {
			t.Fatalf("version %q not reported: %s", want, rec.Body.String())
		}
	}
}

func TestDashboardShowsNoInventedReadings(t *testing.T) {
	rec := httptest.NewRecorder()
	NewServer(ServerConfig{}, nil, nil).handleDashboard(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, fake := range []string{"48.5", "12.45"} {
		if strings.Contains(rec.Body.String(), fake) {
			t.Fatalf("dashboard still hard-codes %s", fake)
		}
	}
}
