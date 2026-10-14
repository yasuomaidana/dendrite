package perf

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidConfig   = errors.New("benchmark config validation failed")
	ErrNilRunner       = errors.New("cannot provision session with nil runner")
	ErrExecutionFailed = errors.New("benchmark execution failed")
)

// NewSession initializes a benchmark session in the UninitializedSession state.
func NewSession(cfg BenchmarkConfig) UninitializedSession {
	if cfg.RunID == "" {
		cfg.RunID = BenchmarkRunID(fmt.Sprintf("bench-%d", time.Now().UnixNano()))
	}
	if cfg.VirtualUsers == 0 {
		cfg.VirtualUsers = 1
	}
	if cfg.Duration <= 0 {
		cfg.Duration = 10 * time.Second
	}
	if cfg.Prompt == "" {
		cfg.Prompt = "Explain the difference between TCP and UDP in two concise paragraphs."
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 256
	}

	return UninitializedSession{
		id:     cfg.RunID,
		config: cfg,
	}
}

// ID returns the benchmark session ID.
func (s UninitializedSession) ID() BenchmarkRunID {
	return s.id
}

// Config returns the configuration.
func (s UninitializedSession) Config() BenchmarkConfig {
	return s.config
}

// Provision transitions an uninitialized session to ProvisionedSession by validating and assigning a runner.
func (s UninitializedSession) Provision(runner K6Runner) (ProvisionedSession, error) {
	if runner == nil {
		return ProvisionedSession{}, ErrNilRunner
	}
	if s.config.TargetEndpoint == "" && s.config.Mode != ModeMatrix {
		return ProvisionedSession{}, fmt.Errorf("%w: target_endpoint is required", ErrInvalidConfig)
	}

	return ProvisionedSession{
		id:     s.id,
		config: s.config,
		runner: runner,
	}, nil
}

// ID returns the benchmark session ID.
func (s ProvisionedSession) ID() BenchmarkRunID {
	return s.id
}

// Config returns the configuration.
func (s ProvisionedSession) Config() BenchmarkConfig {
	return s.config
}

// Runner returns the assigned runner.
func (s ProvisionedSession) Runner() K6Runner {
	return s.runner
}

// Run executes the provisioned session synchronously and transitions to CompletedSession.
func (s ProvisionedSession) Run(ctx context.Context) (CompletedSession, error) {
	summary, err := s.runner.Execute(ctx, s.config)
	if err != nil {
		return CompletedSession{}, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}

	return CompletedSession{
		id:      s.id,
		config:  s.config,
		summary: summary,
	}, nil
}

// ID returns the benchmark session ID.
func (s CompletedSession) ID() BenchmarkRunID {
	return s.id
}

// Config returns the configuration.
func (s CompletedSession) Config() BenchmarkConfig {
	return s.config
}

// Summary returns the final benchmark results from a completed session.
func (s CompletedSession) Summary() *BenchmarkSummary {
	return s.summary
}
