package uitest

import (
	"context"
	"testing"
	"time"
)

func TestUIHarness_RunAllJourneys(t *testing.T) {
	driver := NewMockUIDriver()
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
	driver := NewMockUIDriver()
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
	driver := NewMockUIDriver()

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
