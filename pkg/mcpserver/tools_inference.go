package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/inference"
)

func (s *Server) registerInferenceTools(deps Dependencies) {
	// dendrite_inference_migrate_model
	s.RegisterTool(Tool{
		Name:        "dendrite_inference_migrate_model",
		Description: "Migrate models from Ollama to OxiLLaMa pure-Rust inference engine and evict source blobs to reclaim memory",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Model identifier to migrate (e.g. 'qwen3.5:9b', 'qwen3.8:27b')",
				},
				"source_engine": map[string]interface{}{
					"type":        "string",
					"description": "Source engine identifier (default: 'ollama')",
				},
				"target_engine": map[string]interface{}{
					"type":        "string",
					"description": "Target engine identifier (default: 'oxillama')",
				},
				"delete_source": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, evicts model from source engine upon successful verification (default: true)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, simulates migration without modifying disk (default: false)",
				},
			},
			"required": []interface{}{"model"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		model, _ := args["model"].(string)
		if strings.TrimSpace(model) == "" {
			return nil, fmt.Errorf("model name is required")
		}
		sourceEngine, _ := args["source_engine"].(string)
		if sourceEngine == "" {
			sourceEngine = "ollama"
		}
		targetEngine, _ := args["target_engine"].(string)
		if targetEngine == "" {
			targetEngine = "oxillama"
		}
		deleteSource := true
		if ds, ok := args["delete_source"].(bool); ok {
			deleteSource = ds
		}
		dryRun, _ := args["dry_run"].(bool)

		migrator := inference.NewModelMigrator("", "", nil)
		res, err := migrator.Migrate(ctx, inference.ModelMigrationOptions{
			ModelName:    model,
			SourceEngine: sourceEngine,
			TargetEngine: targetEngine,
			DeleteSource: deleteSource,
			DryRun:       dryRun,
		})
		if err != nil {
			return nil, err
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})
}
