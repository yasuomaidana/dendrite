package assert

// Verdict represents the outcome of an assertion stage.
type Verdict string

const (
	VerifiedComplete   Verdict = "VERIFIED_COMPLETE"
	IncompleteLogic    Verdict = "INCOMPLETE_LOGIC"
	InvalidMocks       Verdict = "INVALID_MOCKS"
	HallucinatedAPI    Verdict = "HALLUCINATED_API"
	TestsFailing       Verdict = "TESTS_FAILING"
	CoverageBelowGate  Verdict = "COVERAGE_BELOW_GATE"
	ComplexityExceeded Verdict = "COMPLEXITY_EXCEEDED"
	PendingTODOs       Verdict = "PENDING_TODOS"
)

// Standard gate thresholds for Stage B verification.
const (
	DefaultCoverageGate      = 85.0 // Minimum statement coverage percentage (85%)
	DefaultMaxCyclomaticGate = 10   // Maximum allowed cyclomatic complexity per function
)

// ExecutionEvidence captures Stage B deterministic execution output.
type ExecutionEvidence struct {
	Toolchain     string         `json:"toolchain"`
	ExitCode      int            `json:"exit_code"`
	StdoutTail    string         `json:"stdout_tail"`
	Coverage      float64        `json:"coverage"`
	MaxCyclomatic map[string]int `json:"max_cyclomatic"`
}

// StageBResult captures the combined verdict and evidence for deterministic checks.
type StageBResult struct {
	Verdict  Verdict           `json:"verdict"`
	Evidence ExecutionEvidence `json:"evidence"`
	Reasons  []string          `json:"reasons,omitempty"`
}
