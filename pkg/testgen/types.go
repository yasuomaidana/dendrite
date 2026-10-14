package testgen

import "time"

// TestRunID represents a unique test synthesis run identifier.
type TestRunID string

// IterationCount represents the self-healing attempt number.
type IterationCount uint8

// CoveragePercent represents the measured code coverage percentage.
type CoveragePercent float32

// SessionState models the typestate lifecycle of test synthesis.
type SessionState string

const (
	StateDrafting  SessionState = "Drafting"
	StateCompiling SessionState = "Compiling"
	StateExecuting SessionState = "Executing"
	StateHealing   SessionState = "Healing"
	StateVerified  SessionState = "Verified"
	StateFailed    SessionState = "Failed"
)

// DependencyType classifies persistence and network infrastructure requirements.
type DependencyType string

const (
	DepNone       DependencyType = "NONE"
	DepPostgres   DependencyType = "POSTGRESQL"
	DepSQLite     DependencyType = "SQLITE"
	DepRedis      DependencyType = "REDIS"
	DepHTTPClient DependencyType = "HTTP_CLIENT"
)

// SourceSymbol represents an extracted function, struct, or method signature.
type SourceSymbol struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"` // "func", "struct", "method"
	Receiver    string   `json:"receiver,omitempty"`
	Signature   string   `json:"signature"`
	ParamTypes  []string `json:"param_types"`
	ReturnTypes []string `json:"return_types"`
	IsExported  bool     `json:"is_exported"`
	Doc         string   `json:"doc,omitempty"`
}

// TestSynthesisConfig configures the test generation and healing loop.
type TestSynthesisConfig struct {
	SourceFile           string          `json:"source_file"`
	OutputPath           string          `json:"output_path,omitempty"`
	WithTestcontainers   bool            `json:"with_testcontainers"`
	MaxHealingIterations IterationCount  `json:"max_healing_iterations"`
	TargetCoverage       CoveragePercent `json:"target_coverage"`
	Timeout              time.Duration   `json:"timeout"`
}

// HealingLogEntry records an iterative repair attempt.
type HealingLogEntry struct {
	Iteration IterationCount `json:"iteration"`
	ErrorMsg  string         `json:"error_msg"`
	PatchDesc string         `json:"patch_desc"`
	Timestamp time.Time      `json:"timestamp"`
}

// SynthesisReport contains the outcome of test generation and validation.
type SynthesisReport struct {
	RunID                TestRunID         `json:"run_id"`
	SourceFile           string            `json:"source_file"`
	TestFile             string            `json:"test_file"`
	State                SessionState      `json:"state"`
	Iterations           IterationCount    `json:"iterations"`
	Success              bool              `json:"success"`
	Coverage             CoveragePercent   `json:"coverage"`
	DependenciesDetected []DependencyType  `json:"dependencies_detected"`
	HealingLog           []HealingLogEntry `json:"healing_log"`
	Duration             time.Duration     `json:"duration"`
}
