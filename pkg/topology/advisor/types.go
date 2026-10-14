package advisor

import (
	"errors"
	"time"

	"Dendrite/pkg/topology"
)

var (
	ErrUnreachableNode      = errors.New("node is unreachable")
	ErrCommandExecution     = errors.New("command execution failed")
	ErrInsufficientCapacity = errors.New("no node meets minimum resource criteria")
	ErrEmptyNodeList        = errors.New("no nodes provided for evaluation")
	ErrEmptyServiceList     = errors.New("no services provided for placement")
	ErrInvalidTarget        = errors.New("invalid node target specification")
)

// GPUType classifies supported GPU acceleration engines.
type GPUType string

const (
	GPUTypeNone  GPUType = "none"
	GPUTypeMetal GPUType = "metal"
	GPUTypeCUDA  GPUType = "cuda"
	GPUTypeROCm  GPUType = "rocm"
)

// NodeTarget defines connection parameters for an infrastructure node.
type NodeTarget struct {
	ID      string        `json:"id" yaml:"id"`
	Host    string        `json:"host" yaml:"host"`
	Port    int           `json:"port" yaml:"port"`
	User    string        `json:"user" yaml:"user"`
	KeyPath string        `json:"key_path,omitempty" yaml:"key_path,omitempty"`
	IsLocal bool          `json:"is_local" yaml:"is_local"`
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
}

// NodeHardware captures audited hardware metrics of a compute node.
type NodeHardware struct {
	NodeID     string            `json:"node_id"`
	Hostname   string            `json:"hostname"`
	OS         string            `json:"os"`   // "darwin", "linux"
	Arch       string            `json:"arch"` // "arm64", "amd64"
	CPUCores   int               `json:"cpu_cores"`
	TotalRAMMB uint64            `json:"total_ram_mb"`
	FreeRAMMB  uint64            `json:"free_ram_mb"`
	DiskFreeGB uint64            `json:"disk_free_gb"`
	HasGPU     bool              `json:"has_gpu"`
	GPUType    GPUType           `json:"gpu_type"`
	VRAMMB     uint64            `json:"vram_mb"`
	LatencyMs  float64           `json:"latency_ms"`
	IsLocal    bool              `json:"is_local"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// ServiceRequirement specifies the hardware prerequisites and affinity for a service.
type ServiceRequirement struct {
	Name            string                  `json:"name"`
	Component       topology.StackComponent `json:"component"`
	MinRAMMB        uint64                  `json:"min_ram_mb"`
	MinCores        int                     `json:"min_cores"`
	MinDiskGB       uint64                  `json:"min_disk_gb"`
	RequiresGPU     bool                    `json:"requires_gpu"`
	PreferredGPU    GPUType                 `json:"preferred_gpu,omitempty"`
	RequiredOS      string                  `json:"required_os,omitempty"` // "darwin", "linux", or ""
	PersistentState bool                    `json:"persistent_state"`
	Interactive     bool                    `json:"interactive"`
}

// ServicePlacement represents an assigned mapping from service to node with score justification.
type ServicePlacement struct {
	Service        string                  `json:"service"`
	Component      topology.StackComponent `json:"component"`
	AssignedNodeID string                  `json:"assigned_node_id"`
	Score          float64                 `json:"score"`
	Rationale      string                  `json:"rationale"`
}

// PlacementPlan is the synthesized result of the PlacementAdvisor router.
type PlacementPlan struct {
	Timestamp       time.Time                `json:"timestamp"`
	Placements      []ServicePlacement       `json:"placements"`
	NodeAllocations map[string][]string      `json:"node_allocations"`
	TopologyConfig  *topology.TopologyConfig `json:"topology_config,omitempty"`
	Warnings        []string                 `json:"warnings,omitempty"`
}

// DefaultDendriteServices returns standard service requirement profiles.
func DefaultDendriteServices() []ServiceRequirement {
	return []ServiceRequirement{
		{
			Name:         "Ollama Engine (Local Inference)",
			Component:    topology.ComponentInference,
			MinRAMMB:     8192,
			MinCores:     4,
			MinDiskGB:    20,
			RequiresGPU:  true,
			PreferredGPU: GPUTypeMetal,
			Interactive:  true,
		},
		{
			Name:            "LiteLLM Proxy Gateway",
			Component:       topology.ComponentControlPlane,
			MinRAMMB:        512,
			MinCores:        1,
			MinDiskGB:       5,
			PersistentState: true,
		},
		{
			Name:            "PostgreSQL 16 & Redis 7 Storage",
			Component:       topology.ComponentDatabase,
			MinRAMMB:        2048,
			MinCores:        2,
			MinDiskGB:       30,
			PersistentState: true,
		},
		{
			Name:        "Headless Ghidra & Rizin Linux RE",
			Component:   topology.ComponentLinuxRE,
			MinRAMMB:    4096,
			MinCores:    4,
			MinDiskGB:   15,
			RequiredOS:  "linux",
			Interactive: false,
		},
		{
			Name:        "Repo-Intel Sandbox (ast-grep / Tree-sitter)",
			Component:   topology.ComponentRepoIntel,
			MinRAMMB:    2048,
			MinCores:    2,
			MinDiskGB:   10,
			Interactive: false,
		},
		{
			Name:            "Docusaurus Documentation Portal",
			Component:       topology.ComponentDocsPortal,
			MinRAMMB:        512,
			MinCores:        1,
			MinDiskGB:       5,
			PersistentState: true,
		},
	}
}
