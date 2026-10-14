package showcase

import (
	"context"

	"Dendrite/pkg/rea"
)

// Target identifier constants.
const (
	TargetAegis      = "aegis"
	TargetDXBall     = "dxball"
	TargetCTF        = "ctf"
	TargetTH04       = "th04"
	TargetMattermost = "mattermost"
	TargetAll        = "all"
)

// ShowcaseMetadata describes the open-source pedigree and technical traits of a showcase target.
type ShowcaseMetadata struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	UpstreamRepo        string           `json:"upstream_repo"`
	LicenseSPDX         string           `json:"license_spdx"`
	BinaryFormat        rea.BinaryFormat `json:"binary_format"`
	ExtractedCapability string           `json:"extracted_capability"`
	GroundTruthOracle   string           `json:"ground_truth_oracle"`
	RoleOrSubstitution  string           `json:"role_or_substitution"`
}

// TestParityResult captures the results of running synthesized code against the ground-truth test oracle.
type TestParityResult struct {
	TotalVectors  int     `json:"total_vectors"`
	PassedVectors int     `json:"passed_vectors"`
	FailedVectors int     `json:"failed_vectors"`
	ParityPercent float64 `json:"parity_percent"`
	Details       string  `json:"details,omitempty"`
}

// SPDXAuditResult captures open-source license governance auditing.
type SPDXAuditResult struct {
	Target      string `json:"target"`
	LicenseID   string `json:"license_id"`
	OSIApproved bool   `json:"osi_approved"`
	IsCopyleft  bool   `json:"is_copyleft"`
	Compliant   bool   `json:"compliant"`
	Notes       string `json:"notes"`
}

// ShowcaseResult contains the full analysis, synthesis, verification, and ADR output for a single target.
type ShowcaseResult struct {
	Target           string              `json:"target"`
	Metadata         ShowcaseMetadata    `json:"metadata"`
	Evidence         *rea.EvidenceBundle `json:"evidence"`
	Parity           TestParityResult    `json:"parity"`
	SPDX             SPDXAuditResult     `json:"spdx"`
	ADR              *rea.ADRDocument    `json:"adr"`
	ADRMarkdown      string              `json:"adr_markdown"`
	SynthesizedCode  string              `json:"synthesized_code"`
	ExecutionTimeMs  int64               `json:"execution_time_ms"`
	SandboxSecured   bool                `json:"sandbox_secured"`
	PassedValidation bool                `json:"passed_validation"`
	Error            string              `json:"error,omitempty"`
}

// ShowcaseReport aggregates results across multiple or all showcase targets.
type ShowcaseReport struct {
	Timestamp            string           `json:"timestamp"`
	OverallStatus        string           `json:"overall_status"`
	TotalTargets         int              `json:"total_targets"`
	SucceededTargets     int              `json:"succeeded_targets"`
	FailedTargets        int              `json:"failed_targets"`
	OverallParityPercent float64          `json:"overall_parity_percent"`
	Results              []ShowcaseResult `json:"results"`
}

// Handler defines the contract for an individual showcase target implementation.
type Handler interface {
	Metadata() ShowcaseMetadata
	Execute(ctx context.Context) (*ShowcaseResult, error)
}
