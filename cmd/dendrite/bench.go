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
		PreFlightCascade:  true,
		CascadeFallbackID: "ollama-metal",
		FailoverEndpoint:  "http://127.0.0.1:11434/v1/chat/completions",
		Prompt:            "Explain the SlidingWindowLimiter concurrency invariants in Dendrite",
		MaxTokens:         256,
	}

	fmt.Printf("[+] Initializing Inference Benchmark for '%s' (%s at %s)...\n", *engineID, *model, targetURL)
	fmt.Printf("    Virtual Users: %d | Duration: %v\n", *vus, *duration)

	sensor := telemetry.NewHostThermalSensor()
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
		if err := os.WriteFile(*output, []byte(scorecard), 0644); err != nil {
			fmt.Printf("[-] Failed to write markdown scorecard: %v\n", err)
			return
		}
		fmt.Printf("[✓] Benchmark scorecard saved to %s\n", *output)
	} else {
		fmt.Println("\n" + scorecard)
	}
}

func runBenchGateway(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bench gateway", flag.ContinueOnError)
	faultStr := fs.String("fault", "none", "Injected fault type: none, rate-limit-429, node-glitch-503")
	vus := fs.Int("vus", 10, "Number of concurrent virtual users")
	duration := fs.Duration("duration", 20*time.Second, "Test duration")
	output := fs.String("output", "", "Optional markdown file output")

	if err := fs.Parse(args); err != nil {
		return
	}

	var fault perf.FaultInjectionType
	switch strings.ToLower(*faultStr) {
	case "rate-limit-429", "429":
		fault = perf.FaultRateLimit429
	case "node-glitch-503", "503":
		fault = perf.FaultNodeGlitch503
	default:
		fault = perf.FaultNone
	}

	cfg := perf.BenchmarkConfig{
		Mode:              perf.ModeGateway,
		TargetEndpoint:    "http://localhost:4000/v1/chat/completions",
		FailoverEndpoint:  "http://127.0.0.1:11434/v1/chat/completions",
		EngineID:          "litellm-gateway",
		Model:             "qwen2.5-coder:14b",
		VirtualUsers:      perf.VirtualUsers(*vus),
		Duration:          *duration,
		Prompt:            "Benchmark gateway cascade recovery under fault injection",
		MaxTokens:         128,
		FaultInjection:    fault,
		ThermalMonitoring: true,
	}

	fmt.Printf("[+] Initializing LiteLLM Gateway Stress Benchmark (Fault: %s)...\n", fault)
	fmt.Printf("    Virtual Users: %d | Duration: %v\n", *vus, *duration)

	sensor := telemetry.NewHostThermalSensor()
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
		if err := os.WriteFile(*output, []byte(scorecard), 0644); err != nil {
			fmt.Printf("[-] Failed to write markdown scorecard: %v\n", err)
			return
		}
		fmt.Printf("[✓] Gateway benchmark scorecard saved to %s\n", *output)
	} else {
		fmt.Println("\n" + scorecard)
	}
}

func runBenchMatrix(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bench matrix", flag.ContinueOnError)
	duration := fs.Duration("duration", 10*time.Second, "Test duration per engine")
	vus := fs.Int("vus", 5, "Number of concurrent virtual users")
	output := fs.String("output", "", "Optional output file")

	if err := fs.Parse(args); err != nil {
		return
	}

	fmt.Printf("[+] Initializing Multi-Engine Benchmark Matrix (VUs: %d, Duration: %v per engine)...\n", *vus, *duration)
	mgr := inference.DefaultManager()
	engines := mgr.ListEngines()

	var summaries []*perf.BenchmarkSummary
	sensor := telemetry.NewHostThermalSensor()
	runner := perf.DefaultRunner(sensor)

	for _, eng := range engines {
		cfg := perf.BenchmarkConfig{
			Mode:              perf.ModeMatrix,
			TargetEndpoint:    eng.Config().Endpoint,
			EngineID:          eng.ID(),
			Model:             "matrix-eval",
			VirtualUsers:      perf.VirtualUsers(*vus),
			Duration:          *duration,
			ThermalMonitoring: true,
			PreFlightCascade:  true,
			CascadeFallbackID: "ollama-metal",
			FailoverEndpoint:  "http://127.0.0.1:11434/v1/chat/completions",
			Prompt:            "Benchmark matrix comparison prompt",
			MaxTokens:         128,
		}

		session := perf.NewSession(cfg)
		prov, err := session.Provision(runner)
		if err != nil {
			continue
		}
		completed, err := prov.Run(ctx)
		if err == nil {
			summaries = append(summaries, completed.Summary())
		}
	}

	var sb strings.Builder
	sb.WriteString("# Multi-Engine Benchmark Comparison Matrix\n\n")
	sb.WriteString("| Engine ID | p50 TTFT | Avg TPS | Peak TPS | Error Rate | Failover / Cascade |\n")
	sb.WriteString("|:---|:---|:---|:---|:---|:---|\n")

	for _, s := range summaries {
		failoverStr := "Direct"
		if s.FailoverMetrics != nil && s.FailoverMetrics.PreFlightSwitched {
			failoverStr = fmt.Sprintf("Cascaded -> %s", s.FailoverMetrics.CascadedEngineID)
		} else if s.FailoverMetrics != nil && s.FailoverMetrics.FailoverTriggered {
			failoverStr = "Recovered"
		}
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%.2f tok/s` | `%.2f tok/s` | `%.2f%%` | `%s` |\n",
			s.EngineID, s.TTFTPercentiles.P50.Round(time.Millisecond), float64(s.AvgTPS), float64(s.PeakTPS), s.ErrorRatePercent, failoverStr))
	}

	resultStr := sb.String()
	if *output != "" {
		_ = os.WriteFile(*output, []byte(resultStr), 0644)
		fmt.Printf("[✓] Matrix scorecard saved to %s\n", *output)
	} else {
		fmt.Println("\n" + resultStr)
	}
}

func runBenchStress(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("bench stress", flag.ContinueOnError)
	vus := fs.Int("vus", 20, "Number of concurrent virtual users")
	spends := fs.Int("spends", 500, "Total spend operations")

	if err := fs.Parse(args); err != nil {
		return
	}

	fmt.Printf("[+] Executing Budget Limiter Concurrency Stress Test (%d VUs, %d spends)...\n", *vus, *spends)
	cfg := perf.GatewayStressConfig{
		VirtualUsers: perf.VirtualUsers(*vus),
		TotalSpends:  *spends,
	}
	res, err := perf.StressBudgetLimiter(ctx, cfg)
	if err != nil {
		fmt.Printf("[-] Stress test error: %v\n", err)
		return
	}

	fmt.Printf("[✓] Stress Test Completed in %v (%.2f ops/sec, Race Detected: %v)\n",
		res.Duration, res.OpsPerSec, res.RaceDetected)
	fmt.Printf("    - Allowed Spends:  %d\n", res.TotalAllowed)
	fmt.Printf("    - Exceeded Limits: %d\n", res.TotalExceeded)
}
