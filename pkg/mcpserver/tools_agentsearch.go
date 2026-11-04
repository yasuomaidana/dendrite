package mcpserver

import (
	"context"
	"fmt"

	"Dendrite/pkg/agentsearch"
)

func registerAgentSearchTools(s *Server, deps Dependencies) {
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
	}, agentSearchHandler())
}

// agentSearchHandler handles the dendrite_agent_search tool.
func agentSearchHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		qStr, _ := args["query"].(string)
		if qStr == "" {
			return nil, fmt.Errorf("query is required")
		}
		targetPkg, _ := args["target_package"].(string)
		requireLocal, _ := args["require_local"].(bool)
		maxCost, _ := args["max_cost"].(float64)
		routeMode, _ := args["route"].(bool)

		query := agentsearch.NewQueryBuilder().
			WithText(qStr).
			WithTargetPackage(targetPkg).
			WithTargetSymbols(stringList(args["target_symbols"])...).
			RequireLocalInference(requireLocal).
			WithMaxCost(maxCost).
			WithMaxResults(int(floatArgOr(args, "max_results", 5))).
			Build()

		// Ranked by the dendrite-rag sidecar
		engine := agentsearch.NewSearchEngine(nil, nil, nil)
		if routeMode {
			decision, err := engine.Route(ctx, query)
			if err != nil {
				return nil, fmt.Errorf("agent routing failed: %w", err)
			}
			return textResult(string(indentJSON(decision))), nil
		}
		results, err := engine.Search(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("agent search failed: %w", err)
		}
		return textResult(string(indentJSON(results))), nil
	}
}
