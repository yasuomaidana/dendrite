package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/env/bootstrap"
)

func (s *Server) registerEnvPrepareTools(deps Dependencies) {
	// dendrite_env_prepare
	s.RegisterTool(Tool{
		Name:        "dendrite_env_prepare",
		Description: "Prepares, synchronizes, and enforces CI/CD practices on a project repository with GitHub Copilot as default, scaffolding copilot instructions, CI workflows with >= 85% coverage gate, and agent rules",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to target project root (defaults to '.')",
				},
				"default_tool": map[string]interface{}{
					"type":        "string",
					"description": "Default CI/CD and developer tool (defaults to 'copilot')",
				},
				"enforce_cicd": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to enforce CI/CD workflows and copilot instructions (default true)",
				},
				"sync_skills": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to synchronize agent rules and skills (default true)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "Preview changes without modifying filesystem",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		opts := bootstrap.PrepareOptions{
			ProjectPath: ".",
			DefaultTool: "copilot",
			EnforceCICD: true,
			SyncSkills:  true,
		}

		if p, ok := args["project_path"].(string); ok && p != "" {
			opts.ProjectPath = p
		}
		if t, ok := args["default_tool"].(string); ok && t != "" {
			opts.DefaultTool = t
		}
		if e, ok := args["enforce_cicd"].(bool); ok {
			opts.EnforceCICD = e
		}
		if s, ok := args["sync_skills"].(bool); ok {
			opts.SyncSkills = s
		}
		if d, ok := args["dry_run"].(bool); ok {
			opts.DryRun = d
		}

		preparer := bootstrap.NewPreparer()
		report, err := preparer.Prepare(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare project environment: %w", err)
		}

		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
			},
		}, nil
	})
}
