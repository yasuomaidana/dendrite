package gateway_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"Dendrite/pkg/gateway"
)

func TestWindowDuration(t *testing.T) {
	cases := []struct {
		w        gateway.WindowDuration
		expected time.Duration
	}{
		{gateway.Window1h, 1 * time.Hour},
		{gateway.Window24h, 24 * time.Hour},
		{gateway.Window7d, 7 * 24 * time.Hour},
		{gateway.Window30d, 30 * 24 * time.Hour},
		{"custom", 24 * time.Hour},
	}

	for _, c := range cases {
		if c.w.Duration() != c.expected {
			t.Errorf("for %s: expected %v, got %v", c.w, c.expected, c.w.Duration())
		}
	}
}

func TestBudgetLimiter_SlidingWindows(t *testing.T) {
	limiter := gateway.NewBudgetLimiter()

	key := gateway.VirtualKeyConfig{
		KeyAlias: "autonomous-coding-agent",
		BudgetLimits: []gateway.BudgetLimit{
			{Duration: gateway.Window24h, MaxBudgetUSD: 5.00},
			{Duration: gateway.Window7d, MaxBudgetUSD: 20.00},
		},
		Fallbacks: map[string][]string{
			"gemini-2.5-pro": {"copilot-sonnet", "local-qwen-coder"},
		},
	}
	limiter.RegisterKey(key)

	// Initial spend check
	allowed, selectedModel, err := limiter.CheckBudget("autonomous-coding-agent", "gemini-2.5-pro")
	if err != nil || !allowed || selectedModel != "gemini-2.5-pro" {
		t.Fatalf("expected allowed within budget, got allowed=%v, model=%s", allowed, selectedModel)
	}

	// Record $3.00 spend
	limiter.RecordSpend("autonomous-coding-agent", "gemini-2.5-pro", 1000, 3.00)
	spent, tokens := limiter.GetWindowSpend("autonomous-coding-agent", gateway.Window24h)
	if spent != 3.00 || tokens != 1000 {
		t.Errorf("expected $3.00 and 1000 tokens, got $%.2f and %d", spent, tokens)
	}

	// Still under 24h limit ($5.00)
	allowed, _, _ = limiter.CheckBudget("autonomous-coding-agent", "gemini-2.5-pro")
	if !allowed {
		t.Errorf("expected still allowed with $3.00 / $5.00 spent")
	}

	// Record another $2.50 spend ($5.50 total > $5.00 cap)
	limiter.RecordSpend("autonomous-coding-agent", "gemini-2.5-pro", 800, 2.50)
	allowed, fallbackModel, _ := limiter.CheckBudget("autonomous-coding-agent", "gemini-2.5-pro")
	if allowed {
		t.Errorf("expected budget check to fail when cumulative spend ($5.50) exceeds limit ($5.00)")
	}
	if fallbackModel != "local-qwen-coder" {
		t.Errorf("expected fallback to local-qwen-coder, got %s", fallbackModel)
	}

	// ZERO-COST EXEMPTION: local-qwen-coder must ALWAYS be allowed even if $100 spent!
	allowed, exemptModel, _ := limiter.CheckBudget("autonomous-coding-agent", "local-qwen-coder")
	if !allowed || exemptModel != "local-qwen-coder" {
		t.Errorf("expected local-qwen-coder to be exempt from budget checks")
	}

	// Check unregistered key
	allowed, _, _ = limiter.CheckBudget("unregistered-key", "gemini-2.5-pro")
	if !allowed {
		t.Errorf("expected unregistered key to be allowed by default")
	}
}

func TestCalculateRateBoundedCeiling(t *testing.T) {
	// Formula: R_target = R_consumed + (0.30 * R_weekly_limit)
	// Consumed = $10.00, Weekly Limit = $50.00 -> Ceiling = 10 + (0.30 * 50) = $25.00
	ceiling := gateway.CalculateRateBoundedCeiling(50.00, 10.00)
	expected := 25.00
	if ceiling != expected {
		t.Errorf("expected rate-bounded ceiling %.2f, got %.2f", expected, ceiling)
	}
}

func TestFailoverEngine_Execution(t *testing.T) {
	limiter := gateway.NewBudgetLimiter()
	limiter.RegisterKey(gateway.VirtualKeyConfig{
		KeyAlias: "agent-1",
		BudgetLimits: []gateway.BudgetLimit{
			{Duration: gateway.Window1h, MaxBudgetUSD: 2.00},
		},
	})

	engine := gateway.NewFailoverEngine(limiter)
	ctx := context.Background()

	// 1. Successful execution on primary model
	resp, usedModel, err := engine.ExecuteWithFailover(ctx, "agent-1", "gemini-2.5-pro", func(ctx context.Context, model string) (string, int, float64, error) {
		return "pro-response", 500, 0.50, nil
	})
	if err != nil || resp != "pro-response" || usedModel != "gemini-2.5-pro" {
		t.Fatalf("expected successful primary execution, got resp=%s, model=%s, err=%v", resp, usedModel, err)
	}

	// 2. HTTP 429 quota exhaustion failover to local-qwen-coder
	resp429, used429Model, err429 := engine.ExecuteWithFailover(ctx, "agent-1", "gemini-2.5-pro", func(ctx context.Context, model string) (string, int, float64, error) {
		if model == "gemini-2.5-pro" {
			return "", 0, 0, gateway.ErrQuotaExhausted
		}
		if model == "local-qwen-coder" {
			return "local-failover-response", 300, 0.0, nil
		}
		return "", 0, 0, errors.New("unexpected model")
	})
	if err429 != nil || resp429 != "local-failover-response" || used429Model != "local-qwen-coder" {
		t.Fatalf("expected failover to local-qwen-coder on 429 error, got resp=%s, model=%s, err=%v", resp429, used429Model, err429)
	}

	// 3. Exceeded budget directly invokes local-qwen-coder without calling primary
	limiter.RecordSpend("agent-1", "gemini-2.5-pro", 2000, 2.50) // Total $3.00 > $2.00
	primaryCalled := false
	respCap, usedCapModel, errCap := engine.ExecuteWithFailover(ctx, "agent-1", "gemini-2.5-pro", func(ctx context.Context, model string) (string, int, float64, error) {
		if model == "gemini-2.5-pro" {
			primaryCalled = true
			return "should-not-be-called", 0, 0, nil
		}
		return "budget-capped-local-response", 400, 0.0, nil
	})
	if primaryCalled {
		t.Errorf("expected primary model NOT to be called when budget is exceeded")
	}
	if errCap != nil || respCap != "budget-capped-local-response" || usedCapModel != "local-qwen-coder" {
		t.Fatalf("expected budget capped redirect to local-qwen-coder, got resp=%s, model=%s, err=%v", respCap, usedCapModel, errCap)
	}

	// 4. Non-quota error does not trigger failover
	customErr := errors.New("network timeout")
	_, _, errTimeout := engine.ExecuteWithFailover(ctx, "agent-1", "copilot-sonnet", func(ctx context.Context, model string) (string, int, float64, error) {
		return "", 0, 0, customErr
	})
	if !errors.Is(errTimeout, customErr) {
		t.Errorf("expected customErr to be returned directly without failover, got: %v", errTimeout)
	}

	// 5. Failover failure propagates error
	_, _, errBothFailed := engine.ExecuteWithFailover(ctx, "agent-1", "gemini-2.5-flash", func(ctx context.Context, model string) (string, int, float64, error) {
		if model == "gemini-2.5-flash" {
			return "", 0, 0, gateway.ErrQuotaExhausted
		}
		return "", 0, 0, errors.New("ollama dead")
	})
	if errBothFailed == nil {
		t.Errorf("expected error when both primary and fallback fail")
	}
}
