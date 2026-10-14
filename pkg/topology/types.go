package topology

import (
	"errors"
	"fmt"
)

// Scenario defines the infrastructure deployment scenario.
type Scenario string

const (
	ScenarioLocalHybrid Scenario = "local-hybrid"
	ScenarioAzureBurst  Scenario = "azure-burst"
	ScenarioAWSBurst    Scenario = "aws-burst"
	ScenarioDistributed Scenario = "distributed"
)

// ComputeNode represents an execution or control plane host node.
type ComputeNode string

const (
	NodeDebian  ComputeNode = "debian"
	NodeMacBook ComputeNode = "macbook"
	NodeAzure   ComputeNode = "azure"
	NodeAWS     ComputeNode = "aws"
)

// StackComponent defines individual architectural service tiers.
type StackComponent string

const (
	ComponentControlPlane StackComponent = "control_plane"
	ComponentInference    StackComponent = "inference"
	ComponentLinuxRE      StackComponent = "linux_re"
	ComponentRepoIntel    StackComponent = "repo_intel"
	ComponentDocsPortal   StackComponent = "docs_portal"
	ComponentDatabase     StackComponent = "database"
)

// TailscaleEndpoints holds the encrypted mesh IP addresses or hostnames for each node.
type TailscaleEndpoints struct {
	ControlPlane   string `json:"control_plane" yaml:"control_plane"`
	ExecutionPlane string `json:"execution_plane" yaml:"execution_plane"`
	AzureBurst     string `json:"azure_burst,omitempty" yaml:"azure_burst,omitempty"`
	AWSBurst       string `json:"aws_burst,omitempty" yaml:"aws_burst,omitempty"`
}

// StackPlacement maps each StackComponent to its assigned ComputeNode.
type StackPlacement map[StackComponent]ComputeNode

// TopologyConfig encapsulates the full deployment topology and node placement scenario.
type TopologyConfig struct {
	Scenario           Scenario           `json:"scenario" yaml:"scenario"`
	TailscaleEndpoints TailscaleEndpoints `json:"tailscale_endpoints" yaml:"tailscale_endpoints"`
	StackPlacement     StackPlacement     `json:"stack_placement" yaml:"stack_placement"`
	Metadata           map[string]string  `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Custom error definitions.
var (
	ErrInvalidScenario     = errors.New("invalid scenario")
	ErrInvalidComputeNode  = errors.New("invalid compute node")
	ErrInvalidComponent    = errors.New("invalid stack component")
	ErrMissingComponent    = errors.New("missing mandatory stack component")
	ErrInvalidPlacement    = errors.New("invalid placement for scenario")
	ErrInvalidEndpointHost = errors.New("invalid endpoint host format")
)

// ValidationError provides detailed context on validation failures.
type ValidationError struct {
	Field   string
	Message string
	Err     error
}

func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("validation failed on %s: %s (%v)", e.Field, e.Message, e.Err)
	}
	return fmt.Sprintf("validation failed on %s: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}
