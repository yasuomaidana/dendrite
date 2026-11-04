package codetools

import (
	"Dendrite/pkg/query/fuse"
	"Dendrite/pkg/query/reuse"
	"Dendrite/pkg/query/slice"
	"Dendrite/pkg/query/swap"
	"Dendrite/pkg/query/transpile"
	"Dendrite/pkg/statedir"
	"context"
	"fmt"

	. "Dendrite/pkg/mcpserver"
)

// sliceRunner computes a reachability slice; tests give the query tool a runner with a prepared answer.
type sliceRunner interface {
	Slice(ctx context.Context, req slice.SliceRequest) (*slice.SliceResult, error)
}

var DefaultMCPQuerySlicer sliceRunner = slice.NewIntentSlicer()
var defaultMCPSwapper = swap.NewSubsystemSwapper()
var defaultMCPTranspiler = transpile.NewRustTranspiler()
var defaultMCPFuser = fuse.NewArchitectureFuser()
var defaultMCPReuseAuditor = reuse.NewAuditor(nil, nil, statedir.Resolve(".dendrite/pool.json"))
var DefaultMCPRefMgr = reuse.NewRefManager(".dendrite/refs.json")

func registerQueryTools(s *Server, deps Dependencies) {
	// 31. dendrite_query_slice
	s.RegisterTool(Tool{
		Name:        "dendrite_query_slice",
		Description: "Universal intent-driven CFG/AST reachability slicer pruning dead features and generating golden I/O vectors",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sources_path":      TypedProp("string", "Target source directory or binary path"),
				"entrypoint_symbol": TypedProp("string", "Root function or method symbol for reachability traversal"),
				"intent":            TypedProp("string", "Natural language architectural intent for extraction"),
				"direction":         map[string]interface{}{"type": "string", "enum": []string{"forward", "backward", "bidirectional"}, "description": "Traversal direction"},
				"output_dir":        TypedProp("string", "Optional output path to emit debloated project & golden tests"),
			},
			"required": []interface{}{"sources_path", "entrypoint_symbol"},
		},
	}, querySliceHandler())

	// 32. dendrite_query_swap
	s.RegisterTool(Tool{
		Name:        "dendrite_query_swap",
		Description: "Transforms legacy graphics/IO subsystems into clean trait boundaries and modern adapters (wgpu/threejs) while verifying kinematic parity",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sources_path":    TypedProp("string", "Path to legacy application sources"),
				"from_subsystem":  map[string]interface{}{"type": "string", "enum": []string{"opengl", "directx", "vulkan", "wgpu", "threejs"}, "description": "Current legacy subsystem"},
				"to_subsystem":    map[string]interface{}{"type": "string", "enum": []string{"opengl", "directx", "vulkan", "wgpu", "threejs"}, "description": "Target modern subsystem"},
				"target_language": map[string]interface{}{"type": "string", "enum": []string{"rust", "go", "typescript"}, "description": "Target implementation language"},
				"output_dir":      TypedProp("string", "Optional destination directory to emit trait boundary & adapter"),
			},
			"required": []interface{}{"sources_path", "from_subsystem", "to_subsystem"},
		},
	}, querySwapHandler())

	// 33. dendrite_query_transpile
	s.RegisterTool(Tool{
		Name:        "dendrite_query_transpile",
		Description: "Translates legacy binaries or repos to safe, idiomatic Rust with Newtypes, zero unsafe blocks, and differential tests",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sources_artifact":    TypedProp("string", "Target compiled binary or repository path"),
				"target_language":     TypedProp("string", "Target language (default: rust)"),
				"no_std":              TypedProp("boolean", "Generate no_std compatible embedded crate"),
				"audit_library_reuse": TypedProp("boolean", "Enforce 3-tier library reuse hierarchy"),
				"prompt":              TypedProp("string", "Natural language architectural guidance"),
				"output_dir":          TypedProp("string", "Destination directory for synthesized crate"),
			},
			"required": []interface{}{"sources_artifact"},
		},
	}, queryTranspileHandler())

	// 34. dendrite_query_fuse
	s.RegisterTool(Tool{
		Name:        "dendrite_query_fuse",
		Description: "Combines multiple binaries/projects, deduplicating libraries and synthesizing zero-copy in-process channels",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sources":           map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "List of source project or artifact paths"},
				"fused_member_name": TypedProp("string", "Identifier for the combined member"),
				"target_language":   map[string]interface{}{"type": "string", "enum": []string{"rust", "go"}, "description": "Target unified language"},
				"target_directory":  TypedProp("string", "Destination directory for unified project"),
				"register_in_pool":  TypedProp("boolean", "Register unified member in .dendrite/pool.json"),
			},
			"required": []interface{}{"sources", "fused_member_name", "target_directory"},
		},
	}, queryFuseHandler())
	// 35. dendrite_query_reuse_audit
	s.RegisterTool(Tool{
		Name:        "dendrite_query_reuse_audit",
		Description: "Evaluates strict 3-tier library reuse hierarchy (Source Refs > Pool Members > Ecosystem Registries) and emits an Architecture Decision Record (ADR)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"capability": TypedProp("string", "Capability or domain required (e.g. linear_algebra, serialization, kinematics)"),
				"language":   TypedProp("string", "Target programming language (e.g. rust, go, typescript, python)"),
				"ticket_id":  TypedProp("string", "Associated Plane ticket identifier (e.g. DEN-058)"),
				"intent":     TypedProp("string", "Contextual intent or user prompt description"),
			},
			"required": []interface{}{"capability"},
		},
	}, queryReuseAuditHandler())

	// 36. dendrite_ref_list
	s.RegisterTool(Tool{
		Name:        "dendrite_ref_list",
		Description: "Lists registered Tier 1 source references for library reuse",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, refListHandler())

	// 37. dendrite_ref_add
	s.RegisterTool(Tool{
		Name:        "dendrite_ref_add",
		Description: "Registers a new Tier 1 source reference repository for library reuse",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":           TypedProp("string", "Unique reference ID (e.g. ref-custom)"),
				"name":         TypedProp("string", "Repository or library name"),
				"repository":   TypedProp("string", "Git repository URL or local path"),
				"language":     TypedProp("string", "Primary language"),
				"capabilities": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "List of capability tags"},
				"license":      TypedProp("string", "Open source or proprietary license"),
			},
			"required": []interface{}{"id", "repository"},
		},
	}, refAddHandler())
}

// querySliceHandler handles the dendrite_query_slice tool.
func querySliceHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

		result, err := DefaultMCPQuerySlicer.Slice(ctx, req)
		if err != nil {
			return ErrorResult("[-] Slicing failed: %v", err), nil
		}

		data := IndentJSON(result)

		return TextResult(string(data)), nil
	}
}

// querySwapHandler handles the dendrite_query_swap tool.
func querySwapHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
			return ErrorResult("[-] Subsystem swap failed: %v", err), nil
		}

		data := IndentJSON(result)

		return TextResult(string(data)), nil
	}
}

// queryTranspileHandler handles the dendrite_query_transpile tool.
func queryTranspileHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
			return ErrorResult("[-] Transpilation failed: %v", err), nil
		}

		data := IndentJSON(result)

		return TextResult(string(data)), nil
	}
}

// queryFuseHandler handles the dendrite_query_fuse tool.
func queryFuseHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		rawSources, ok := args["sources"].([]interface{})
		if !ok || len(rawSources) < 2 {
			return nil, fmt.Errorf("sources must contain at least two entries")
		}
		var sources []string
		for _, s := range rawSources {
			if str, ok := s.(string); ok {
				sources = append(sources, str)
			}
		}

		memberName, _ := args["fused_member_name"].(string)
		targetLang, _ := args["target_language"].(string)
		targetDir, _ := args["target_directory"].(string)
		registerPool, _ := args["register_in_pool"].(bool)

		req := fuse.FuseRequest{
			Sources:         sources,
			FusedMemberName: memberName,
			TargetLanguage:  targetLang,
			TargetDirectory: targetDir,
			RegisterInPool:  registerPool,
		}

		result, err := defaultMCPFuser.Fuse(ctx, req)
		if err != nil {
			return ErrorResult("[-] Architecture fusion failed: %v", err), nil
		}

		data := IndentJSON(result)

		return TextResult(string(data)), nil
	}
}

// queryReuseAuditHandler handles the dendrite_query_reuse_audit tool.
func queryReuseAuditHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		cap, _ := args["capability"].(string)
		lang, _ := args["language"].(string)
		ticket, _ := args["ticket_id"].(string)
		intent, _ := args["intent"].(string)

		req := reuse.AuditRequest{
			Capability: cap,
			Language:   lang,
			TicketID:   ticket,
			Intent:     intent,
		}

		res, err := defaultMCPReuseAuditor.AuditReuse(ctx, req)
		if err != nil {
			return ErrorResult("[-] 3-Tier library reuse audit failed: %v", err), nil
		}

		data := IndentJSON(res)

		return TextResult(string(data)), nil
	}
}

// refListHandler handles the dendrite_ref_list tool.
func refListHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		refs, err := DefaultMCPRefMgr.ListRefs()
		if err != nil {
			return ErrorResult("[-] Failed to list refs: %v", err), nil
		}

		data := IndentJSON(refs)

		return TextResult(string(data)), nil
	}
}

// refAddHandler handles the dendrite_ref_add tool.
func refAddHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		id, _ := args["id"].(string)
		name, _ := args["name"].(string)
		repo, _ := args["repository"].(string)
		lang, _ := args["language"].(string)
		lic, _ := args["license"].(string)
		if name == "" {
			name = id
		}

		var caps []string
		if rawCaps, ok := args["capabilities"].([]interface{}); ok {
			for _, c := range rawCaps {
				if str, ok := c.(string); ok {
					caps = append(caps, str)
				}
			}
		}

		ref := reuse.SourceReference{
			ID:           id,
			Name:         name,
			Repository:   repo,
			Language:     lang,
			Capabilities: caps,
			License:      lic,
		}

		if err := DefaultMCPRefMgr.AddRef(ref); err != nil {
			return ErrorResult("[-] Failed to register ref: %v", err), nil
		}

		return TextResult(fmt.Sprintf("[+] Successfully registered Tier 1 source reference %s (%s)", name, repo)), nil
	}
}
