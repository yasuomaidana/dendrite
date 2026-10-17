package tokens

import (
	"Dendrite/pkg/statedir"
	"time"
)

// WindowTokenMetrics captures capacity, consumed, and remaining token metrics for a time horizon.
type WindowTokenMetrics struct {
	WindowName      string        `json:"window_name"`
	TotalCapacity   int64         `json:"total_capacity"`
	ConsumedTokens  int64         `json:"consumed_tokens"`
	RemainingTokens int64         `json:"remaining_tokens"`
	RemainingRatio  float64       `json:"remaining_ratio"`
	BudgetUSD       float64       `json:"budget_usd"`
	SpentUSD        float64       `json:"spent_usd"`
	RemainingUSD    float64       `json:"remaining_usd"`
	ResetTimestamp  time.Time     `json:"reset_timestamp"`
	TimeUntilReset  time.Duration `json:"time_until_reset"`
}

// AGYAccessStatus provides verified connectivity and readiness status for Google Antigravity / AGY.
type AGYAccessStatus struct {
	AccessConfirmed   bool          `json:"access_confirmed"`
	StatusReason      string        `json:"status_reason"`
	ActiveModel       string        `json:"active_model"`
	GatewayBaseURL    string        `json:"gateway_base_url"`
	CLIAvailable      bool          `json:"cli_available"`
	CLIPath           string        `json:"cli_path,omitempty"`
	CLIVersion        string        `json:"cli_version,omitempty"`
	InNightWindow     bool          `json:"in_night_window"`
	InReserveFloor    bool          `json:"in_reserve_floor"`
	ReserveFloorRatio float64       `json:"reserve_floor_ratio"`
	ProbeSuccess      bool          `json:"probe_success"`
	ProbeLatency      time.Duration `json:"probe_latency,omitempty"`
}

// TokenStatusReport consolidates day and week window token availability with AGY access verification.
type TokenStatusReport struct {
	Timestamp   time.Time          `json:"timestamp"`
	DayWindow   WindowTokenMetrics `json:"day_window"`
	WeekWindow  WindowTokenMetrics `json:"week_window"`
	AGYStatus   AGYAccessStatus    `json:"agy_status"`
	CanRunCloud bool               `json:"can_run_cloud"`
}

// TokenTrackerConfig allows customizing capacity ceilings, file paths, and model selections.
type TokenTrackerConfig struct {
	DayCapacityTokens  int64   `json:"day_capacity_tokens"`
	WeekCapacityTokens int64   `json:"week_capacity_tokens"`
	DayBudgetUSD       float64 `json:"day_budget_usd"`
	WeekBudgetUSD      float64 `json:"week_budget_usd"`
	Model              string  `json:"model"`
	GatewayBaseURL     string  `json:"gateway_base_url"`
	PolicyFile         string  `json:"policy_file"`
	SpendFile          string  `json:"spend_file"`
}

// DefaultConfig returns default limits tuned for commercial Gemini Pro / Gemini Flash quotas.
func DefaultConfig() TokenTrackerConfig {
	return TokenTrackerConfig{
		DayCapacityTokens:  1000000, // 1,000,000 tokens / day
		WeekCapacityTokens: 7000000, // 7,000,000 tokens / week
		DayBudgetUSD:       50.0,
		WeekBudgetUSD:      350.0,
		Model:              "gemini-3.8-flash-high",
		GatewayBaseURL:     "http://localhost:4000/v1",
		PolicyFile:         statedir.Resolve(".dendrite/scheduler_policy.json"),
		SpendFile:          statedir.Resolve(".dendrite/spend.json"),
	}
}
