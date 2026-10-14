package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Standard errors for budget limiter and failover operations.
var (
	ErrBudgetExceeded = errors.New("budget ceiling exceeded")
	ErrQuotaExhausted = errors.New("upstream provider quota exhausted (429)")
	ErrKeyNotFound    = errors.New("virtual key not found")
)

// WindowDuration represents budget evaluation time horizons.
type WindowDuration string

const (
	Window1h  WindowDuration = "1h"
	Window24h WindowDuration = "24h"
	Window7d  WindowDuration = "7d"
	Window30d WindowDuration = "30d"
)

// Duration converts WindowDuration to time.Duration.
func (w WindowDuration) Duration() time.Duration {
	switch w {
	case Window1h:
		return 1 * time.Hour
	case Window24h:
		return 24 * time.Hour
	case Window7d:
		return 7 * 24 * time.Hour
	case Window30d:
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// SpendRecord captures an individual LLM inference invocation for auditing and budgeting.
type SpendRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	Tokens    int       `json:"tokens"`
	CostUSD   float64   `json:"cost_usd"`
}

// BudgetLimit defines a monetary ceiling over a specific duration window.
type BudgetLimit struct {
	Duration     WindowDuration `json:"duration"`
	MaxBudgetUSD float64        `json:"max_budget_usd"`
}

// VirtualKeyConfig defines budget ceilings and fallback chains for an agent or client key.
type VirtualKeyConfig struct {
	KeyAlias       string              `json:"key_alias"`
	BudgetLimits   []BudgetLimit       `json:"budget_limits"`
	ModelMaxBudget map[string]float64  `json:"model_max_budget,omitempty"`
	Fallbacks      map[string][]string `json:"fallbacks,omitempty"`
}

// BudgetLimiter provides thread-safe sliding window budget governance.
type BudgetLimiter struct {
	mu      sync.RWMutex
	keys    map[string]*VirtualKeyConfig
	history map[string][]SpendRecord
}

// NewBudgetLimiter initializes an in-memory sliding window budget governor.
func NewBudgetLimiter() *BudgetLimiter {
	return &BudgetLimiter{
		keys:    make(map[string]*VirtualKeyConfig),
		history: make(map[string][]SpendRecord),
	}
}

// RegisterKey configures a virtual key with budget constraints.
func (b *BudgetLimiter) RegisterKey(cfg VirtualKeyConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.keys[cfg.KeyAlias] = &cfg
	if _, ok := b.history[cfg.KeyAlias]; !ok {
		b.history[cfg.KeyAlias] = make([]SpendRecord, 0)
	}
}

// RecordSpend registers token consumption and monetary cost for a key.
func (b *BudgetLimiter) RecordSpend(keyAlias, model string, tokens int, costUSD float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	rec := SpendRecord{
		Timestamp: time.Now(),
		Model:     model,
		Tokens:    tokens,
		CostUSD:   costUSD,
	}

	b.history[keyAlias] = append(b.history[keyAlias], rec)
}

// GetWindowSpend calculates total spend and tokens within the specified window.
func (b *BudgetLimiter) GetWindowSpend(keyAlias string, window WindowDuration) (float64, int) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	records, exists := b.history[keyAlias]
	if !exists || len(records) == 0 {
		return 0.0, 0
	}

	cutoff := time.Now().Add(-window.Duration())
	var totalCost float64
	var totalTokens int

	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Timestamp.Before(cutoff) {
			break
		}
		totalCost += rec.CostUSD
		totalTokens += rec.Tokens
	}

	return totalCost, totalTokens
}

// CalculateRateBoundedCeiling computes the rate-bounded throttling ceiling:
// R_target = R_consumed + (0.30 * R_weekly_limit)
func CalculateRateBoundedCeiling(weeklyLimitUSD, currentConsumedUSD float64) float64 {
	return currentConsumedUSD + (0.30 * weeklyLimitUSD)
}

// CheckBudget determines whether a model invocation is allowed, or returns a fallback candidate.
// Returns: (allowed bool, selectedModel string, err error)
func (b *BudgetLimiter) CheckBudget(keyAlias, requestedModel string) (bool, string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Zero-cost local Ollama model is ALWAYS exempt from budget limits
	if requestedModel == "local-qwen-coder" || strings.HasPrefix(requestedModel, "ollama/") {
		return true, requestedModel, nil
	}

	keyCfg, exists := b.keys[keyAlias]
	if !exists {
		// If key is not registered, allow by default
		return true, requestedModel, nil
	}

	// Check sliding window limits
	for _, limit := range keyCfg.BudgetLimits {
		spent, _ := b.getWindowSpendUnlocked(keyAlias, limit.Duration)
		if spent >= limit.MaxBudgetUSD {
			// Select fallback model
			fallbackModel := "local-qwen-coder"
			if chain, hasChain := keyCfg.Fallbacks[requestedModel]; hasChain && len(chain) > 0 {
				fallbackModel = chain[len(chain)-1] // Default to end of chain (local-qwen-coder)
			}
			return false, fallbackModel, nil
		}
	}

	return true, requestedModel, nil
}

func (b *BudgetLimiter) getWindowSpendUnlocked(keyAlias string, window WindowDuration) (float64, int) {
	records, exists := b.history[keyAlias]
	if !exists || len(records) == 0 {
		return 0.0, 0
	}

	cutoff := time.Now().Add(-window.Duration())
	var totalCost float64
	var totalTokens int

	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Timestamp.Before(cutoff) {
			break
		}
		totalCost += rec.CostUSD
		totalTokens += rec.Tokens
	}

	return totalCost, totalTokens
}

// FailoverEngine transparently executes LLM calls with automated fallback on budget caps or HTTP 429 quota errors.
type FailoverEngine struct {
	limiter *BudgetLimiter
}

// NewFailoverEngine creates an automated failover engine backed by the budget governor.
func NewFailoverEngine(limiter *BudgetLimiter) *FailoverEngine {
	if limiter == nil {
		limiter = NewBudgetLimiter()
	}
	return &FailoverEngine{limiter: limiter}
}

// ModelCaller defines the invocation function for a target model.
type ModelCaller func(ctx context.Context, model string) (response string, tokens int, costUSD float64, err error)

// ExecuteWithFailover dispatches the request with sliding budget checks and 429 quota failover.
func (f *FailoverEngine) ExecuteWithFailover(ctx context.Context, keyAlias, requestedModel string, caller ModelCaller) (string, string, error) {
	allowed, targetModel, err := f.limiter.CheckBudget(keyAlias, requestedModel)
	if err != nil {
		return "", "", err
	}

	// If budget exceeded, redirect directly to zero-cost fallback
	if !allowed {
		targetModel = "local-qwen-coder"
	}

	resp, tokens, cost, callErr := caller(ctx, targetModel)
	if callErr == nil {
		f.limiter.RecordSpend(keyAlias, targetModel, tokens, cost)
		return resp, targetModel, nil
	}

	// Check if call failed due to provider 429 quota exhaustion
	if isQuotaExhaustedError(callErr) && targetModel != "local-qwen-coder" {
		// Seamlessly failover to local unmetered Ollama
		fallbackModel := "local-qwen-coder"
		fallbackResp, fbTokens, fbCost, fbErr := caller(ctx, fallbackModel)
		if fbErr == nil {
			f.limiter.RecordSpend(keyAlias, fallbackModel, fbTokens, fbCost)
			return fallbackResp, fallbackModel, nil
		}
		return "", fallbackModel, fmt.Errorf("primary model failed (%w), failover to %s failed: %w", callErr, fallbackModel, fbErr)
	}

	return "", targetModel, callErr
}

func isQuotaExhaustedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "quota") ||
		strings.Contains(msg, "rate limit") ||
		errors.Is(err, ErrQuotaExhausted)
}
