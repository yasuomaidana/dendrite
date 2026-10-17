package uitest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type scriptedUIDriver struct {
	mu       sync.Mutex
	domState map[string]string
}

func newScriptedUIDriver() *scriptedUIDriver {
	return &scriptedUIDriver{
		domState: map[string]string{
			"#scenario-selector":  "local-hybrid",
			"#spend-ceiling":      "10.00",
			"#preflight-badge":    "READY",
			"#agent-stream-panel": "Idle",
		},
	}
}

func (d *scriptedUIDriver) ExecuteJourney(ctx context.Context, journey UserJourney) (*JourneyResult, error) {
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

func (d *scriptedUIDriver) RunTelemetryBurst(_ context.Context, targetMsgsPerSec int, duration time.Duration) (*UIMetrics, error) {
	if targetMsgsPerSec <= 0 {
		targetMsgsPerSec = 100
	}
	if duration <= 0 {
		duration = 1 * time.Second
	}
	return &UIMetrics{
		LCPMs:              120.0,
		FIDMs:              8.5,
		CLS:                0.01,
		MessagesPerSec:     float64(targetMsgsPerSec),
		MinFPS:             58.4,
		MemoryLeakDetected: false,
		Success:            true,
	}, nil
}

func TestUIHarness_RunAllJourneys(t *testing.T) {
	driver := newScriptedUIDriver()
	harness := NewUIHarness(driver)

	ctx := context.Background()
	results, err := harness.RunAllJourneys(ctx)
	if err != nil {
		t.Fatalf("RunAllJourneys failed: %v", err)
	}

	if len(results) < 3 {
		t.Fatalf("expected at least 3 journey results, got %d", len(results))
	}

	for _, res := range results {
		if !res.Passed {
			t.Errorf("journey %s failed: %v", res.JourneyID, res.Errors)
		}
		if res.StepsCompleted != res.TotalSteps {
			t.Errorf("expected %d steps completed, got %d", res.TotalSteps, res.StepsCompleted)
		}
	}
}

func TestUIHarness_TelemetryBurst(t *testing.T) {
	driver := newScriptedUIDriver()
	harness := NewUIHarness(driver)

	ctx := context.Background()
	metrics, err := harness.RunTelemetryBurstTest(ctx, 100, 2*time.Second)
	if err != nil {
		t.Fatalf("RunTelemetryBurstTest failed: %v", err)
	}

	if !metrics.Success {
		t.Errorf("expected telemetry burst test success")
	}
	if metrics.MessagesPerSec != 100.0 {
		t.Errorf("expected 100 msgs/sec, got %f", metrics.MessagesPerSec)
	}
	if metrics.MinFPS < 30.0 {
		t.Errorf("expected MinFPS >= 30, got %f", metrics.MinFPS)
	}
	if metrics.MemoryLeakDetected {
		t.Errorf("unexpected memory leak detected")
	}
}

func TestUIHarness_ContextCancellationAndErrors(t *testing.T) {
	driver := newScriptedUIDriver()

	// Cancel context test
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	journey := UserJourney{
		ID:   "cancelled-journey",
		Name: "Cancelled Test",
		Steps: []Step{
			{Name: "step1", Action: "navigate", Value: "/"},
		},
	}

	res, err := driver.ExecuteJourney(ctx, journey)
	if err == nil {
		t.Errorf("expected error on cancelled context")
	}
	if res.Passed {
		t.Errorf("expected journey to fail on cancelled context")
	}

	// Failed assertion test
	badAssertJourney := UserJourney{
		ID:   "bad-assert",
		Name: "Bad Assert",
		Steps: []Step{
			{Name: "assert", Action: "assert_text", Selector: "#preflight-badge", ExpectedContent: "NON_EXISTENT_VALUE"},
		},
	}
	res2, err2 := driver.ExecuteJourney(context.Background(), badAssertJourney)
	if err2 != nil {
		t.Fatalf("unexpected driver error: %v", err2)
	}
	if res2.Passed {
		t.Errorf("expected journey with failed assertion to not pass")
	}
	if len(res2.Errors) == 0 {
		t.Errorf("expected assertion error details")
	}
}

func TestBrowserDriver(t *testing.T) {
	ctx := context.Background()

	t.Run("no browser driver installed returns ErrNoBrowserDriverInstalled", func(t *testing.T) {
		driver := NewBrowserDriver("", true)
		driver.LookPath = func(file string) (string, error) {
			return "", errors.New("not found")
		}
		harness := NewUIHarness(driver)
		if _, err := harness.RunAllJourneys(ctx); !errors.Is(err, ErrNoBrowserDriverInstalled) {
			t.Fatalf("expected ErrNoBrowserDriverInstalled, got %v", err)
		}
		if _, err := harness.RunTelemetryBurstTest(ctx, 0, 0); !errors.Is(err, ErrNoBrowserDriverInstalled) {
			t.Fatalf("expected ErrNoBrowserDriverInstalled, got %v", err)
		}
	})

	t.Run("playwright temp-dir executable succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		pwBin := filepath.Join(tmpDir, "playwright")
		script := "#!/bin/sh\necho '{\"passed\":true}'\nexit 0\n"
		if err := os.WriteFile(pwBin, []byte(script), 0755); err != nil {
			t.Fatalf("failed to write executable: %v", err)
		}

		driver := NewBrowserDriver("http://127.0.0.1:3456", true)
		driver.LookPath = func(file string) (string, error) {
			if file == "playwright" {
				return pwBin, nil
			}
			return "", errors.New("not found")
		}

		harness := NewUIHarness(driver)
		results, err := harness.RunAllJourneys(ctx)
		if err != nil || len(results) < 3 {
			t.Fatalf("unexpected error or results: err=%v, len=%d", err, len(results))
		}
		metrics, err := harness.RunTelemetryBurstTest(ctx, 0, 0)
		if err != nil || !metrics.Success {
			t.Fatalf("unexpected burst error or metrics: err=%v, metrics=%+v", err, metrics)
		}
	})

	t.Run("k6 temp-dir executable and failure handling", func(t *testing.T) {
		tmpDir := t.TempDir()
		k6Bin := filepath.Join(tmpDir, "k6")
		script := "#!/bin/sh\nexit 1\n"
		if err := os.WriteFile(k6Bin, []byte(script), 0755); err != nil {
			t.Fatalf("failed to write executable: %v", err)
		}

		driver := &BrowserDriver{
			BaseURL:  "http://127.0.0.1:3456",
			Headless: false,
			LookPath: func(file string) (string, error) {
				if file == "k6" {
					return k6Bin, nil
				}
				return "", errors.New("not found")
			},
			CommandFunc: exec.CommandContext,
		}

		_, err := driver.ExecuteJourney(ctx, CoreJourneys()[0])
		if err == nil {
			t.Fatal("expected error from failing k6 executable")
		}
		_, err = driver.RunTelemetryBurst(ctx, 100, time.Second)
		if err == nil {
			t.Fatal("expected error from failing k6 burst")
		}
	})
}
