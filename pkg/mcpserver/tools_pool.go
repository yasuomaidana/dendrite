package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/pool/dag"
)

var defaultMCPPoolManager = dag.NewPoolManager("")
var defaultMCPDAGSolver = dag.NewDAGSolver()

func (s *Server) registerPoolTools(deps Dependencies) {
	// 26. dendrite_pool_resolve
	s.RegisterTool(Tool{
		Name:        "dendrite_pool_resolve",
		Description: "Computes deterministic topological DAG across multi-directory pool projects and detects circular dependency hazards",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"directories": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional ad-hoc list of directory paths to resolve directly without loading pool config",
				},
			},
		},
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
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: "[-] Pool is empty. No projects registered.",
				}},
			}, nil
		}

		res, err := defaultMCPDAGSolver.Resolve(projects)
		if err != nil {
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: fmt.Sprintf("[-] Dependency resolution failed: %v", err),
				}},
				IsError: true,
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling DAG resolution: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	})

	// 27. dendrite_pool_list
	s.RegisterTool(Tool{
		Name:        "dendrite_pool_list",
		Description: "List all registered project members in the multi-project pool",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		_ = defaultMCPPoolManager.Load()
		projects := defaultMCPPoolManager.List()

		data, err := json.MarshalIndent(map[string]interface{}{
			"count":    len(projects),
			"projects": projects,
		}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshaling pool projects: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	})

	// 28. dendrite_pool_add
	s.RegisterTool(Tool{
		Name:        "dendrite_pool_add",
		Description: "Register one or more project directories into .dendrite/pool.json",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"directories": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "List of project directory paths to register",
				},
			},
			"required": []interface{}{"directories"},
		},
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
			return &CallToolResult{
				Content: []ContentItem{{
					Type: "text",
					Text: fmt.Sprintf("[-] Failed adding projects: %v", err),
				}},
				IsError: true,
			}, nil
		}

		if err := defaultMCPPoolManager.Save(); err != nil {
			return nil, fmt.Errorf("saving pool config: %w", err)
		}

		data, _ := json.MarshalIndent(map[string]interface{}{
			"status": "success",
			"added":  added,
		}, "", "  ")

		return &CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	})
}
