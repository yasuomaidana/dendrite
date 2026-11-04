package codetools

import (
	"Dendrite/pkg/agentsearch"
	"Dendrite/pkg/distcodebase"
	"Dendrite/pkg/testgen"
	"Dendrite/pkg/workspace"
	"context"
	"fmt"
	"strings"

	. "Dendrite/pkg/mcpserver"
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
				"query":          TypedProp("string", "Task description or capability search query"),
				"target_package": TypedProp("string", "Optional target Go/Rust package (e.g. 'pkg/ast', 'pkg/gateway')"),
				"target_symbols": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional list of target AST symbols",
				},
				"require_local": TypedProp("boolean", "Constrain selection to zero-cost local inference engines (Ollama Metal, OxiLLaMa)"),
				"max_cost":      TypedProp("number", "Optional maximum cost ceiling in USD"),
				"max_results":   TypedProp("integer", "Maximum number of candidate agents to return (default: 5)"),
				"route":         TypedProp("boolean", "If true, auto-selects top agent and returns execution routing decision"),
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
			WithTargetSymbols(StringList(args["target_symbols"])...).
			RequireLocalInference(requireLocal).
			WithMaxCost(maxCost).
			WithMaxResults(int(FloatArgOr(args, "max_results", 5))).
			Build()

		// Ranked by the dendrite-rag sidecar
		engine := agentsearch.NewSearchEngine(nil, nil, nil)
		if routeMode {
			decision, err := engine.Route(ctx, query)
			if err != nil {
				return nil, fmt.Errorf("agent routing failed: %w", err)
			}
			return TextResult(string(IndentJSON(decision))), nil
		}
		results, err := engine.Search(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("agent search failed: %w", err)
		}
		return TextResult(string(IndentJSON(results))), nil
	}
}

func registerTestGenTools(s *Server, deps Dependencies) {
	// 25. dendrite_test_auto
	s.RegisterTool(Tool{
		Name:        "dendrite_test_auto",
		Description: "Analyzes AST symbol boundaries, detects persistence dependencies (PostgreSQL, Redis, SQLite), scaffolds Testcontainers environments, and executes a self-healing test convergence loop (<= 3 iterations) achieving >= 85% coverage",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"file"},
			"properties": map[string]interface{}{
				"file":                TypedProp("string", "Path to target source file (e.g. 'pkg/gateway/limiter.go')"),
				"with_testcontainers": TypedProp("boolean", "Force Testcontainers scaffolding generation even if no persistent dependencies detected"),
				"max_iterations":      TypedProp("integer", "Maximum self-healing convergence iterations (default: 3)"),
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

		data := IndentJSON(report)

		return TextResult(string(data)), nil
	})
}

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
				"priority": TypedProp("integer", "Filter by priority level (1 = own repo, 2 = curated OSS, 3 = registry)"),
				"language": TypedProp("string", "Filter by language (e.g. rust, go, typescript)"),
				"query":    TypedProp("string", "Capability or library name query (e.g. wgpu, cte, turso, pixels)"),
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
				"project_name":          TypedProp("string", "Name of the project to decompose"),
				"target_binary_or_spec": TypedProp("string", "Target binary path or specification file"),
				"language":              TypedProp("string", "Target language (default: rust)"),
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
				"coordination_id":    TypedProp("string", "Coordination record ID (for complete/reconcile)"),
				"requester_project":  TypedProp("string", "Project requesting capability (e.g. Docula)"),
				"dependency_project": TypedProp("string", "Project supplying capability (e.g. Oxipix)"),
				"feature_requested":  TypedProp("string", "Feature name or signature to add"),
				"allow_fallback":     TypedProp("boolean", "Whether to unblock requester immediately with interim fallback and TODO"),
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
				"project_name":         TypedProp("string", "Name of the sub-project or satellite project"),
				"identifier":           TypedProp("string", "Short code identifier (e.g. CSEN)"),
				"target_dir":           TypedProp("string", "Target filesystem directory path"),
				"language":             TypedProp("string", "Project language (rust, go, typescript, python)"),
				"is_satellite":         TypedProp("boolean", "True if spawned as a side-project (e.g. Carla Rust rewrite)"),
				"create_plane_project": TypedProp("boolean", "Whether to create a dedicated project in Plane v2"),
				"init_git":             TypedProp("boolean", "Whether to run git init (default: true)"),
				"description":          TypedProp("string", "Description of the sub-project"),
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
				data := IndentJSON(map[string]interface{}{
					"query":    query,
					"resolved": matched,
				})
				return TextResult(string(data)), nil
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
		data := IndentJSON(res)
		return TextResult(string(data)), nil
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
			RequestedModules:    StringList(args["modules"]),
			UserCustomOverrides: StringList(args["user_custom_overrides"]),
		})
		if err != nil {
			return nil, fmt.Errorf("decomposition failed: %w", err)
		}
		return TextResult(string(IndentJSON(plan))), nil
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
			fallback := BoolArgOr(args, "allow_fallback", true)

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
			data := IndentJSON(res)
			return TextResult(string(data)), nil

		case "complete":
			coordID, _ := args["coordination_id"].(string)
			coord, err := globalInteropCoordinator.CompleteDependencyFeature(coordID)
			if err != nil {
				return nil, fmt.Errorf("failed to complete feature: %w", err)
			}
			data := IndentJSON(coord)
			return TextResult(string(data)), nil

		case "reconcile":
			coordID, _ := args["coordination_id"].(string)
			rec, err := globalInteropCoordinator.ReconcileProject(coordID)
			if err != nil {
				return nil, fmt.Errorf("failed to reconcile: %w", err)
			}
			data := IndentJSON(rec)
			return TextResult(string(data)), nil

		case "list":
			coords := globalInteropCoordinator.ListCoordinations()
			data := IndentJSON(coords)
			return TextResult(string(data)), nil

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
		createPlane := BoolArgOr(args, "create_plane_project", true)
		initGit := BoolArgOr(args, "init_git", true)
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

		data := IndentJSON(result)
		return TextResult(string(data)), nil
	}
}
