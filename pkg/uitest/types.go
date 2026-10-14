package uitest

import "time"

// JourneyID represents a unique user workflow identifier.
type JourneyID string

// UIConfig configures the browser harness parameters.
type UIConfig struct {
	BaseURL        string        `json:"base_url"`
	Headless       bool          `json:"headless"`
	VirtualUsers   int           `json:"virtual_users"`
	Duration       time.Duration `json:"duration"`
	BurstRate      int           `json:"burst_rate"` // msgs/sec
	BrowserMetrics bool          `json:"browser_metrics"`
}

// Step represents a discrete user action in a UI journey.
type Step struct {
	Name            string `json:"name"`
	Action          string `json:"action"` // "click", "fill", "navigate", "wait_for_selector", "assert_text"
	Selector        string `json:"selector,omitempty"`
	Value           string `json:"value,omitempty"`
	ExpectedContent string `json:"expected_content,omitempty"`
}

// UserJourney specifies an end-to-end interactive workflow.
type UserJourney struct {
	ID          JourneyID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Steps       []Step    `json:"steps"`
}

// UIMetrics aggregates frontend Web Vitals and WebSocket telemetry resilience.
type UIMetrics struct {
	LCPMs              float64 `json:"lcp_ms"` // Largest Contentful Paint
	FIDMs              float64 `json:"fid_ms"` // First Input Delay
	CLS                float64 `json:"cls"`    // Cumulative Layout Shift
	MessagesPerSec     float64 `json:"messages_per_sec"`
	MinFPS             float64 `json:"min_fps"`
	MemoryLeakDetected bool    `json:"memory_leak_detected"`
	Success            bool    `json:"success"`
}

// JourneyResult captures the status of an executed UI journey.
type JourneyResult struct {
	JourneyID        JourneyID     `json:"journey_id"`
	Name             string        `json:"name"`
	Passed           bool          `json:"passed"`
	StepsCompleted   int           `json:"steps_completed"`
	TotalSteps       int           `json:"total_steps"`
	Duration         time.Duration `json:"duration"`
	AssertionsPassed int           `json:"assertions_passed"`
	Errors           []string      `json:"errors,omitempty"`
}
