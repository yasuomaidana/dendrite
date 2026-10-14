package perf

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ThermalSensor abstracts reading host SoC temperatures.
type ThermalSensor interface {
	GetSoCTemperature(ctx context.Context) (float64, error)
}

// ThermalMonitor polls thermal sensors and detects thermal-induced throttling.
type ThermalMonitor struct {
	sensor   ThermalSensor
	interval time.Duration
	mu       sync.Mutex
	temps    []float64
}

// NewThermalMonitor creates a new monitor with specified sensor and poll interval.
func NewThermalMonitor(sensor ThermalSensor, interval time.Duration) *ThermalMonitor {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &ThermalMonitor{
		sensor:   sensor,
		interval: interval,
		temps:    make([]float64, 0),
	}
}

// Start spawns a background polling routine until ctx is cancelled.
func (tm *ThermalMonitor) Start(ctx context.Context) {
	if tm.sensor == nil {
		return
	}

	// Sample once immediately on startup
	if temp, err := tm.sensor.GetSoCTemperature(ctx); err == nil && temp > 0 {
		tm.mu.Lock()
		tm.temps = append(tm.temps, temp)
		tm.mu.Unlock()
	}

	go func() {
		ticker := time.NewTicker(tm.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if temp, err := tm.sensor.GetSoCTemperature(ctx); err == nil && temp > 0 {
					tm.mu.Lock()
					tm.temps = append(tm.temps, temp)
					tm.mu.Unlock()
				}
			}
		}
	}()
}

// Evaluate analyzes collected temperatures and compares initial vs final TPS to assess throttling.
func (tm *ThermalMonitor) Evaluate(initialTPS, finalTPS TokensPerSecond) ThermalMetrics {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	metrics := ThermalMetrics{
		PeakSoCTempC: 45.0,
		AvgSoCTempC:  45.0,
	}

	if len(tm.temps) > 0 {
		var sum, maxTemp float64
		for _, t := range tm.temps {
			sum += t
			if t > maxTemp {
				maxTemp = t
			}
		}
		metrics.PeakSoCTempC = maxTemp
		metrics.AvgSoCTempC = sum / float64(len(tm.temps))
	}

	// Rule 1: SoC Temperature exceeds 85°C
	if metrics.PeakSoCTempC > 85.0 {
		metrics.ThrottleDetected = true
		metrics.ThrottleWarning = fmt.Sprintf("SoC temperature reached %.1f°C (> 85°C critical threshold)", metrics.PeakSoCTempC)
	}

	// Rule 2: TPS degradation >= 20%
	if initialTPS > 0 && finalTPS > 0 && finalTPS < initialTPS {
		dropPercent := (float64(initialTPS) - float64(finalTPS)) / float64(initialTPS) * 100.0
		if dropPercent >= 20.0 {
			metrics.ThrottleDetected = true
			warning := fmt.Sprintf("Token generation rate dropped by %.1f%% (from %.1f to %.1f tok/s)", dropPercent, initialTPS, finalTPS)
			if metrics.ThrottleWarning != "" {
				metrics.ThrottleWarning += "; " + warning
			} else {
				metrics.ThrottleWarning = warning
			}
		}
	}

	return metrics
}
