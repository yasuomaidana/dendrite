package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"Dendrite/pkg/gateway"
	"Dendrite/pkg/topology"
)

func handleGatewayCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		printGatewayUsage()
		return
	}

	sub := strings.ToLower(args[0])

	switch sub {
	case "update", "sync", "refresh":
		handleGatewayUpdateCommand(ctx, args[1:])

	case "schedule", "daemon", "watch":
		handleGatewayScheduleCommand(ctx, args[1:])

	case "zero-cost", "zerocost", "models":
		handleZeroCostCommand(args[1:])

	case "render":
		scenarioStr := "local-hybrid"
		if len(args) > 1 {
			scenarioStr = args[1]
		}
		renderGatewayScenario(ctx, scenarioStr)

	default:
		// If argument looks like a topology scenario (e.g. local-hybrid, full-local)
		if strings.Contains(sub, "local") || strings.Contains(sub, "hybrid") || strings.Contains(sub, "cloud") {
			renderGatewayScenario(ctx, sub)
			return
		}
		printGatewayUsage()
	}
}

func handleGatewayUpdateCommand(ctx context.Context, args []string) {
	opts := gateway.DefaultUpdaterOptions()
	outputJSON := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			if i+1 < len(args) {
				opts.ConfigPath = args[i+1]
				i++
			}
		case "--dry-run", "-n":
			opts.DryRun = true
		case "--docker", "--container":
			opts.UseContainerHost = true
		case "--no-docker", "--local-host":
			opts.UseContainerHost = false
		case "--json":
			outputJSON = true
		}
	}

	updater := gateway.NewRoutingTableUpdater(opts)
	report, err := updater.UpdateNow(ctx)
	if err != nil {
		if outputJSON {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("[-] Failed to update LiteLLM routing table: %v\n", err)
		}
		return
	}

	if outputJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Printf("[-] Failed to serialize JSON report: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	modeStr := "Updated"
	if report.DryRun {
		modeStr = "Simulated Update (Dry-Run)"
	}

	fmt.Printf("[✓] LiteLLM Routing Table %s Successfully in %v\n", modeStr, report.Duration)
	fmt.Printf("    * Config Path:             %s\n", report.ConfigPath)
	fmt.Printf("    * Engines Probed:          %d (Online: %d, Offline: %d)\n",
		report.EnginesProbed, len(report.OnlineEngines), len(report.OfflineEngines))
	if len(report.OnlineEngines) > 0 {
		fmt.Printf("    * Online Engines:          %s\n", strings.Join(report.OnlineEngines, ", "))
	}
	if len(report.OfflineEngines) > 0 {
		fmt.Printf("    * Offline Engines:         %s\n", strings.Join(report.OfflineEngines, ", "))
	}
	fmt.Printf("    * Discovered Local Models: %d\n", len(report.DiscoveredModels))
	if report.ActiveLocalPrimary != "" {
		fmt.Printf("    * Active Local Primary:    %s (Score: %d on %s)\n",
			report.ActiveLocalPrimary, report.ActiveLocalPrimaryScore, report.ActiveLocalPrimaryEngine)
	}
	fmt.Printf("    * Total Configured Routes: %d\n", report.TotalModelsConfigured)
	if !report.DryRun {
		fmt.Printf("    * Bytes Written:           %d bytes\n", report.BytesWritten)
	}
}

func handleGatewayScheduleCommand(ctx context.Context, args []string) {
	opts := gateway.DefaultUpdaterOptions()
	daemonMode := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--interval", "-i":
			if i+1 < len(args) {
				if d, err := time.ParseDuration(args[i+1]); err == nil && d > 0 {
					opts.Interval = d
				}
				i++
			}
		case "--file", "-f":
			if i+1 < len(args) {
				opts.ConfigPath = args[i+1]
				i++
			}
		case "--docker", "--container":
			opts.UseContainerHost = true
		case "--daemon", "-d":
			daemonMode = true
		}
	}

	updater := gateway.NewRoutingTableUpdater(opts)
	fmt.Printf("[+] Starting LiteLLM Routing Table Scheduler Daemon (Interval: %v)\n", opts.Interval)
	fmt.Printf("    * Target Configuration: %s\n", opts.ConfigPath)
	fmt.Printf("    * Container Host Rewriting: %v\n", opts.UseContainerHost)

	daemonCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	repCh, stopFunc := updater.StartDailyDaemon(daemonCtx)
	defer stopFunc()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	fmt.Println("[+] Daemon active. Monitoring inference engines and keeping routes updated.")
	if !daemonMode {
		fmt.Println("    (Press Ctrl+C to terminate)")
	}

	for {
		select {
		case sig := <-sigCh:
			fmt.Printf("\n[!] Received signal %v. Terminating LiteLLM routing scheduler...\n", sig)
			stopFunc()
			return
		case rep, ok := <-repCh:
			if !ok {
				return
			}
			if rep != nil {
				fmt.Printf("[%s] [✓] Synced %d routes (%d discovered models). Primary: %s (Score: %d on %s)\n",
					rep.Timestamp.Format("15:04:05"), rep.TotalModelsConfigured, len(reportModels(rep)),
					rep.ActiveLocalPrimary, rep.ActiveLocalPrimaryScore, rep.ActiveLocalPrimaryEngine)
			}
		}
	}
}

func reportModels(rep *gateway.UpdateReport) []gateway.DiscoveredModel {
	if rep == nil {
		return nil
	}
	return rep.DiscoveredModels
}

func renderGatewayScenario(ctx context.Context, scenarioStr string) {
	sc, err := topology.ParseScenario(scenarioStr)
	if err != nil {
		fmt.Printf("[-] Invalid scenario '%s': %v\n", scenarioStr, err)
		return
	}

	ctrlIP := os.Getenv("TAILSCALE_CONTROL_PLANE_IP")
	if ctrlIP == "" {
		ctrlIP = "100.84.12.10"
	}
	execIP := os.Getenv("TAILSCALE_EXECUTION_PLANE_IP")
	if execIP == "" {
		execIP = "100.84.12.25"
	}

	top, err := topology.NewTopologyConfig(sc, topology.TailscaleEndpoints{
		ControlPlane:   ctrlIP,
		ExecutionPlane: execIP,
	})
	if err != nil {
		fmt.Printf("[-] Failed to create topology: %v\n", err)
		return
	}
	yamlData, err := gateway.RenderForTopology(top)
	if err != nil {
		fmt.Printf("[-] Failed to render LiteLLM gateway config: %v\n", err)
		return
	}
	fmt.Printf("[+] Rendered LiteLLM Config for scenario '%s':\n\n%s\n", sc, string(yamlData))
}

func printGatewayUsage() {
	fmt.Println("Usage: dendrite gateway <update|schedule|render|zero-cost> [args...]")
	fmt.Println("\nCommands:")
	fmt.Println("  update [--file <path>] [--dry-run] [--docker] [--json]")
	fmt.Println("                        Manually trigger dynamic LiteLLM routing table discovery and update")
	fmt.Println("  schedule [--interval 24h] [--file <path>] [--daemon]")
	fmt.Println("                        Run daily background daemon updating LiteLLM routing table once every 24h")
	fmt.Println("  render [scenario]     Render dynamic LiteLLM configuration for topology scenario")
	fmt.Println("  zero-cost <list|add|remove|test|reset>")
	fmt.Println("                        Inspect and manage zero-cost self-hosted models and prefix rules")
}
