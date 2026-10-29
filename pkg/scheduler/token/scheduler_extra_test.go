package token

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/gateway"
)

type extraTestClock struct {
	now time.Time
}

func (c *extraTestClock) Now() time.Time {
	return c.now
}

func TestEvaluator_ForcedModes_And_EdgeCases(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) // Thursday noon

	// 1. Forced Mode == "night"
	pNight := DefaultPolicy()
	pNight.Mode = "night"
	stNight, err := Evaluate(now, &pNight, 1.0, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stNight.ActiveTier != TierNightLocal || stNight.SelectedModel != "local-qwen3-27b" {
		t.Errorf("expected TierNightLocal for forced night mode, got: %s / %s", stNight.ActiveTier, stNight.SelectedModel)
	}

	// 2. Forced Mode == "sprint"
	pSprint := DefaultPolicy()
	pSprint.Mode = "sprint"
	stSprint, err := Evaluate(now, &pSprint, 1.0, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stSprint.ActiveTier != TierCloudSprint || stSprint.SelectedModel != "gemini-2.5-pro" {
		t.Errorf("expected TierCloudSprint for forced sprint mode, got: %s / %s", stSprint.ActiveTier, stSprint.SelectedModel)
	}

	// 3. In sprint window with AllowZero == true and quota <= floor (e.g. 10% <= 20%)
	satAfternoon := time.Date(2026, 10, 17, 14, 0, 0, 0, time.UTC)
	pAllowZero := DefaultPolicy()
	pAllowZero.AllowZero = true
	stAllowZero, err := Evaluate(satAfternoon, &pAllowZero, 0.10, 10.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stAllowZero.ActiveTier != TierCloudSprint {
		t.Errorf("expected TierCloudSprint when AllowZero is true and quota > 0, got: %s", stAllowZero.ActiveTier)
	}

	// AllowZero == true but remaining quota is 0.0 -> pauses at floor
	stZeroQuota, err := Evaluate(satAfternoon, &pAllowZero, 0.0, 10.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stZeroQuota.ActiveTier != TierQuotaFloorPaused {
		t.Errorf("expected TierQuotaFloorPaused when quota is 0.0, got: %s", stZeroQuota.ActiveTier)
	}
}

func TestIsNightWindow_SameDayWindow(t *testing.T) {
	p := DefaultPolicy()
	p.NightWindow = "01:00-05:00" // same-day window

	// Inside window at 03:00
	t1 := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	in, err := IsNightWindow(t1, &p)
	if err != nil || !in {
		t.Errorf("expected inside same-day night window at 03:00, got in=%v err=%v", in, err)
	}

	// Before window at 00:30
	t2 := time.Date(2026, 10, 15, 0, 30, 0, 0, time.UTC)
	in, err = IsNightWindow(t2, &p)
	if err != nil || in {
		t.Errorf("expected outside same-day night window at 00:30, got in=%v err=%v", in, err)
	}

	// After window at 06:00
	t3 := time.Date(2026, 10, 15, 6, 0, 0, 0, time.UTC)
	in, err = IsNightWindow(t3, &p)
	if err != nil || in {
		t.Errorf("expected outside same-day night window at 06:00, got in=%v err=%v", in, err)
	}
}

func TestNextWeeklyReset_Branches(t *testing.T) {
	p := DefaultPolicy()
	p.WeeklyReset = "SUN 23:59"

	// 1. Same day, earlier than reset time: candidate.After(t) is true
	sunMorning := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	next, err := NextWeeklyReset(sunMorning, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.Day() != 18 || next.Hour() != 23 || next.Minute() != 59 {
		t.Errorf("expected reset later same day on Oct 18, got: %v", next)
	}

	// 2. Same day, later than reset time: candidate.After(t) is false (next week)
	pEarly := DefaultPolicy()
	pEarly.WeeklyReset = "SUN 10:00"
	sunNight := time.Date(2026, 10, 18, 22, 0, 0, 0, time.UTC)
	nextWeek, err := NextWeeklyReset(sunNight, &pEarly)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextWeek.Day() != 25 { // Oct 18 + 7 days = Oct 25
		t.Errorf("expected reset next Sunday Oct 25, got: %v", nextWeek)
	}
}

func TestScheduler_EvaluateFunc_Seam_And_UnreachableBranches(t *testing.T) {
	origEval := evaluateFunc
	defer func() { evaluateFunc = origEval }()

	clk := &extraTestClock{now: time.Now()}
	limiter := gateway.NewBudgetLimiter()
	sched := NewTokenScheduler(clk, limiter, "")

	// 1. evaluateFunc returns an error -> SelectModel and ProcessChunk propagate it
	evaluateFunc = func(t time.Time, p *Policy, remainingQuotaRatio, cloudSpend float64) (*SchedulerStatus, error) {
		return nil, errors.New("simulated evaluation failure")
	}

	_, _, err := sched.SelectModel()
	if err == nil {
		t.Error("expected error from SelectModel when evaluateFunc fails")
	}

	_, err = sched.ProcessChunk(context.Background(), "chk-err", nil)
	if err == nil {
		t.Error("expected error from ProcessChunk when evaluateFunc fails")
	}

	// 2. evaluateFunc returns an unknown tier -> triggers default case in ProcessChunk: ErrNoEligibleModel
	evaluateFunc = func(t time.Time, p *Policy, remainingQuotaRatio, cloudSpend float64) (*SchedulerStatus, error) {
		return &SchedulerStatus{
			CurrentTime: t,
			ActiveTier:  ExecutionTier("UNKNOWN_TIER"),
		}, nil
	}

	_, err = sched.ProcessChunk(context.Background(), "chk-unknown", nil)
	if !errors.Is(err, ErrNoEligibleModel) {
		t.Errorf("expected ErrNoEligibleModel for unknown tier, got: %v", err)
	}
}

func TestProcessChunk_CallerNil_And_QuotaClamping(t *testing.T) {
	// 1. ProcessChunk in Night with caller == nil
	clkNight := &extraTestClock{now: time.Date(2026, 10, 14, 23, 30, 0, 0, time.UTC)}
	schedNight := NewTokenScheduler(clkNight, nil, "")
	resNight, err := schedNight.ProcessChunk(context.Background(), "chk-night-nil", nil)
	if err != nil {
		t.Fatalf("unexpected error with nil caller at night: %v", err)
	}
	if resNight.SelectedModel != "local-qwen3-27b" || resNight.CostUSD != 0.0 {
		t.Errorf("unexpected night result with nil caller: %+v", resNight)
	}

	// 2. ProcessChunk in Sprint with caller == nil
	clkSprint := &extraTestClock{now: time.Date(2026, 10, 17, 14, 0, 0, 0, time.UTC)}
	schedSprint := NewTokenScheduler(clkSprint, nil, "")
	schedSprint.SetRemainingQuota(0.8)
	resSprint, err := schedSprint.ProcessChunk(context.Background(), "chk-sprint-nil", nil)
	if err != nil {
		t.Fatalf("unexpected error with nil caller in sprint: %v", err)
	}
	if resSprint.SelectedModel != "gemini-2.5-pro" {
		t.Errorf("unexpected sprint result with nil caller: %+v", resSprint)
	}

	// 3. ProcessChunk in Sprint with cost high enough to drive quota below 0.0 (clamps to 0.0)
	costlyCaller := func(ctx context.Context, model string) (string, int, float64, error) {
		return "expensive output", 100000, 500.0, nil // $500 cost
	}
	schedSprint.SetRemainingQuota(0.50) // 50% quota
	_, err = schedSprint.ProcessChunk(context.Background(), "chk-expensive", costlyCaller)
	if err != nil {
		t.Fatalf("unexpected error from costly caller: %v", err)
	}
	if schedSprint.remainingQuota != 0.0 {
		t.Errorf("expected quota clamped to 0.0, got: %f", schedSprint.remainingQuota)
	}

	// 4. ProcessChunk in Sprint when quota is already 0.0 (and AllowZero is true)
	p := schedSprint.GetPolicy()
	p.AllowZero = true
	_ = schedSprint.SetPolicy(p)
	schedSprint.SetRemainingQuota(0.0)
	_, err = schedSprint.ProcessChunk(context.Background(), "chk-zero-quota", costlyCaller)
	if err != nil {
		t.Fatalf("unexpected error from costly caller on zero quota: %v", err)
	}
}

func TestLoadPolicy_And_SavePolicy_Errors(t *testing.T) {
	tempDir := t.TempDir()

	// LoadPolicy on a directory (os.ReadFile returns an error that is not os.ErrNotExist)
	dirPath := filepath.Join(tempDir, "directory_as_policy")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	_, err := LoadPolicy(dirPath)
	if err == nil {
		t.Error("expected error loading directory path as policy file")
	}

	// SavePolicy MkdirAll error (parent is a file)
	parentFile := filepath.Join(tempDir, "file_as_parent")
	if err := os.WriteFile(parentFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	invalidSavePath := filepath.Join(parentFile, "sub", "policy.json")
	p := DefaultPolicy()
	err = SavePolicy(invalidSavePath, &p)
	if err == nil {
		t.Error("expected error saving policy when parent path is a file")
	}

	// Scheduler Pause/Resume/SetPolicy when policyPath is empty
	schedNoPath := NewTokenScheduler(nil, nil, "")
	schedNoPath.policyPath = "" // explicitly clear path
	schedNoPath.Pause()
	schedNoPath.Resume()
	if err := schedNoPath.SetPolicy(DefaultPolicy()); err != nil {
		t.Errorf("unexpected error setting policy with empty policyPath: %v", err)
	}
}
