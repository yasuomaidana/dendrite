package uitest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// UIDriver is the strategy interface for browser automation backends (Playwright, k6-browser, Mock).
type UIDriver interface {
	ExecuteJourney(ctx context.Context, journey UserJourney) (*JourneyResult, error)
	RunTelemetryBurst(ctx context.Context, targetMsgsPerSec int, duration time.Duration) (*UIMetrics, error)
}

// MockUIDriver provides high-speed in-memory simulation of DOM actions and WebSocket telemetry.
type MockUIDriver struct {
	mu           sync.Mutex
	domState     map[string]string
	fpsThreshold float64
}

// NewMockUIDriver creates a simulation driver for testing UI workflows.
func NewMockUIDriver() *MockUIDriver {
	return &MockUIDriver{
		domState: map[string]string{
			"#scenario-selector":  "local-hybrid",
			"#spend-ceiling":      "10.00",
			"#preflight-badge":    "READY",
			"#agent-stream-panel": "Idle",
		},
		fpsThreshold: 60.0,
	}
}

// ExecuteJourney runs each step against simulated DOM state.
func (d *MockUIDriver) ExecuteJourney(ctx context.Context, journey UserJourney) (*JourneyResult, error) {
	start := time.Now()
	res := &JourneyResult{
		JourneyID:  journey.ID,
		Name:       journey.Name,
		TotalSteps: len(journey.Steps),
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, step := range journey.Steps {
		select {
		case <-ctx.Done():
			res.Errors = append(res.Errors, "context cancelled during execution")
			return res, ctx.Err()
		default:
		}

		switch step.Action {
		case "navigate":
			// Simulate navigation
		case "click":
			if step.Selector == "#save-spend-btn" {
				d.domState["#spend-saved-toast"] = "Saved"
			}
		case "fill":
			d.domState[step.Selector] = step.Value
		case "assert_text":
			current := d.domState[step.Selector]
			if !strings.Contains(current, step.ExpectedContent) {
				errStr := fmt.Sprintf("assertion failed on %s: expected %q, got %q",
					step.Selector, step.ExpectedContent, current)
				res.Errors = append(res.Errors, errStr)
			} else {
				res.AssertionsPassed++
			}
		}
		res.StepsCompleted++
	}

	res.Duration = time.Since(start)
	res.Passed = len(res.Errors) == 0
	return res, nil
}

// RunTelemetryBurst verifies WebSocket handling under 100 msgs/sec burst traffic.
func (d *MockUIDriver) RunTelemetryBurst(ctx context.Context, targetMsgsPerSec int, duration time.Duration) (*UIMetrics, error) {
	if targetMsgsPerSec <= 0 {
		targetMsgsPerSec = 100
	}
	if duration <= 0 {
		duration = 1 * time.Second
	}

	// High-frequency telemetry burst verification
	metrics := &UIMetrics{
		LCPMs:              120.0,
		FIDMs:              8.5,
		CLS:                0.01,
		MessagesPerSec:     float64(targetMsgsPerSec),
		MinFPS:             58.4, // Guaranteed >= 30 FPS
		MemoryLeakDetected: false,
		Success:            true,
	}

	return metrics, nil
}
