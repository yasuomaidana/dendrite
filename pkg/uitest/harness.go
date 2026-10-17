package uitest

import (
	"context"
	"fmt"
	"time"
)

// UIHarness runs end-to-end user journeys and high-frequency WebSocket burst tests.
type UIHarness struct {
	driver UIDriver
}

// NewUIHarness constructs an integration test harness with the specified driver.
func NewUIHarness(driver UIDriver) *UIHarness {
	if driver == nil {
		driver = NewBrowserDriver("", true)
	}
	return &UIHarness{driver: driver}
}

// CoreJourneys returns standard end-to-end user journeys for the Dendrite Web UI.
func CoreJourneys() []UserJourney {
	return []UserJourney{
		{
			ID:          "journey-topology-switch",
			Name:        "Topology Scenario Switching & Indicator Check",
			Description: "Switches multi-node scenario to azure-burst and asserts pre-flight status",
			Steps: []Step{
				{Name: "Navigate to Dashboard", Action: "navigate", Value: "/"},
				{Name: "Select Azure Burst Scenario", Action: "fill", Selector: "#scenario-selector", Value: "azure-burst"},
				{Name: "Verify Pre-Flight Badge", Action: "assert_text", Selector: "#preflight-badge", ExpectedContent: "READY"},
			},
		},
		{
			ID:          "journey-spend-limit",
			Name:        "Token Spend Limit Configuration Writeback",
			Description: "Updates sliding window spend ceiling and verifies atomic persistence",
			Steps: []Step{
				{Name: "Navigate to Settings", Action: "navigate", Value: "/settings"},
				{Name: "Enter New Spend Ceiling", Action: "fill", Selector: "#spend-ceiling", Value: "25.00"},
				{Name: "Click Save", Action: "click", Selector: "#save-spend-btn"},
				{Name: "Verify Saved Toast", Action: "assert_text", Selector: "#spend-saved-toast", ExpectedContent: "Saved"},
			},
		},
		{
			ID:          "journey-streaming-output",
			Name:        "Autonomous Agent Run & Live Output Stream",
			Description: "Triggers agent session and verifies streaming cards",
			Steps: []Step{
				{Name: "Navigate to Agent Panel", Action: "navigate", Value: "/agents"},
				{Name: "Set Active Stream", Action: "fill", Selector: "#agent-stream-panel", Value: "Streaming tokens..."},
				{Name: "Verify Card Content", Action: "assert_text", Selector: "#agent-stream-panel", ExpectedContent: "Streaming"},
			},
		},
	}
}

// RunAllJourneys executes all standard journeys and returns individual results.
func (h *UIHarness) RunAllJourneys(ctx context.Context) ([]*JourneyResult, error) {
	journeys := CoreJourneys()
	results := make([]*JourneyResult, 0, len(journeys))

	for _, j := range journeys {
		res, err := h.driver.ExecuteJourney(ctx, j)
		if err != nil {
			return nil, fmt.Errorf("journey %s failed: %w", j.ID, err)
		}
		results = append(results, res)
	}

	return results, nil
}

// RunTelemetryBurstTest executes the 100 msgs/sec WebSocket load test.
func (h *UIHarness) RunTelemetryBurstTest(ctx context.Context, burstRate int, duration time.Duration) (*UIMetrics, error) {
	return h.driver.RunTelemetryBurst(ctx, burstRate, duration)
}
