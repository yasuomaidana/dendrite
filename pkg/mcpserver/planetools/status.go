package planetools

import (
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/env"
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/topology"
	"Dendrite/pkg/topology/advisor"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
	"context"
	"fmt"
	"strings"

	. "Dendrite/pkg/mcpserver"
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
	}, envStatusHandler())

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
	}, ticketListHandler(deps))

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
	}, ticketUpdateHandler(deps))

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
	}, planeIssuesHandler(deps))
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
	}, ollamaProbeHandler(deps))

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
	}, topologyAdvisorHandler())

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
	}, gatewayRenderHandler())

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
	}, gatewayUpdateHandler())
}

// envStatusHandler handles the dendrite_env_status tool.
func envStatusHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		detector := env.NewDetector(nil)
		host, _ := detector.Detect(ctx) // Detect reports what it could read and never fails
		replicator := env.NewReplicator(nil, *host)
		statuses := replicator.AuditComponents(ctx)

		data := IndentJSON(statuses)
		return TextResult(string(data)), nil
	}
}

// ticketListHandler handles the dendrite_ticket_list tool.
func ticketListHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		file := ledgerFile(deps, args)
		tr := tracker.NewMemoryTracker(file)
		if err := tr.LoadFromMarkdown(file); err != nil {
			if client := planeFallback(deps); client != nil {
				if issues, pErr := client.ListPlaneIssues(ctx); pErr == nil && len(issues) > 0 {
					return TextResult(string(IndentJSON(issues))), nil
				}
			}
			return nil, fmt.Errorf("failed to load markdown tickets: %w", err)
		}
		tickets, _ := tr.ListTickets(ctx) // an in-memory list cannot fail
		return TextResult(string(IndentJSON(tickets))), nil
	}
}

// ledgerFile is the status ledger a call names with "file", otherwise the configured one.
func ledgerFile(deps Dependencies, args map[string]interface{}) string {
	if f, ok := args["file"].(string); ok && strings.TrimSpace(f) != "" {
		return f
	}
	return deps.DevStatus
}

// planeFallback is the Plane client a tool falls back to when the markdown ledger cannot be used, nil when there is none.
func planeFallback(deps Dependencies) *plane.Client {
	if deps.PlaneClient != nil {
		return deps.PlaneClient
	}
	client, _ := plane.NewClientFromEnv()
	return client
}

// requiredText reads a non-blank text argument or fails with "missing <key>".
func requiredText(args map[string]interface{}, key string) (string, error) {
	v, ok := args[key].(string)
	if !ok || strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("missing %s", key)
	}
	return v, nil
}

// ticketUpdateHandler handles the dendrite_ticket_update tool.
func ticketUpdateHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		tid, err := requiredText(args, "ticket_id")
		if err != nil {
			return nil, err
		}
		stStr, err := requiredText(args, "status")
		if err != nil {
			return nil, err
		}
		file := ledgerFile(deps, args)
		st := tracker.Status(strings.ToUpper(strings.TrimSpace(stStr)))
		if err := devtool.UpdateTicketInMarkdown(ctx, file, tid, st); err != nil {
			if client := planeFallback(deps); client != nil {
				if _, pErr := planesvc.Default().SetStatus(ctx, client, tracker.TicketID(tid), st); pErr == nil {
					return TextResult(fmt.Sprintf("Successfully updated %s to %s in Plane service", tid, st)), nil
				}
			}
			return nil, fmt.Errorf("failed to update ticket in markdown: %w", err)
		}
		return TextResult(fmt.Sprintf("Successfully updated %s to %s in %s", tid, st, file)), nil
	}
}

// planeIssuesHandler handles the dendrite_plane_issues tool.
func planeIssuesHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		client, err := PlaneClientOf(deps)
		if err != nil {
			return nil, err
		}

		issues, err := client.ListPlaneIssues(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Plane: %w", err)
		}

		data := IndentJSON(issues)
		return TextResult(string(data)), nil
	}
}

// ollamaProbeHandler handles the dendrite_ollama_probe tool.
func ollamaProbeHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
		data := IndentJSON(result)
		return TextResult(string(data)), nil
	}
}

// topologyAdvisorHandler handles the dendrite_topology_advisor tool.
func topologyAdvisorHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
		hwList, _ := probe.ProbeAll(ctx, targets) // fails only for an empty node list, and targets is never empty here
		adv := advisor.NewPlacementAdvisor()
		plan, _ := adv.Advise(hwList, advisor.DefaultDendriteServices()) // likewise fails only on empty lists

		data := IndentJSON(plan)
		return TextResult(string(data)), nil
	}
}

// gatewayRenderHandler handles the dendrite_gateway_render tool.
func gatewayRenderHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		scStr := "local-hybrid"
		if s, ok := args["scenario"].(string); ok && s != "" {
			scStr = s
		}
		sc, err := topology.ParseScenario(scStr)
		if err != nil {
			return nil, err
		}
		// A parsed scenario with these fixed endpoints always validates and renders.
		top, _ := topology.NewTopologyConfig(sc, topology.TailscaleEndpoints{
			ControlPlane:   "100.84.12.10",
			ExecutionPlane: "100.84.12.25",
		})
		yamlData, _ := gateway.RenderForTopology(top)
		return TextResult(string(yamlData)), nil
	}
}

// gatewayUpdateHandler handles the dendrite_gateway_update tool.
func gatewayUpdateHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

		data := IndentJSON(rep)
		return TextResult(string(data)), nil
	}
}
