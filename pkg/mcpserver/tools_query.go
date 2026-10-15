package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/query/slice"
	"Dendrite/pkg/query/swap"
	"Dendrite/pkg/query/transpile"
)

var defaultMCPQuerySlicer = slice.NewIntentSlicer()
var defaultMCPSwapper = swap.NewSubsystemSwapper()
var defaultMCPTranspiler = transpile.NewRustTranspiler()

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

	// 32. dendrite_query_swap
	s.RegisterTool(Tool{
		Name:        "dendrite_query_swap",
		Description: "Transforms legacy graphics/IO subsystems into clean trait boundaries and modern adapters (wgpu/threejs) while verifying kinematic parity",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sources_path":    map[string]interface{}{"type": "string", "description": "Path to legacy application sources"},
				"from_subsystem":  map[string]interface{}{"type": "string", "enum": []string{"opengl", "directx", "vulkan", "wgpu", "threejs"}, "description": "Current legacy subsystem"},
				"to_subsystem":    map[string]interface{}{"type": "string", "enum": []string{"opengl", "directx", "vulkan", "wgpu", "threejs"}, "description": "Target modern subsystem"},
				"target_language": map[string]interface{}{"type": "string", "enum": []string{"rust", "go", "typescript"}, "description": "Target implementation language"},
				"output_dir":      map[string]interface{}{"type": "string", "description": "Optional destination directory to emit trait boundary & adapter"},
			},
			"required": []interface{}{"sources_path", "from_subsystem", "to_subsystem"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		sourcesPath, _ := args["sources_path"].(string)
		fromStr, _ := args["from_subsystem"].(string)
		toStr, _ := args["to_subsystem"].(string)
		lang, _ := args["target_language"].(string)
		outputDir, _ := args["output_dir"].(string)

		req := swap.SwapRequest{
			SourcesPath:    sourcesPath,
			FromSubsystem:  swap.SubsystemType(fromStr),
			ToSubsystem:    swap.SubsystemType(toStr),
			TargetLanguage: lang,
			OutputDir:      outputDir,
		}

		result, err := defaultMCPSwapper.Swap(ctx, req)
		if err != nil {
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: fmt.Sprintf("[-] Subsystem swap failed: %v", err),
				}},
				IsError: true,
			}, nil
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling swap result: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	})

	// 33. dendrite_query_transpile
	s.RegisterTool(Tool{
		Name:        "dendrite_query_transpile",
		Description: "Translates legacy binaries or repos to safe, idiomatic Rust with Newtypes, zero unsafe blocks, and differential tests",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sources_artifact":    map[string]interface{}{"type": "string", "description": "Target compiled binary or repository path"},
				"target_language":     map[string]interface{}{"type": "string", "description": "Target language (default: rust)"},
				"no_std":              map[string]interface{}{"type": "boolean", "description": "Generate no_std compatible embedded crate"},
				"audit_library_reuse": map[string]interface{}{"type": "boolean", "description": "Enforce 3-tier library reuse hierarchy"},
				"prompt":              map[string]interface{}{"type": "string", "description": "Natural language architectural guidance"},
				"output_dir":          map[string]interface{}{"type": "string", "description": "Destination directory for synthesized crate"},
			},
			"required": []interface{}{"sources_artifact"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		artifact, _ := args["sources_artifact"].(string)
		targetLang, _ := args["target_language"].(string)
		noStd, _ := args["no_std"].(bool)
		auditReuse, _ := args["audit_library_reuse"].(bool)
		prompt, _ := args["prompt"].(string)
		outputDir, _ := args["output_dir"].(string)

		req := transpile.TranspileRequest{
			SourcesArtifact:   artifact,
			TargetLanguage:    targetLang,
			NoStd:             noStd,
			AuditLibraryReuse: auditReuse,
			Prompt:            prompt,
			OutputDir:         outputDir,
		}

		result, err := defaultMCPTranspiler.Transpile(ctx, req)
		if err != nil {
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: fmt.Sprintf("[-] Transpilation failed: %v", err),
				}},
				IsError: true,
			}, nil
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling transpile result: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	})
}
