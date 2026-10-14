package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/perf"
	"Dendrite/pkg/telemetry"
)

func (s *Server) registerPerfTools(deps Dependencies) {
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
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		modeStr, _ := args["mode"].(string)
		if modeStr == "" {
			modeStr = "inference"
		}
		mode := perf.BenchmarkMode(strings.ToLower(modeStr))

		engineID, _ := args["engine_id"].(string)
		if engineID == "" {
			engineID = "oxillama"
		}
		model, _ := args["model"].(string)
		if model == "" {
			model = "qwen2.5-coder-7b"
		}

		targetURL, _ := args["endpoint"].(string)
		if targetURL == "" {
			mgr := inference.DefaultManager()
			if eng, err := mgr.GetEngine(engineID); err == nil {
				targetURL = eng.Config().Endpoint
			} else {
				targetURL = "http://localhost:8080/v1/chat/completions"
			}
		}

		failoverURL, _ := args["failover_endpoint"].(string)
		vusFloat, _ := args["virtual_users"].(float64)
		vus := perf.VirtualUsers(vusFloat)
		if vus <= 0 {
			vus = 5
		}

		durFloat, _ := args["duration_seconds"].(float64)
		durSec := int(durFloat)
		if durSec <= 0 {
			durSec = 5
		}
		duration := time.Duration(durSec) * time.Second

		faultStr, _ := args["fault_injection"].(string)
		fault := perf.FaultNone
		if faultStr == "rate-limit-429" {
			fault = perf.FaultRateLimit429
		} else if faultStr == "node-glitch-503" {
			fault = perf.FaultNodeGlitch503
		}

		if mode == "stress" {
			res, err := perf.StressBudgetLimiter(ctx, perf.GatewayStressConfig{
				VirtualUsers: vus,
				TotalSpends:  int(vus) * 20,
			})
			if err != nil {
				return nil, fmt.Errorf("budget stress test failed: %w", err)
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return &CallToolResult{
				Content: []ContentItem{{Type: "text", Text: string(data)}},
			}, nil
		}

		cfg := perf.BenchmarkConfig{
			Mode:              mode,
			TargetEndpoint:    targetURL,
			FailoverEndpoint:  failoverURL,
			EngineID:          engineID,
			Model:             model,
			VirtualUsers:      vus,
			Duration:          duration,
			FaultInjection:    fault,
			ThermalMonitoring: true,
		}

		sensor := telemetry.NewMockThermalSensor(48.0)
		runner := perf.DefaultRunner(sensor)
		session := perf.NewSession(cfg)
		prov, err := session.Provision(runner)
		if err != nil {
			return nil, fmt.Errorf("session provisioning failed: %w", err)
		}

		completed, err := prov.Run(ctx)
		if err != nil {
			return nil, fmt.Errorf("benchmark execution failed: %w", err)
		}

		summary := completed.Summary()
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return nil, err
		}

		scorecard := perf.FormatMarkdownScorecard(summary)
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
				{Type: "text", Text: "\n" + scorecard},
			},
		}, nil
	})
}
