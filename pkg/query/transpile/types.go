package transpile

import (
	"errors"
	"time"
)

// Standard typed errors.
var (
	ErrUnsafeCodeDetected       = errors.New("unsafe block detected in safe Rust transpilation output")
	ErrDifferentialParityFailed = errors.New("differential test parity failed: delta exceeds epsilon tolerance")
	ErrEmptyArtifact            = errors.New("sources artifact path cannot be empty")
)

// DifferentialTestResult records the legacy vs Rust output comparison for a test vector.
type DifferentialTestResult struct {
	TestCaseID            string  `json:"test_case_id"`
	LegacyOutput          float64 `json:"legacy_output"`
	SynthesizedRustOutput float64 `json:"synthesized_rust_output"`
	Delta                 float64 `json:"delta"`
	Passed                bool    `json:"passed"`
}

// TranspileRequest encapsulates inputs for translating legacy artifacts to safe Rust.
type TranspileRequest struct {
	SourcesArtifact   string `json:"sources_artifact"`
	TargetLanguage    string `json:"target_language"` // default: "rust"
	NoStd             bool   `json:"no_std"`
	AuditLibraryReuse bool   `json:"audit_library_reuse"`
	Prompt            string `json:"prompt"`
	OutputDir         string `json:"output_dir,omitempty"`
}

// TranspileResult encapsulates the synthesized Rust crate metadata and verification metrics.
type TranspileResult struct {
	CrateName               string                   `json:"crate_name"`
	TargetLanguage          string                   `json:"target_language"`
	NoStd                   bool                     `json:"no_std"`
	NewtypesEmitted         []string                 `json:"newtypes_emitted"`
	ZeroUnsafeVerified      bool                     `json:"zero_unsafe_verified"`
	DifferentialTestsPassed bool                     `json:"differential_tests_passed"`
	TestCasesEvaluated      int                      `json:"test_cases_evaluated"`
	MaxDelta                float64                  `json:"max_delta"`
	EmittedCratePath        string                   `json:"emitted_crate_path"`
	DifferentialTestResults []DifferentialTestResult `json:"differential_test_results"`
	Timestamp               time.Time                `json:"timestamp"`
}
