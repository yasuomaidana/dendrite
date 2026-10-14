package orchestrator

import (
	"errors"
	"fmt"

	"Dendrite/pkg/topology/network"
)

// Errors returned by orchestrator operations.
var (
	ErrNilTopologyConfig = errors.New("topology configuration is nil")
	ErrInvalidNode       = errors.New("invalid or unsupported compute node")
	ErrEmptyAction       = errors.New("compose action cannot be empty")
	ErrMissingAuthKey    = errors.New("tailscale auth key is required")
)

// CloudInitConfig defines parameters for bootstrapping a Linux compute node.
type CloudInitConfig struct {
	Hostname           string                  `yaml:"hostname" json:"hostname"`
	TailscaleAuthKey   string                  `yaml:"tailscale_auth_key" json:"tailscale_auth_key"`
	NetworkProvider    network.NetworkProvider `yaml:"-" json:"-"`
	DockerUsers        []string                `yaml:"docker_users,omitempty" json:"docker_users,omitempty"`
	Packages           []string                `yaml:"packages,omitempty" json:"packages,omitempty"`
	SSHAuthorizedKeys  []string                `yaml:"ssh_authorized_keys,omitempty" json:"ssh_authorized_keys,omitempty"`
	ExtraRunCmd        []string                `yaml:"extra_run_cmd,omitempty" json:"extra_run_cmd,omitempty"`
	EnableTailscaleSSH bool                    `yaml:"enable_tailscale_ssh" json:"enable_tailscale_ssh"`
}

// ComposeCommand encapsulates all flags and arguments for executing Docker Compose.
type ComposeCommand struct {
	Binary       string            `json:"binary"`
	ComposeFiles []string          `json:"compose_files"`
	Profiles     []string          `json:"profiles"`
	Action       string            `json:"action"`
	Detached     bool              `json:"detached"`
	Environment  map[string]string `json:"environment,omitempty"`
	ExtraArgs    []string          `json:"extra_args,omitempty"`
}

// NodeDeploymentPlan summarizes the cloud-init and compose actions for a target compute node.
type NodeDeploymentPlan struct {
	Node            string            `json:"node"`
	ComposeCommand  *ComposeCommand   `json:"compose_command,omitempty"`
	CloudInitScript string            `json:"cloud_init_script,omitempty"`
	Environment     map[string]string `json:"environment"`
}

// OrchestrationError wraps validation and execution failures with node context.
type OrchestrationError struct {
	Node string
	Op   string
	Err  error
}

func (e *OrchestrationError) Error() string {
	return fmt.Sprintf("orchestrator error on node '%s' during %s: %v", e.Node, e.Op, e.Err)
}

func (e *OrchestrationError) Unwrap() error {
	return e.Err
}
