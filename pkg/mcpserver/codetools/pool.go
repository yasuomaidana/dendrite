package codetools

import (
	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/pool/split"
	"Dendrite/pkg/pool/switcher"
	"context"
	"fmt"

	. "Dendrite/pkg/mcpserver"
)

var DefaultMCPPoolManager = dag.NewPoolManager("")
var defaultMCPDAGSolver = dag.NewDAGSolver()
var defaultMCPSwitcher = switcher.NewDependencySwitcher()
var defaultMCPSplitter = split.NewProjectSplitter()

func registerPoolTools(s *Server, deps Dependencies) {
	// 26. dendrite_pool_resolve
	AddTool(s, "dendrite_pool_resolve", "Computes deterministic topological DAG across multi-directory pool projects and detects circular dependency hazards",
		PropsSchema(map[string]interface{}{
			"directories": ListProp("string", "Optional ad-hoc list of directory paths to resolve directly without loading pool config"),
		}),
		poolResolveHandler())

	// 27. dendrite_pool_list
	AddTool(s, "dendrite_pool_list", "List all registered project members in the multi-project pool",
		PropsSchema(map[string]interface{}{}),
		poolListHandler())

	// 28. dendrite_pool_add
	AddTool(s, "dendrite_pool_add", "Register one or more project directories into .dendrite/pool.json",
		ObjectSchema([]string{"directories"}, map[string]interface{}{
			"directories": ListProp("string", "List of project directory paths to register"),
		}),
		poolAddHandler())

	// 29. dendrite_pool_switch
	AddTool(s, "dendrite_pool_switch", "Atomically transition a dependency mode across Published, Branch, Local, or Fork with JetBrains scope sync",
		ObjectSchema([]string{"target_project_dir", "dependency_name", "mode"}, map[string]interface{}{
			"target_project_dir": TypedProp("string", "Target project directory containing manifests"),
			"dependency_name":    TypedProp("string", "Name or module path of target dependency"),
			"mode":               map[string]interface{}{"type": "string", "enum": []string{"published", "branch", "local", "fork"}, "description": "Target dependency mode"},
			"version":            TypedProp("string", "Version string (required for published mode)"),
			"branch_name":        TypedProp("string", "Branch name (for branch mode)"),
			"commit_sha":         TypedProp("string", "Commit SHA (optional pin for branch mode)"),
			"local_path":         TypedProp("string", "Relative or absolute filesystem path (required for local mode)"),
			"fork_url":           TypedProp("string", "Remote git fork URL (required for fork mode)"),
			"fork_branch":        TypedProp("string", "Remote branch on fork (optional for fork mode)"),
			"sync_ide":           TypedProp("boolean", "Whether to synchronize JetBrains IDE scopes (.idea/scopes)"),
		}),
		poolSwitchHandler())

	// 30. dendrite_pool_split
	AddTool(s, "dendrite_pool_split", "Extract an AST slice and its tests into a new standalone pool member project",
		ObjectSchema([]string{"source_project_dir", "module_subpath", "new_member_name", "target_directory"}, map[string]interface{}{
			"source_project_dir": TypedProp("string", "Path to source project directory"),
			"module_subpath":     TypedProp("string", "Relative path to module being extracted (e.g. internal/matcher)"),
			"new_member_name":    TypedProp("string", "Identifier for the new project member"),
			"target_directory":   TypedProp("string", "Target destination directory on filesystem"),
			"wire_local_mode":    TypedProp("boolean", "Wire parent project to depend on new member in local mode"),
			"register_in_pool":   TypedProp("boolean", "Register newly extracted project in .dendrite/pool.json"),
		}),
		poolSplitHandler())
}

// projectsToResolve are the projects a pool_resolve call works on: the scanned directories it names, otherwise the
// registered pool.
func projectsToResolve(args map[string]interface{}) ([]dag.ProjectInfo, error) {
	rawDirs, _ := args["directories"].([]interface{})
	if len(rawDirs) == 0 {
		_ = DefaultMCPPoolManager.Load()
		return DefaultMCPPoolManager.List(), nil
	}
	var projects []dag.ProjectInfo
	scanner := dag.NewManifestScanner()
	for _, d := range rawDirs {
		dirStr, ok := d.(string)
		if !ok {
			continue
		}
		info, err := scanner.ScanProject(dirStr)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", dirStr, err)
		}
		projects = append(projects, *info)
	}
	return projects, nil
}

// poolResolveHandler handles the dendrite_pool_resolve tool.
func poolResolveHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		projects, err := projectsToResolve(args)
		if err != nil {
			return nil, err
		}
		if len(projects) == 0 {
			return TextResult("[-] Pool is empty. No projects registered."), nil
		}
		res, err := defaultMCPDAGSolver.Resolve(projects)
		return ResultOrError(res, err, "[-] Dependency resolution failed: %v"), nil
	}
}

// poolAddHandler handles the dendrite_pool_add tool.
func poolAddHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		rawDirs, ok := args["directories"].([]interface{})
		if !ok || len(rawDirs) == 0 {
			return nil, fmt.Errorf("directories array is required and must not be empty")
		}

		var dirs []string
		for _, d := range rawDirs {
			if str, ok := d.(string); ok {
				dirs = append(dirs, str)
			}
		}

		added, err := DefaultMCPPoolManager.Add(dirs...)
		if err != nil {
			return ErrorResult("[-] Failed adding projects: %v", err), nil
		}

		if err := DefaultMCPPoolManager.Save(); err != nil {
			return nil, fmt.Errorf("saving pool config: %w", err)
		}

		data := IndentJSON(map[string]interface{}{
			"status": "success",
			"added":  added,
		})

		return TextResult(string(data)), nil
	}
}

// poolSwitchHandler handles the dendrite_pool_switch tool.
func poolSwitchHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		targetDir, _ := args["target_project_dir"].(string)
		depName, _ := args["dependency_name"].(string)
		modeStr, _ := args["mode"].(string)
		version, _ := args["version"].(string)
		branchName, _ := args["branch_name"].(string)
		commitSHA, _ := args["commit_sha"].(string)
		localPath, _ := args["local_path"].(string)
		forkURL, _ := args["fork_url"].(string)
		forkBranch, _ := args["fork_branch"].(string)
		syncIDE, _ := args["sync_ide"].(bool)

		req := switcher.SwitchRequest{
			TargetProjectDir: targetDir,
			DependencyName:   depName,
			Mode:             switcher.DependencyMode(modeStr),
			Version:          version,
			BranchName:       branchName,
			CommitSHA:        commitSHA,
			LocalPath:        localPath,
			ForkURL:          forkURL,
			ForkBranch:       forkBranch,
			SyncIDE:          syncIDE,
		}

		result, err := defaultMCPSwitcher.Switch(ctx, req)
		return ResultOrError(result, err, "[-] Dependency mode switch failed: %v"), nil
	}
}

// poolSplitHandler handles the dendrite_pool_split tool.
func poolSplitHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		srcDir, _ := args["source_project_dir"].(string)
		modSubpath, _ := args["module_subpath"].(string)
		memberName, _ := args["new_member_name"].(string)
		targetDir, _ := args["target_directory"].(string)
		wireLocal, _ := args["wire_local_mode"].(bool)
		regPool, _ := args["register_in_pool"].(bool)

		req := split.SplitRequest{
			SourceProjectDir: srcDir,
			ModuleSubpath:    modSubpath,
			NewMemberName:    memberName,
			TargetDirectory:  targetDir,
			WireLocalMode:    wireLocal,
			RegisterInPool:   regPool,
		}

		result, err := defaultMCPSplitter.Split(ctx, req)
		return ResultOrError(result, err, "[-] Project split failed: %v"), nil
	}
}

// poolListHandler handles the dendrite_pool_list tool.
func poolListHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		_ = DefaultMCPPoolManager.Load()
		projects := DefaultMCPPoolManager.List()

		data := IndentJSON(map[string]interface{}{
			"count":    len(projects),
			"projects": projects,
		})

		return TextResult(string(data)), nil
	}
}
