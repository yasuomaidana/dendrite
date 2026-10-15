package reuse

import "errors"

var (
	// ErrEmptyCapability is returned when no capability query is provided.
	ErrEmptyCapability = errors.New("capability cannot be empty")
	// ErrReferenceNotFound is returned when a requested source reference does not exist.
	ErrReferenceNotFound = errors.New("source reference not found")
	// ErrDuplicateReference is returned when a reference already exists.
	ErrDuplicateReference = errors.New("source reference with this ID already registered")
)

// ReuseTier identifies the provenance tier in the 3-tier hierarchy.
type ReuseTier string

const (
	Tier1SourceRef  ReuseTier = "Tier 1: Source Reference"
	Tier2PoolMember ReuseTier = "Tier 2: Pool Member"
	Tier3Registry   ReuseTier = "Tier 3: Ecosystem Registry"
	TierUnresolved  ReuseTier = "Unresolved (Synthesize)"
)

// SourceReference represents a registered repository reference managed via `dendrite ref`.
type SourceReference struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Repository   string   `json:"repository"`
	Path         string   `json:"path,omitempty"`
	Language     string   `json:"language"`
	Capabilities []string `json:"capabilities"`
	License      string   `json:"license"`
}

// AuditRequest contains the inputs for the 3-tier library reuse auditor.
type AuditRequest struct {
	Capability      string `json:"capability"`
	Language        string `json:"language"`
	TicketID        string `json:"ticket_id,omitempty"`
	Intent          string `json:"intent,omitempty"`
	TargetDirectory string `json:"target_directory,omitempty"`
}

// AuditResult summarizes the evaluation decision and emitted ADR record.
type AuditResult struct {
	TierSelected  ReuseTier `json:"tier_selected"`
	CandidateName string    `json:"candidate_name"`
	PackageSpec   string    `json:"package_spec"`
	SourceURI     string    `json:"source_uri"`
	ImportPath    string    `json:"import_path"`
	Action        string    `json:"action"` // slice_and_import, wire_pool_edge, add_registry_dep, synthesize_new
	Rationale     string    `json:"rationale"`
	ADRPath       string    `json:"adr_path"`
	ADRID         string    `json:"adr_id"`
	PreCheckHash  string    `json:"pre_check_hash"`
}
