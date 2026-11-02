package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/rag"
)

func registerGraphTools(s *Server, deps Dependencies) {
	// dendrite_graph_communities
	s.RegisterTool(Tool{
		Name:        "dendrite_graph_communities",
		Description: "List Leiden communities of the project file graph, largest first, with top symbols and summary",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Target project directory to index and analyze (default: '.')",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of communities to return (default: 50)",
				},
				"min_size": map[string]interface{}{
					"type":        "integer",
					"description": "Smallest community listed, in files (default: 2)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}
		limit := positiveIntArg(args, "limit")
		minSize := positiveIntArg(args, "min_size")

		communities, err := rag.NewRetriever().Communities(ctx, projectPath, rag.CommunitiesOptions{
			Limit:   limit,
			MinSize: minSize,
		})
		if err != nil {
			return nil, fmt.Errorf("graph communities failed for %s: %w", projectPath, err)
		}

		data := indentJSON(communities)
		return textResult(string(data)), nil
	})

	// dendrite_graph_god_nodes
	s.RegisterTool(Tool{
		Name:        "dendrite_graph_god_nodes",
		Description: "List god nodes (symbols with the most callers and type users) in the project code graph",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Target project directory to index and analyze (default: '.')",
				},
				"min_in_degree": map[string]interface{}{
					"type":        "integer",
					"description": "Minimum in-degree threshold (default: 21 for FEAT-045 god nodes)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of god nodes to return (default: 50)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}
		minInDegree := positiveIntArg(args, "min_in_degree")
		limit := positiveIntArg(args, "limit")

		nodes, err := rag.NewRetriever().GodNodes(ctx, projectPath, rag.GodNodesOptions{
			MinInDegree: minInDegree,
			Limit:       limit,
		})
		if err != nil {
			return nil, fmt.Errorf("graph god nodes failed for %s: %w", projectPath, err)
		}

		data := indentJSON(nodes)
		return textResult(string(data)), nil
	})

	// dendrite_graph_path
	s.RegisterTool(Tool{
		Name:        "dendrite_graph_path",
		Description: "Find shortest path between two symbols or files in the code graph",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Target project directory to index and analyze (default: '.')",
				},
				"from": map[string]interface{}{
					"type":        "string",
					"description": "Starting symbol name or project-relative file path",
				},
				"to": map[string]interface{}{
					"type":        "string",
					"description": "Destination symbol name or project-relative file path",
				},
				"kinds": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Edge kinds to follow (call, import, type; default: all)",
				},
				"max_hops": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of hops (default: 6)",
				},
			},
			"required": []interface{}{"from", "to"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		from, _ := args["from"].(string)
		to, _ := args["to"].(string)
		if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
			return nil, fmt.Errorf("from and to are required")
		}

		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}

		var kinds []string
		if rawKinds, ok := args["kinds"].([]interface{}); ok {
			for _, rk := range rawKinds {
				if s, ok := rk.(string); ok && s != "" {
					kinds = append(kinds, s)
				}
			}
		} else if strKinds, ok := args["kinds"].([]string); ok {
			kinds = strKinds
		}

		maxHops := positiveIntArg(args, "max_hops")

		path, err := rag.NewRetriever().Path(ctx, projectPath, from, to, rag.PathOptions{
			Kinds:   kinds,
			MaxHops: maxHops,
		})
		if err != nil {
			return nil, fmt.Errorf("graph path failed for %s: %w", projectPath, err)
		}

		data := indentJSON(path)
		return textResult(string(data)), nil
	})

	// dendrite_graph_explain
	s.RegisterTool(Tool{
		Name:        "dendrite_graph_explain",
		Description: "Explain a symbol or file: callers, callees, definitions, imports, and community membership",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Target project directory to index and analyze (default: '.')",
				},
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Symbol name or project-relative file path to explain",
				},
			},
			"required": []interface{}{"target"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		target, _ := args["target"].(string)
		if strings.TrimSpace(target) == "" {
			return nil, fmt.Errorf("target is required")
		}

		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}

		matches, err := rag.NewRetriever().Explain(ctx, projectPath, target)
		if err != nil {
			return nil, fmt.Errorf("graph explain failed for %s: %w", projectPath, err)
		}

		data := indentJSON(matches)
		return textResult(string(data)), nil
	})
}
