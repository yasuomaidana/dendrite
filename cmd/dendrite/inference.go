package main

import (
	"context"
	"os/exec"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
			if inference.IsEngineZeroCost(eng) {
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

	case "migrate", "move":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite dev inference migrate <model_name> [--from ollama] [--to oxillama] [--delete-source] [--dry-run]")
			return
		}
		modelName := args[1]
		fromEngine := "ollama"
		toEngine := "oxillama"
		deleteSource := false
		dryRun := false

		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--from":
				if i+1 < len(args) {
					fromEngine = args[i+1]
					i++
				}
			case "--to":
				if i+1 < len(args) {
					toEngine = args[i+1]
					i++
				}
			case "--delete-source", "--rm-source", "--evict":
				deleteSource = true
			case "--dry-run":
				dryRun = true
			}
		}

		fmt.Printf("[+] Initiating model migration for '%s' (%s -> %s)...\n", modelName, fromEngine, toEngine)
		migrator := inference.NewModelMigrator("", "", nil)
		res, err := migrator.Migrate(ctx, inference.ModelMigrationOptions{
			ModelName:    modelName,
			SourceEngine: fromEngine,
			TargetEngine: toEngine,
			DeleteSource: deleteSource,
			DryRun:       dryRun,
		})
		if err != nil {
			fmt.Printf("[-] Migration failed: %v\n", err)
			return
		}

		fmt.Printf("[✓] Successfully migrated model '%s' to %s in %v\n", res.ModelName, res.TargetEngine, res.Duration)
		fmt.Printf("    * Blobs Migrated:     %d\n", res.BlobsMigrated)
		fmt.Printf("    * Total Data Size:    %.2f GB (%d bytes)\n", float64(res.TotalBytesMigrated)/(1024*1024*1024), res.TotalBytesMigrated)
		fmt.Printf("    * Target Manifest:    %s\n", res.TargetManifest)
		if res.SourceEvicted {
			fmt.Printf("    * Source Evicted:     YES (Reclaimed %.2f GB memory/disk from %s)\n", float64(res.BytesFreed)/(1024*1024*1024), res.SourceEngine)
		} else if deleteSource {
			fmt.Printf("    * Source Evicted:     NO (%s)\n", res.Error)
		} else {
			fmt.Println("    * Source Retained:    (pass --delete-source to evict from source and reclaim memory)")
		}

	case "serve", "start":
		port := 8080
		daemonMode := false
		for i := 1; i < len(args); i++ {
			if args[i] == "--port" && i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					port = p
				}
				i++
			} else if args[i] == "--daemon" || args[i] == "-d" {
				daemonMode = true
			}
		}

		srv := inference.NewOxiLLaMaServer(inference.OxiLLaMaServerConfig{
			Host: "127.0.0.1",
			Port: port,
		})

		if err := srv.Start(); err != nil {
			fmt.Printf("[-] Failed to start OxiLLaMa server: %v\n", err)
			return
		}

		fmt.Printf("[✓] OxiLLaMa Pure-Rust OpenAI Server listening at http://127.0.0.1:%d\n", port)
		fmt.Printf("    * Models Endpoint:     http://127.0.0.1:%d/v1/models\n", port)
		fmt.Printf("    * Chat Endpoint:       http://127.0.0.1:%d/v1/chat/completions\n", port)
		fmt.Printf("    * Storage Directory:   ~/.oxillama/models\n")

		if daemonMode {
			fmt.Println("[+] Running in daemon mode. Press Ctrl+C to terminate.")
			select {}
		}

	case "stop", "kill":
		port := 8080
		for i := 1; i < len(args); i++ {
			if args[i] == "--port" && i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					port = p
				}
				i++
			}
		}

		fmt.Printf("[+] Stopping OxiLLaMa server on port %d...\n", port)
		lsofCmd := exec.CommandContext(ctx, "lsof", "-ti", fmt.Sprintf(":%d", port))
		out, _ := lsofCmd.Output()
		pids := strings.Fields(string(out))
		stopped := 0
		for _, pidStr := range pids {
			if pid, pErr := strconv.Atoi(pidStr); pErr == nil && pid > 0 {
				if proc, fErr := os.FindProcess(pid); fErr == nil {
					_ = proc.Kill()
					stopped++
				}
			}
		}
		_ = exec.CommandContext(ctx, "pkill", "-f", "oxillama serve").Run()

		if stopped > 0 {
			fmt.Printf("[✓] Successfully stopped OxiLLaMa server (terminated %d process(es) on port %d)\n", stopped, port)
		} else {
			fmt.Printf("[✓] OxiLLaMa server is not currently running on port %d.\n", port)
		}

	case "zero-cost", "zerocost", "models":
		handleZeroCostCommand(args[1:])

	default:
		printInferenceUsage()
	}
}

func handleZeroCostCommand(args []string) {
	reg := inference.GetZeroCostRegistry()

	if len(args) == 0 || args[0] == "list" || args[0] == "ls" {
		outputJSON := false
		for _, a := range args {
			if a == "--json" {
				outputJSON = true
			}
		}

		details := reg.GetDetails()

		if outputJSON {
			data, err := json.MarshalIndent(details, "", "  ")
			if err != nil {
				fmt.Printf("[-] Failed to serialize JSON: %v\n", err)
				return
			}
			fmt.Println(string(data))
			return
		}

		fmt.Printf("[+] Configured Zero-Cost & Self-Hosted Models / Prefixes (%d active):\n", details.Total)
		fmt.Println("    --------------------------------------------------------------------------------")
		fmt.Printf("    %-32s %-10s %-12s %s\n", "PATTERN", "KIND", "SOURCE", "STATUS")
		fmt.Println("    --------------------------------------------------------------------------------")

		for _, m := range details.Models {
			st := "Active"
			if !m.Active {
				st = "Overridden / Disabled"
			}
			fmt.Printf("    %-32s %-10s %-12s %s\n", m.Pattern, "Exact", m.Source, st)
		}
		for _, p := range details.Prefixes {
			st := "Active"
			if !p.Active {
				st = "Overridden / Disabled"
			}
			fmt.Printf("    %-32s %-10s %-12s %s\n", p.Pattern, "Prefix", p.Source, st)
		}
		fmt.Println("    --------------------------------------------------------------------------------")
		fmt.Println("    Commands:")
		fmt.Println("      add <model|prefix/> [--prefix] [--env]  Register a new zero-cost model/prefix")
		fmt.Println("      remove <model|prefix/> [--env]          Unregister a model/prefix")
		fmt.Println("      test <model_name>                       Check zero-cost qualification for model")
		fmt.Println("      reset                                   Restore factory defaults and .env rules")
		return
	}

	sub := strings.ToLower(args[0])

	switch sub {
	case "add":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite dev inference zero-cost add <model_or_prefix> [--prefix] [--env]")
			return
		}
		target := args[1]
		forcePrefix := false
		syncEnv := false

		for _, a := range args[2:] {
			if a == "--prefix" || a == "-p" {
				forcePrefix = true
			} else if a == "--env" || a == "-e" {
				syncEnv = true
			}
		}

		var ok, isPrefix bool
		if forcePrefix {
			ok = reg.AddPrefix(target)
			isPrefix = true
		} else {
			ok, isPrefix = reg.Add(target)
		}

		if !ok {
			fmt.Printf("[-] Failed to register: invalid pattern '%s'\n", target)
			return
		}

		_ = reg.SaveToFile(inference.DefaultZeroCostConfigFile)

		kind := "exact model"
		if isPrefix {
			kind = "prefix rule"
		}
		fmt.Printf("[✓] Registered '%s' as zero-cost %s (editable on the go)\n", target, kind)
		fmt.Printf("    Saved to dynamic config: %s\n", inference.DefaultZeroCostConfigFile)

		// Check if .env exists and sync if requested or default .env found
		envPath := ".env"
		if _, err := os.Stat(envPath); err == nil || syncEnv {
			if err := reg.SaveToEnvFile(envPath); err == nil {
				fmt.Printf("    Synchronized with environment file: %s\n", envPath)
			}
		}

	case "remove", "rm", "delete":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite dev inference zero-cost remove <model_or_prefix> [--env]")
			return
		}
		target := args[1]
		syncEnv := false
		for _, a := range args[2:] {
			if a == "--env" || a == "-e" {
				syncEnv = true
			}
		}

		ok, isPrefix := reg.Remove(target)
		if !ok {
			fmt.Printf("[-] Failed to remove: '%s'\n", target)
			return
		}

		_ = reg.SaveToFile(inference.DefaultZeroCostConfigFile)

		kind := "exact model"
		if isPrefix {
			kind = "prefix rule"
		}
		fmt.Printf("[✓] Removed '%s' from zero-cost %s\n", target, kind)
		fmt.Printf("    Updated dynamic config: %s\n", inference.DefaultZeroCostConfigFile)

		envPath := ".env"
		if _, err := os.Stat(envPath); err == nil || syncEnv {
			if err := reg.SaveToEnvFile(envPath); err == nil {
				fmt.Printf("    Synchronized with environment file: %s\n", envPath)
			}
		}

	case "test", "check":
		if len(args) < 2 {
			fmt.Println("[-] Usage: dendrite dev inference zero-cost test <model_name>")
			return
		}
		target := args[1]
		isZeroCost, reason := reg.Test(target)
		if isZeroCost {
			fmt.Printf("[✓] Zero-Cost: TRUE\n    Reason: %s\n", reason)
		} else {
			fmt.Printf("[-] Zero-Cost: FALSE\n    Reason: %s\n", reason)
		}

	case "reset":
		reg.Reset()
		_ = os.Remove(inference.DefaultZeroCostConfigFile)
		reg.AutoLoad()
		fmt.Println("[✓] Successfully reset zero-cost models to defaults + active .env configuration.")

	default:
		fmt.Printf("[-] Unknown zero-cost command: %s. Use list, add, remove, test, or reset.\n", sub)
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
	fmt.Println("Usage: dendrite dev inference <list|probe|switch|render|migrate|zero-cost> [args...]")
	fmt.Println("\nCommands:")
	fmt.Println("  list                          List registered engines, models, and fallback cascade order")
	fmt.Println("  probe [engine_id]             Check reachability, latency, and available models (<2s)")
	fmt.Println("  switch <engine_id>            Set primary inference engine at front of fallback cascade")
	fmt.Println("  render                        Render upstream model routes into litellm_config.yaml")
	fmt.Println("  serve [--port 8080] [-d]      Start local OxiLLaMa OpenAI-compatible HTTP server")
	fmt.Println("  stop [--port 8080]            Stop running OxiLLaMa server and free port")
	fmt.Println("  migrate <model> [flags]       Migrate model from Ollama to OxiLLaMa and evict source blobs")
	fmt.Println("  zero-cost <list|add|rm|test>  Manage and test zero-cost self-hosted models and prefixes")
}
