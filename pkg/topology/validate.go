package topology

import (
	"fmt"
	"net"
	"strings"
)

// AllScenarios lists all recognized deployment scenarios.
func AllScenarios() []Scenario {
	return []Scenario{
		ScenarioLocalHybrid,
		ScenarioAzureBurst,
		ScenarioAWSBurst,
		ScenarioDistributed,
	}
}

// AllComputeNodes lists all supported compute nodes.
func AllComputeNodes() []ComputeNode {
	return []ComputeNode{
		NodeDebian,
		NodeMacBook,
		NodeAzure,
		NodeAWS,
	}
}

// AllStackComponents lists all architectural stack components.
func AllStackComponents() []StackComponent {
	return []StackComponent{
		ComponentControlPlane,
		ComponentInference,
		ComponentLinuxRE,
		ComponentRepoIntel,
		ComponentDocsPortal,
		ComponentDatabase,
	}
}

// IsValid checks if scenario matches a known scenario.
func (s Scenario) IsValid() bool {
	switch s {
	case ScenarioLocalHybrid, ScenarioAzureBurst, ScenarioAWSBurst, ScenarioDistributed:
		return true
	default:
		return false
	}
}

type validatableString interface {
	~string
	IsValid() bool
}

func parseValidString[T validatableString](s string, errBase error) (T, error) {
	norm := T(strings.ToLower(strings.TrimSpace(s)))
	if !norm.IsValid() {
		return "", fmt.Errorf("%w: '%s'", errBase, s)
	}
	return norm, nil
}

// ParseScenario parses and validates a scenario string.
func ParseScenario(s string) (Scenario, error) {
	return parseValidString[Scenario](s, ErrInvalidScenario)
}

// IsValid checks if compute node is valid.
func (n ComputeNode) IsValid() bool {
	switch n {
	case NodeDebian, NodeMacBook, NodeAzure, NodeAWS:
		return true
	default:
		return false
	}
}

// ParseComputeNode parses and validates a compute node string.
func ParseComputeNode(s string) (ComputeNode, error) {
	return parseValidString[ComputeNode](s, ErrInvalidComputeNode)
}

// IsValid checks if stack component is valid.
func (c StackComponent) IsValid() bool {
	switch c {
	case ComponentControlPlane, ComponentInference, ComponentLinuxRE,
		ComponentRepoIntel, ComponentDocsPortal, ComponentDatabase:
		return true
	default:
		return false
	}
}

// ParseStackComponent parses and validates a stack component string.
func ParseStackComponent(s string) (StackComponent, error) {
	return parseValidString[StackComponent](s, ErrInvalidComponent)
}

// ValidateEndpointHost checks if a host string is a valid IP, localhost, or .ts.net domain.
func ValidateEndpointHost(host string) error {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return nil
	}
	// Check if valid IP (IPv4 or IPv6)
	if ip := net.ParseIP(trimmed); ip != nil {
		return nil
	}
	// Check hostname rules (localhost or Tailscale domain)
	if trimmed == "localhost" || strings.HasSuffix(trimmed, ".ts.net") || strings.HasSuffix(trimmed, ".local") {
		return nil
	}
	return fmt.Errorf("%w: host '%s' must be an IP or .ts.net/.local/localhost domain", ErrInvalidEndpointHost, host)
}

// Validate verifies the consistency and validity of the entire configuration.
func (c *TopologyConfig) Validate() error {
	if c == nil {
		return &ValidationError{Field: "config", Message: "topology configuration is nil"}
	}

	if !c.Scenario.IsValid() {
		return &ValidationError{
			Field:   "scenario",
			Message: fmt.Sprintf("unrecognized scenario '%s'", c.Scenario),
			Err:     ErrInvalidScenario,
		}
	}

	// Validate endpoint formats
	if err := ValidateEndpointHost(c.TailscaleEndpoints.ControlPlane); err != nil {
		return &ValidationError{Field: "tailscale_endpoints.control_plane", Message: err.Error(), Err: err}
	}
	if err := ValidateEndpointHost(c.TailscaleEndpoints.ExecutionPlane); err != nil {
		return &ValidationError{Field: "tailscale_endpoints.execution_plane", Message: err.Error(), Err: err}
	}
	if err := ValidateEndpointHost(c.TailscaleEndpoints.AzureBurst); err != nil {
		return &ValidationError{Field: "tailscale_endpoints.azure_burst", Message: err.Error(), Err: err}
	}
	if err := ValidateEndpointHost(c.TailscaleEndpoints.AWSBurst); err != nil {
		return &ValidationError{Field: "tailscale_endpoints.aws_burst", Message: err.Error(), Err: err}
	}

	if len(c.StackPlacement) == 0 {
		return &ValidationError{Field: "stack_placement", Message: "stack placement mapping cannot be empty"}
	}

	// Mandatory components check
	if _, ok := c.StackPlacement[ComponentControlPlane]; !ok {
		return &ValidationError{
			Field:   "stack_placement",
			Message: "missing mandatory component: control_plane",
			Err:     ErrMissingComponent,
		}
	}
	if _, ok := c.StackPlacement[ComponentInference]; !ok {
		return &ValidationError{
			Field:   "stack_placement",
			Message: "missing mandatory component: inference",
			Err:     ErrMissingComponent,
		}
	}

	// Validate components and nodes
	for comp, node := range c.StackPlacement {
		if !comp.IsValid() {
			return &ValidationError{
				Field:   fmt.Sprintf("stack_placement[%s]", comp),
				Message: "invalid component name",
				Err:     ErrInvalidComponent,
			}
		}
		if !node.IsValid() {
			return &ValidationError{
				Field:   fmt.Sprintf("stack_placement[%s]", comp),
				Message: fmt.Sprintf("invalid node '%s'", node),
				Err:     ErrInvalidComputeNode,
			}
		}
	}

	// Validate scenario-specific placement constraints
	return c.validateScenarioConstraints()
}

func (c *TopologyConfig) validateScenarioConstraints() error {
	switch c.Scenario {
	case ScenarioLocalHybrid:
		// Cloud resources must be off in local-hybrid
		for comp, node := range c.StackPlacement {
			if node == NodeAzure || node == NodeAWS {
				return &ValidationError{
					Field:   fmt.Sprintf("stack_placement[%s]", comp),
					Message: fmt.Sprintf("local-hybrid scenario forbids cloud compute node '%s'", node),
					Err:     ErrInvalidPlacement,
				}
			}
		}

	case ScenarioAzureBurst:
		// Debian server is off in azure-burst
		for comp, node := range c.StackPlacement {
			if node == NodeDebian {
				return &ValidationError{
					Field:   fmt.Sprintf("stack_placement[%s]", comp),
					Message: "azure-burst scenario requires debian server to be off",
					Err:     ErrInvalidPlacement,
				}
			}
		}
		// Control plane should be on Azure in azure-burst
		if c.StackPlacement[ComponentControlPlane] != NodeAzure {
			return &ValidationError{
				Field:   "stack_placement[control_plane]",
				Message: "azure-burst scenario requires control plane to reside on azure",
				Err:     ErrInvalidPlacement,
			}
		}

	case ScenarioAWSBurst:
		// AWS burst requires offloading compute to AWS
		hasAWS := false
		for _, node := range c.StackPlacement {
			if node == NodeAWS {
				hasAWS = true
				break
			}
		}
		if !hasAWS {
			return &ValidationError{
				Field:   "stack_placement",
				Message: "aws-burst scenario requires at least one stack component assigned to aws",
				Err:     ErrInvalidPlacement,
			}
		}

	case ScenarioDistributed:
		// Control plane must remain on debian server as state anchor
		if c.StackPlacement[ComponentControlPlane] != NodeDebian {
			return &ValidationError{
				Field:   "stack_placement[control_plane]",
				Message: "distributed scenario requires debian server as permanent control plane state anchor",
				Err:     ErrInvalidPlacement,
			}
		}
	}

	return nil
}
