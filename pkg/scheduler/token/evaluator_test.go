package token_test

import (
	"testing"
	"time"

	"Dendrite/pkg/scheduler/token"
)

func TestNightWindowEvaluation(t *testing.T) {
	p := token.DefaultPolicy() // NightWindow = "23:00-06:00"

	loc := time.UTC
	tests := []struct {
		name    string
		hour    int
		minute  int
		wantIn  bool
	}{
		{"before window evening", 22, 59, false},
		{"start of window", 23, 0, true},
		{"midnight", 0, 0, true},
		{"deep night", 3, 30, true},
		{"last minute of night", 5, 59, true},
		{"end boundary morning", 6, 0, false},
		{"midday", 14, 0, false},
	}

	for _, tt := range tests {
		tm := time.Date(2026, 10, 14, tt.hour, tt.minute, 0, 0, loc)
		got, err := token.IsNightWindow(tm, &p)
		if err != nil {
			t.Fatalf("[%s] unexpected error: %v", tt.name, err)
		}
		if got != tt.wantIn {
			t.Errorf("[%s] at %02d:%02d got %v, want %v", tt.name, tt.hour, tt.minute, got, tt.wantIn)
		}
	}

	// Same-day window test (e.g. 02:00 to 05:00)
	dayPolicy := token.Policy{NightWindow: "02:00-05:00", WeeklyReset: "SUN 23:59", SprintBeforeReset: 48 * time.Hour, ReserveFloor: 0.2, Mode: "auto"}
	dayTests := []struct {
		hour   int
		minute int
		wantIn bool
	}{
		{1, 59, false},
		{2, 0, true},
		{4, 59, true},
		{5, 0, false},
	}
	for _, dt := range dayTests {
		tm := time.Date(2026, 10, 14, dt.hour, dt.minute, 0, 0, loc)
		got, err := token.IsNightWindow(tm, &dayPolicy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != dt.wantIn {
			t.Errorf("day-window at %02d:%02d got %v, want %v", dt.hour, dt.minute, got, dt.wantIn)
		}
	}
}

func TestNextWeeklyReset(t *testing.T) {
	p := token.DefaultPolicy() // WeeklyReset = "SUN 23:59"
	loc := time.UTC

	// Monday Oct 12, 2026 at 10:00 -> Next Sunday is Oct 18, 2026 at 23:59
	mon := time.Date(2026, 10, 12, 10, 0, 0, 0, loc)
	next, err := token.NextWeeklyReset(mon, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantSunday := time.Date(2026, 10, 18, 23, 59, 0, 0, loc)
	if !next.Equal(wantSunday) {
		t.Errorf("from Mon got %v, want %v", next, wantSunday)
	}

	// Sunday Oct 18, 2026 at 12:00 -> Same day Sunday Oct 18 at 23:59
	sunMidday := time.Date(2026, 10, 18, 12, 0, 0, 0, loc)
	nextSun, err := token.NextWeeklyReset(sunMidday, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nextSun.Equal(wantSunday) {
		t.Errorf("from Sun midday got %v, want %v", nextSun, wantSunday)
	}

	// Sunday Oct 18, 2026 at 23:59:30 -> Next week Sunday Oct 25 at 23:59
	sunPast := time.Date(2026, 10, 18, 23, 59, 30, 0, loc)
	nextSunNextWeek, err := token.NextWeeklyReset(sunPast, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNextSunday := time.Date(2026, 10, 25, 23, 59, 0, 0, loc)
	if !nextSunNextWeek.Equal(wantNextSunday) {
		t.Errorf("from Sun past got %v, want %v", nextSunNextWeek, wantNextSunday)
	}
}

func Test48HourSprintWindowTransition(t *testing.T) {
	p := token.DefaultPolicy() // Reset: "SUN 23:59", Sprint: 48h
	loc := time.UTC

	// Reset is Sunday Oct 18, 2026 23:59:00
	// 48 hours prior is Friday Oct 16, 2026 23:59:00

	// 1. One second before 48h boundary (Friday 23:58:59) -> outside sprint
	beforeSprint := time.Date(2026, 10, 16, 23, 58, 59, 0, loc)
	inSprint, err := token.IsSprintWindow(beforeSprint, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inSprint {
		t.Errorf("expected outside sprint at 48h 1s before reset")
	}

	// 2. Exact 48h boundary (Friday 23:59:00) -> inside sprint
	exactSprint := time.Date(2026, 10, 16, 23, 59, 0, 0, loc)
	inSprintExact, err := token.IsSprintWindow(exactSprint, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inSprintExact {
		t.Errorf("expected inside sprint at exact 48h boundary")
	}

	// 3. Inside sprint (Saturday Oct 17, 14:00) -> inside sprint
	midSprint := time.Date(2026, 10, 17, 14, 0, 0, 0, loc)
	inSprintMid, err := token.IsSprintWindow(midSprint, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inSprintMid {
		t.Errorf("expected inside sprint on Saturday afternoon")
	}

	// 4. One minute before reset (Sunday Oct 18, 23:58:00) -> inside sprint
	nearReset := time.Date(2026, 10, 18, 23, 58, 0, 0, loc)
	inSprintNear, err := token.IsSprintWindow(nearReset, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inSprintNear {
		t.Errorf("expected inside sprint 1 min before reset")
	}

	// 5. One second after reset (Sunday Oct 18, 23:59:01) -> outside sprint (next reset 7d away)
	afterReset := time.Date(2026, 10, 18, 23, 59, 1, 0, loc)
	inSprintAfter, err := token.IsSprintWindow(afterReset, &p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inSprintAfter {
		t.Errorf("expected outside sprint immediately after reset")
	}
}

func TestEvaluate_TiersAndQuotaFloor(t *testing.T) {
	p := token.DefaultPolicy()
	loc := time.UTC

	// Scenario A: Wednesday 14:00 (Day window outside sprint) -> TierZeroTokenPass, 0 cloud tokens
	wedDay := time.Date(2026, 10, 14, 14, 0, 0, 0, loc)
	stA, err := token.Evaluate(wedDay, &p, 1.0, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stA.ActiveTier != token.TierZeroTokenPass {
		t.Errorf("expected TierZeroTokenPass on Wednesday day, got %s", stA.ActiveTier)
	}
	if stA.SelectedModel != "none" {
		t.Errorf("expected model 'none', got %s", stA.SelectedModel)
	}

	// Scenario B: Wednesday 23:30 (Night window outside sprint) -> TierNightLocal, local model
	wedNight := time.Date(2026, 10, 14, 23, 30, 0, 0, loc)
	stB, err := token.Evaluate(wedNight, &p, 1.0, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stB.ActiveTier != token.TierNightLocal {
		t.Errorf("expected TierNightLocal at night, got %s", stB.ActiveTier)
	}
	if stB.SelectedModel != "local-qwen3-27b" {
		t.Errorf("expected local model, got %s", stB.SelectedModel)
	}

	// Scenario C: Saturday 14:00 (Inside 48h sprint, quota = 80% > 20% floor) -> TierCloudSprint, Gemini Pro
	satSprint := time.Date(2026, 10, 17, 14, 0, 0, 0, loc)
	stC, err := token.Evaluate(satSprint, &p, 0.80, 20.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stC.ActiveTier != token.TierCloudSprint {
		t.Errorf("expected TierCloudSprint, got %s", stC.ActiveTier)
	}
	if stC.SelectedModel != "gemini-2.5-pro" {
		t.Errorf("expected gemini-2.5-pro, got %s", stC.SelectedModel)
	}

	// Scenario D: Inside sprint, quota = 20% (reserve floor reached) -> TierQuotaFloorPaused
	stD, err := token.Evaluate(satSprint, &p, 0.20, 80.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stD.ActiveTier != token.TierQuotaFloorPaused {
		t.Errorf("expected TierQuotaFloorPaused at reserve floor, got %s", stD.ActiveTier)
	}
	if stD.SelectedModel != "local-qwen3-27b" {
		t.Errorf("expected fallback to local model, got %s", stD.SelectedModel)
	}

	// Scenario E: Inside sprint, quota = 10% (< 20%), but AllowZero = true -> TierCloudSprint
	pAllowZero := p
	pAllowZero.AllowZero = true
	stE, err := token.Evaluate(satSprint, &pAllowZero, 0.10, 90.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stE.ActiveTier != token.TierCloudSprint {
		t.Errorf("expected TierCloudSprint when allow_zero is enabled, got %s", stE.ActiveTier)
	}

	// Scenario F: User Paused override
	pPaused := p
	pPaused.Mode = "paused"
	stF, err := token.Evaluate(satSprint, &pPaused, 0.80, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stF.ActiveTier != token.TierPaused {
		t.Errorf("expected TierPaused, got %s", stF.ActiveTier)
	}

	// Scenario G: Forced Night mode
	pNight := p
	pNight.Mode = "night"
	stG, err := token.Evaluate(wedDay, &pNight, 1.0, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stG.ActiveTier != token.TierNightLocal {
		t.Errorf("expected TierNightLocal in forced night mode, got %s", stG.ActiveTier)
	}

	// Scenario H: Forced Sprint mode
	pSprint := p
	pSprint.Mode = "sprint"
	stH, err := token.Evaluate(wedDay, &pSprint, 1.0, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stH.ActiveTier != token.TierCloudSprint {
		t.Errorf("expected TierCloudSprint in forced sprint mode, got %s", stH.ActiveTier)
	}
}
