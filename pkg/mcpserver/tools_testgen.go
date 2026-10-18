package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/testgen"
)

func registerTestGenTools(s *Server, deps Dependencies) {
	// 25. dendrite_test_auto
	s.RegisterTool(Tool{
		Name:        "dendrite_test_auto",
		Description: "Analyzes AST symbol boundaries, detects persistence dependencies (PostgreSQL, Redis, SQLite), scaffolds Testcontainers environments, and executes a self-healing test convergence loop (<= 3 iterations) achieving >= 85% coverage",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"file"},
			"properties": map[string]interface{}{
				"file": map[string]interface{}{
					"type":        "string",
					"description": "Path to target source file (e.g. 'pkg/gateway/limiter.go')",
				},
				"with_testcontainers": map[string]interface{}{
					"type":        "boolean",
					"description": "Force Testcontainers scaffolding generation even if no persistent dependencies detected",
				},
				"max_iterations": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum self-healing convergence iterations (default: 3)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		filePath, _ := args["file"].(string)
		if filePath == "" {
			return nil, fmt.Errorf("file parameter is required")
		}

		withContainers, _ := args["with_testcontainers"].(bool)
		maxIter := 3
		if mi, ok := args["max_iterations"].(float64); ok && mi > 0 {
			maxIter = int(mi)
		}

		runner := testgen.NewSelfHealingRunner(testgen.IterationCount(maxIter))
		cfg := testgen.TestSynthesisConfig{
			SourceFile:           filePath,
			WithTestcontainers:   withContainers,
			MaxHealingIterations: testgen.IterationCount(maxIter),
			TargetCoverage:       85.0,
		}

		report, err := runner.Run(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("test synthesis error: %w", err)
		}

		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})
}
