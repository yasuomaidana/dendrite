package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/testgen"
	"Dendrite/pkg/uitest"
)

func handleTestCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "auto":
		handleTestAutoCommand(ctx, args)
	case "ui":
		handleTestUICommand(ctx, args)
	default:
		printTestUsage()
	}
}

func printTestUsage() {
	fmt.Println("Usage: dendrite test <auto|ui> [flags...]")
	fmt.Println("  auto --file <path> [--with-testcontainers]   Synthesize table-driven tests via self-healing loop")
	fmt.Println("  ui [--headless] [--base-url <url>]           Execute Web UI automated browser journeys and telemetry load")
}

func handleTestAutoCommand(ctx context.Context, args []string) {
	var filePath string
	var withContainers bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			if i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		case "--with-testcontainers", "--testcontainers":
			withContainers = true
		default:
			if filePath == "" && !strings.HasPrefix(args[i], "--") {
				filePath = args[i]
			}
		}
	}

	if filePath == "" {
		fmt.Println("[-] Please specify a source file via --file <path>")
		return
	}

	cfg := testgen.TestSynthesisConfig{
		SourceFile:           filePath,
		WithTestcontainers:   withContainers,
		MaxHealingIterations: 3,
		TargetCoverage:       85.0,
	}

	fmt.Printf("[+] Launching AST Test Synthesizer on: %s\n", filePath)
	runner := testgen.NewSelfHealingRunner(3)
	report, err := runner.Run(ctx, cfg)
	if err != nil {
		fmt.Printf("[-] Test synthesis error: %v\n", err)
		return
	}

	fmt.Printf("[✓] Test Synthesis Succeeded! (Run ID: %s, State: %s)\n", report.RunID, report.State)
	fmt.Printf("    - Generated Test: %s\n", report.TestFile)
	fmt.Printf("    - Convergence:    %d iteration(s)\n", report.Iterations)
	fmt.Printf("    - Code Coverage:  %.1f%%\n", report.Coverage)
	fmt.Printf("    - Dependencies:   %v\n", report.DependenciesDetected)
	fmt.Printf("    - Duration:       %v\n", report.Duration)
}

func handleTestUICommand(ctx context.Context, args []string) {
	baseURL := "http://127.0.0.1:3456"
	headless := true

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--base-url":
			if i+1 < len(args) {
				baseURL = args[i+1]
				i++
			}
		case "--no-headless":
			headless = false
		}
	}

	fmt.Printf("[+] Executing Web UI Test Harness (Target: %s, Headless: %v)...\n", baseURL, headless)
	driver := uitest.NewBrowserDriver(baseURL, headless)
	harness := uitest.NewUIHarness(driver)

	results, err := harness.RunAllJourneys(ctx)
	if err != nil {
		fmt.Printf("[-] UI Test Harness error: %v\n", err)
		return
	}

	fmt.Println("[+] Executed User Journeys:")
	for _, res := range results {
		status := "PASS"
		if !res.Passed {
			status = "FAIL"
		}
		fmt.Printf("    - [%s] %s (%d/%d steps in %v)\n",
			status, res.Name, res.StepsCompleted, res.TotalSteps, res.Duration)
	}

	fmt.Println("\n[+] Running High-Frequency WebSocket Telemetry Burst Test (100 msgs/sec)...")
	metrics, err := harness.RunTelemetryBurstTest(ctx, 100, 2*time.Second)
	if err != nil {
		fmt.Printf("[-] Telemetry burst error: %v\n", err)
		return
	}

	fmt.Printf("[✓] Telemetry Burst Passed: %.1f msgs/sec (Min FPS: %.1f, Memory Leaks: %v)\n",
		metrics.MessagesPerSec, metrics.MinFPS, metrics.MemoryLeakDetected)
}
