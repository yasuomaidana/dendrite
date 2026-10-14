package env

import (
	"context"
	"time"
)

// InstallMode defines the installation strategy for replicating the dev environment.
type InstallMode string

const (
	// ModeContainer provisions all components inside isolated Docker containers.
	ModeContainer InstallMode = "container"
	// ModeHost installs and configures tools directly on the host computer.
	ModeHost InstallMode = "host"
	// ModeHybrid runs services in Docker while installing CLI/dev tools natively on the host.
	ModeHybrid InstallMode = "hybrid"
)

// IsValid validates whether the install mode is supported.
func (m InstallMode) IsValid() bool {
	switch m {
	case ModeContainer, ModeHost, ModeHybrid:
		return true
	default:
		return false
	}
}

// ReplicationTarget identifies an environment component or tool that can be replicated.
type ReplicationTarget string

const (
	// TargetLinear represents the Linear issue tracking service (containerized local mock/server or host CLI).
	TargetLinear ReplicationTarget = "linear"
	// TargetPlane represents the open-source Plane v2 project management service.
	TargetPlane ReplicationTarget = "plane"
	// TargetPenpot represents the open-source Penpot design and prototyping platform.
	TargetPenpot ReplicationTarget = "penpot"
	// TargetLiteLLM represents the LiteLLM proxy gateway for token governance.
	TargetLiteLLM ReplicationTarget = "litellm"
	// TargetOllama represents the local LLM inference engine.
	TargetOllama ReplicationTarget = "ollama"
	// TargetPostgres represents the persistent PostgreSQL database.
	TargetPostgres ReplicationTarget = "postgres"
	// TargetRedis represents the persistent Redis cache.
	TargetRedis ReplicationTarget = "redis"
	// TargetTailscale represents the zero-trust mesh networking daemon.
	TargetTailscale ReplicationTarget = "tailscale"
	// TargetCloudflare represents the Cloudflare Tunnel connectivity daemon.
	TargetCloudflare ReplicationTarget = "cloudflare"
	// TargetGrafana represents the Grafana + Prometheus telemetry observability suite.
	TargetGrafana ReplicationTarget = "grafana"
	// TargetDocker represents the container runtime engine.
	TargetDocker ReplicationTarget = "docker"
)

// AllTargets returns the standard list of replicable environment targets.
func AllTargets() []ReplicationTarget {
	return []ReplicationTarget{
		TargetDocker,
		TargetTailscale,
		TargetCloudflare,
		TargetPostgres,
		TargetRedis,
		TargetLiteLLM,
		TargetOllama,
		TargetLinear,
		TargetPlane,
		TargetPenpot,
		TargetGrafana,
	}
}

// PackageManager represents host OS package managers.
type PackageManager string

const (
	PkgBrew    PackageManager = "homebrew"
	PkgApt     PackageManager = "apt"
	PkgDnf     PackageManager = "dnf"
	PkgPacman  PackageManager = "pacman"
	PkgUnknown PackageManager = "unknown"
)

// HostInfo represents detected target computer operating system and installed tooling.
type HostInfo struct {
	OS             string         `json:"os"`
	Arch           string         `json:"arch"`
	PackageManager PackageManager `json:"package_manager"`
	HasDocker      bool           `json:"has_docker"`
	HasDockerComp  bool           `json:"has_docker_compose"`
	HasOllama      bool           `json:"has_ollama"`
	HasGo          bool           `json:"has_go"`
	HasNode        bool           `json:"has_node"`
	HasNpm         bool           `json:"has_npm"`
	HasTailscale   bool           `json:"has_tailscale"`
	HasLinearCLI   bool           `json:"has_linear_cli"`
}

// ActionType categorizes actions in an environment replication plan.
type ActionType string

const (
	ActionCheckDependency ActionType = "check_dependency"
	ActionInstallHostTool ActionType = "install_host_tool"
	ActionRenderCompose   ActionType = "render_compose"
	ActionStartContainer  ActionType = "start_container"
	ActionStopContainer   ActionType = "stop_container"
	ActionConfigureEnv    ActionType = "configure_env"
	ActionVerifyHealth    ActionType = "verify_health"
)

// PlanAction defines an atomic operation within a replication execution plan.
type PlanAction struct {
	ID          string            `json:"id"`
	Type        ActionType        `json:"type"`
	Target      ReplicationTarget `json:"target"`
	Description string            `json:"description"`
	Command     string            `json:"command"`
	Rollback    string            `json:"rollback,omitempty"`
	Mandatory   bool              `json:"mandatory"`
}

// ReplicationRequest specifies user instructions for environment replication.
type ReplicationRequest struct {
	Mode        InstallMode         `json:"mode"`
	Targets     []ReplicationTarget `json:"targets"`
	WithLinear  bool                `json:"with_linear"`
	WithPlane   bool                `json:"with_plane"`
	WithPlaneUI *bool               `json:"with_plane_ui,omitempty"`
	WithPenpot  bool                `json:"with_penpot"`
	WithGrafana bool                `json:"with_grafana"`
	DryRun      bool                `json:"dry_run"`
	OutputDir   string              `json:"output_dir"`
}

// ReplicationPlan encapsulates the sequenced actions required to replicate the environment.
type ReplicationPlan struct {
	CreatedAt     time.Time    `json:"created_at"`
	Mode          InstallMode  `json:"mode"`
	TargetHost    HostInfo     `json:"target_host"`
	Actions       []PlanAction `json:"actions"`
	ComposeFiles  []string     `json:"compose_files,omitempty"`
	EstimatedSecs int          `json:"estimated_secs"`
}

// ActionExecutionResult records the outcome of a single plan action.
type ActionExecutionResult struct {
	ActionID string        `json:"action_id"`
	Success  bool          `json:"success"`
	Output   string        `json:"output,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Skipped  bool          `json:"skipped"`
}

// ReplicationResult summarizes the complete execution of an environment replication.
type ReplicationResult struct {
	Mode           InstallMode             `json:"mode"`
	Success        bool                    `json:"success"`
	TotalDuration  time.Duration           `json:"total_duration"`
	ActionResults  []ActionExecutionResult `json:"action_results"`
	Components     []ComponentStatus       `json:"components"`
	GeneratedFiles []string                `json:"generated_files"`
	Error          string                  `json:"error,omitempty"`
}

// ComponentStatus captures runtime health and availability of an environment tool or service.
type ComponentStatus struct {
	Target    ReplicationTarget `json:"target"`
	Mode      InstallMode       `json:"mode"`
	Installed bool              `json:"installed"`
	Reachable bool              `json:"reachable"`
	Version   string            `json:"version,omitempty"`
	Endpoint  string            `json:"endpoint,omitempty"`
	Details   string            `json:"details,omitempty"`
}

// LinearConfig defines parameters for containerized or host Linear installation.
type LinearConfig struct {
	Port          int    `json:"port"`
	DBPort        int    `json:"db_port"`
	ContainerName string `json:"container_name"`
	LocalEndpoint string `json:"local_endpoint"`
	APIKey        string `json:"api_key"`
	DockerImage   string `json:"docker_image"`
}

// DefaultLinearConfig returns sensible defaults for replicating Linear in a container.
func DefaultLinearConfig() LinearConfig {
	return LinearConfig{
		Port:          8085,
		DBPort:        5433,
		ContainerName: "dendrite-linear-server",
		LocalEndpoint: "http://localhost:8085",
		APIKey:        "lin_local_mock_key_dendrite",
		DockerImage:   "ghcr.io/dendrite-dev/linear-mock:latest",
	}
}

// CommandRunner abstracts command execution for testability and portability.
type CommandRunner interface {
	Run(ctx context.Context, cmd string, env map[string]string) (string, error)
	LookPath(file string) (string, error)
}
