package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/env/bootstrap"
)

func registerEnvBootstrapTools(s *Server, deps Dependencies) {
	registerEnvPrepareTools(s, deps)
}

func registerEnvPrepareTools(s *Server, deps Dependencies) {
	// dendrite_env_prepare
	s.RegisterTool(Tool{
		Name:        "dendrite_env_prepare",
		Description: "Prepares or synchronizes project environment, updates agent rules and skills, and enforces CI/CD tooling (default: GitHub Copilot)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_path": map[string]interface{}{
					"type":        "string",
					"description": "Target workspace project root directory (default: current directory)",
				},
				"default_tool": map[string]interface{}{
					"type":        "string",
					"description": "Default CI/CD assistant tool (default: 'copilot')",
				},
				"enforce_cicd": map[string]interface{}{
					"type":        "boolean",
					"description": "Enforce CI/CD practices and GitHub Copilot configuration (default: true)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, simulates preparation without writing files",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		path := "."
		if p, ok := args["project_path"].(string); ok && strings.TrimSpace(p) != "" {
			path = p
		}
		tool := "copilot"
		if t, ok := args["default_tool"].(string); ok && strings.TrimSpace(t) != "" {
			tool = t
		}
		enforceCICD := true
		if e, ok := args["enforce_cicd"].(bool); ok {
			enforceCICD = e
		}
		dryRun := false
		if d, ok := args["dry_run"].(bool); ok {
			dryRun = d
		}

		prep := bootstrap.NewPreparer()
		report, err := prep.Prepare(ctx, bootstrap.PrepareOptions{
			ProjectPath: path,
			DefaultTool: tool,
			EnforceCICD: enforceCICD,
			SyncSkills:  true,
			DryRun:      dryRun,
		})
		if err != nil {
			return nil, fmt.Errorf("environment preparation failed: %w", err)
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
