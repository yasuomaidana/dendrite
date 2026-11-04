package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/distcodebase"
	"Dendrite/pkg/workspace"
)

var globalDistRegistry = distcodebase.NewDefaultRegistry()
var globalInteropCoordinator = distcodebase.NewInteropCoordinator(globalDistRegistry)

func registerDistCodebaseTools(s *Server, deps Dependencies) {
	// 16. dendrite_dist_sources
	s.RegisterTool(Tool{
		Name:        "dendrite_dist_sources",
		Description: "List or filter registered codebase sources (own-repo, curated-oss, registry) and libraries with priority order",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"priority": map[string]interface{}{"type": "integer", "description": "Filter by priority level (1 = own repo, 2 = curated OSS, 3 = registry)"},
				"language": map[string]interface{}{"type": "string", "description": "Filter by language (e.g. rust, go, typescript)"},
				"query":    map[string]interface{}{"type": "string", "description": "Capability or library name query (e.g. wgpu, cte, turso, pixels)"},
			},
		},
	}, distSourcesHandler())

	// 17. dendrite_dist_decompose
	s.RegisterTool(Tool{
		Name:        "dendrite_dist_decompose",
		Description: "Forcibly decompose target project or REA evidence into reusable modules and map ecosystem libraries",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"project_name"},
			"properties": map[string]interface{}{
				"project_name":          map[string]interface{}{"type": "string", "description": "Name of the project to decompose"},
				"target_binary_or_spec": map[string]interface{}{"type": "string", "description": "Target binary path or specification file"},
				"language":              map[string]interface{}{"type": "string", "description": "Target language (default: rust)"},
				"modules":               map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Custom module names if overriding default modular split"},
				"user_custom_overrides": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Modules where user explicitly forces in-house custom development"},
			},
		},
	}, distDecomposeHandler())

	// 18. dendrite_dist_interop
	s.RegisterTool(Tool{
		Name:        "dendrite_dist_interop",
		Description: "Coordinate cross-project dependencies across owned repositories (e.g. Docula -> Oxipix), generate unblocking fallbacks and reconcile",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"action"},
			"properties": map[string]interface{}{
				"action":             map[string]interface{}{"type": "string", "enum": []string{"coordinate", "complete", "reconcile", "list"}, "description": "Interop coordination action"},
				"coordination_id":    map[string]interface{}{"type": "string", "description": "Coordination record ID (for complete/reconcile)"},
				"requester_project":  map[string]interface{}{"type": "string", "description": "Project requesting capability (e.g. Docula)"},
				"dependency_project": map[string]interface{}{"type": "string", "description": "Project supplying capability (e.g. Oxipix)"},
				"feature_requested":  map[string]interface{}{"type": "string", "description": "Feature name or signature to add"},
				"allow_fallback":     map[string]interface{}{"type": "boolean", "description": "Whether to unblock requester immediately with interim fallback and TODO"},
			},
		},
	}, distInteropHandler())

	// 19. dendrite_dist_spawn
	s.RegisterTool(Tool{
		Name:        "dendrite_dist_spawn",
		Description: "Spawn parallel sub-projects or generative satellite projects with git, scaffolding, and Plane integration",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"project_name"},
			"properties": map[string]interface{}{
				"project_name":         map[string]interface{}{"type": "string", "description": "Name of the sub-project or satellite project"},
				"identifier":           map[string]interface{}{"type": "string", "description": "Short code identifier (e.g. CSEN)"},
				"target_dir":           map[string]interface{}{"type": "string", "description": "Target filesystem directory path"},
				"language":             map[string]interface{}{"type": "string", "description": "Project language (rust, go, typescript, python)"},
				"is_satellite":         map[string]interface{}{"type": "boolean", "description": "True if spawned as a side-project (e.g. Carla Rust rewrite)"},
				"create_plane_project": map[string]interface{}{"type": "boolean", "description": "Whether to create a dedicated project in Plane v2"},
				"init_git":             map[string]interface{}{"type": "boolean", "description": "Whether to run git init (default: true)"},
				"description":          map[string]interface{}{"type": "string", "description": "Description of the sub-project"},
			},
		},
	}, distSpawnHandler(deps))
}

// distSourcesHandler handles the dendrite_dist_sources tool.
func distSourcesHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		priorityFilter, _ := args["priority"].(float64)
		langFilter, _ := args["language"].(string)
		query, _ := args["query"].(string)

		sources := globalDistRegistry.ListSources()
		libs := globalDistRegistry.ListLibraries()

		if query != "" {
			if matched, found := globalDistRegistry.ResolveCapability(query, langFilter); found {
				data := indentJSON(map[string]interface{}{
					"query":    query,
					"resolved": matched,
				})
				return textResult(string(data)), nil
			}
		}

		filteredLibs := make([]distcodebase.LibraryEntry, 0)
		for _, l := range libs {
			if priorityFilter > 0 && float64(l.Priority) != priorityFilter {
				continue
			}
			if langFilter != "" && !strings.EqualFold(l.Language, langFilter) {
				continue
			}
			filteredLibs = append(filteredLibs, l)
		}

		res := map[string]interface{}{
			"total_sources":   len(sources),
			"sources":         sources,
			"total_libraries": len(filteredLibs),
			"libraries":       filteredLibs,
		}
		data := indentJSON(res)
		return textResult(string(data)), nil
	}
}

// distDecomposeHandler handles the dendrite_dist_decompose tool.
func distDecomposeHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		pName, _ := args["project_name"].(string)
		target, _ := args["target_binary_or_spec"].(string)
		lang, _ := args["language"].(string)
		forced := true
		plan, err := distcodebase.NewDecomposer(globalDistRegistry).DecomposeProject(distcodebase.DecomposeRequest{
			ProjectName:         pName,
			TargetBinaryOrSpec:  target,
			Language:            lang,
			ForcedModular:       &forced,
			RequestedModules:    stringList(args["modules"]),
			UserCustomOverrides: stringList(args["user_custom_overrides"]),
		})
		if err != nil {
			return nil, fmt.Errorf("decomposition failed: %w", err)
		}
		return textResult(string(indentJSON(plan))), nil
	}
}

// distInteropHandler handles the dendrite_dist_interop tool.
func distInteropHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		action, _ := args["action"].(string)

		switch action {
		case "coordinate":
			reqProject, _ := args["requester_project"].(string)
			depProject, _ := args["dependency_project"].(string)
			feature, _ := args["feature_requested"].(string)
			fallback := boolArgOr(args, "allow_fallback", true)

			coord, err := globalInteropCoordinator.CoordinateFeature(distcodebase.InteropRequest{
				RequesterProject:  reqProject,
				DependencyProject: depProject,
				FeatureRequested:  feature,
				AllowFallback:     fallback,
			})
			if err != nil {
				return nil, fmt.Errorf("interop coordination failed: %w", err)
			}
			prompt := globalInteropCoordinator.FormatLocalLLMPrompt(coord)
			res := map[string]interface{}{
				"coordination":     coord,
				"local_llm_prompt": prompt,
			}
			data := indentJSON(res)
			return textResult(string(data)), nil

		case "complete":
			coordID, _ := args["coordination_id"].(string)
			coord, err := globalInteropCoordinator.CompleteDependencyFeature(coordID)
			if err != nil {
				return nil, fmt.Errorf("failed to complete feature: %w", err)
			}
			data := indentJSON(coord)
			return textResult(string(data)), nil

		case "reconcile":
			coordID, _ := args["coordination_id"].(string)
			rec, err := globalInteropCoordinator.ReconcileProject(coordID)
			if err != nil {
				return nil, fmt.Errorf("failed to reconcile: %w", err)
			}
			data := indentJSON(rec)
			return textResult(string(data)), nil

		case "list":
			coords := globalInteropCoordinator.ListCoordinations()
			data := indentJSON(coords)
			return textResult(string(data)), nil

		default:
			return nil, fmt.Errorf("unknown interop action: %s", action)
		}
	}
}

// distSpawnHandler handles the dendrite_dist_spawn tool.
func distSpawnHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		pName, _ := args["project_name"].(string)
		ident, _ := args["identifier"].(string)
		targetDir, _ := args["target_dir"].(string)
		lang, _ := args["language"].(string)
		isSat, _ := args["is_satellite"].(bool)
		createPlane := boolArgOr(args, "create_plane_project", true)
		initGit := boolArgOr(args, "init_git", true)
		desc, _ := args["description"].(string)

		wsReg, _ := workspace.NewRegistry("", "")
		spawner := distcodebase.NewProjectSpawner(deps.PlaneClient, wsReg)

		result, err := spawner.Spawn(ctx, distcodebase.SpawnRequest{
			ProjectName:        pName,
			Identifier:         ident,
			TargetDir:          targetDir,
			Language:           lang,
			IsSatellite:        isSat,
			CreatePlaneProject: createPlane,
			InitGit:            initGit,
			Description:        desc,
		})
		if err != nil {
			return nil, fmt.Errorf("spawning project failed: %w", err)
		}

		data := indentJSON(result)
		return textResult(string(data)), nil
	}
}
