package token

import (
	"context"
	"fmt"
	"sync"
	"time"

	"Dendrite/pkg/gateway"
	"Dendrite/pkg/statedir"
)

// ChunkProcessResult records the outcome of scheduling and executing a chunk analysis.
type ChunkProcessResult struct {
	ChunkID       string        `json:"chunk_id"`
	ActiveTier    ExecutionTier `json:"active_tier"`
	SelectedModel string        `json:"selected_model"`
	Tokens        int           `json:"tokens"`
	CostUSD       float64       `json:"cost_usd"`
	Response      string        `json:"response,omitempty"`
	ExecutedAt    time.Time     `json:"executed_at"`
}

// TokenScheduler coordinates token-aware hybrid execution.
type TokenScheduler struct {
	mu             sync.RWMutex
	policy         Policy
	clock          Clock
	limiter        *gateway.BudgetLimiter
	policyPath     string
	remainingQuota float64 // 0.0 to 1.0 (e.g. 0.85 = 85% remaining)
	cloudSpendUSD  float64
}

// NewTokenScheduler initializes a scheduler with the specified clock and limiter.
func NewTokenScheduler(clock Clock, limiter *gateway.BudgetLimiter, policyPath string) *TokenScheduler {
	if clock == nil {
		clock = RealClock{}
	}
	if limiter == nil {
		limiter = gateway.NewBudgetLimiter()
	}
	if policyPath == "" {
		policyPath = DefaultPolicyFile
	}
	policyPath = statedir.Resolve(policyPath)

	pol, err := LoadPolicy(policyPath)
	if err != nil {
		def := DefaultPolicy()
		pol = &def
	}

	return &TokenScheduler{
		policy:         *pol,
		clock:          clock,
		limiter:        limiter,
		policyPath:     policyPath,
		remainingQuota: 1.0,
		cloudSpendUSD:  0.0,
	}
}

// GetPolicy returns a copy of the active scheduling policy.
func (s *TokenScheduler) GetPolicy() Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

// SetPolicy updates and saves the active scheduling policy.
func (s *TokenScheduler) SetPolicy(p Policy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := p.Validate(); err != nil {
		return err
	}

	s.policy = p
	if s.policyPath != "" {
		_ = SavePolicy(s.policyPath, &p)
	}
	return nil
}

// SetRemainingQuota adjusts the current tracked remaining quota ratio (0.0 to 1.0).
func (s *TokenScheduler) SetRemainingQuota(ratio float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ratio < 0.0 {
		ratio = 0.0
	} else if ratio > 1.0 {
		ratio = 1.0
	}
	s.remainingQuota = ratio
}

// RecordCloudSpend increments tracked cloud spending in USD.
func (s *TokenScheduler) RecordCloudSpend(costUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cloudSpendUSD += costUSD
}

// Pause transitions the scheduler into a paused state.
func (s *TokenScheduler) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy.Mode = "paused"
	if s.policyPath != "" {
		_ = SavePolicy(s.policyPath, &s.policy)
	}
}

// Resume restores the scheduler to automatic mode.
func (s *TokenScheduler) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy.Mode = "auto"
	if s.policyPath != "" {
		_ = SavePolicy(s.policyPath, &s.policy)
	}
}

var evaluateFunc = Evaluate

// Status evaluates the current state and returns detailed scheduling diagnostics.
func (s *TokenScheduler) Status() (*SchedulerStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return evaluateFunc(s.clock.Now(), &s.policy, s.remainingQuota, s.cloudSpendUSD)
}

// SelectModel resolves the appropriate model and execution tier for current conditions.
func (s *TokenScheduler) SelectModel() (string, ExecutionTier, error) {
	st, err := s.Status()
	if err != nil {
		return "", "", err
	}

	if st.ActiveTier == TierPaused {
		return "", TierPaused, ErrSchedulerPaused
	}

	return st.SelectedModel, st.ActiveTier, nil
}

// ProcessChunk dispatches analysis of a chunk according to the active token schedule.
func (s *TokenScheduler) ProcessChunk(ctx context.Context, chunkID string, caller gateway.ModelCaller) (*ChunkProcessResult, error) {
	st, err := s.Status()
	if err != nil {
		return nil, err
	}

	result := &ChunkProcessResult{
		ChunkID:    chunkID,
		ActiveTier: st.ActiveTier,
		ExecutedAt: s.clock.Now(),
	}

	switch st.ActiveTier {
	case TierPaused:
		return nil, ErrSchedulerPaused

	case TierZeroTokenPass:
		result.SelectedModel = "ast-parser-zero-token"
		result.Tokens = 0
		result.CostUSD = 0.0
		result.Response = "Extracted AST & CFG symbols at 0 tokens"
		return result, nil

	case TierNightLocal, TierQuotaFloorPaused:
		model := "local-qwen3-27b"
		result.SelectedModel = model

		if caller != nil {
			resp, tokens, cost, callErr := caller(ctx, model)
			if callErr != nil {
				return nil, fmt.Errorf("local night analysis failed: %w", callErr)
			}
			result.Response = resp
			result.Tokens = tokens
			result.CostUSD = cost
		}
		return result, nil

	case TierCloudSprint:
		model := "gemini-2.5-pro"
		result.SelectedModel = model

		if caller != nil {
			resp, tokens, cost, callErr := caller(ctx, model)
			if callErr != nil {
				return nil, fmt.Errorf("cloud sprint analysis failed: %w", callErr)
			}
			result.Response = resp
			result.Tokens = tokens
			result.CostUSD = cost

			s.mu.Lock()
			s.cloudSpendUSD += cost
			if s.remainingQuota > 0.0 {
				s.remainingQuota -= (cost / 100.0)
				if s.remainingQuota < 0.0 {
					s.remainingQuota = 0.0
				}
			}
			s.mu.Unlock()
		}
		return result, nil

	default:
		return nil, ErrNoEligibleModel
	}
}
