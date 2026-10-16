package migrate

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrZeroCredentialViolation = errors.New("zero-credential invariant violated: plain text credentials prohibited")
	ErrInteractiveAuthDenied   = errors.New("interactive migration authorization denied by user")
	ErrNonInteractiveNoForce   = errors.New("interactive terminal confirmation required; cannot proceed in non-interactive environment without explicit authorization")
	ErrRemoteHealthCheckFailed = errors.New("remote health check probe failed within deadline")
	ErrServiceNotFound         = errors.New("requested service is not registered in migration catalogue")
	ErrInsufficientCapacity    = errors.New("target host has insufficient RAM or disk capacity for requested services")
	ErrUnsupportedOS           = errors.New("target host operating system is not supported for this service")
)

// ServiceProfile defines resource footprints and operational metadata for a migratable service.
type ServiceProfile struct {
	Name           string   `json:"name"`
	DisplayName    string   `json:"display_name"`
	Description    string   `json:"description"`
	ComposeFile    string   `json:"compose_file"`
	Port           int      `json:"port"`
	HealthEndpoint string   `json:"health_endpoint"`
	MinRAMMB       uint64   `json:"min_ram_mb"`
	MinDiskGB      uint64   `json:"min_disk_gb"`
	RequiresGPU    bool     `json:"requires_gpu"`
	Containers     []string `json:"containers"`
	EnvKey         string   `json:"env_key"`
	SupportedOS    []string `json:"supported_os"`
}

// MigrationRequest encapsulates parameters for moving services to an SSH server.
type MigrationRequest struct {
	Services     []string      `json:"services"`
	TargetHost   string        `json:"target_host"` // e.g. "user@debian-server" or "debian-server"
	SSHPort      int           `json:"ssh_port"`
	SSHUser      string        `json:"ssh_user"`
	KeyPath      string        `json:"key_path,omitempty"`
	TargetRAMMB  uint64        `json:"target_ram_mb"`
	TargetDiskGB uint64        `json:"target_disk_gb"`
	AutoApprove  bool          `json:"auto_approve"`
	DryRun       bool          `json:"dry_run"`
	PruneLocal   bool          `json:"prune_local"`
	Timeout      time.Duration `json:"timeout"`
	In           io.Reader     `json:"-"`
	Out          io.Writer     `json:"-"`
}

// MigrationPhase designates an atomic step in the migration lifecycle.
type MigrationPhase string

const (
	PhasePreFlight     MigrationPhase = "Phase 1: Pre-Flight Hardware & Capacity Validation"
	PhaseAuthorization MigrationPhase = "Phase 2: Interactive Zero-Credential Authorization"
	PhaseDataSync      MigrationPhase = "Phase 3: Ephemeral SSH Streaming & Volume Transfer"
	PhaseRemoteDeploy  MigrationPhase = "Phase 4: Remote Container Launch & Health Verification"
	PhaseCutoverPrune  MigrationPhase = "Phase 5: Atomic Endpoint Propagation & Local Resource Pruning"
)

// StepRecord records the execution result of an atomic migration phase.
type StepRecord struct {
	Phase    MigrationPhase `json:"phase"`
	Status   string         `json:"status"` // "SUCCESS", "FAILED", "SKIPPED", "DRY_RUN"
	Details  string         `json:"details"`
	Duration time.Duration  `json:"duration"`
}

// MigrationResult is the final output of a service migration run.
type MigrationResult struct {
	Success          bool              `json:"success"`
	ServicesMigrated []string          `json:"services_migrated"`
	TargetHost       string            `json:"target_host"`
	Steps            []StepRecord      `json:"steps"`
	UpdatedEndpoints map[string]string `json:"updated_endpoints"`
	TotalDuration    time.Duration     `json:"total_duration"`
	Error            string            `json:"error,omitempty"`
}

// DebianEligibility reflects the evaluation of whether a service can run on a Debian server.
type DebianEligibility struct {
	Service     string `json:"service"`
	DisplayName string `json:"display_name"`
	Eligible    bool   `json:"eligible"`
	RAMMB       uint64 `json:"ram_mb"`
	DiskGB      uint64 `json:"disk_gb"`
	Reason      string `json:"reason"`
}

// DebianCapacityPlan summarizes the allocation of services on a Debian server.
type DebianCapacityPlan struct {
	ServerProfile     string              `json:"server_profile"`
	TotalRAMMB        uint64              `json:"total_ram_mb"`
	TotalDiskGB       uint64              `json:"total_disk_gb"`
	AllocatedRAMMB    uint64              `json:"allocated_ram_mb"`
	AllocatedDiskGB   uint64              `json:"allocated_disk_gb"`
	AvailableRAMMB    uint64              `json:"available_ram_mb"`
	AvailableDiskGB   uint64              `json:"available_disk_gb"`
	EligibleServices  []DebianEligibility `json:"eligible_services"`
	ExcludedServices  []DebianEligibility `json:"excluded_services"`
	RecommendedAction string              `json:"recommended_action"`
}

// RemoteCommandExecutor abstracts command execution on remote or local targets.
type RemoteCommandExecutor interface {
	Execute(ctx context.Context, host string, port int, user string, keyPath string, cmd string) (string, error)
	StreamFile(ctx context.Context, host string, port int, user string, keyPath string, localPath string, remotePath string) error
	ProbeHealth(ctx context.Context, url string, timeout time.Duration) (bool, error)
}
