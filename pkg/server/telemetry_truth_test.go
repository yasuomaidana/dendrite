package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestSnapshotKeepsTheFrameTimeAndFlagsOldFrames(t *testing.T) {
	hub := NewTelemetryHub()
	old := time.Now().UTC().Add(-time.Hour)
	hub.Broadcast(TelemetryFrame{Timestamp: old, SoCTemperatureC: 55})
	f := snapshotOf(t, NewServer(ServerConfig{}, nil, hub))
	if !f.Stale || !f.Timestamp.Equal(old) {
		t.Fatalf("an hour-old frame must keep its time and be stale: %+v", f)
	}
	hub.Broadcast(TelemetryFrame{SoCTemperatureC: 56})
	if f = snapshotOf(t, NewServer(ServerConfig{}, nil, hub)); f.Stale || time.Since(f.Timestamp) > time.Minute {
		t.Fatalf("a fresh frame is stamped when broadcast and is not stale: %+v", f)
	}
}

// countingSensor is a ThermalSensor that returns a fixed reading and counts its reads.
type countingSensor struct {
	temp  float64
	err   error
	reads int
}

func (c *countingSensor) GetSoCTemperature(context.Context) (float64, error) {
	c.reads++
	return c.temp, c.err
}

func TestThermalReadsAreCachedAndZeroIsNotATemperature(t *testing.T) {
	cases := []struct {
		name    string
		sensor  *countingSensor
		wantGap bool
	}{
		{"measured", &countingSensor{temp: 61}, false},
		{"zero reading", &countingSensor{temp: 0}, true},
		{"failing sensor", &countingSensor{err: errors.New("no powermetrics")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewServer(ServerConfig{}, nil, nil)
			s.thermal = newThermalCache(c.sensor)
			for i := 0; i < 5; i++ {
				f := snapshotOf(t, s)
				gap := strings.Contains(strings.Join(f.Unavailable, ","), "soc_temperature_c")
				if gap != c.wantGap || (c.wantGap && f.SoCTemperatureC != 0) {
					t.Fatalf("temperature gap=%v want %v: %+v", gap, c.wantGap, f)
				}
			}
			if c.sensor.reads != 1 {
				t.Fatalf("five snapshots must read the sensor once, read %d times", c.sensor.reads)
			}
		})
	}
}

func TestThermalCacheReadsAgainAfterTheTTL(t *testing.T) {
	sensor := &countingSensor{temp: 50}
	c := newThermalCache(sensor)
	now := time.Now()
	c.now = func() time.Time { return now }
	for _, step := range []time.Duration{0, time.Second, thermalCacheTTL + time.Second} {
		now = now.Add(step)
		if _, err := c.read(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if sensor.reads != 2 {
		t.Fatalf("want 2 reads (first, and after the TTL), got %d", sensor.reads)
	}
}
