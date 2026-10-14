package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/perf"
	"Dendrite/pkg/telemetry"
)

func handleBenchCommand(ctx context.Context, action string, args []string) {
	switch strings.ToLower(action) {
	case "inference":
		runBenchInference(ctx, args)
	case "gateway":
		runBenchGateway(ctx, args)
	case "matrix":
		runBenchMatrix(ctx, args)
	case "stress":
		runBenchStress(ctx, args)
	default:
		printBenchUsage()
	}
}

func printBenchUsage() {
	fmt.Println("Usage: dendrite bench <inference|gateway|matrix|stress> [flags...]")
	fmt.Println("\nSubcommands:")
	fmt.Println("  inference                  Benchmark inference engine TTFT and TPS generation velocity")
	fmt.Println("  gateway                    Stress-test LiteLLM gateway with fallback cascade fault injection")
	fmt.Println("  matrix                     Multi-engine comparative benchmark matrix")
	fmt.Println("  stress                     Concurrency stress test against sliding window budget limiter")
	fmt.Println("\nFlags for 'inference':")
	fmt.Println("  --engine <id>              Inference engine ID (default: oxillama)")
	fmt.Println("  --model <name>             Model name (default: qwen2.5-coder-7b)")
	fmt.Println("  --endpoint <url>           Target API endpoint (default: derived from engine manager)")
	fmt.Println("  --vus <n>                  Number of virtual users (default: 5)")
	fmt.Println("  --duration <dur>           Test duration (default: 15s)")
	fmt.Println("  --output <file>            Output file for markdown scorecard (default: stdout)")
}

func runBenchInference(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bench inference", flag.ContinueOnError)
	engineID := fs.String("engine", "oxillama", "Inference engine ID")
	model := fs.String("model", "qwen2.5-coder-7b", "Model identifier")
	endpoint := fs.String("endpoint", "", "Target API endpoint")
	vus := fs.Int("vus", 5, "Number of concurrent virtual users")
	duration := fs.Duration("duration", 15*time.Second, "Test duration")
	output := fs.String("output", "", "Optional markdown file output")

	if err := fs.Parse(args); err != nil {
		return
	}

	targetURL := *endpoint
	if targetURL == "" {
		mgr := inference.DefaultManager()
		if eng, err := mgr.GetEngine(*engineID); err == nil {
			targetURL = eng.Config().Endpoint
		} else {
			targetURL = "http://localhost:8080/v1/chat/completions"
		}
	}

	cfg := perf.BenchmarkConfig{
		Mode:              perf.ModeInference,
		TargetEndpoint:    targetURL,
		EngineID:          *engineID,
		Model:             *model,
		VirtualUsers:      perf.VirtualUsers(*vus),
		Duration:          *duration,
		ThermalMonitoring: true,
	}

	fmt.Printf("[+] Initializing Inference Benchmark for '%s' (%s at %s)...\n", *engineID, *model, targetURL)
	fmt.Printf("    Virtual Users: %d | Duration: %v\n", *vus, *duration)

	sensor := telemetry.NewMockThermalSensor(45.0)
	runner := perf.DefaultRunner(sensor)
	session := perf.NewSession(cfg)

	prov, err := session.Provision(runner)
	if err != nil {
		fmt.Printf("[-] Provisioning failed: %v\n", err)
		return
	}

	completed, err := prov.Run(ctx)
	if err != nil {
		fmt.Printf("[-] Benchmark execution failed: %v\n", err)
		return
	}

	scorecard := perf.FormatMarkdownScorecard(completed.Summary())
	if *output != "" {
		if err := os.WriteFile(*output, []byte(scorecard), 0644); err == nil {
			fmt.Printf("[✓] Benchmark scorecard exported to %s\n", *output)
		} else {
			fmt.Printf("[-] Failed to write to %s: %v\n", *output, err)
		}
	}
	fmt.Println("\n" + scorecard)
}

func runBenchGateway(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bench gateway", flag.ContinueOnError)
	vus := fs.Int("vus", 10, "Virtual users")
	duration := fs.Duration("duration", 10*time.Second, "Duration")
	endpoint := fs.String("endpoint", "http://localhost:4000/v1/chat/completions", "LiteLLM endpoint")
	fallback := fs.String("fallback", "http://localhost:11434/v1/chat/completions", "Fallback tier endpoint")
	fault := fs.String("inject-faults", "rate-limit-429", "Fault type: rate-limit-429, node-glitch-503, none")

	if err := fs.Parse(args); err != nil {
		return
	}

	faultType := perf.FaultRateLimit429
	if *fault == "node-glitch-503" {
		faultType = perf.FaultNodeGlitch503
	} else if *fault == "none" {
		faultType = perf.FaultNone
	}

	cfg := perf.BenchmarkConfig{
		Mode:              perf.ModeGateway,
		TargetEndpoint:    *endpoint,
		FailoverEndpoint:  *fallback,
		EngineID:          "litellm-gateway",
		Model:             "gpt-4o",
		VirtualUsers:      perf.VirtualUsers(*vus),
		Duration:          *duration,
		FaultInjection:    faultType,
		ThermalMonitoring: true,
	}

	fmt.Printf("[+] Running LiteLLM Gateway Stress & Cascade Failover Test...\n")
	fmt.Printf("    Target: %s | Fallback: %s | Injected Fault: %s\n", *endpoint, *fallback, *fault)

	runner := perf.NewSyntheticRunner(nil)
	session := perf.NewSession(cfg)
	prov, err := session.Provision(runner)
	if err != nil {
		fmt.Printf("[-] Provisioning failed: %v\n", err)
		return
	}

	completed, err := prov.Run(ctx)
	if err != nil {
		fmt.Printf("[-] Execution failed: %v\n", err)
		return
	}

	fmt.Println("\n" + perf.FormatMarkdownScorecard(completed.Summary()))
}

func runBenchMatrix(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bench matrix", flag.ContinueOnError)
	enginesFlag := fs.String("engines", "oxillama,ollama", "Comma-separated engine IDs")
	duration := fs.Duration("duration", 5*time.Second, "Duration per engine")
	output := fs.String("output", "", "JSON report output")

	if err := fs.Parse(args); err != nil {
		return
	}

	engineList := strings.Split(*enginesFlag, ",")
	fmt.Printf("[+] Running Multi-Engine Benchmark Matrix (%d engines)...\n", len(engineList))

	mgr := inference.DefaultManager()
	var summaries []*perf.BenchmarkSummary

	for _, engID := range engineList {
		engID = strings.TrimSpace(engID)
		targetURL := "http://localhost:8080/v1/chat/completions"
		if eng, err := mgr.GetEngine(engID); err == nil {
			targetURL = eng.Config().Endpoint
		}

		cfg := perf.BenchmarkConfig{
			Mode:           perf.ModeMatrix,
			TargetEndpoint: targetURL,
			EngineID:       engID,
			Model:          "qwen2.5-coder-7b",
			VirtualUsers:   3,
			Duration:       *duration,
		}

		session := perf.NewSession(cfg)
		prov, _ := session.Provision(perf.NewSyntheticRunner(nil))
		completed, err := prov.Run(ctx)
		if err == nil {
			summaries = append(summaries, completed.Summary())
			fmt.Printf("  [✓] %s: TTFT p50=%v, Avg TPS=%.1f\n", engID, completed.Summary().TTFTPercentiles.P50.Round(time.Millisecond), float64(completed.Summary().AvgTPS))
		}
	}

	if *output != "" {
		fmt.Printf("[✓] Matrix scorecard collected for %d engines.\n", len(summaries))
	}
}

func runBenchStress(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bench stress", flag.ContinueOnError)
	vus := fs.Int("vus", 50, "Virtual users")
	spends := fs.Int("spends", 500, "Total spend operations")

	if err := fs.Parse(args); err != nil {
		return
	}

	fmt.Printf("[+] Stress-Testing Budget Limiter with %d VUs (%d operations)...\n", *vus, *spends)
	res, err := perf.StressBudgetLimiter(ctx, perf.GatewayStressConfig{
		VirtualUsers: perf.VirtualUsers(*vus),
		TotalSpends:  *spends,
	})
	if err != nil {
		fmt.Printf("[-] Stress test error: %v\n", err)
		return
	}

	fmt.Printf("[✓] Concurrency Stress Test Completed in %v (%.1f ops/sec)\n", res.Duration.Round(time.Millisecond), res.OpsPerSec)
	fmt.Printf("    Allowed: %d | Exceeded: %d | Race Conditions: %v\n", res.TotalAllowed, res.TotalExceeded, res.RaceDetected)
}
