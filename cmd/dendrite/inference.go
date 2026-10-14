package main

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/gateway"
	"Dendrite/pkg/inference"
)

func handleInferenceCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		printInferenceUsage()
		return
	}

	action := strings.ToLower(args[0])
	mgr := inference.DefaultManager()

	switch action {
	case "list":
		engines := mgr.ListEngines()
		cascade := mgr.GetCascadeOrder()
		fmt.Printf("[+] Registered Multi-Provider Inference Engines (%d):\n", len(engines))
		for _, eng := range engines {
			cfg := eng.Config()
			zeroCostTag := ""
			if eng.IsSelfHosted() {
				zeroCostTag = " [Zero-Cost / Self-Hosted]"
			}
			modelsStr := strings.Join(cfg.Models, ", ")
			if modelsStr == "" {
				modelsStr = "(auto-discover)"
			}
			fmt.Printf("    [%s] %s (%s)%s\n", eng.Type(), eng.ID(), cfg.Endpoint, zeroCostTag)
			fmt.Printf("        Models: %s\n", modelsStr)
		}
		fmt.Printf("\n[+] Dynamic Cascade Failover Priority Order:\n")
		for i, id := range cascade {
			fmt.Printf("    %d. %s\n", i+1, id)
		}

	case "probe":
		targetID := ""
		if len(args) > 1 {
			targetID = args[1]
		}

		if targetID != "" {
			eng, err := mgr.GetEngine(targetID)
			if err != nil {
				fmt.Printf("[-] Unknown inference engine: %s\n", targetID)
				return
			}
			fmt.Printf("[+] Probing engine '%s' (%s at %s)...\n", eng.ID(), eng.Type(), eng.Config().Endpoint)
			h := eng.Health(ctx)
			printEngineHealth(h)
			return
		}

		fmt.Println("[+] Probing all registered inference engines (< 2s concurrent health check)...")
		results := mgr.PollHealthAll(ctx)
		for _, h := range results {
			printEngineHealth(h)
		}

	case "switch":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite dev inference switch <engine_id>")
			return
		}
		primaryID := args[1]
		if _, err := mgr.GetEngine(primaryID); err != nil {
			fmt.Printf("[-] Unknown engine '%s': %v\n", primaryID, err)
			return
		}
		// Move primaryID to the front of the cascade
		current := mgr.GetCascadeOrder()
		newOrder := []string{primaryID}
		for _, id := range current {
			if id != primaryID {
				newOrder = append(newOrder, id)
			}
		}
		_ = mgr.SetCascadeOrder(newOrder)
		fmt.Printf("[✓] Successfully updated cascade order. Primary inference engine is now: %s\n", primaryID)
		fmt.Printf("    New cascade: %s\n", strings.Join(newOrder, " -> "))

	case "render":
		yamlBytes, err := gateway.RenderForInferenceManager(mgr)
		if err != nil {
			fmt.Printf("[-] Failed to render LiteLLM configuration: %v\n", err)
			return
		}
		fmt.Printf("[+] Rendered LiteLLM Configuration for Multi-Engine Inference:\n\n%s\n", string(yamlBytes))

	default:
		printInferenceUsage()
	}
}

func printEngineHealth(h inference.EngineHealth) {
	status := "Offline"
	if h.Reachable {
		status = "Online"
	}
	costInfo := "commercial"
	if h.ZeroCost {
		costInfo = "zero-cost"
	}

	fmt.Printf("    * [%s] Engine: %-15s Type: %-18s Status: %-7s Latency: %-8v Cost: %s\n",
		status, h.EngineID, h.Type, status, h.Latency, costInfo)
	if len(h.Models) > 0 {
		fmt.Printf("        Models: %s\n", strings.Join(h.Models, ", "))
	}
	if h.Error != "" {
		fmt.Printf("        Error:  %s\n", h.Error)
	}
}

func printInferenceUsage() {
	fmt.Println("Usage: dendrite dev inference <list|probe|switch|render> [args...]")
	fmt.Println("\nCommands:")
	fmt.Println("  list               List registered engines, models, and fallback cascade order")
	fmt.Println("  probe [engine_id]  Check reachability, latency, and available models (<2s)")
	fmt.Println("  switch <engine_id> Set primary inference engine at front of fallback cascade")
	fmt.Println("  render             Render upstream model routes into litellm_config.yaml")
}
