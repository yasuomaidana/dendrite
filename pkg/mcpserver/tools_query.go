package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/query/slice"
)

var defaultMCPQuerySlicer = slice.NewIntentSlicer()

func (s *Server) registerQueryTools(deps Dependencies) {
	// 31. dendrite_query_slice
	s.RegisterTool(Tool{
		Name:        "dendrite_query_slice",
		Description: "Universal intent-driven CFG/AST reachability slicer pruning dead features and generating golden I/O vectors",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sources_path":      map[string]interface{}{"type": "string", "description": "Target source directory or binary path"},
				"entrypoint_symbol": map[string]interface{}{"type": "string", "description": "Root function or method symbol for reachability traversal"},
				"intent":            map[string]interface{}{"type": "string", "description": "Natural language architectural intent for extraction"},
				"direction":         map[string]interface{}{"type": "string", "enum": []string{"forward", "backward", "bidirectional"}, "description": "Traversal direction"},
				"output_dir":        map[string]interface{}{"type": "string", "description": "Optional output path to emit debloated project & golden tests"},
			},
			"required": []interface{}{"sources_path", "entrypoint_symbol"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		sourcesPath, _ := args["sources_path"].(string)
		entrypoint, _ := args["entrypoint_symbol"].(string)
		intent, _ := args["intent"].(string)
		directionStr, _ := args["direction"].(string)
		outputDir, _ := args["output_dir"].(string)

		req := slice.SliceRequest{
			SourcesPath:      sourcesPath,
			EntryPointSymbol: entrypoint,
			Intent:           intent,
			Direction:        slice.SliceDirection(directionStr),
			OutputDir:        outputDir,
		}

		result, err := defaultMCPQuerySlicer.Slice(ctx, req)
		if err != nil {
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: fmt.Sprintf("[-] Slicing failed: %v", err),
				}},
				IsError: true,
			}, nil
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling slice result: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	})
}
