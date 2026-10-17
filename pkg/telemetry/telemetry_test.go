package telemetry

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fixedSensor struct {
	mu   sync.RWMutex
	temp float64
	err  error
}

func newFixedSensor(temp float64) *fixedSensor {
	return &fixedSensor{temp: temp}
}

func (s *fixedSensor) SetTemperature(t float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.temp = t
}

func (s *fixedSensor) GetSoCTemperature(_ context.Context) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.temp, s.err
}

func TestHostThermalSensor(t *testing.T) {
	ctx := context.Background()

	t.Run("reads Linux sysfs millidegrees", func(t *testing.T) {
		tmpDir := t.TempDir()
		zoneDir := filepath.Join(tmpDir, "thermal_zone0")
		_ = os.MkdirAll(zoneDir, 0755)
		_ = os.WriteFile(filepath.Join(zoneDir, "temp"), []byte("54200\n"), 0644)

		sensor := &HostThermalSensor{
			SysfsGlob: filepath.Join(tmpDir, "thermal_zone*", "temp"),
		}
		temp, err := sensor.GetSoCTemperature(ctx)
		if err != nil || temp != 54.2 {
			t.Fatalf("expected 54.2, got %v, err: %v", temp, err)
		}
	})

	t.Run("reads powermetrics output via temp-dir executable", func(t *testing.T) {
		tmpDir := t.TempDir()
		pmBin := filepath.Join(tmpDir, "powermetrics")
		script := "#!/bin/sh\necho \"CPU die temperature: 63.50 C\"\n"
		if err := os.WriteFile(pmBin, []byte(script), 0755); err != nil {
			t.Fatalf("failed to write executable: %v", err)
		}

		sensor := &HostThermalSensor{
			SysfsGlob: filepath.Join(tmpDir, "nonexistent*"),
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, pmBin, arg...)
			},
		}
		temp, err := sensor.GetSoCTemperature(ctx)
		if err != nil || temp != 63.5 {
			t.Fatalf("expected 63.5, got %v, err: %v", temp, err)
		}
	})

	t.Run("returns ErrThermalSensorUnavailable when powermetrics fails", func(t *testing.T) {
		sensor := NewHostThermalSensor()
		sensor.SysfsGlob = "/nonexistent/sysfs/*"
		sensor.CommandFunc = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "false")
		}
		_, err := sensor.GetSoCTemperature(ctx)
		if !errors.Is(err, ErrThermalSensorUnavailable) {
			t.Fatalf("expected ErrThermalSensorUnavailable, got: %v", err)
		}
	})

	t.Run("returns ErrThermalSensorUnavailable when powermetrics output has no temperature", func(t *testing.T) {
		sensor := &HostThermalSensor{
			SysfsGlob: "/nonexistent/sysfs/*",
			CommandFunc: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "echo", "no temperature info")
			},
		}
		_, err := sensor.GetSoCTemperature(ctx)
		if !errors.Is(err, ErrThermalSensorUnavailable) {
			t.Fatalf("expected ErrThermalSensorUnavailable, got: %v", err)
		}
	})
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
	sensor := newFixedSensor(45.0)

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

	sensor := newFixedSensor(48.5)
	daemon := NewDaemon(DaemonConfig{
		Interval: 20 * time.Millisecond,
	}, sensor, nil, nil, func(r TelemetryReport) {
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
