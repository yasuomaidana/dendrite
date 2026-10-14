package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMockThermalSensor(t *testing.T) {
	sensor := NewMockThermalSensor(42.0)
	ctx := context.Background()

	temp, err := sensor.GetSoCTemperature(ctx)
	if err != nil || temp != 42.0 {
		t.Fatalf("expected 42.0, got %v, err: %v", temp, err)
	}

	sensor.SetTemperature(81.5)
	temp, _ = sensor.GetSoCTemperature(ctx)
	if temp != 81.5 {
		t.Errorf("expected 81.5 after SetTemperature, got %v", temp)
	}
}

func TestParsePowermetricsOutput(t *testing.T) {
	sample := `
*** Running powermetrics ***
Machine model: Mac14,6
CPU die temperature: 62.40 C
GPU die temperature: 55.10 C
`
	temp := ParsePowermetricsOutput(sample)
	if temp != 62.40 {
		t.Errorf("expected 62.40 C, got %v", temp)
	}

	fallback := ParsePowermetricsOutput("no matching temp data here")
	if fallback != 45.0 {
		t.Errorf("expected fallback 45.0, got %v", fallback)
	}
}

func TestFileSpendProvider(t *testing.T) {
	ctx := context.Background()

	// 1. Missing file
	provMissing := NewFileSpendProvider("/non/existent/spend.json", 100.0)
	s24, s7d, b, err := provMissing.GetTokenSpend(ctx)
	if err != nil || s24 != 0.0 || s7d != 0.0 || b != 100.0 {
		t.Errorf("unexpected results for missing file: %v, %v, %v", s24, s7d, b)
	}

	// 2. Corrupt file
	tempDir := t.TempDir()
	corruptFile := filepath.Join(tempDir, "corrupt.json")
	_ = os.WriteFile(corruptFile, []byte("{bad json"), 0644)
	provCorrupt := NewFileSpendProvider(corruptFile, 0) // Tests budget <= 0 default
	s24, s7d, b, _ = provCorrupt.GetTokenSpend(ctx)
	if b != 50.0 || s24 != 0.0 {
		t.Errorf("expected default budget 50.0, got %v", b)
	}

	// 3. Valid spend file
	validFile := filepath.Join(tempDir, "spend.json")
	validJSON := `{"total_spend_usd": 15.50, "spend_24h_usd": 10.25, "spend_7d_usd": 35.00}`
	_ = os.WriteFile(validFile, []byte(validJSON), 0644)
	provValid := NewFileSpendProvider(validFile, 75.0)
	s24, s7d, b, _ = provValid.GetTokenSpend(ctx)
	if s24 != 10.25 || s7d != 35.00 || b != 75.0 {
		t.Errorf("unexpected spend values: 24h=%v, 7d=%v, b=%v", s24, s7d, b)
	}

	// 4. Fallback to total_spend_usd when 24h/7d is zero
	fallbackFile := filepath.Join(tempDir, "fallback.json")
	_ = os.WriteFile(fallbackFile, []byte(`{"total_spend_usd": 18.20}`), 0644)
	provFallback := NewFileSpendProvider(fallbackFile, 50.0)
	s24, s7d, _, _ = provFallback.GetTokenSpend(ctx)
	if s24 != 18.20 || s7d != 18.20 {
		t.Errorf("expected fallback to total_spend_usd 18.20, got: %v, %v", s24, s7d)
	}
}

func TestStaticNodePinger(t *testing.T) {
	ctx := context.Background()

	// Default
	defPinger := NewStaticNodePinger(nil)
	nodes := defPinger.PingNodes(ctx)
	if len(nodes) < 4 || nodes["MacBook"] != "online" {
		t.Errorf("unexpected default nodes: %v", nodes)
	}

	// Custom
	customPinger := NewStaticNodePinger(map[string]string{"NodeA": "active"})
	customNodes := customPinger.PingNodes(ctx)
	if len(customNodes) != 1 || customNodes["NodeA"] != "active" {
		t.Errorf("unexpected custom nodes: %v", customNodes)
	}
}

func TestDaemon_ThermalAlertThresholds(t *testing.T) {
	ctx := context.Background()
	sensor := NewMockThermalSensor(45.0)

	daemon := NewDaemon(DaemonConfig{
		WarningTempC:  80.0,
		CriticalTempC: 85.0,
	}, sensor, nil, nil, nil)

	// 1. Normal temperature (45.0C)
	rep := daemon.CollectOnce(ctx)
	if rep.ThermalAlert || rep.ThermalCritical {
		t.Errorf("expected no alert at 45.0C: %+v", rep)
	}

	// 2. Warning temperature (81.0C)
	sensor.SetTemperature(81.0)
	repWarn := daemon.CollectOnce(ctx)
	if !repWarn.ThermalAlert || repWarn.ThermalCritical {
		t.Errorf("expected warning alert at 81.0C: %+v", repWarn)
	}

	// 3. Critical temperature (86.0C)
	sensor.SetTemperature(86.0)
	repCrit := daemon.CollectOnce(ctx)
	if !repCrit.ThermalAlert || !repCrit.ThermalCritical {
		t.Errorf("expected critical alert at 86.0C: %+v", repCrit)
	}
}

func TestDaemon_StartAndStop(t *testing.T) {
	var mu sync.Mutex
	var collected []TelemetryReport

	daemon := NewDaemon(DaemonConfig{
		Interval: 20 * time.Millisecond,
	}, nil, nil, nil, func(r TelemetryReport) {
		mu.Lock()
		defer mu.Unlock()
		collected = append(collected, r)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start in background goroutine
	started := make(chan struct{})
	go func() {
		close(started)
		daemon.Start(ctx)
	}()
	<-started

	// Wait briefly so daemon sets d.running = true
	time.Sleep(20 * time.Millisecond)

	// Calling Start again while running should be an immediate no-op
	daemon.Start(ctx)

	// Wait for ticks
	time.Sleep(60 * time.Millisecond)

	mu.Lock()
	count := len(collected)
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 reports, got %d", count)
	}

	// Test Stop()
	daemon.Stop()
	// Test calling Stop() again (idempotent)
	daemon.Stop()
}
