package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"Dendrite/pkg/testgen"
	"Dendrite/pkg/uitest"

	"github.com/spf13/cobra"
)

// newReflexCmd builds `dendrite reflex` (alias test): test synthesis and UI journey testing.
func newReflexCmd() *cobra.Command {
	cmd := newGroupCmd("reflex", "Table-driven test synthesis, Testcontainers & UI tests", "effectors",
		"test", "myotatic-reflex", "tst")
	cmd.Example = "dendrite reflex auto pkg/env/detector.go\n" +
		"dendrite reflex auto --file pkg/env/detector.go --with-testcontainers\n" +
		"dendrite reflex ui --base-url http://127.0.0.1:3456"
	cmd.AddCommand(newReflexAutoCmd(), newReflexUICmd())
	return cmd
}

func newReflexAutoCmd() *cobra.Command {
	cfg := testgen.TestSynthesisConfig{MaxHealingIterations: 3, TargetCoverage: 85.0}
	cmd := &cobra.Command{
		Use:   "auto [file]",
		Short: "Synthesize table-driven tests for a source file with a self-healing loop",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if cfg.SourceFile != "" {
					return usageErrorf("give the file either as an argument or with --file, not both")
				}
				cfg.SourceFile = args[0]
			}
			if cfg.SourceFile == "" {
				return usageErrorf("give a source file")
			}
			return runReflexAuto(cmd.Context(), cfg, cmd.OutOrStdout())
		},
	}
	addFileFlag(cmd, &cfg.SourceFile, "", "Go source file to cover")
	cmd.Flags().BoolVar(&cfg.WithTestcontainers, "with-testcontainers", false, "Generate Testcontainers fixtures for detected dependencies")
	return cmd
}

func runReflexAuto(ctx context.Context, cfg testgen.TestSynthesisConfig, out io.Writer) error {
	fmt.Fprintf(out, "[+] Launching AST Test Synthesizer on: %s\n", cfg.SourceFile)
	report, err := testgen.NewSelfHealingRunner(cfg.MaxHealingIterations).Run(ctx, cfg)
	if err != nil {
		return fmt.Errorf("test synthesis: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, report)
	}
	fmt.Fprintf(out, "[✓] Test Synthesis Succeeded! (Run ID: %s, State: %s)\n", report.RunID, report.State)
	fmt.Fprintf(out, "    - Generated Test: %s\n", report.TestFile)
	fmt.Fprintf(out, "    - Convergence:    %d iteration(s)\n", report.Iterations)
	fmt.Fprintf(out, "    - Code Coverage:  %.1f%%\n", report.Coverage)
	fmt.Fprintf(out, "    - Dependencies:   %v\n", report.DependenciesDetected)
	fmt.Fprintf(out, "    - Duration:       %v\n", report.Duration)
	return nil
}

func newReflexUICmd() *cobra.Command {
	var baseURL string
	var noHeadless bool
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Run the web UI browser journeys and a WebSocket telemetry burst test",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReflexUI(cmd.Context(), baseURL, !noHeadless, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "http://127.0.0.1:3456", "Cockpit URL (see 'cortex serve')")
	cmd.Flags().BoolVar(&noHeadless, "no-headless", false, "Show the browser")
	return cmd
}

func runReflexUI(ctx context.Context, baseURL string, headless bool, out io.Writer) error {
	fmt.Fprintf(out, "[+] Executing Web UI Test Harness (Target: %s, Headless: %v)...\n", baseURL, headless)
	harness := uitest.NewUIHarness(uitest.NewBrowserDriver(baseURL, headless))
	results, err := harness.RunAllJourneys(ctx)
	if err != nil {
		return fmt.Errorf("UI test harness: %w", err)
	}
	failed := 0
	fmt.Fprintln(out, "[+] Executed User Journeys:")
	for _, res := range results {
		status := "PASS"
		if !res.Passed {
			status = "FAIL"
			failed++
		}
		fmt.Fprintf(out, "    - [%s] %s (%d/%d steps in %v)\n", status, res.Name, res.StepsCompleted, res.TotalSteps, res.Duration)
	}

	fmt.Fprintln(out, "\n[+] Running High-Frequency WebSocket Telemetry Burst Test (100 msgs/sec)...")
	metrics, err := harness.RunTelemetryBurstTest(ctx, 100, 2*time.Second)
	if err != nil {
		return fmt.Errorf("telemetry burst: %w", err)
	}
	fmt.Fprintf(out, "[✓] Telemetry Burst Passed: %.1f msgs/sec (Min FPS: %.1f, Memory Leaks: %v)\n",
		metrics.MessagesPerSec, metrics.MinFPS, metrics.MemoryLeakDetected)
	if failed > 0 {
		return fmt.Errorf("%d of %d UI journeys failed", failed, len(results))
	}
	return nil
}
