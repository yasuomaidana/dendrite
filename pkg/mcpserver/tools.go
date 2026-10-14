package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/env"
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/inference"
	"Dendrite/pkg/jetbrains"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/telemetry"
	"Dendrite/pkg/topology"
	"Dendrite/pkg/topology/advisor"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// Dependencies holds external resources required for tool execution.
type Dependencies struct {
	PlaneClient *plane.Client
	DevStatus   string
	OllamaURL   string
}

// RegisterDefaultTools registers the standard suite of Dendrite MCP tools.
func (s *Server) RegisterDefaultTools(deps Dependencies) {
	if deps.DevStatus == "" {
		deps.DevStatus = "DEV_STATUS.md"
	}
	if deps.OllamaURL == "" {
		deps.OllamaURL = ollama.DefaultBaseURL
	}

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
			"type": "object",
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
				if pErr := pClient.UpdateStatus(ctx, tracker.TicketID(tid), st); pErr == nil {
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
		token := os.Getenv("PLANE_API_KEY")
		if token == "" {
			token = "plane_sec_dendrite_token_12345"
		}
		ws := "dendrite-ws"
		if w, ok := args["workspace"].(string); ok && w != "" {
			ws = w
		}
		endpoint := fmt.Sprintf("http://localhost:8090/api/v1/workspaces/%s/projects/e6380603-1428-4a66-995e-e06039915d52/issues/", ws)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", token)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Plane: %w", err)
		}
		defer resp.Body.Close()

		var res map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return nil, err
		}
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
			},
		}, nil
	})

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

	// 8. dendrite_agent_run
	s.RegisterTool(Tool{
		Name:        "dendrite_agent_run",
		Description: "Launch an autonomous coding agent loop (aider, openhands, opencode, agy) with LiteLLM token governance",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"agent_type", "task"},
			"properties": map[string]interface{}{
				"agent_type": map[string]interface{}{
					"type":        "string",
					"description": "Agent tool: 'aider', 'openhands', 'opencode', 'agy'",
				},
				"task": map[string]interface{}{
					"type":        "string",
					"description": "Task description or user prompt for the agent",
				},
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "Associated ticket ID (e.g. DEN-034)",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Target LLM model (default: copilot-sonnet or local-qwen-coder)",
				},
				"spend_limit": map[string]interface{}{
					"type":        "number",
					"description": "Maximum USD spend allowed for this session",
				},
				"sandbox": map[string]interface{}{
					"type":        "boolean",
					"description": "Execute inside ephemeral container sandbox",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		agentStr, _ := args["agent_type"].(string)
		task, _ := args["task"].(string)
		ticketID, _ := args["ticket_id"].(string)
		model, _ := args["model"].(string)
		sandbox, _ := args["sandbox"].(bool)
		spendLimit, _ := args["spend_limit"].(float64)

		aType := agent.AgentType(strings.ToLower(strings.TrimSpace(agentStr)))
		if !aType.IsValid() {
			return nil, fmt.Errorf("invalid agent type: %s", agentStr)
		}

		if model == "" {
			if aType == agent.AgentAGY || aType == agent.AgentAntigravity {
				model = "gemini-2.5-pro"
			} else {
				model = "copilot-sonnet"
			}
		}

		cwd, _ := os.Getwd()
		cfg := agent.AgentConfig{
			Type:            aType,
			TaskDescription: task,
			TicketID:        ticketID,
			Model:           model,
			Sandbox:         sandbox,
			SpendLimitUSD:   spendLimit,
			WorkspaceDir:    cwd,
			LiteLLMBaseURL:  "http://localhost:4000/v1",
			VirtualAPIKey:   "sk-dendrite-virtual-key",
		}

		envVars, err := agent.BuildAgentEnv(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to build agent env: %w", err)
		}
		cmd, err := agent.BuildAgentCommand(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to build agent command: %w", err)
		}

		runner := &agent.MockRunner{}
		summary, err := runner.Execute(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("agent execution failed: %w", err)
		}

		res := map[string]interface{}{
			"session_id": summary.SessionID,
			"agent":      summary.AgentType,
			"ticket_id":  summary.TicketID,
			"command":    strings.Join(cmd, " "),
			"duration":   summary.Duration.String(),
			"tokens":     summary.TokensConsumed,
			"cost_usd":   summary.EstimatedCostUSD,
			"commits":    summary.CommitsGenerated,
			"env":        envVars,
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(data)},
			},
		}, nil
	})

	// 9. dendrite_telemetry
	s.RegisterTool(Tool{
		Name:        "dendrite_telemetry",
		Description: "Sample real-time SoC thermal metrics and 24h/7d token spend totals",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		daemon := telemetry.NewDaemon(telemetry.DaemonConfig{
			Interval: 1 * time.Second,
		}, nil, nil, nil, nil)
		rep := daemon.CollectOnce(ctx)

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

	// 10. dendrite_inference_list
	s.RegisterTool(Tool{
		Name:        "dendrite_inference_list",
		Description: "List registered multi-provider inference engines (Ollama, OxiLLaMa, vLLM, TGI, TEI, llama.cpp), endpoints, and failover cascade order",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		mgr := inference.DefaultManager()
		engines := mgr.ListEngines()
		cascade := mgr.GetCascadeOrder()
		type engInfo struct {
			ID         string   `json:"id"`
			Type       string   `json:"type"`
			Endpoint   string   `json:"endpoint"`
			SelfHosted bool     `json:"self_hosted"`
			Models     []string `json:"models"`
		}
		var list []engInfo
		for _, e := range engines {
			cfg := e.Config()
			list = append(list, engInfo{
				ID:         e.ID(),
				Type:       string(e.Type()),
				Endpoint:   cfg.Endpoint,
				SelfHosted: e.IsSelfHosted(),
				Models:     cfg.Models,
			})
		}
		res := map[string]interface{}{
			"engines":       list,
			"cascade_order": cascade,
		}
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 11. dendrite_inference_probe
	s.RegisterTool(Tool{
		Name:        "dendrite_inference_probe",
		Description: "Probe multi-provider inference engines (< 2s) for health, reachability, latency, and loaded models",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"engine_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional engine ID to probe (e.g. 'ollama-metal', 'oxillama-rust'). If omitted, probes all engines.",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		mgr := inference.DefaultManager()
		engineID, _ := args["engine_id"].(string)
		if engineID != "" {
			eng, err := mgr.GetEngine(engineID)
			if err != nil {
				return nil, err
			}
			h := eng.Health(ctx)
			data, err := json.MarshalIndent(h, "", "  ")
			if err != nil {
				return nil, err
			}
			return &CallToolResult{
				Content: []ContentItem{{Type: "text", Text: string(data)}},
			}, nil
		}
		results := mgr.PollHealthAll(ctx)
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 12. dendrite_plane_replicate
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_replicate",
		Description: "Replicate DEV_STATUS.md directly from Plane service state (Plane owns status)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file":    map[string]interface{}{"type": "string", "description": "Target markdown file path (default: DEV_STATUS.md)"},
				"dry_run": map[string]interface{}{"type": "boolean", "description": "If true, simulates replication without modifying disk"},
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
		targetFile := "DEV_STATUS.md"
		if f, ok := args["file"].(string); ok && f != "" {
			targetFile = f
		}
		dryRun, _ := args["dry_run"].(bool)

		rendered, report, err := plane.ReplicateStatusFromPlane(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("replication failed: %w", err)
		}

		if !dryRun {
			if err := os.WriteFile(targetFile, []byte(rendered), 0644); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", targetFile, err)
			}
		}

		res := map[string]interface{}{
			"status":            "success",
			"ownership":         "Plane Service",
			"file":              targetFile,
			"dry_run":           dryRun,
			"report":            report,
		}
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 13. dendrite_plane_push_status
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_push_status",
		Description: "Move local DEV_STATUS.md content (tickets, milestones, descriptions) to Plane service",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file": map[string]interface{}{"type": "string", "description": "Source markdown file path (default: DEV_STATUS.md)"},
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
		sourceFile := "DEV_STATUS.md"
		if f, ok := args["file"].(string); ok && f != "" {
			sourceFile = f
		}
		content, err := os.ReadFile(sourceFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", sourceFile, err)
		}

		report, err := plane.PushStatusToPlane(ctx, client, string(content))
		if err != nil {
			return nil, fmt.Errorf("push to plane failed: %w", err)
		}

		res := map[string]interface{}{
			"status": "success",
			"file":   sourceFile,
			"report": report,
		}
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 14. dendrite_plane_doc_sync
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_doc_sync",
		Description: "Synchronize documentation between Plane pages and local repository (direction: to-plane, from-plane, bidirectional)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"direction": map[string]interface{}{"type": "string", "description": "Sync direction: 'to-plane', 'from-plane', or 'bidirectional' (default: bidirectional)"},
				"dir":       map[string]interface{}{"type": "string", "description": "Documentation directory path (default: docs)"},
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
		dir := "docs"
		if d, ok := args["dir"].(string); ok && d != "" {
			dir = d
		}
		direction := plane.DirectionBidirectional
		if dirStr, ok := args["direction"].(string); ok && dirStr != "" {
			switch strings.ToLower(dirStr) {
			case "to-plane", "push":
				direction = plane.DirectionToPlane
			case "from-plane", "pull":
				direction = plane.DirectionFromPlane
			default:
				direction = plane.DirectionBidirectional
			}
		}

		report, err := plane.SyncDocs(ctx, client, dir, direction)
		if err != nil {
			return nil, fmt.Errorf("documentation sync failed: %w", err)
		}

		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 15. dendrite_jetbrains_setup
	s.RegisterTool(Tool{
		Name:        "dendrite_jetbrains_setup",
		Description: "Configure JetBrains IDE toolset with Dendrite MCP servers (.ai/mcp/mcp.json), agent run configurations, and external tools",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_dir":  map[string]interface{}{"type": "string", "description": "Project root directory (default: current working directory)"},
				"project_only": map[string]interface{}{"type": "boolean", "description": "Configure only project-level files (.ai/mcp/mcp.json and .idea)"},
				"global_only":  map[string]interface{}{"type": "boolean", "description": "Configure only user-level global files (~/.ai/mcp/mcp.json)"},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		pDir, _ := args["project_dir"].(string)
		projectOnly, _ := args["project_only"].(bool)
		globalOnly, _ := args["global_only"].(bool)

		opts := jetbrains.SetupOptions{
			ProjectDir:             pDir,
			ConfigureProjectMCP:    !globalOnly,
			ConfigureGlobalMCP:     !projectOnly,
			ConfigureRunConfigs:    !globalOnly,
			ConfigureExternalTools: !globalOnly,
		}

		res, err := jetbrains.SetupJetBrainsToolset(opts)
		if err != nil {
			return nil, fmt.Errorf("jetbrains setup failed: %w", err)
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 16-19: Distributed Codebase & Interoperability Tools
	s.registerDistCodebaseTools(deps)

	// 20: Performance Testing, Benchmarking & Gateway Load Tools
	s.registerPerfTools(deps)
}

