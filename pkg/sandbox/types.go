package sandbox

import (
	"errors"
	"time"
)

// Standard errors for sandbox operations.
var (
	ErrEmptyImage        = errors.New("sandbox container image cannot be empty")
	ErrEmptyCommand      = errors.New("sandbox execution command cannot be empty")
	ErrSandboxTerminated = errors.New("sandbox execution terminated early")
	ErrSessionClosed     = errors.New("sandbox session is closed")
)

// SandboxConfig encapsulates the security hardening options for an ephemeral container.
type SandboxConfig struct {
	ContainerName       string            `json:"container_name"`
	Image               string            `json:"image"`
	Command             []string          `json:"command"`
	NetworkMode         string            `json:"network_mode"` // "none", "bridge", "mesh"
	ReadOnlyRoot        bool              `json:"read_only_root"`
	DropAllCapabilities bool              `json:"drop_all_capabilities"`
	NoNewPrivileges     bool              `json:"no_new_privileges"`
	MemoryLimitMB       int               `json:"memory_limit_mb"`
	CPULimit            float64           `json:"cpu_limit"`
	TmpfsSize           string            `json:"tmpfs_size"`
	HostScratchDir      string            `json:"host_scratch_dir"`
	Timeout             time.Duration     `json:"timeout"`
	Environment         map[string]string `json:"environment"`
}

// ExecutionResult captures the output, exit status, and duration of sandboxed execution.
type ExecutionResult struct {
	ContainerName string        `json:"container_name"`
	ExitCode      int           `json:"exit_code"`
	Stdout        string        `json:"stdout"`
	Stderr        string        `json:"stderr"`
	Duration      time.Duration `json:"duration"`
	CleanedUp     bool          `json:"cleaned_up"`
}
