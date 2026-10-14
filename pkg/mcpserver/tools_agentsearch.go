package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/agentsearch"
)

func (s *Server) registerAgentSearchTools(deps Dependencies) {
	// 24. dendrite_agent_search
	s.RegisterTool(Tool{
		Name:        "dendrite_agent_search",
		Description: "Searches and routes tasks to optimal autonomous agents (Aider, OpenHands, OpenCode, Google AGY) using hybrid vector (TEI) and lexical (BM25) search with Reciprocal Rank Fusion, Graphify AST Leiden cluster coupling, and historical performance scoring",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"query"},
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Task description or capability search query",
				},
				"target_package": map[string]interface{}{
					"type":        "string",
					"description": "Optional target Go/Rust package (e.g. 'pkg/ast', 'pkg/gateway')",
				},
				"target_symbols": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional list of target AST symbols",
				},
				"require_local": map[string]interface{}{
					"type":        "boolean",
					"description": "Constrain selection to zero-cost local inference engines (Ollama Metal, OxiLLaMa)",
				},
				"max_cost": map[string]interface{}{
					"type":        "number",
					"description": "Optional maximum cost ceiling in USD",
				},
				"max_results": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of candidate agents to return (default: 5)",
				},
				"route": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, auto-selects top agent and returns execution routing decision",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		qStr, _ := args["query"].(string)
		if qStr == "" {
			return nil, fmt.Errorf("query is required")
		}

		targetPkg, _ := args["target_package"].(string)
		requireLocal, _ := args["require_local"].(bool)
		maxCost, _ := args["max_cost"].(float64)
		routeMode, _ := args["route"].(bool)

		maxResults := 5
		if mr, ok := args["max_results"].(float64); ok && mr > 0 {
			maxResults = int(mr)
		}

		var targetSymbols []string
		if rawSyms, ok := args["target_symbols"].([]interface{}); ok {
			for _, sym := range rawSyms {
				if s, ok := sym.(string); ok {
					targetSymbols = append(targetSymbols, s)
				}
			}
		}

		builder := agentsearch.NewQueryBuilder().
			WithText(qStr).
			WithTargetPackage(targetPkg).
			WithTargetSymbols(targetSymbols...).
			RequireLocalInference(requireLocal).
			WithMaxCost(maxCost).
			WithMaxResults(maxResults)

		query := builder.Build()

		// Construct engine with mock/TEI client
		engine := agentsearch.NewSearchEngine(nil, nil, nil)

		if routeMode {
			decision, err := engine.Route(ctx, query)
			if err != nil {
				return nil, fmt.Errorf("agent routing failed: %w", err)
			}
			data, err := json.MarshalIndent(decision, "", "  ")
			if err != nil {
				return nil, err
			}
			return &CallToolResult{
				Content: []ContentItem{{Type: "text", Text: string(data)}},
			}, nil
		}

		results, err := engine.Search(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("agent search failed: %w", err)
		}

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})
}
