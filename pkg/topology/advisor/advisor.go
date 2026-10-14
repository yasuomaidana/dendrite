package advisor

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/topology"
)

// PlacementAdvisor calculates optimal service-to-node routing based on system metrics.
type PlacementAdvisor struct{}

// NewPlacementAdvisor creates an initialized PlacementAdvisor instance.
func NewPlacementAdvisor() *PlacementAdvisor {
	return &PlacementAdvisor{}
}

// Advise computes a PlacementPlan for the given nodes and service requirements.
func (a *PlacementAdvisor) Advise(nodes []*NodeHardware, services []ServiceRequirement) (*PlacementPlan, error) {
	if len(nodes) == 0 {
		return nil, ErrEmptyNodeList
	}
	if len(services) == 0 {
		return nil, ErrEmptyServiceList
	}

	plan := &PlacementPlan{
		Timestamp:       time.Now(),
		Placements:      make([]ServicePlacement, 0, len(services)),
		NodeAllocations: make(map[string][]string),
		Warnings:        make([]string, 0),
	}

	for _, n := range nodes {
		plan.NodeAllocations[n.NodeID] = make([]string, 0)
	}

	// Track dynamic allocated resources per node during placement
	allocatedRAM := make(map[string]uint64)
	serviceCounts := make(map[string]int)

	for _, svc := range services {
		placement, err := a.selectBestNode(svc, nodes, allocatedRAM, serviceCounts)
		if err != nil {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("Service '%s': %v", svc.Name, err))
			continue
		}

		plan.Placements = append(plan.Placements, placement)
		plan.NodeAllocations[placement.AssignedNodeID] = append(plan.NodeAllocations[placement.AssignedNodeID], svc.Name)
		allocatedRAM[placement.AssignedNodeID] += svc.MinRAMMB
		serviceCounts[placement.AssignedNodeID]++
	}

	// Synthesize TopologyConfig
	plan.TopologyConfig = a.buildTopologyConfig(plan, nodes)

	return plan, nil
}

func (a *PlacementAdvisor) selectBestNode(
	svc ServiceRequirement,
	nodes []*NodeHardware,
	allocatedRAM map[string]uint64,
	counts map[string]int,
) (ServicePlacement, error) {
	var bestNode *NodeHardware
	bestScore := -1.0
	bestRationale := ""

	for _, node := range nodes {
		score, rationale, eligible := a.scoreNodeForService(svc, node, allocatedRAM[node.NodeID], counts[node.NodeID])
		if !eligible {
			continue
		}
		if score > bestScore {
			bestScore = score
			bestNode = node
			bestRationale = rationale
		}
	}

	if bestNode == nil {
		return ServicePlacement{}, fmt.Errorf("%w for '%s'", ErrInsufficientCapacity, svc.Name)
	}

	return ServicePlacement{
		Service:        svc.Name,
		Component:      svc.Component,
		AssignedNodeID: bestNode.NodeID,
		Score:          math.Round(bestScore*10) / 10,
		Rationale:      bestRationale,
	}, nil
}

func (a *PlacementAdvisor) scoreNodeForService(
	svc ServiceRequirement,
	node *NodeHardware,
	alreadyAllocatedRAM uint64,
	assignedCount int,
) (score float64, rationale string, eligible bool) {
	// Hard eligibility constraints
	if svc.RequiresGPU && !node.HasGPU {
		return 0, "disqualified: missing required GPU acceleration", false
	}
	if svc.RequiredOS != "" && !strings.EqualFold(node.OS, svc.RequiredOS) {
		return 0, fmt.Sprintf("disqualified: requires OS '%s', host is '%s'", svc.RequiredOS, node.OS), false
	}
	if node.TotalRAMMB < svc.MinRAMMB {
		return 0, fmt.Sprintf("disqualified: total RAM (%d MB) below minimum (%d MB)", node.TotalRAMMB, svc.MinRAMMB), false
	}
	if node.DiskFreeGB < svc.MinDiskGB {
		return 0, fmt.Sprintf("disqualified: free disk space (%d GB) below required (%d GB)", node.DiskFreeGB, svc.MinDiskGB), false
	}

	var reasons []string

	// 1. RAM Headroom (max 30 pts)
	availableRAM := node.FreeRAMMB
	if availableRAM > alreadyAllocatedRAM {
		availableRAM -= alreadyAllocatedRAM
	} else {
		availableRAM = 100
	}
	ramRatio := float64(availableRAM) / float64(svc.MinRAMMB)
	ramPts := math.Min(ramRatio*10.0, 30.0)
	reasons = append(reasons, fmt.Sprintf("RAM headroom (+%.1f pts)", ramPts))

	// 2. CPU Capacity (max 20 pts)
	coreRatio := float64(node.CPUCores) / float64(svc.MinCores)
	cpuPts := math.Min(coreRatio*5.0, 20.0)
	reasons = append(reasons, fmt.Sprintf("CPU cores (%d, +%.1f pts)", node.CPUCores, cpuPts))

	// 3. GPU match (max 25 pts)
	gpuPts := 0.0
	if svc.RequiresGPU {
		if svc.PreferredGPU != "" && node.GPUType == svc.PreferredGPU {
			gpuPts = 25.0
			reasons = append(reasons, fmt.Sprintf("Preferred %s GPU match (+25 pts)", node.GPUType))
		} else if node.HasGPU {
			gpuPts = 20.0
			reasons = append(reasons, fmt.Sprintf("Available %s GPU (+20 pts)", node.GPUType))
		}
	} else {
		if !node.HasGPU {
			gpuPts = 10.0
			reasons = append(reasons, "Preserves GPU nodes for AI inference (+10 pts)")
		} else {
			gpuPts = 5.0
		}
	}

	// 4. Locality & Affinity (max 15 pts)
	locPts := 0.0
	if svc.Interactive {
		if node.IsLocal || node.LatencyMs < 5.0 {
			locPts = 15.0
			reasons = append(reasons, "Ultra-low latency interactive locality (+15 pts)")
		} else {
			locPts = math.Max(15.0-(node.LatencyMs/10.0), 2.0)
			reasons = append(reasons, fmt.Sprintf("Remote latency %.1fms (+%.1f pts)", node.LatencyMs, locPts))
		}
	} else if svc.PersistentState {
		if !node.IsLocal || strings.EqualFold(node.OS, "linux") {
			locPts = 15.0
			reasons = append(reasons, "Persistent server node suitability (+15 pts)")
		} else {
			locPts = 5.0
			reasons = append(reasons, "Desktop host persistent state (+5 pts)")
		}
	} else {
		locPts = 10.0
	}

	// 5. Workload Balance Penalty (subtract up to 15 pts)
	penalty := float64(assignedCount) * 3.0
	if penalty > 0 {
		reasons = append(reasons, fmt.Sprintf("Workload load penalty (-%.0f pts)", penalty))
	}

	totalScore := math.Max(ramPts+cpuPts+gpuPts+locPts-penalty, 1.0)
	return totalScore, strings.Join(reasons, "; "), true
}

func (a *PlacementAdvisor) buildTopologyConfig(plan *PlacementPlan, nodes []*NodeHardware) *topology.TopologyConfig {
	placementMap := make(topology.StackPlacement)
	endpoints := topology.TailscaleEndpoints{}

	nodeLookup := make(map[string]*NodeHardware)
	for _, n := range nodes {
		nodeLookup[n.NodeID] = n
	}

	for _, p := range plan.Placements {
		cNode := toComputeNode(p.AssignedNodeID, nodeLookup[p.AssignedNodeID])
		placementMap[p.Component] = cNode
	}

	// Assign endpoints based on node types
	for _, n := range nodes {
		cNode := toComputeNode(n.NodeID, n)
		host := n.Hostname
		if host == "" {
			host = n.NodeID
		}
		switch cNode {
		case topology.NodeMacBook:
			endpoints.ExecutionPlane = host
		case topology.NodeDebian:
			endpoints.ControlPlane = host
		case topology.NodeAzure:
			endpoints.AzureBurst = host
		case topology.NodeAWS:
			endpoints.AWSBurst = host
		}
	}

	scenario := detectScenario(placementMap)
	cfg := &topology.TopologyConfig{
		Scenario:           scenario,
		TailscaleEndpoints: endpoints,
		StackPlacement:     placementMap,
		Metadata: map[string]string{
			"advisor_generated": "true",
			"timestamp":         plan.Timestamp.Format(time.RFC3339),
		},
	}

	return cfg
}

func toComputeNode(nodeID string, hw *NodeHardware) topology.ComputeNode {
	lower := strings.ToLower(nodeID)
	if hw != nil {
		lower += " " + strings.ToLower(hw.Hostname)
	}

	if strings.Contains(lower, "mac") || strings.Contains(lower, "darwin") {
		return topology.NodeMacBook
	}
	if strings.Contains(lower, "deb") || strings.Contains(lower, "server") || strings.Contains(lower, "linux") {
		return topology.NodeDebian
	}
	if strings.Contains(lower, "azure") {
		return topology.NodeAzure
	}
	if strings.Contains(lower, "aws") || strings.Contains(lower, "ec2") {
		return topology.NodeAWS
	}
	return topology.ComputeNode(strings.ToLower(nodeID))
}

func detectScenario(placement topology.StackPlacement) topology.Scenario {
	nodes := make(map[topology.ComputeNode]bool)
	for _, n := range placement {
		nodes[n] = true
	}

	if nodes[topology.NodeAzure] && (nodes[topology.NodeAWS] || nodes[topology.NodeDebian]) {
		return topology.ScenarioDistributed
	}
	if nodes[topology.NodeAzure] && !nodes[topology.NodeDebian] {
		return topology.ScenarioAzureBurst
	}
	if nodes[topology.NodeAWS] && !nodes[topology.NodeDebian] {
		return topology.ScenarioAWSBurst
	}
	return topology.ScenarioLocalHybrid
}

// FormatTable returns a formatted text overview of the placement plan.
func (p *PlacementPlan) FormatTable() string {
	var sb strings.Builder
	sb.WriteString("========================================================================================\n")
	sb.WriteString("                      DENDRITE TOPOLOGY SERVICE PLACEMENT PLAN                          \n")
	sb.WriteString("========================================================================================\n\n")

	sb.WriteString(fmt.Sprintf("Recommended Scenario: %s\n\n", p.TopologyConfig.Scenario))

	sb.WriteString(fmt.Sprintf("%-35s %-16s %-12s %-7s %s\n", "SERVICE", "COMPONENT", "TARGET NODE", "SCORE", "RATIONALE"))
	sb.WriteString(strings.Repeat("-", 100) + "\n")

	for _, item := range p.Placements {
		sb.WriteString(fmt.Sprintf("%-35s %-16s %-12s %-7.1f %s\n",
			truncate(item.Service, 34),
			item.Component,
			item.AssignedNodeID,
			item.Score,
			truncate(item.Rationale, 38),
		))
	}
	sb.WriteString("\nNode Allocations:\n")
	keys := make([]string, 0, len(p.NodeAllocations))
	for k := range p.NodeAllocations {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		svcs := p.NodeAllocations[k]
		if len(svcs) == 0 {
			sb.WriteString(fmt.Sprintf("  - %-12s : (idle / no services assigned)\n", k))
		} else {
			sb.WriteString(fmt.Sprintf("  - %-12s : %s\n", k, strings.Join(svcs, ", ")))
		}
	}

	if len(p.Warnings) > 0 {
		sb.WriteString("\nWarnings & Resource Constraints:\n")
		for _, w := range p.Warnings {
			sb.WriteString(fmt.Sprintf("  [!] %s\n", w))
		}
	}

	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
