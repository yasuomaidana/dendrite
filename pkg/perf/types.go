package perf

import (
	"context"
	"time"
)

// Domain Newtypes for strong typing and semantic clarity.
type (
	// VirtualUsers represents the number of concurrent simulated agent sessions.
	VirtualUsers uint32

	// TTFT represents Time-To-First-Token in nanoseconds/duration.
	TTFT time.Duration

	// TokensPerSecond represents generation speed in completion tokens per second.
	TokensPerSecond float64

	// BenchmarkRunID uniquely identifies a performance testing session.
	BenchmarkRunID string
)

// BenchmarkMode denotes the benchmark archetype.
type BenchmarkMode string

const (
	ModeInference BenchmarkMode = "inference"
	ModeGateway   BenchmarkMode = "gateway"
	ModeMatrix    BenchmarkMode = "matrix"
)

// FaultInjectionType specifies simulated faults for failover testing.
type FaultInjectionType string

const (
	FaultNone          FaultInjectionType = "none"
	FaultRateLimit429  FaultInjectionType = "rate-limit-429"
	FaultNodeGlitch503 FaultInjectionType = "node-glitch-503"
)

// BenchmarkConfig defines inputs and runtime constraints for a benchmark run.
type BenchmarkConfig struct {
	RunID             BenchmarkRunID     `json:"run_id"`
	Mode              BenchmarkMode      `json:"mode"`
	TargetEndpoint    string             `json:"target_endpoint"`
	EngineID          string             `json:"engine_id"`
	Model             string             `json:"model"`
	VirtualUsers      VirtualUsers       `json:"virtual_users"`
	Duration          time.Duration      `json:"duration"`
	Prompt            string             `json:"prompt"`
	MaxTokens         int                `json:"max_tokens"`
	FaultInjection    FaultInjectionType `json:"fault_injection,omitempty"`
	FailoverEndpoint  string             `json:"failover_endpoint,omitempty"`
	ThermalMonitoring bool               `json:"thermal_monitoring"`
	PreFlightCascade  bool               `json:"pre_flight_cascade"`
	CascadeFallbackID string             `json:"cascade_fallback_id,omitempty"`
}

// LatencyPercentiles aggregates TTFT or full response percentiles.
type LatencyPercentiles struct {
	Min time.Duration `json:"min"`
	P50 time.Duration `json:"p50"`
	P90 time.Duration `json:"p90"`
	P95 time.Duration `json:"p95"`
	P99 time.Duration `json:"p99"`
	Max time.Duration `json:"max"`
	Avg time.Duration `json:"avg"`
}

// ThermalMetrics records temperature and throttling indicators during load.
type ThermalMetrics struct {
	PeakSoCTempC     float64 `json:"peak_soc_temp_c"`
	AvgSoCTempC      float64 `json:"avg_soc_temp_c"`
	ThrottleDetected bool    `json:"throttle_detected"`
	ThrottleWarning  string  `json:"throttle_warning,omitempty"`
}

// SwitchEvent records a discrete failover cascade switch transition.
type SwitchEvent struct {
	FromEngine string        `json:"from_engine"`
	ToEngine   string        `json:"to_engine"`
	Reason     string        `json:"reason"`
	Latency    time.Duration `json:"latency"`
	Timestamp  time.Time     `json:"timestamp"`
}

// FailoverMetrics captures cascade failover resilience metrics.
type FailoverMetrics struct {
	InjectedFaults    int           `json:"injected_faults"`
	FailoverTriggered bool          `json:"failover_triggered"`
	FailoverLatency   time.Duration `json:"failover_latency"`
	LostRequests      int           `json:"lost_requests"`
	SuccessAfterFail  int           `json:"success_after_failover"`
	PreFlightSwitched bool          `json:"pre_flight_switched,omitempty"`
	PrimaryEngineID   string        `json:"primary_engine_id,omitempty"`
	CascadedEngineID  string        `json:"cascaded_engine_id,omitempty"`
	CascadeReason     string        `json:"cascade_reason,omitempty"`
	SwitchEvents      []SwitchEvent `json:"switch_events,omitempty"`
}

// SingleStreamResult captures metrics from an individual request stream.
type SingleStreamResult struct {
	StreamID          string
	TTFT              TTFT
	Duration          time.Duration
	CompletionTokens  int
	TPS               TokensPerSecond
	StatusCode        int
	FailoverRecovered bool
	Error             string
}

// BenchmarkSummary holds comprehensive performance scorecard data.
type BenchmarkSummary struct {
	RunID            BenchmarkRunID     `json:"run_id"`
	Mode             BenchmarkMode      `json:"mode"`
	EngineID         string             `json:"engine_id"`
	Model            string             `json:"model"`
	VirtualUsers     VirtualUsers       `json:"virtual_users"`
	TotalRequests    int                `json:"total_requests"`
	SuccessfulReqs   int                `json:"successful_requests"`
	FailedReqs       int                `json:"failed_requests"`
	ErrorRatePercent float64            `json:"error_rate_percent"`
	TotalDuration    time.Duration      `json:"total_duration"`
	TTFTPercentiles  LatencyPercentiles `json:"ttft_percentiles"`
	AvgTPS           TokensPerSecond    `json:"avg_tps"`
	PeakTPS          TokensPerSecond    `json:"peak_tps"`
	PromptTokPerSec  float64            `json:"prompt_tokens_per_sec"`
	ThermalMetrics   ThermalMetrics     `json:"thermal_metrics"`
	FailoverMetrics  *FailoverMetrics   `json:"failover_metrics,omitempty"`
	Timestamp        time.Time          `json:"timestamp"`
}

// K6Runner defines the execution strategy contract.
type K6Runner interface {
	Name() string
	Execute(ctx context.Context, cfg BenchmarkConfig) (*BenchmarkSummary, error)
}

// Typestate types for compile-time verified BenchmarkSession lifecycle.
type (
	// UninitializedSession represents a benchmark session before runner allocation.
	UninitializedSession struct {
		id     BenchmarkRunID
		config BenchmarkConfig
	}

	// ProvisionedSession represents a benchmark session validated and paired with a runner.
	ProvisionedSession struct {
		id     BenchmarkRunID
		config BenchmarkConfig
		runner K6Runner
	}

	// CompletedSession represents a terminated benchmark session with finalized metrics.
	CompletedSession struct {
		id      BenchmarkRunID
		config  BenchmarkConfig
		summary *BenchmarkSummary
	}
)
