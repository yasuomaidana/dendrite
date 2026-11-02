package mcpserver

import (
	"context"
	"fmt"

	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/pool/split"
	"Dendrite/pkg/pool/switcher"
)

var defaultMCPPoolManager = dag.NewPoolManager("")
var defaultMCPDAGSolver = dag.NewDAGSolver()
var defaultMCPSwitcher = switcher.NewDependencySwitcher()
var defaultMCPSplitter = split.NewProjectSplitter()

func registerPoolTools(s *Server, deps Dependencies) {
	// 26. dendrite_pool_resolve
	s.RegisterTool(Tool{
		Name:        "dendrite_pool_resolve",
		Description: "Computes deterministic topological DAG across multi-directory pool projects and detects circular dependency hazards",
		InputSchema: objectSchema([]string{"directories"}, map[string]interface{}{
			"directories": listProp("string", "Optional ad-hoc list of directory paths to resolve directly without loading pool config"),
		}),
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		var projects []dag.ProjectInfo

		if rawDirs, ok := args["directories"].([]interface{}); ok && len(rawDirs) > 0 {
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
		} else {
			_ = defaultMCPPoolManager.Load()
			projects = defaultMCPPoolManager.List()
		}

		if len(projects) == 0 {
			return textResult("[-] Pool is empty. No projects registered."), nil
		}

		res, err := defaultMCPDAGSolver.Resolve(projects)
		if err != nil {
			return errorResult("[-] Dependency resolution failed: %v", err), nil
		}

		data := indentJSON(res)

		return textResult(string(data)), nil
	})

	// 27. dendrite_pool_list
	s.RegisterTool(Tool{
		Name:        "dendrite_pool_list",
		Description: "List all registered project members in the multi-project pool",
		InputSchema: propsSchema(map[string]interface{}{}),
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		_ = defaultMCPPoolManager.Load()
		projects := defaultMCPPoolManager.List()

		data := indentJSON(map[string]interface{}{
			"count":    len(projects),
			"projects": projects,
		})

		return textResult(string(data)), nil
	})

	// 28. dendrite_pool_add
	s.RegisterTool(Tool{
		Name:        "dendrite_pool_add",
		Description: "Register one or more project directories into .dendrite/pool.json",
		InputSchema: propsSchema(map[string]interface{}{
			"directories": listProp("string", "List of project directory paths to register"),
		}),
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

		added, err := defaultMCPPoolManager.Add(dirs...)
		if err != nil {
			return errorResult("[-] Failed adding projects: %v", err), nil
		}

		if err := defaultMCPPoolManager.Save(); err != nil {
			return nil, fmt.Errorf("saving pool config: %w", err)
		}

		data := indentJSON(map[string]interface{}{
			"status": "success",
			"added":  added,
		})

		return textResult(string(data)), nil
	})

	// 29. dendrite_pool_switch
	s.RegisterTool(Tool{
		Name:        "dendrite_pool_switch",
		Description: "Atomically transition a dependency mode across Published, Branch, Local, or Fork with JetBrains scope sync",
		InputSchema: objectSchema([]string{"target_project_dir", "dependency_name", "mode"}, map[string]interface{}{
			"target_project_dir": typedProp("string", "Target project directory containing manifests"),
			"dependency_name":    typedProp("string", "Name or module path of target dependency"),
			"mode":               map[string]interface{}{"type": "string", "enum": []string{"published", "branch", "local", "fork"}, "description": "Target dependency mode"},
			"version":            typedProp("string", "Version string (required for published mode)"),
			"branch_name":        typedProp("string", "Branch name (for branch mode)"),
			"commit_sha":         typedProp("string", "Commit SHA (optional pin for branch mode)"),
			"local_path":         typedProp("string", "Relative or absolute filesystem path (required for local mode)"),
			"fork_url":           typedProp("string", "Remote git fork URL (required for fork mode)"),
			"fork_branch":        typedProp("string", "Remote branch on fork (optional for fork mode)"),
			"sync_ide":           typedProp("boolean", "Whether to synchronize JetBrains IDE scopes (.idea/scopes)"),
		}),
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
		if err != nil {
			return errorResult("[-] Dependency mode switch failed: %v", err), nil
		}

		data := indentJSON(result)

		return textResult(string(data)), nil
	})

	// 30. dendrite_pool_split
	s.RegisterTool(Tool{
		Name:        "dendrite_pool_split",
		Description: "Extract an AST slice and its tests into a new standalone pool member project",
		InputSchema: objectSchema([]string{"source_project_dir", "module_subpath", "new_member_name", "target_directory"}, map[string]interface{}{
			"source_project_dir": typedProp("string", "Path to source project directory"),
			"module_subpath":     typedProp("string", "Relative path to module being extracted (e.g. internal/matcher)"),
			"new_member_name":    typedProp("string", "Identifier for the new project member"),
			"target_directory":   typedProp("string", "Target destination directory on filesystem"),
			"wire_local_mode":    typedProp("boolean", "Wire parent project to depend on new member in local mode"),
			"register_in_pool":   typedProp("boolean", "Register newly extracted project in .dendrite/pool.json"),
		}),
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
		if err != nil {
			return errorResult("[-] Project split failed: %v", err), nil
		}

		data := indentJSON(result)

		return textResult(string(data)), nil
	})
}
