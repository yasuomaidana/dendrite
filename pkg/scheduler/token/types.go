package token

import (
	"errors"
	"time"
)

var (
	// ErrInvalidPolicy is returned when scheduler policy parameters are invalid.
	ErrInvalidPolicy = errors.New("invalid scheduler policy")
	// ErrSchedulerPaused is returned when trying to trigger an analysis while paused.
	ErrSchedulerPaused = errors.New("scheduler is paused")
	// ErrQuotaFloorReached is returned when remaining quota reaches the reserve floor.
	ErrQuotaFloorReached = errors.New("quota floor reached (safety reserve preserved)")
	// ErrNoEligibleModel is returned when no model matches the active tier.
	ErrNoEligibleModel = errors.New("no eligible model for active tier")
)

// ExecutionTier represents the active processing mode governed by the token schedule.
type ExecutionTier string

const (
	// TierZeroTokenPass executes AST/CFG extraction with 0 cloud tokens.
	TierZeroTokenPass ExecutionTier = "ZERO_TOKEN_PASS"
	// TierNightLocal executes semantic analysis using local zero-cost models (23:00-06:00).
	TierNightLocal ExecutionTier = "NIGHT_LOCAL"
	// TierCloudSprint executes accelerated analysis using paid cloud models (e.g. Gemini Pro).
	TierCloudSprint ExecutionTier = "CLOUD_SPRINT"
	// TierQuotaFloorPaused holds cloud calls to preserve the 20% safety reserve.
	TierQuotaFloorPaused ExecutionTier = "QUOTA_FLOOR_PAUSED"
	// TierPaused indicates an explicit user-paused state.
	TierPaused ExecutionTier = "PAUSED"
)

// Clock provides an interface for time measurement, allowing deterministic testing with fixed clocks.
type Clock interface {
	Now() time.Time
}

// RealClock implements Clock using the system clock.
type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now()
}

// Policy defines scheduling rules, time windows, and token ceilings.
type Policy struct {
	NightWindow       string        `json:"night_window"`        // e.g. "23:00-06:00"
	WeeklyReset       string        `json:"weekly_reset"`        // e.g. "SUN 23:59"
	SprintBeforeReset time.Duration `json:"sprint_before_reset"` // e.g. 48h
	ReserveFloor      float64       `json:"reserve_floor"`       // e.g. 0.20 (20% safety floor)
	AllowZero         bool          `json:"allow_zero"`          // allow draining down to 0% quota
	Mode              string        `json:"mode"`                // "auto", "always", "night", "sprint", "paused"
}

// SchedulerStatus summarizes the current state and next transition.
type SchedulerStatus struct {
	CurrentTime         time.Time     `json:"current_time"`
	ActiveTier          ExecutionTier `json:"active_tier"`
	InNightWindow       bool          `json:"in_night_window"`
	InSprintWindow      bool          `json:"in_sprint_window"`
	NextReset           time.Time     `json:"next_reset"`
	TimeUntilReset      time.Duration `json:"time_until_reset"`
	TimeUntilSprint     time.Duration `json:"time_until_sprint"`
	RemainingQuotaRatio float64       `json:"remaining_quota_ratio"`
	CloudSpendUSD       float64       `json:"cloud_spend_usd"`
	SelectedModel       string        `json:"selected_model"`
	Reason              string        `json:"reason"`
	Policy              Policy        `json:"policy"`
}
