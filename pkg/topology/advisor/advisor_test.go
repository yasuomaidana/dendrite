package advisor

import (
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/topology"
)

func TestPlacementAdvisor_LocalHybridScenario(t *testing.T) {
	macbook := &NodeHardware{
		NodeID:     "macbook",
		Hostname:   "macbook-air.ts.net",
		OS:         "darwin",
		Arch:       "arm64",
		CPUCores:   10,
		TotalRAMMB: 24576,
		FreeRAMMB:  16384,
		DiskFreeGB: 200,
		HasGPU:     true,
		GPUType:    GPUTypeMetal,
		VRAMMB:     18432,
		LatencyMs:  0.2,
		IsLocal:    true,
	}

	debian := &NodeHardware{
		NodeID:     "debian",
		Hostname:   "debian-server.ts.net",
		OS:         "linux",
		Arch:       "amd64",
		CPUCores:   8,
		TotalRAMMB: 32768,
		FreeRAMMB:  24576,
		DiskFreeGB: 500,
		HasGPU:     false,
		GPUType:    GPUTypeNone,
		LatencyMs:  1.5,
		IsLocal:    false,
	}

	nodes := []*NodeHardware{macbook, debian}
	services := DefaultDendriteServices()

	advisor := NewPlacementAdvisor()
	plan, err := advisor.Advise(nodes, services)
	if err != nil {
		t.Fatalf("Advise returned unexpected error: %v", err)
	}

	if len(plan.Placements) != len(services) {
		t.Errorf("expected %d placements, got %d", len(services), len(plan.Placements))
	}

	// Verify Ollama assigned to MacBook (Metal GPU)
	var ollamaPlacement *ServicePlacement
	var dbPlacement *ServicePlacement
	var linuxREPlacement *ServicePlacement

	for i := range plan.Placements {
		p := &plan.Placements[i]
		switch p.Component {
		case topology.ComponentInference:
			ollamaPlacement = p
		case topology.ComponentDatabase:
			dbPlacement = p
		case topology.ComponentLinuxRE:
			linuxREPlacement = p
		}
	}

	if ollamaPlacement == nil || ollamaPlacement.AssignedNodeID != "macbook" {
		t.Errorf("expected Ollama inference assigned to 'macbook', got %+v", ollamaPlacement)
	}
	if dbPlacement == nil || dbPlacement.AssignedNodeID != "debian" {
		t.Errorf("expected Database assigned to 'debian', got %+v", dbPlacement)
	}
	if linuxREPlacement == nil || linuxREPlacement.AssignedNodeID != "debian" {
		t.Errorf("expected Linux RE assigned to 'debian', got %+v", linuxREPlacement)
	}

	// Verify generated TopologyConfig
	if plan.TopologyConfig == nil {
		t.Fatalf("expected non-nil TopologyConfig")
	}
	if plan.TopologyConfig.Scenario != topology.ScenarioLocalHybrid {
		t.Errorf("expected scenario 'local-hybrid', got '%s'", plan.TopologyConfig.Scenario)
	}

	// Verify Table Formatting
	table := plan.FormatTable()
	if !strings.Contains(table, "DENDRITE TOPOLOGY SERVICE PLACEMENT PLAN") {
		t.Errorf("table output missing header: %s", table)
	}
	if !strings.Contains(table, "local-hybrid") {
		t.Errorf("table output missing scenario: %s", table)
	}
}

func TestPlacementAdvisor_AzureBurstScenario(t *testing.T) {
	macbook := &NodeHardware{
		NodeID:     "macbook",
		Hostname:   "macbook.ts.net",
		OS:         "darwin",
		Arch:       "arm64",
		CPUCores:   8,
		TotalRAMMB: 16384,
		FreeRAMMB:  8192,
		DiskFreeGB: 50,
		HasGPU:     true,
		GPUType:    GPUTypeMetal,
		VRAMMB:     12288,
		IsLocal:    true,
	}

	azure := &NodeHardware{
		NodeID:     "azure-node",
		Hostname:   "azure-node.ts.net",
		OS:         "linux",
		Arch:       "amd64",
		CPUCores:   16,
		TotalRAMMB: 65536,
		FreeRAMMB:  50000,
		DiskFreeGB: 500,
		HasGPU:     false,
		GPUType:    GPUTypeNone,
		LatencyMs:  25.0,
	}

	advisor := NewPlacementAdvisor()
	plan, err := advisor.Advise([]*NodeHardware{macbook, azure}, DefaultDendriteServices())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.TopologyConfig.Scenario != topology.ScenarioAzureBurst {
		t.Errorf("expected scenario 'azure-burst', got '%s'", plan.TopologyConfig.Scenario)
	}
}

func TestPlacementAdvisor_AWSBurstAndDistributed(t *testing.T) {
	macbook := &NodeHardware{
		NodeID:     "macbook",
		OS:         "darwin",
		Arch:       "arm64",
		CPUCores:   8,
		TotalRAMMB: 16384,
		FreeRAMMB:  8192,
		DiskFreeGB: 50,
		HasGPU:     true,
		GPUType:    GPUTypeMetal,
		IsLocal:    true,
	}

	awsNode := &NodeHardware{
		NodeID:     "aws-burst",
		Hostname:   "aws-node.ts.net",
		OS:         "linux",
		Arch:       "amd64",
		CPUCores:   16,
		TotalRAMMB: 65536,
		FreeRAMMB:  50000,
		DiskFreeGB: 500,
		LatencyMs:  30.0,
	}

	advisor := NewPlacementAdvisor()
	planAWS, err := advisor.Advise([]*NodeHardware{macbook, awsNode}, DefaultDendriteServices())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if planAWS.TopologyConfig.Scenario != topology.ScenarioAWSBurst {
		t.Errorf("expected scenario 'aws-burst', got '%s'", planAWS.TopologyConfig.Scenario)
	}

	// Distributed Scenario: debian + macbook + azure
	debian := &NodeHardware{
		NodeID:     "debian-server",
		OS:         "linux",
		Arch:       "amd64",
		CPUCores:   8,
		TotalRAMMB: 32768,
		FreeRAMMB:  24576,
		DiskFreeGB: 300,
	}
	azureNode := &NodeHardware{
		NodeID:     "azure-gpu",
		OS:         "linux",
		Arch:       "amd64",
		CPUCores:   16,
		TotalRAMMB: 65536,
		FreeRAMMB:  50000,
		DiskFreeGB: 500,
	}

	planDist, err := advisor.Advise([]*NodeHardware{macbook, debian, azureNode}, DefaultDendriteServices())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if planDist.TopologyConfig.Scenario != topology.ScenarioDistributed {
		t.Errorf("expected scenario 'distributed', got '%s'", planDist.TopologyConfig.Scenario)
	}
}

func TestPlacementAdvisor_EmptyInputs(t *testing.T) {
	advisor := NewPlacementAdvisor()

	_, err := advisor.Advise(nil, DefaultDendriteServices())
	if !errors.Is(err, ErrEmptyNodeList) {
		t.Errorf("expected ErrEmptyNodeList, got %v", err)
	}

	_, err = advisor.Advise([]*NodeHardware{{NodeID: "n1"}}, nil)
	if !errors.Is(err, ErrEmptyServiceList) {
		t.Errorf("expected ErrEmptyServiceList, got %v", err)
	}
}

func TestPlacementAdvisor_UnmetResourceWarning(t *testing.T) {
	weakNode := &NodeHardware{
		NodeID:     "weak-pi",
		OS:         "linux",
		Arch:       "arm64",
		CPUCores:   1,
		TotalRAMMB: 512,
		FreeRAMMB:  256,
		DiskFreeGB: 2,
		HasGPU:     false,
	}

	services := []ServiceRequirement{
		{
			Name:        "Giant LLM Server",
			Component:   topology.ComponentInference,
			MinRAMMB:    131072, // 128 GB
			MinCores:    32,
			MinDiskGB:   1000,
			RequiresGPU: true,
		},
		{
			Name:       "Linux Only App",
			Component:  topology.ComponentLinuxRE,
			MinRAMMB:   256,
			MinCores:   1,
			MinDiskGB:  1,
			RequiredOS: "windows",
		},
	}

	advisor := NewPlacementAdvisor()
	plan, err := advisor.Advise([]*NodeHardware{weakNode}, services)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Warnings) != 2 {
		t.Errorf("expected 2 warnings due to unmet resources, got %d", len(plan.Warnings))
	}
	if len(plan.Placements) != 0 {
		t.Errorf("expected 0 placements for unmet service, got %d", len(plan.Placements))
	}

	table := plan.FormatTable()
	if !strings.Contains(table, "Warnings & Resource Constraints:") {
		t.Errorf("expected warnings in table output, got: %s", table)
	}
	if !strings.Contains(table, "(idle / no services assigned)") {
		t.Errorf("expected idle notice in table output, got: %s", table)
	}
}

func TestPlacementAdvisor_ScoringVariations(t *testing.T) {
	nodeCUDA := &NodeHardware{
		NodeID:     "gpu-box",
		Hostname:   "cuda.local",
		OS:         "linux",
		Arch:       "amd64",
		CPUCores:   16,
		TotalRAMMB: 65536,
		FreeRAMMB:  60000,
		DiskFreeGB: 500,
		HasGPU:     true,
		GPUType:    GPUTypeCUDA,
		VRAMMB:     24576,
		LatencyMs:  12.0,
	}

	svc := ServiceRequirement{
		Name:         "CUDA Inference",
		Component:    topology.ComponentInference,
		MinRAMMB:     16384,
		MinCores:     4,
		MinDiskGB:    20,
		RequiresGPU:  true,
		PreferredGPU: GPUTypeCUDA,
	}

	advisor := NewPlacementAdvisor()
	plan, err := advisor.Advise([]*NodeHardware{nodeCUDA}, []ServiceRequirement{svc})
	if err != nil {
		t.Fatalf("Advise error: %v", err)
	}
	if len(plan.Placements) != 1 {
		t.Fatalf("expected 1 placement, got %d", len(plan.Placements))
	}
	if plan.Placements[0].Score < 50.0 {
		t.Errorf("expected high score for CUDA match, got %.1f", plan.Placements[0].Score)
	}
}
