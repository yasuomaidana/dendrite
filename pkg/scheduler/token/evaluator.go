package token

import (
	"fmt"
	"time"
)

// IsNightWindow determines if the given time falls within the configured night window.
func IsNightWindow(t time.Time, p *Policy) (bool, error) {
	startH, startM, endH, endM, err := ParseNightWindow(p.NightWindow)
	if err != nil {
		return false, err
	}

	currMinutes := t.Hour()*60 + t.Minute()
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	if startMinutes > endMinutes {
		// Overnight window (e.g. 23:00 to 06:00)
		return currMinutes >= startMinutes || currMinutes < endMinutes, nil
	}

	// Same-day window (e.g. 01:00 to 05:00)
	return currMinutes >= startMinutes && currMinutes < endMinutes, nil
}

// NextWeeklyReset calculates the exact timestamp of the upcoming weekly quota reset.
func NextWeeklyReset(t time.Time, p *Policy) (time.Time, error) {
	targetWeekday, targetH, targetM, err := ParseWeeklyReset(p.WeeklyReset)
	if err != nil {
		return time.Time{}, err
	}

	// Create candidate time on the same date as t
	candidate := time.Date(t.Year(), t.Month(), t.Day(), targetH, targetM, 0, 0, t.Location())

	daysAhead := (int(targetWeekday) - int(t.Weekday()) + 7) % 7
	if daysAhead == 0 {
		if candidate.After(t) {
			return candidate, nil
		}
		daysAhead = 7
	}

	candidate = candidate.AddDate(0, 0, daysAhead)
	return candidate, nil
}

// TimeUntilReset calculates the duration remaining until the upcoming weekly reset.
func TimeUntilReset(t time.Time, p *Policy) (time.Duration, error) {
	next, err := NextWeeklyReset(t, p)
	if err != nil {
		return 0, err
	}
	return next.Sub(t), nil
}

// IsSprintWindow checks whether the timestamp is within the pre-reset cloud sprint window.
func IsSprintWindow(t time.Time, p *Policy) (bool, error) {
	dur, err := TimeUntilReset(t, p)
	if err != nil {
		return false, err
	}
	return dur <= p.SprintBeforeReset, nil
}

// Evaluate performs a comprehensive evaluation of current time, policy, and quota state.
func Evaluate(t time.Time, p *Policy, remainingQuotaRatio, cloudSpend float64) (*SchedulerStatus, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	inNight, err := IsNightWindow(t, p)
	if err != nil {
		return nil, err
	}

	nextReset, err := NextWeeklyReset(t, p)
	if err != nil {
		return nil, err
	}

	timeUntilReset := nextReset.Sub(t)
	inSprint := timeUntilReset <= p.SprintBeforeReset

	var timeUntilSprint time.Duration
	if !inSprint {
		timeUntilSprint = timeUntilReset - p.SprintBeforeReset
	}

	status := &SchedulerStatus{
		CurrentTime:         t,
		InNightWindow:       inNight,
		InSprintWindow:      inSprint,
		NextReset:           nextReset,
		TimeUntilReset:      timeUntilReset,
		TimeUntilSprint:     timeUntilSprint,
		RemainingQuotaRatio: remainingQuotaRatio,
		CloudSpendUSD:       cloudSpend,
		Policy:              *p,
	}

	// Determine active execution tier and chosen model based on policy and state
	if p.Mode == "paused" {
		status.ActiveTier = TierPaused
		status.SelectedModel = "none"
		status.Reason = "Scheduler paused by user command"
		return status, nil
	}

	if p.Mode == "night" {
		status.ActiveTier = TierNightLocal
		status.SelectedModel = "local-qwen3-27b"
		status.Reason = "Forced night-local mode active: zero-cost local execution"
		return status, nil
	}

	if p.Mode == "sprint" {
		status.ActiveTier = TierCloudSprint
		status.SelectedModel = "gemini-2.5-pro"
		status.Reason = "Forced cloud-sprint mode active: accelerated cloud execution"
		return status, nil
	}

	// Automatic tier selection
	if inSprint {
		// Inside 48h sprint window
		if remainingQuotaRatio > p.ReserveFloor || (p.AllowZero && remainingQuotaRatio > 0.0) {
			status.ActiveTier = TierCloudSprint
			status.SelectedModel = "gemini-2.5-pro"
			status.Reason = fmt.Sprintf("Pre-reset cloud sprint active (%.1fh before reset): draining quota to prevent waste", timeUntilReset.Hours())
		} else {
			status.ActiveTier = TierQuotaFloorPaused
			status.SelectedModel = "local-qwen3-27b"
			status.Reason = fmt.Sprintf("Quota reached safety floor (%.1f%% remaining <= %.1f%% floor): cloud calls paused", remainingQuotaRatio*100, p.ReserveFloor*100)
		}
		return status, nil
	}

	if inNight {
		status.ActiveTier = TierNightLocal
		status.SelectedModel = "local-qwen3-27b"
		status.Reason = "Scheduled night window active (23:00-06:00): zero-cost local execution ($0.00)"
		return status, nil
	}

	// Day window outside sprint
	status.ActiveTier = TierZeroTokenPass
	status.SelectedModel = "none"
	status.Reason = "Day window outside sprint: AST/CFG extraction only at 0 cloud tokens"
	return status, nil
}
