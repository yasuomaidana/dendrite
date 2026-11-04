package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/perf"
	"Dendrite/pkg/telemetry"
)

func registerPerfTools(s *Server, deps Dependencies) {
	// 20. dendrite_bench_run
	s.RegisterTool(Tool{
		Name:        "dendrite_bench_run",
		Description: "Execute an automated k6 or synthetic performance benchmark measuring TTFT, TPS, gateway cascade failover latency, or sliding window budget concurrency",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{
					"type":        "string",
					"description": "Benchmark mode: 'inference', 'gateway', 'stress', 'matrix' (default: 'inference')",
				},
				"engine_id": map[string]interface{}{
					"type":        "string",
					"description": "Inference engine identifier (e.g. 'oxillama', 'ollama-metal', 'vllm-cluster')",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Model identifier (default: 'qwen2.5-coder-7b')",
				},
				"endpoint": map[string]interface{}{
					"type":        "string",
					"description": "Target endpoint URL (default: auto-resolved from registered engine)",
				},
				"failover_endpoint": map[string]interface{}{
					"type":        "string",
					"description": "Secondary endpoint URL for failover testing",
				},
				"virtual_users": map[string]interface{}{
					"type":        "integer",
					"description": "Number of concurrent virtual users (default: 5)",
				},
				"duration_seconds": map[string]interface{}{
					"type":        "integer",
					"description": "Benchmark duration in seconds (default: 10)",
				},
				"fault_injection": map[string]interface{}{
					"type":        "string",
					"description": "Simulated fault injection: 'none', 'rate-limit-429', 'node-glitch-503'",
				},
			},
		},
	}, benchRunHandler())
}

// benchTargetURL is the endpoint a benchmark hits: the one given, else the engine's own, else a local default.
func benchTargetURL(args map[string]interface{}, engineID string) string {
	if u, _ := args["endpoint"].(string); u != "" {
		return u
	}
	if eng, err := inference.DefaultManager().GetEngine(engineID); err == nil {
		return eng.Config().Endpoint
	}
	return "http://localhost:8080/v1/chat/completions"
}

// benchDuration is how long a benchmark runs: the seconds asked for, otherwise five.
func benchDuration(args map[string]interface{}) time.Duration {
	secs := positiveIntArg(args, "duration_seconds")
	if secs == 0 {
		secs = 5
	}
	return time.Duration(secs) * time.Second
}

// benchFault maps the fault_injection argument to a fault kind.
func benchFault(name string) perf.FaultInjectionType {
	switch name {
	case "rate-limit-429":
		return perf.FaultRateLimit429
	case "node-glitch-503":
		return perf.FaultNodeGlitch503
	}
	return perf.FaultNone
}

// benchRunHandler handles the dendrite_bench_run tool.
func benchRunHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		mode := perf.BenchmarkMode(strings.ToLower(stringArgOr(args, "mode", "inference")))
		engineID := stringArgOr(args, "engine_id", "oxillama")
		vus := perf.VirtualUsers(positiveIntArg(args, "virtual_users"))
		if vus <= 0 {
			vus = 5
		}
		if mode == "stress" {
			res, _ := perf.StressBudgetLimiter(ctx, perf.GatewayStressConfig{VirtualUsers: vus, TotalSpends: int(vus) * 20})
			return textResult(string(indentJSON(res))), nil
		}
		failoverURL, _ := args["failover_endpoint"].(string)
		faultStr, _ := args["fault_injection"].(string)
		session := perf.NewSession(perf.BenchmarkConfig{
			Mode:              mode,
			TargetEndpoint:    benchTargetURL(args, engineID),
			FailoverEndpoint:  failoverURL,
			EngineID:          engineID,
			Model:             stringArgOr(args, "model", "qwen2.5-coder-7b"),
			VirtualUsers:      vus,
			Duration:          benchDuration(args),
			FaultInjection:    benchFault(faultStr),
			ThermalMonitoring: true,
		})
		prov, _ := session.Provision(perf.DefaultRunner(telemetry.NewHostThermalSensor())) // fails only for a nil runner or an empty endpoint, and neither is possible here
		completed, err := prov.Run(ctx)
		if err != nil {
			return nil, fmt.Errorf("benchmark execution failed: %w", err)
		}
		summary := completed.Summary()
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(indentJSON(summary))},
				{Type: "text", Text: "\n" + perf.FormatMarkdownScorecard(summary)},
			},
		}, nil
	}
}
