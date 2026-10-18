package main

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/topology"
	"Dendrite/pkg/topology/advisor"
	"encoding/json"
)

func handleTopologyCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "docusaurus", "site":
		handleDocusaurusCommand(ctx, args)
	case "advisor", "advise":
		runTopologyAdvisor(ctx, args)
	case "inspect", "show":
		scenarioStr := "local-hybrid"
		if len(args) > 0 {
			scenarioStr = args[0]
		}
		runTopologyInspect(ctx, scenarioStr)
	default:
		if _, err := topology.ParseScenario(action); err == nil {
			runTopologyInspect(ctx, action)
			return
		}
		fmt.Printf("[-] Unknown topology action: %s\n", action)
		fmt.Println("Usage: dendrite topology <advisor|inspect> [args...]")
	}
}

func runTopologyInspect(_ context.Context, scenarioStr string) {
	sc, err := topology.ParseScenario(scenarioStr)
	if err != nil {
		fmt.Printf("[-] Invalid scenario: %v\n", err)
		return
	}
	cfg, err := topology.NewTopologyConfig(sc, topology.TailscaleEndpoints{
		ControlPlane:   "100.84.12.10",
		ExecutionPlane: "100.84.12.25",
	})
	if err != nil {
		fmt.Printf("[-] Failed to construct topology: %v\n", err)
		return
	}
	fmt.Printf("[+] Active Scenario: %s\n", cfg.Scenario)
	fmt.Println("[+] Stack Placements:")
	for comp, node := range cfg.StackPlacement {
		fmt.Printf("    - %-15s -> %s\n", comp, node)
	}
	fmt.Println("[+] Node Compose Profiles:")
	for _, node := range topology.AllComputeNodes() {
		profiles := cfg.ActiveComposeProfiles(node)
		if len(profiles) > 0 {
			fmt.Printf("    - %-10s: %s\n", node, strings.Join(profiles, ", "))
		}
	}
}

func runTopologyAdvisor(ctx context.Context, args []string) {
	nodeArgs := "local=localhost"
	outputJSON := false

	for i := 0; i < len(args); i++ {
		if args[i] == "--nodes" && i+1 < len(args) {
			nodeArgs = args[i+1]
			i++
		} else if args[i] == "--json" {
			outputJSON = true
		}
	}

	targets := parseNodeTargets(nodeArgs)
	fmt.Printf("[+] Probing %d compute node(s) for hardware metrics...\n", len(targets))
	probe := advisor.NewHardwareProbe(nil)
	hardwareList, err := probe.ProbeAll(ctx, targets)
	if err != nil {
		fmt.Printf("[-] Failed to probe nodes: %v\n", err)
		return
	}

	fmt.Printf("[✓] Discovered %d compute node(s):\n", len(hardwareList))
	for _, hw := range hardwareList {
		gpuInfo := "None"
		if hw.HasGPU {
			gpuInfo = fmt.Sprintf("%s (%d MB VRAM)", hw.GPUType, hw.VRAMMB)
		}
		fmt.Printf("    - %-12s: %s/%s, %d CPU cores, %d MB RAM (%d MB free), %d GB disk, GPU: %s\n",
			hw.NodeID, hw.OS, hw.Arch, hw.CPUCores, hw.TotalRAMMB, hw.FreeRAMMB, hw.DiskFreeGB, gpuInfo)
	}

	adv := advisor.NewPlacementAdvisor()
	plan, err := adv.Advise(hardwareList, advisor.DefaultDendriteServices())
	if err != nil {
		fmt.Printf("[-] Placement advisor failed: %v\n", err)
		return
	}

	if outputJSON {
		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			fmt.Printf("[-] Failed to serialize JSON: %v\n", err)
			return
		}
		fmt.Println(string(data))
	} else {
		fmt.Print(plan.FormatTable())
	}
}

func parseNodeTargets(nodeArgs string) []advisor.NodeTarget {
	var targets []advisor.NodeTarget
	tokens := strings.Split(nodeArgs, ",")
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		var id, host string
		if strings.Contains(tok, "=") {
			parts := strings.SplitN(tok, "=", 2)
			id = strings.TrimSpace(parts[0])
			host = strings.TrimSpace(parts[1])
		} else {
			id = tok
			host = tok
		}
		isLocal := (host == "localhost" || host == "127.0.0.1" || host == "local")
		targets = append(targets, advisor.NodeTarget{
			ID:      id,
			Host:    host,
			IsLocal: isLocal,
		})
	}
	if len(targets) == 0 {
		targets = append(targets, advisor.NodeTarget{
			ID:      "local",
			Host:    "localhost",
			IsLocal: true,
		})
	}
	return targets
}
