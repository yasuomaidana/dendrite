package topology

import (
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// DefaultPlacement returns the canonical stack component mapping for a given scenario.
func DefaultPlacement(s Scenario) (StackPlacement, error) {
	switch s {
	case ScenarioLocalHybrid:
		return StackPlacement{
			ComponentControlPlane: NodeDebian,
			ComponentInference:    NodeMacBook,
			ComponentLinuxRE:      NodeDebian,
			ComponentRepoIntel:    NodeDebian,
			ComponentDocsPortal:   NodeDebian,
			ComponentDatabase:     NodeDebian,
		}, nil

	case ScenarioAzureBurst:
		return StackPlacement{
			ComponentControlPlane: NodeAzure,
			ComponentInference:    NodeMacBook,
			ComponentLinuxRE:      NodeAzure,
			ComponentRepoIntel:    NodeAzure,
			ComponentDocsPortal:   NodeAzure,
			ComponentDatabase:     NodeAzure,
		}, nil

	case ScenarioAWSBurst:
		return StackPlacement{
			ComponentControlPlane: NodeDebian,
			ComponentInference:    NodeMacBook,
			ComponentLinuxRE:      NodeAWS,
			ComponentRepoIntel:    NodeAWS,
			ComponentDocsPortal:   NodeDebian,
			ComponentDatabase:     NodeDebian,
		}, nil

	case ScenarioDistributed:
		return StackPlacement{
			ComponentControlPlane: NodeDebian,
			ComponentInference:    NodeMacBook,
			ComponentLinuxRE:      NodeAWS,
			ComponentRepoIntel:    NodeAzure,
			ComponentDocsPortal:   NodeDebian,
			ComponentDatabase:     NodeDebian,
		}, nil

	default:
		return nil, fmt.Errorf("%w: '%s'", ErrInvalidScenario, s)
	}
}

// NewTopologyConfig builds a validated TopologyConfig using default placements for the scenario.
func NewTopologyConfig(s Scenario, endpoints TailscaleEndpoints) (*TopologyConfig, error) {
	placement, err := DefaultPlacement(s)
	if err != nil {
		return nil, err
	}

	cfg := &TopologyConfig{
		Scenario:           s,
		TailscaleEndpoints: endpoints,
		StackPlacement:     placement,
		Metadata:           make(map[string]string),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// GetNodeForComponent returns the compute node assigned to a given stack component.
func (c *TopologyConfig) GetNodeForComponent(comp StackComponent) (ComputeNode, bool) {
	if c == nil || c.StackPlacement == nil {
		return "", false
	}
	node, exists := c.StackPlacement[comp]
	return node, exists
}

// GetComponentsForNode returns all stack components assigned to a given compute node.
func (c *TopologyConfig) GetComponentsForNode(node ComputeNode) []StackComponent {
	if c == nil || c.StackPlacement == nil {
		return nil
	}
	res := make([]StackComponent, 0)
	for comp, n := range c.StackPlacement {
		if n == node {
			res = append(res, comp)
		}
	}
	// Sort for deterministic results
	sort.Slice(res, func(i, j int) bool {
		return res[i] < res[j]
	})
	return res
}

// ActiveComposeProfiles computes the required Docker Compose profiles for a specific node.
func (c *TopologyConfig) ActiveComposeProfiles(node ComputeNode) []string {
	comps := c.GetComponentsForNode(node)
	profileSet := make(map[string]struct{})

	for _, comp := range comps {
		switch comp {
		case ComponentControlPlane, ComponentDatabase, ComponentDocsPortal:
			profileSet["control-plane"] = struct{}{}
		case ComponentLinuxRE:
			profileSet["rea-worker"] = struct{}{}
		case ComponentRepoIntel:
			profileSet["repo-intel"] = struct{}{}
		case ComponentInference:
			profileSet["inference"] = struct{}{}
		}
	}

	profiles := make([]string, 0, len(profileSet))
	for p := range profileSet {
		profiles = append(profiles, p)
	}
	sort.Strings(profiles)
	return profiles
}

// RenderEnv generates standard environment variable key-value pairs representing the topology.
func (c *TopologyConfig) RenderEnv() map[string]string {
	if c == nil {
		return nil
	}

	ctrlIP := c.TailscaleEndpoints.ControlPlane
	if ctrlIP == "" {
		ctrlIP = "127.0.0.1"
	}
	execIP := c.TailscaleEndpoints.ExecutionPlane
	if execIP == "" {
		execIP = "127.0.0.1"
	}

	env := map[string]string{
		"DENDRITE_SCENARIO":          string(c.Scenario),
		"TAILSCALE_CONTROL_PLANE_IP": ctrlIP,
		"TAILSCALE_EXECUTION_PLANE":  execIP,
		"LITELLM_HOST":               fmt.Sprintf("http://%s:4000", ctrlIP),
		"OLLAMA_HOST":                fmt.Sprintf("http://%s:11434", execIP),
		"DOCUSAURUS_HOST":            fmt.Sprintf("http://%s:8080", ctrlIP),
	}

	if c.TailscaleEndpoints.AzureBurst != "" {
		env["TAILSCALE_AZURE_BURST_IP"] = c.TailscaleEndpoints.AzureBurst
	}
	if c.TailscaleEndpoints.AWSBurst != "" {
		env["TAILSCALE_AWS_BURST_IP"] = c.TailscaleEndpoints.AWSBurst
	}

	return env
}

// ToJSON serializes the configuration into formatted JSON bytes.
func (c *TopologyConfig) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

func decodeTopologyConfig(data []byte, unmarshal func([]byte, any) error, formatName string) (*TopologyConfig, error) {
	var cfg TopologyConfig
	if err := unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s topology config: %w", formatName, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// FromJSON deserializes JSON bytes into a validated TopologyConfig.
func FromJSON(data []byte) (*TopologyConfig, error) {
	return decodeTopologyConfig(data, json.Unmarshal, "JSON")
}

// ToYAML serializes the configuration into YAML format bytes.
func (c *TopologyConfig) ToYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

// FromYAML deserializes YAML bytes into a validated TopologyConfig.
func FromYAML(data []byte) (*TopologyConfig, error) {
	return decodeTopologyConfig(data, yaml.Unmarshal, "YAML")
}
