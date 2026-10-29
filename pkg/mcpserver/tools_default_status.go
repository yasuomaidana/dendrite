package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/env"
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/topology"
	"Dendrite/pkg/topology/advisor"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// registerStatusAndTicketTools registers dendrite_env_status, dendrite_ticket_list, dendrite_ticket_update and dendrite_plane_issues.
func registerStatusAndTicketTools(s *Server, deps Dependencies) {
	// 1. dendrite_env_status
	s.RegisterTool(Tool{
		Name:        "dendrite_env_status",
		Description: "Query real-time health and status of replicated environment components (Docker, Plane, LiteLLM, Ollama)",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		detector := env.NewDetector(nil)
		host, err := detector.Detect(ctx)
		if err != nil {
			return nil, fmt.Errorf("detector error: %w", err)
		}
		replicator := env.NewReplicator(nil, *host)
		statuses := replicator.AuditComponents(ctx)

		data, err := json.MarshalIndent(statuses, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
			},
		}, nil
	})

	// 2. dendrite_ticket_list
	s.RegisterTool(Tool{
		Name:        "dendrite_ticket_list",
		Description: "List all project tickets, their statuses, complexity scores, and milestones from DEV_STATUS.md",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file": map[string]interface{}{
					"type":        "string",
					"description": "Path to DEV_STATUS.md (default: DEV_STATUS.md)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		file := deps.DevStatus
		if f, ok := args["file"].(string); ok && strings.TrimSpace(f) != "" {
			file = f
		}
		tr := tracker.NewMemoryTracker(file)
		if err := tr.LoadFromMarkdown(file); err != nil {
			pClient := deps.PlaneClient
			if pClient == nil {
				pClient, _ = plane.NewClientFromEnv()
			}
			if pClient != nil {
				issues, pErr := pClient.ListPlaneIssues(ctx)
				if pErr == nil && len(issues) > 0 {
					data, mErr := json.MarshalIndent(issues, "", "  ")
					if mErr == nil {
						return &CallToolResult{
							Content: []ContentItem{{Type: "text", Text: string(data)}},
						}, nil
					}
				}
			}
			return nil, fmt.Errorf("failed to load markdown tickets: %w", err)
		}
		tickets, err := tr.ListTickets(ctx)
		if err != nil {
			return nil, err
		}
		data, err := json.MarshalIndent(tickets, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
			},
		}, nil
	})

	// 3. dendrite_ticket_update
	s.RegisterTool(Tool{
		Name:        "dendrite_ticket_update",
		Description: "Update the status of a project ticket in DEV_STATUS.md (e.g. DONE, IN_PROGRESS, TODO)",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"ticket_id", "status"},
			"properties": map[string]interface{}{
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "The ticket identifier, e.g. DEN-034",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "New status, e.g. DONE, IN_PROGRESS, TODO",
				},
				"file": map[string]interface{}{
					"type":        "string",
					"description": "Path to DEV_STATUS.md (default: DEV_STATUS.md)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		tid, ok := args["ticket_id"].(string)
		if !ok || strings.TrimSpace(tid) == "" {
			return nil, fmt.Errorf("missing ticket_id")
		}
		stStr, ok := args["status"].(string)
		if !ok || strings.TrimSpace(stStr) == "" {
			return nil, fmt.Errorf("missing status")
		}
		file := deps.DevStatus
		if f, ok := args["file"].(string); ok && strings.TrimSpace(f) != "" {
			file = f
		}

		st := tracker.Status(strings.ToUpper(strings.TrimSpace(stStr)))
		if err := devtool.UpdateTicketInMarkdown(ctx, file, tid, st); err != nil {
			pClient := deps.PlaneClient
			if pClient == nil {
				pClient, _ = plane.NewClientFromEnv()
			}
			if pClient != nil {
				if _, pErr := planesvc.Default().SetStatus(ctx, pClient, tracker.TicketID(tid), st); pErr == nil {
					return &CallToolResult{
						Content: []ContentItem{
							{Type: "text", Text: fmt.Sprintf("Successfully updated %s to %s in Plane service", tid, st)},
						},
					}, nil
				}
			}
			return nil, fmt.Errorf("failed to update ticket in markdown: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: fmt.Sprintf("Successfully updated %s to %s in %s", tid, st, file)},
			},
		}, nil
	})

	// 4. dendrite_plane_issues
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_issues",
		Description: "Query issues from self-hosted Plane v2 project management container",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"workspace": map[string]interface{}{"type": "string", "description": "Workspace slug (default: dendrite-ws)"},
				"project":   map[string]interface{}{"type": "string", "description": "Project ID or Identifier (default: DEN)"},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		client := deps.PlaneClient
		if client == nil {
			var err error
			client, err = plane.NewClientFromEnv()
			if err != nil {
				return nil, fmt.Errorf("plane client error: %w", err)
			}
		}

		issues, err := client.ListPlaneIssues(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Plane: %w", err)
		}

		data, err := json.MarshalIndent(issues, "", "  ")
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

// registerOllamaAndGatewayTools registers dendrite_ollama_probe, dendrite_topology_advisor, dendrite_gateway_render and dendrite_gateway_update.
func registerOllamaAndGatewayTools(s *Server, deps Dependencies) {
	// 5. dendrite_ollama_probe
	s.RegisterTool(Tool{
		Name:        "dendrite_ollama_probe",
		Description: "Probe local Ollama inference engine for latency and available local models (qwen2.5-coder, llama3.2, hermes3)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{"type": "string", "description": "Ollama base URL (default: http://127.0.0.1:11434)"},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		target := deps.OllamaURL
		if u, ok := args["url"].(string); ok && u != "" {
			target = u
		}
		status, models, err := devtool.ProbeOllama(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("ollama probe failed: %w", err)
		}
		result := map[string]interface{}{
			"endpoint": target,
			"latency":  status.Latency.String(),
			"models":   models,
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
			},
		}, nil
	})

	// 6. dendrite_topology_advisor
	s.RegisterTool(Tool{
		Name:        "dendrite_topology_advisor",
		Description: "Evaluate hardware capabilities of compute nodes and calculate optimal placement of Dendrite microservices",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"nodes": map[string]interface{}{
					"type":        "string",
					"description": "Comma-separated node definitions (e.g. 'local=localhost,worker=100.84.12.25')",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		nodeArgs := "local=localhost"
		if n, ok := args["nodes"].(string); ok && n != "" {
			nodeArgs = n
		}

		var targets []advisor.NodeTarget
		for _, tok := range strings.Split(nodeArgs, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			id, host := tok, tok
			if strings.Contains(tok, "=") {
				parts := strings.SplitN(tok, "=", 2)
				id, host = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			}
			targets = append(targets, advisor.NodeTarget{
				ID:      id,
				Host:    host,
				IsLocal: (host == "localhost" || host == "127.0.0.1" || host == "local"),
			})
		}
		if len(targets) == 0 {
			targets = append(targets, advisor.NodeTarget{ID: "local", Host: "localhost", IsLocal: true})
		}

		probe := advisor.NewHardwareProbe(nil)
		hwList, err := probe.ProbeAll(ctx, targets)
		if err != nil {
			return nil, err
		}
		adv := advisor.NewPlacementAdvisor()
		plan, err := adv.Advise(hwList, advisor.DefaultDendriteServices())
		if err != nil {
			return nil, err
		}

		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
			},
		}, nil
	})

	// 7. dendrite_gateway_render
	s.RegisterTool(Tool{
		Name:        "dendrite_gateway_render",
		Description: "Render dynamic LiteLLM YAML routing configuration for a given topology scenario",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"scenario": map[string]interface{}{
					"type":        "string",
					"description": "Topology scenario: 'local-hybrid', 'cloud-burst', 'full-local'",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		scStr := "local-hybrid"
		if s, ok := args["scenario"].(string); ok && s != "" {
			scStr = s
		}
		sc, err := topology.ParseScenario(scStr)
		if err != nil {
			return nil, err
		}
		top, err := topology.NewTopologyConfig(sc, topology.TailscaleEndpoints{
			ControlPlane:   "100.84.12.10",
			ExecutionPlane: "100.84.12.25",
		})
		if err != nil {
			return nil, err
		}
		yamlData, err := gateway.RenderForTopology(top)
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(yamlData)},
			},
		}, nil
	})

	// dendrite_gateway_update
	s.RegisterTool(Tool{
		Name:        "dendrite_gateway_update",
		Description: "Dynamically discover live inference engines (Ollama, OxiLLaMa, etc.), score loaded models by intelligence, and update LiteLLM routing table (default: ~/.dendrite/stacks/litellm_config.yaml)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file": map[string]interface{}{
					"type":        "string",
					"description": "Target litellm_config.yaml path (default: ~/.dendrite/stacks/litellm_config.yaml)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "Simulate update without modifying disk (default: false)",
				},
				"use_container_host": map[string]interface{}{
					"type":        "boolean",
					"description": "Use host.docker.internal for container networking (default: true for the stacks directory)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		opts := gateway.DefaultUpdaterOptions()
		if f, ok := args["file"].(string); ok && strings.TrimSpace(f) != "" {
			opts.ConfigPath = strings.TrimSpace(f)
		}
		if dr, ok := args["dry_run"].(bool); ok {
			opts.DryRun = dr
		}
		if uch, ok := args["use_container_host"].(bool); ok {
			opts.UseContainerHost = uch
		}

		updater := gateway.NewRoutingTableUpdater(opts)
		rep, err := updater.UpdateNow(ctx)
		if err != nil {
			return nil, fmt.Errorf("gateway update failed: %w", err)
		}

		data, err := json.MarshalIndent(rep, "", "  ")
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
