package distcodebase

import (
	"errors"
	"time"
)

// Typed errors for distributed codebase operations.
var (
	ErrSourceNotFound       = errors.New("codebase source not found")
	ErrDuplicateSource      = errors.New("codebase source already registered")
	ErrInvalidPriority      = errors.New("priority must be a positive integer (1 = highest priority)")
	ErrNoMatchingCapability = errors.New("no matching capability found across distributed codebase sources")
	ErrProjectDirExists     = errors.New("target project directory already exists")
	ErrEmptyProjectName     = errors.New("project name cannot be empty")
	ErrCoordinationNotFound = errors.New("interoperability coordination record not found")
	ErrModuleNotFound       = errors.New("target module definition not found")
)

// SourceType categorizes the hosting platform or registry of a codebase source.
type SourceType string

const (
	SourceTypeOwnRepo    SourceType = "own-repo"    // User/organization owned private git repos or servers (Priority 1)
	SourceTypeCuratedOSS SourceType = "curated-oss" // High-performance curated open-source libraries (Priority 2)
	SourceTypeRegistry   SourceType = "registry"    // Public package registries: crates.io, npm, Maven, PyPI, Go (Priority 3)
)

// CodebaseSource represents a registered repository host, organization, or registry.
type CodebaseSource struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Type         SourceType `json:"type"`
	Priority     int        `json:"priority"` // 1 = highest priority (own repos default to 1)
	URL          string     `json:"url"`
	AuthTokenEnv string     `json:"auth_token_env,omitempty"`
	Languages    []string   `json:"languages"`
	Description  string     `json:"description"`
	CreatedAt    time.Time  `json:"created_at"`
}

// LibraryEntry represents a specific library, package, or repository available in the distributed codebase.
type LibraryEntry struct {
	Name            string   `json:"name"`
	SourceID        string   `json:"source_id"`
	RepositoryURL   string   `json:"repository_url"`
	RegistryPackage string   `json:"registry_package,omitempty"`
	Language        string   `json:"language"`
	Description     string   `json:"description"`
	IsOwned         bool     `json:"is_owned"` // True if owned by user/org (Priority 1)
	Priority        int      `json:"priority"` // Inherited or custom priority
	Capabilities    []string `json:"capabilities"`
	Version         string   `json:"version,omitempty"`
}

// ModuleStrategy defines how a decomposed module should be fulfilled.
type ModuleStrategy string

const (
	StrategyReuseLibrary     ModuleStrategy = "reuse_library"
	StrategyDevelopCustom    ModuleStrategy = "develop_custom"
	StrategySatelliteProject ModuleStrategy = "satellite_side_project"
)

// ModuleDefinition represents a decoupled, reusable sub-module produced by forced decomposition.
type ModuleDefinition struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Language            string         `json:"language"`
	Purpose             string         `json:"purpose"`
	Capabilities        []string       `json:"capabilities"`
	Dependencies        []string       `json:"dependencies"` // IDs of other modules
	Strategy            ModuleStrategy `json:"strategy"`
	MatchedLibrary      *LibraryEntry  `json:"matched_library,omitempty"`
	DevelopCustomReason string         `json:"develop_custom_reason,omitempty"`
	UserRequestedCustom bool           `json:"user_requested_custom"`
	EstimatedComplexity int            `json:"estimated_complexity"`
}

// DecompositionPlan is the structured blueprint produced when dividing a project into reusable modules.
type DecompositionPlan struct {
	TargetProject          string             `json:"target_project"`
	TargetBinaryOrSpec     string             `json:"target_binary_or_spec"`
	ForcedModular          bool               `json:"forced_modular"` // True by default
	Modules                []ModuleDefinition `json:"modules"`
	TotalModules           int                `json:"total_modules"`
	ReusedLibrariesCount   int                `json:"reused_libraries_count"`
	CustomModulesCount     int                `json:"custom_modules_count"`
	SatelliteProjectsCount int                `json:"satellite_projects_count"`
	CreatedAt              time.Time          `json:"created_at"`
}

// InteropState tracks the lifecycle of an interoperability coordination between projects.
type InteropState string

const (
	InteropPending               InteropState = "pending"
	InteropUnblockedWithFallback InteropState = "unblocked_with_fallback"
	InteropDependencyCompleted   InteropState = "dependency_completed"
	InteropIntegrated            InteropState = "integrated"
)

// InteropCoordination tracks an interoperability dependency across owned projects.
type InteropCoordination struct {
	ID                  string       `json:"id"`
	RequesterProject    string       `json:"requester_project"`   // e.g. "Docula"
	DependencyProject   string       `json:"dependency_project"`  // e.g. "Oxipix"
	FeatureRequested    string       `json:"feature_requested"`   // e.g. "blend_modes_simd"
	DependencyIsOwned   bool         `json:"dependency_is_owned"` // Priority 1
	State               InteropState `json:"state"`
	InterimFallbackCode string       `json:"interim_fallback_code,omitempty"`
	FallbackTODO        string       `json:"fallback_todo"` // e.g. "// TODO(interop:Oxipix): Replace with Oxipix native blend modes"
	UnblocksWorkflow    bool         `json:"unblocks_workflow"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
}

// SpawnRequest configures a sub-project or satellite project to be created on disk and in Plane.
type SpawnRequest struct {
	ProjectName        string `json:"project_name"`
	Identifier         string `json:"identifier"` // Short code for Plane & repos, e.g. "CSEN"
	ModuleID           string `json:"module_id,omitempty"`
	ParentProject      string `json:"parent_project,omitempty"`
	TargetDir          string `json:"target_dir"`
	Language           string `json:"language"` // "rust", "go", "typescript", "python"
	IsSatellite        bool   `json:"is_satellite"`
	CreatePlaneProject bool   `json:"create_plane_project"`
	InitGit            bool   `json:"init_git"`
	Description        string `json:"description"`
}

// SpawnResult details the outcome of spawning a sub-project.
type SpawnResult struct {
	ProjectName    string `json:"project_name"`
	ProjectPath    string `json:"project_path"`
	PlaneProjectID string `json:"plane_project_id,omitempty"`
	GitInitialized bool   `json:"git_initialized"`
	Status         string `json:"status"`
}
