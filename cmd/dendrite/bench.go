package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/perf"
	"Dendrite/pkg/telemetry"

	"github.com/spf13/cobra"
)

const ollamaFailoverEndpoint = "http://127.0.0.1:11434/v1/chat/completions"

// newVelocityCmd builds `dendrite velocity` (alias bench): TTFT/TPS and gateway resilience benchmarks.
func newVelocityCmd() *cobra.Command {
	cmd := newGroupCmd("velocity", "TTFT, TPS, and gateway resilience benchmarking suite", "axon",
		"bench", "benchmark", "conduction-velocity", "bnch")
	cmd.Example = "dendrite velocity inference --engine oxillama --vus 5 --duration 15s\n" +
		"dendrite velocity gateway --fault 429 --output gateway.md\n" +
		"dendrite velocity matrix\n" +
		"dendrite velocity stress --vus 50 --spends 1000"
	cmd.AddCommand(newVelocityInferenceCmd(), newVelocityGatewayCmd(), newVelocityMatrixCmd(), newVelocityStressCmd())
	return cmd
}

// benchLoad holds the load-shape flags shared by the benchmarks.
type benchLoad struct {
	VUs      int
	Duration time.Duration
	Output   string
}

func addBenchLoadFlags(cmd *cobra.Command, l *benchLoad, vus int, duration time.Duration) {
	cmd.Flags().IntVar(&l.VUs, "vus", vus, "Concurrent virtual users")
	cmd.Flags().DurationVar(&l.Duration, "duration", duration, "Test duration")
	cmd.Flags().StringVarP(&l.Output, "output", "o", "", "Write the Markdown scorecard to this file instead of stdout")
}

func (l benchLoad) validate() error {
	if l.VUs <= 0 {
		return usageErrorf("--vus must be positive")
	}
	if l.Duration <= 0 {
		return usageErrorf("--duration must be positive")
	}
	return nil
}

// writeScorecard prints the scorecard or saves it to l.Output.
func writeScorecard(l benchLoad, scorecard string, out io.Writer) error {
	if l.Output == "" {
		fmt.Fprintln(out, "\n"+scorecard)
		return nil
	}
	if err := os.WriteFile(l.Output, []byte(scorecard), 0o644); err != nil {
		return fmt.Errorf("write scorecard: %w", err)
	}
	fmt.Fprintf(out, "[✓] Benchmark scorecard saved to %s\n", l.Output)
	return nil
}

// runBenchmark provisions and runs one benchmark session.
func runBenchmark(ctx context.Context, cfg perf.BenchmarkConfig) (*perf.BenchmarkSummary, error) {
	runner := perf.DefaultRunner(telemetry.NewHostThermalSensor())
	prov, err := perf.NewSession(cfg).Provision(runner)
	if err != nil {
		return nil, fmt.Errorf("provision benchmark: %w", err)
	}
	completed, err := prov.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("run benchmark: %w", err)
	}
	return completed.Summary(), nil
}

func newVelocityInferenceCmd() *cobra.Command {
	var load benchLoad
	var engineID, model, endpoint string
	cmd := &cobra.Command{
		Use:   "inference",
		Short: "Benchmark an inference engine's TTFT and TPS",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := load.validate(); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			target := endpoint
			if target == "" {
				target = "http://localhost:8080/v1/chat/completions"
				if eng, err := inference.DefaultManager().GetEngine(engineID); err == nil {
					target = eng.Config().Endpoint
				}
			}
			fmt.Fprintf(out, "[+] Initializing Inference Benchmark for '%s' (%s at %s)...\n", engineID, model, target)
			fmt.Fprintf(out, "    Virtual Users: %d | Duration: %v\n", load.VUs, load.Duration)
			summary, err := runBenchmark(cmd.Context(), perf.BenchmarkConfig{
				Mode:              perf.ModeInference,
				TargetEndpoint:    target,
				EngineID:          engineID,
				Model:             model,
				VirtualUsers:      perf.VirtualUsers(load.VUs),
				Duration:          load.Duration,
				ThermalMonitoring: true,
				PreFlightCascade:  true,
				CascadeFallbackID: "ollama-metal",
				FailoverEndpoint:  ollamaFailoverEndpoint,
				Prompt:            "Explain the SlidingWindowLimiter concurrency invariants in Dendrite",
				MaxTokens:         256,
			})
			if err != nil {
				return err
			}
			return writeScorecard(load, perf.FormatMarkdownScorecard(summary), out)
		},
	}
	cmd.Flags().StringVar(&engineID, "engine", "oxillama", "Inference engine ID")
	addModelFlag(cmd, &model, "qwen2.5-coder-7b")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Target API endpoint (default: the engine's endpoint)")
	addBenchLoadFlags(cmd, &load, 5, 15*time.Second)
	return cmd
}

// parseFault maps a --fault value to a fault injection type.
func parseFault(s string) (perf.FaultInjectionType, error) {
	switch strings.ToLower(s) {
	case "none", "":
		return perf.FaultNone, nil
	case "rate-limit-429", "429":
		return perf.FaultRateLimit429, nil
	case "node-glitch-503", "503":
		return perf.FaultNodeGlitch503, nil
	}
	return "", usageErrorf("unknown --fault %q: use none, 429 (rate-limit-429) or 503 (node-glitch-503)", s)
}

func newVelocityGatewayCmd() *cobra.Command {
	var load benchLoad
	var faultStr string
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Stress-test the LiteLLM gateway's fallback cascade under fault injection",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fault, err := parseFault(faultStr)
			if err != nil {
				return err
			}
			if err := load.validate(); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "[+] Initializing LiteLLM Gateway Stress Benchmark (Fault: %s)...\n", fault)
			fmt.Fprintf(out, "    Virtual Users: %d | Duration: %v\n", load.VUs, load.Duration)
			summary, err := runBenchmark(cmd.Context(), perf.BenchmarkConfig{
				Mode:              perf.ModeGateway,
				TargetEndpoint:    gatewayV1URL() + "/chat/completions",
				FailoverEndpoint:  ollamaFailoverEndpoint,
				EngineID:          "litellm-gateway",
				Model:             "qwen2.5-coder:14b",
				VirtualUsers:      perf.VirtualUsers(load.VUs),
				Duration:          load.Duration,
				Prompt:            "Benchmark gateway cascade recovery under fault injection",
				MaxTokens:         128,
				FaultInjection:    fault,
				ThermalMonitoring: true,
			})
			if err != nil {
				return err
			}
			return writeScorecard(load, perf.FormatMarkdownScorecard(summary), out)
		},
	}
	cmd.Flags().StringVar(&faultStr, "fault", "none", "Injected fault: none, 429 (rate-limit-429) or 503 (node-glitch-503)")
	addBenchLoadFlags(cmd, &load, 10, 20*time.Second)
	return cmd
}

func newVelocityMatrixCmd() *cobra.Command {
	var load benchLoad
	cmd := &cobra.Command{
		Use:   "matrix",
		Short: "Benchmark every registered engine and compare them in one table",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := load.validate(); err != nil {
				return err
			}
			return runVelocityMatrix(cmd.Context(), load, cmd.OutOrStdout())
		},
	}
	addBenchLoadFlags(cmd, &load, 5, 10*time.Second)
	return cmd
}

func runVelocityMatrix(ctx context.Context, load benchLoad, out io.Writer) error {
	fmt.Fprintf(out, "[+] Initializing Multi-Engine Benchmark Matrix (VUs: %d, Duration: %v per engine)...\n", load.VUs, load.Duration)
	var summaries []*perf.BenchmarkSummary
	for _, eng := range inference.DefaultManager().ListEngines() {
		summary, err := runBenchmark(ctx, perf.BenchmarkConfig{
			Mode:              perf.ModeMatrix,
			TargetEndpoint:    eng.Config().Endpoint,
			EngineID:          eng.ID(),
			Model:             "matrix-eval",
			VirtualUsers:      perf.VirtualUsers(load.VUs),
			Duration:          load.Duration,
			ThermalMonitoring: true,
			PreFlightCascade:  true,
			CascadeFallbackID: "ollama-metal",
			FailoverEndpoint:  ollamaFailoverEndpoint,
			Prompt:            "Benchmark matrix comparison prompt",
			MaxTokens:         128,
		})
		if err != nil {
			fmt.Fprintf(out, "[!] Skipped %s: %v\n", eng.ID(), err)
			continue
		}
		summaries = append(summaries, summary)
	}
	return writeScorecard(load, formatMatrix(summaries), out)
}

func formatMatrix(summaries []*perf.BenchmarkSummary) string {
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
		fmt.Fprintf(&sb, "| `%s` | `%s` | `%.2f tok/s` | `%.2f tok/s` | `%.2f%%` | `%s` |\n",
			s.EngineID, s.TTFTPercentiles.P50.Round(time.Millisecond), float64(s.AvgTPS), float64(s.PeakTPS), s.ErrorRatePercent, failoverStr)
	}
	return sb.String()
}

func newVelocityStressCmd() *cobra.Command {
	var vus, spends int
	cmd := &cobra.Command{
		Use:   "stress",
		Short: "Concurrency stress test of the sliding-window budget limiter",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if vus <= 0 || spends <= 0 {
				return usageErrorf("--vus and --spends must be positive")
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "[+] Executing Budget Limiter Concurrency Stress Test (%d VUs, %d spends)...\n", vus, spends)
			res, err := perf.StressBudgetLimiter(cmd.Context(), perf.GatewayStressConfig{
				VirtualUsers: perf.VirtualUsers(vus),
				TotalSpends:  spends,
			})
			if err != nil {
				return fmt.Errorf("stress test: %w", err)
			}
			if globals.JSON {
				return writeJSON(out, res)
			}
			fmt.Fprintf(out, "[✓] Stress Test Completed in %v (%.2f ops/sec, Race Detected: %v)\n",
				res.Duration, res.OpsPerSec, res.RaceDetected)
			fmt.Fprintf(out, "    - Allowed Spends:  %d\n", res.TotalAllowed)
			fmt.Fprintf(out, "    - Exceeded Limits: %d\n", res.TotalExceeded)
			return nil
		},
	}
	cmd.Flags().IntVar(&vus, "vus", 20, "Concurrent virtual users")
	cmd.Flags().IntVar(&spends, "spends", 500, "Total spend operations")
	return cmd
}
