package orchestrator

import (
	"Dendrite/pkg/stacks"
	"fmt"
	"sort"

	"Dendrite/pkg/topology"
)

// DefaultProfileComposeFileMap maps compose profiles to their canonical file locations.
func DefaultProfileComposeFileMap() map[string]string {
	return map[string]string{
		"control-plane": stacks.Path("compose.control.yaml"),
		"rea-worker":    stacks.Path("compose.rea-linux.yaml"),
		"repo-intel":    stacks.Path("compose.sandbox.yaml"),
		"inference":     stacks.Path("compose.inference.yaml"),
	}
}

// Orchestrator coordinates deployment planning, cloud-init rendering, and Compose execution.
type Orchestrator struct {
	topology        *topology.TopologyConfig
	profileFilesMap map[string]string
}

// NewOrchestrator creates a new Orchestrator bound to a verified TopologyConfig.
func NewOrchestrator(cfg *topology.TopologyConfig) (*Orchestrator, error) {
	if cfg == nil {
		return nil, ErrNilTopologyConfig
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid topology config: %w", err)
	}

	return &Orchestrator{
		topology:        cfg,
		profileFilesMap: DefaultProfileComposeFileMap(),
	}, nil
}

// SetProfileFile overrides the Compose file path for a specific profile.
func (o *Orchestrator) SetProfileFile(profile, filePath string) {
	if o.profileFilesMap == nil {
		o.profileFilesMap = DefaultProfileComposeFileMap()
	}
	o.profileFilesMap[profile] = filePath
}

// BuildNodeComposeCommand resolves the active profiles and required Compose files for a compute node.
func (o *Orchestrator) BuildNodeComposeCommand(node topology.ComputeNode, action string) (*ComposeCommand, error) {
	if o.topology == nil {
		return nil, ErrNilTopologyConfig
	}
	if !node.IsValid() {
		return nil, &OrchestrationError{Node: string(node), Op: "BuildNodeComposeCommand", Err: ErrInvalidNode}
	}

	profiles := o.topology.ActiveComposeProfiles(node)
	cmd := NewComposeCommand(action)
	cmd.Profiles = profiles
	cmd.Environment = o.topology.RenderEnv()

	// Collect unique compose files needed for active profiles
	fileSet := make(map[string]struct{})
	for _, p := range profiles {
		if file, exists := o.profileFilesMap[p]; exists && file != "" {
			fileSet[file] = struct{}{}
		}
	}

	composeFiles := make([]string, 0, len(fileSet))
	for f := range fileSet {
		composeFiles = append(composeFiles, f)
	}
	sort.Strings(composeFiles)
	cmd.ComposeFiles = composeFiles

	return cmd, nil
}

// BuildCloudInitForNode renders a cloud-init script for Linux compute nodes.
func (o *Orchestrator) BuildCloudInitForNode(node topology.ComputeNode, authKey string, sshKeys []string) (string, error) {
	if !node.IsValid() {
		return "", &OrchestrationError{Node: string(node), Op: "BuildCloudInitForNode", Err: ErrInvalidNode}
	}
	if node == topology.NodeMacBook {
		return "", &OrchestrationError{
			Node: string(node),
			Op:   "BuildCloudInitForNode",
			Err:  fmt.Errorf("cloud-init is not applicable to macOS workstation"),
		}
	}

	hostname := fmt.Sprintf("dendrite-%s", node)
	cfg := CloudInitConfig{
		Hostname:           hostname,
		TailscaleAuthKey:   authKey,
		SSHAuthorizedKeys:  sshKeys,
		EnableTailscaleSSH: true,
	}

	return GenerateCloudInit(cfg)
}

// BuildDeploymentPlan generates complete orchestration metadata for a target compute node.
func (o *Orchestrator) BuildDeploymentPlan(node topology.ComputeNode, action, authKey string, sshKeys []string) (*NodeDeploymentPlan, error) {
	composeCmd, err := o.BuildNodeComposeCommand(node, action)
	if err != nil {
		return nil, err
	}

	var cloudInit string
	if node != topology.NodeMacBook {
		cloudInit, err = o.BuildCloudInitForNode(node, authKey, sshKeys)
		if err != nil {
			return nil, err
		}
	}

	return &NodeDeploymentPlan{
		Node:            string(node),
		ComposeCommand:  composeCmd,
		CloudInitScript: cloudInit,
		Environment:     o.topology.RenderEnv(),
	}, nil
}
