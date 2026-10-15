package token_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/gateway"
	"Dendrite/pkg/scheduler/token"
)

func TestTokenScheduler_LifecycleAndTransitions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	policyFile := filepath.Join(tmpDir, "policy.json")

	// Start on Wednesday at 14:00 (Day window outside sprint)
	initialTime := time.Date(2026, 10, 14, 14, 0, 0, 0, time.UTC)
	mockClock := &token.MockClock{Current: initialTime}
	mockClock.Advance(0) // test Advance
	limiter := gateway.NewBudgetLimiter()

	sched := token.NewTokenScheduler(mockClock, limiter, policyFile)

	// Verify GetPolicy & SetPolicy
	pol := sched.GetPolicy()
	if pol.NightWindow != "23:00-06:00" {
		t.Errorf("unexpected policy: %+v", pol)
	}

	pol.ReserveFloor = 0.25
	if err := sched.SetPolicy(pol); err != nil {
		t.Fatalf("failed to set policy: %v", err)
	}
	if sched.GetPolicy().ReserveFloor != 0.25 {
		t.Errorf("failed to persist policy update")
	}

	// Verify SelectModel in Day Window
	model, tier, err := sched.SelectModel()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != token.TierZeroTokenPass || model != "none" {
		t.Errorf("expected zero token pass, got tier=%s, model=%s", tier, model)
	}

	// Verify initial day status
	st, err := sched.Status()
	if err != nil {
		t.Fatalf("unexpected error getting status: %v", err)
	}
	if st.ActiveTier != token.TierZeroTokenPass {
		t.Errorf("expected TierZeroTokenPass at day window, got %s", st.ActiveTier)
	}

	// 1. Process Chunk in Day Window: Should run zero-token pass without calling LLM
	called := false
	caller := func(ctx context.Context, model string) (string, int, float64, error) {
		called = true
		return "mock LLM output", 500, 0.01, nil
	}

	res, err := sched.ProcessChunk(context.Background(), "chunk-001", caller)
	if err != nil {
		t.Fatalf("unexpected error processing chunk in zero-token pass: %v", err)
	}
	if called {
		t.Errorf("expected zero-token pass NOT to invoke LLM caller")
	}
	if res.CostUSD != 0.0 || res.Tokens != 0 {
		t.Errorf("expected 0 cost and tokens, got cost=%f, tokens=%d", res.CostUSD, res.Tokens)
	}

	// 2. Advance Mock Clock to Night Window (23:30)
	mockClock.Advance(9*time.Hour + 30*time.Minute)
	stNight, err := sched.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stNight.ActiveTier != token.TierNightLocal {
		t.Errorf("expected TierNightLocal at 23:30, got %s", stNight.ActiveTier)
	}

	// Process Chunk at Night: Uses local model, zero cost
	calledModel := ""
	nightCaller := func(ctx context.Context, model string) (string, int, float64, error) {
		calledModel = model
		return "Local Ollama analysis", 1024, 0.0, nil
	}
	resNight, err := sched.ProcessChunk(context.Background(), "chunk-002", nightCaller)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledModel != "local-qwen3-27b" {
		t.Errorf("expected local model 'local-qwen3-27b', got %s", calledModel)
	}
	if resNight.CostUSD != 0.0 {
		t.Errorf("expected $0.00 cost for local model, got %f", resNight.CostUSD)
	}

	// 3. Advance Mock Clock to Sprint Window (Saturday 14:00)
	mockClock.Set(time.Date(2026, 10, 17, 14, 0, 0, 0, time.UTC))
	sched.SetRemainingQuota(0.80) // 80% quota remaining
	sched.RecordCloudSpend(1.50)

	stSprint, err := sched.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stSprint.ActiveTier != token.TierCloudSprint {
		t.Errorf("expected TierCloudSprint on Saturday afternoon, got %s", stSprint.ActiveTier)
	}

	// Process Chunk in Cloud Sprint: accelerates via Gemini Pro
	cloudCalledModel := ""
	cloudCaller := func(ctx context.Context, model string) (string, int, float64, error) {
		cloudCalledModel = model
		return "Gemini Pro deep semantic analysis", 2048, 0.05, nil
	}
	resSprint, err := sched.ProcessChunk(context.Background(), "chunk-003", cloudCaller)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cloudCalledModel != "gemini-2.5-pro" {
		t.Errorf("expected cloud model 'gemini-2.5-pro', got %s", cloudCalledModel)
	}
	if resSprint.CostUSD != 0.05 {
		t.Errorf("expected 0.05 cost, got %f", resSprint.CostUSD)
	}

	// 4. Test Reserve Floor Pausing
	sched.SetRemainingQuota(0.19) // Below 20% floor
	sched.SetRemainingQuota(-0.5) // test boundary clamping
	sched.SetRemainingQuota(1.5)  // test boundary clamping
	sched.SetRemainingQuota(0.19)

	stFloor, err := sched.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stFloor.ActiveTier != token.TierQuotaFloorPaused {
		t.Errorf("expected TierQuotaFloorPaused when remaining quota <= 0.20, got %s", stFloor.ActiveTier)
	}

	// Processing chunk at floor uses local fallback to protect reserve
	fallbackCalledModel := ""
	fallbackCaller := func(ctx context.Context, model string) (string, int, float64, error) {
		fallbackCalledModel = model
		return "Local fallback analysis", 512, 0.0, nil
	}
	resFloor, err := sched.ProcessChunk(context.Background(), "chunk-004", fallbackCaller)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fallbackCalledModel != "local-qwen3-27b" {
		t.Errorf("expected local fallback model, got %s", fallbackCalledModel)
	}
	if resFloor.CostUSD != 0.0 {
		t.Errorf("expected 0.0 cost on fallback, got %f", resFloor.CostUSD)
	}

	// 5. Test Pause & Resume Controls
	sched.Pause()
	stPaused, err := sched.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stPaused.ActiveTier != token.TierPaused {
		t.Errorf("expected TierPaused after Pause(), got %s", stPaused.ActiveTier)
	}

	// SelectModel when paused
	_, _, err = sched.SelectModel()
	if !errors.Is(err, token.ErrSchedulerPaused) {
		t.Errorf("expected ErrSchedulerPaused from SelectModel()")
	}

	_, err = sched.ProcessChunk(context.Background(), "chunk-005", cloudCaller)
	if !errors.Is(err, token.ErrSchedulerPaused) {
		t.Errorf("expected ErrSchedulerPaused when paused, got %v", err)
	}

	sched.Resume()
	stResumed, err := sched.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stResumed.ActiveTier == token.TierPaused {
		t.Errorf("expected unpaused state after Resume()")
	}
}

func TestTokenScheduler_ErrorPaths(t *testing.T) {
	// Test RealClock
	realClock := token.RealClock{}
	if realClock.Now().IsZero() {
		t.Errorf("RealClock returned zero time")
	}

	sched := token.NewTokenScheduler(nil, nil, "")

	// Invalid policy update
	badPolicy := token.DefaultPolicy()
	badPolicy.ReserveFloor = 5.0
	if err := sched.SetPolicy(badPolicy); err == nil {
		t.Errorf("expected error setting invalid policy")
	}

	// Caller error propagation in Sprint
	mockClock := &token.MockClock{Current: time.Date(2026, 10, 17, 14, 0, 0, 0, time.UTC)}
	schedSprint := token.NewTokenScheduler(mockClock, nil, "")
	schedSprint.SetRemainingQuota(0.8)

	failingCaller := func(ctx context.Context, model string) (string, int, float64, error) {
		return "", 0, 0, errors.New("network timeout")
	}

	_, err := schedSprint.ProcessChunk(context.Background(), "chunk-err", failingCaller)
	if err == nil {
		t.Errorf("expected error propagation from failing caller")
	}

	// Caller error propagation in Night
	mockClockNight := &token.MockClock{Current: time.Date(2026, 10, 14, 23, 30, 0, 0, time.UTC)}
	schedNight := token.NewTokenScheduler(mockClockNight, nil, "")
	_, err = schedNight.ProcessChunk(context.Background(), "chunk-err2", failingCaller)
	if err == nil {
		t.Errorf("expected error propagation from failing night caller")
	}
}
