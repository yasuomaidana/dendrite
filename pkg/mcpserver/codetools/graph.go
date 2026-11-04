package codetools

import (
	"Dendrite/pkg/rag"
	"context"
	"fmt"
	"strings"

	. "Dendrite/pkg/mcpserver"
)

func registerGraphTools(s *Server, deps Dependencies) {
	// dendrite_graph_communities
	AddTool(s, "dendrite_graph_communities", "List Leiden communities of the project file graph, largest first, with top symbols and summary", PropsSchema(map[string]interface{}{
		"project_path": TypedProp("string", "Target project directory to index and analyze (default: '.')"),
		"limit":        TypedProp("integer", "Maximum number of communities to return (default: 50)"),
		"min_size":     TypedProp("integer", "Smallest community listed, in files (default: 2)"),
	}), graphCommunitiesHandler())

	// dendrite_graph_god_nodes
	AddTool(s, "dendrite_graph_god_nodes", "List god nodes (symbols with the most callers and type users) in the project code graph", PropsSchema(map[string]interface{}{
		"project_path":  TypedProp("string", "Target project directory to index and analyze (default: '.')"),
		"min_in_degree": TypedProp("integer", "Minimum in-degree threshold (default: 21 for FEAT-045 god nodes)"),
		"limit":         TypedProp("integer", "Maximum number of god nodes to return (default: 50)"),
	}), graphGodNodesHandler())

	// dendrite_graph_path
	s.RegisterTool(Tool{
		Name:        "dendrite_graph_path",
		Description: "Find shortest path between two symbols or files in the code graph",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": TypedProp("string", "Target project directory to index and analyze (default: '.')"),
				"from":         TypedProp("string", "Starting symbol name or project-relative file path"),
				"to":           TypedProp("string", "Destination symbol name or project-relative file path"),
				"kinds": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Edge kinds to follow (call, import, type; default: all)",
				},
				"max_hops": TypedProp("integer", "Maximum number of hops (default: 6)"),
			},
			"required": []interface{}{"from", "to"},
		},
	}, graphPathHandler())

	// dendrite_graph_explain
	s.RegisterTool(Tool{
		Name:        "dendrite_graph_explain",
		Description: "Explain a symbol or file: callers, callees, definitions, imports, and community membership",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": TypedProp("string", "Target project directory to index and analyze (default: '.')"),
				"target":       TypedProp("string", "Symbol name or project-relative file path to explain"),
			},
			"required": []interface{}{"target"},
		},
	}, graphExplainHandler())
}

// graphCommunitiesHandler handles the dendrite_graph_communities tool.
func graphCommunitiesHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}
		limit := PositiveIntArg(args, "limit")
		minSize := PositiveIntArg(args, "min_size")

		communities, err := rag.NewRetriever().Communities(ctx, projectPath, rag.CommunitiesOptions{
			Limit:   limit,
			MinSize: minSize,
		})
		if err != nil {
			return nil, fmt.Errorf("graph communities failed for %s: %w", projectPath, err)
		}

		data := IndentJSON(communities)
		return TextResult(string(data)), nil
	}
}

// graphGodNodesHandler handles the dendrite_graph_god_nodes tool.
func graphGodNodesHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		projectPath := "."
		if p, ok := args["project_path"].(string); ok && p != "" {
			projectPath = p
		}
		minInDegree := PositiveIntArg(args, "min_in_degree")
		limit := PositiveIntArg(args, "limit")

		nodes, err := rag.NewRetriever().GodNodes(ctx, projectPath, rag.GodNodesOptions{
			MinInDegree: minInDegree,
			Limit:       limit,
		})
		if err != nil {
			return nil, fmt.Errorf("graph god nodes failed for %s: %w", projectPath, err)
		}

		data := IndentJSON(nodes)
		return TextResult(string(data)), nil
	}
}

// kindsArg reads the edge kinds of a path call: a JSON array or a Go string slice, empty items dropped.
func kindsArg(raw interface{}) []string {
	if kinds, ok := raw.([]string); ok {
		return kinds
	}
	return NonEmptyStrings(raw)
}

// graphPathHandler handles the dendrite_graph_path tool.
func graphPathHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		from, _ := args["from"].(string)
		to, _ := args["to"].(string)
		if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
			return nil, fmt.Errorf("from and to are required")
		}
		projectPath := StringArgOr(args, "project_path", ".")
		path, err := rag.NewRetriever().Path(ctx, projectPath, from, to, rag.PathOptions{
			Kinds:   kindsArg(args["kinds"]),
			MaxHops: PositiveIntArg(args, "max_hops"),
		})
		if err != nil {
			return nil, fmt.Errorf("graph path failed for %s: %w", projectPath, err)
		}
		return TextResult(string(IndentJSON(path))), nil
	}
}

// graphExplainHandler handles the dendrite_graph_explain tool.
func graphExplainHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

		data := IndentJSON(matches)
		return TextResult(string(data)), nil
	}
}
