package perf

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// CalculatePercentiles calculates min, max, avg and p50, p90, p95, p99 for a slice of durations.
func CalculatePercentiles(samples []time.Duration) LatencyPercentiles {
	n := len(samples)
	if n == 0 {
		return LatencyPercentiles{}
	}

	sorted := make([]time.Duration, n)
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var total time.Duration
	for _, d := range sorted {
		total += d
	}

	return LatencyPercentiles{
		Min: sorted[0],
		P50: percentile(sorted, 0.50),
		P90: percentile(sorted, 0.90),
		P95: percentile(sorted, 0.95),
		P99: percentile(sorted, 0.99),
		Max: sorted[n-1],
		Avg: total / time.Duration(n),
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// FormatMarkdownScorecard formats a BenchmarkSummary as a GitHub Markdown table.
func FormatMarkdownScorecard(s *BenchmarkSummary) string {
	if s == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Benchmark Scorecard: %s\n\n", s.RunID))
	sb.WriteString(fmt.Sprintf("- **Engine / Model**: `%s` (`%s`)\n", s.EngineID, s.Model))
	sb.WriteString(fmt.Sprintf("- **Mode**: `%s` | **Virtual Users**: `%d` | **Duration**: `%s`\n", s.Mode, s.VirtualUsers, s.TotalDuration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("- **Total Requests**: `%d` (Success: `%d`, Failed: `%d`, Error Rate: `%.2f%%`)\n", s.TotalRequests, s.SuccessfulReqs, s.FailedReqs, s.ErrorRatePercent))
	sb.WriteString("\n## 1. Latency & Generation Throughput\n\n")

	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|:---|:---|\n")
	sb.WriteString(fmt.Sprintf("| **TTFT (p50)** | `%s` |\n", s.TTFTPercentiles.P50.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| **TTFT (p95)** | `%s` |\n", s.TTFTPercentiles.P95.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| **TTFT (p99)** | `%s` |\n", s.TTFTPercentiles.P99.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| **TTFT (Avg / Min / Max)** | `%s / %s / %s` |\n", s.TTFTPercentiles.Avg.Round(time.Millisecond), s.TTFTPercentiles.Min.Round(time.Millisecond), s.TTFTPercentiles.Max.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| **Avg Token Velocity (TPS)** | `%.2f tok/s` |\n", float64(s.AvgTPS)))
	sb.WriteString(fmt.Sprintf("| **Peak Token Velocity (TPS)** | `%.2f tok/s` |\n", float64(s.PeakTPS)))
	sb.WriteString(fmt.Sprintf("| **Prompt Ingestion Speed** | `%.2f tok/s` |\n", s.PromptTokPerSec))

	sb.WriteString("\n## 2. Hardware Thermals & Throttling\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|:---|:---|\n")
	sb.WriteString(fmt.Sprintf("| **Peak SoC Temperature** | `%.1f °C` |\n", s.ThermalMetrics.PeakSoCTempC))
	sb.WriteString(fmt.Sprintf("| **Avg SoC Temperature** | `%.1f °C` |\n", s.ThermalMetrics.AvgSoCTempC))
	throttleStatus := "None (Optimal)"
	if s.ThermalMetrics.ThrottleDetected {
		throttleStatus = fmt.Sprintf("⚠️ Throttle Detected: %s", s.ThermalMetrics.ThrottleWarning)
	}
	sb.WriteString(fmt.Sprintf("| **Throttling Status** | `%s` |\n", throttleStatus))

	if s.FailoverMetrics != nil {
		sb.WriteString("\n## 3. Cascade Failover Resilience\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|:---|:---|\n")
		sb.WriteString(fmt.Sprintf("| **Injected Faults** | `%d` |\n", s.FailoverMetrics.InjectedFaults))
		sb.WriteString(fmt.Sprintf("| **Failover Triggered** | `%v` |\n", s.FailoverMetrics.FailoverTriggered))
		if s.FailoverMetrics.PreFlightSwitched {
			sb.WriteString("| **Pre-Flight Cascade** | `Switched (Zero Prompt Loss)` |\n")
			sb.WriteString(fmt.Sprintf("| **Primary Engine (Offline)** | `%s` |\n", s.FailoverMetrics.PrimaryEngineID))
			sb.WriteString(fmt.Sprintf("| **Cascaded Fallback Engine** | `%s` |\n", s.FailoverMetrics.CascadedEngineID))
			if s.FailoverMetrics.CascadeReason != "" {
				sb.WriteString(fmt.Sprintf("| **Cascade Reason** | `%s` |\n", s.FailoverMetrics.CascadeReason))
			}
		}
		sb.WriteString(fmt.Sprintf("| **Failover Switch Latency** | `%s` |\n", s.FailoverMetrics.FailoverLatency.Round(time.Millisecond)))
		sb.WriteString(fmt.Sprintf("| **Lost Requests** | `%d` |\n", s.FailoverMetrics.LostRequests))
		sb.WriteString(fmt.Sprintf("| **Successful Fallback Invocations** | `%d` |\n", s.FailoverMetrics.SuccessAfterFail))

		if len(s.FailoverMetrics.SwitchEvents) > 0 {
			sb.WriteString("\n### Failover Switch Events\n\n")
			for i, ev := range s.FailoverMetrics.SwitchEvents {
				sb.WriteString(fmt.Sprintf("%d. `%s` -> `%s` (Latency: `%v`): %s\n",
					i+1, ev.FromEngine, ev.ToEngine, ev.Latency.Round(time.Millisecond), ev.Reason))
			}
		}
	}

	return sb.String()
}

// ToJSON serializes the summary to formatted JSON.
func (s *BenchmarkSummary) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}
