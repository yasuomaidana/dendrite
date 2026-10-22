package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/inference"
	"Dendrite/pkg/telemetry"
)

// registerAgentRunTool registers dendrite_agent_run.
func registerAgentRunTool(s *Server, deps Dependencies) {
	// 8. dendrite_agent_run
	s.RegisterTool(Tool{
		Name:        "dendrite_agent_run",
		Description: "Launch an autonomous coding agent loop (aider, openhands, opencode, agy, copilot) with LiteLLM token governance",
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"agent_type", "task"},
			"properties": map[string]interface{}{
				"agent_type": map[string]interface{}{
					"type":        "string",
					"description": "Agent tool: 'aider', 'openhands', 'opencode', 'agy', 'copilot'",
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
					"description": "Target LLM model (default: picked from Gemini 3.8 / Argon by load for agy, the DENDRITE_COPILOT_MODEL default for copilot)",
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
				model, _ = agentjobs.DefaultDispatcher.PickAGYModel(0)
			} else if aType == agent.AgentCopilot || aType == agent.AgentGitHubCopilot {
				model = gateway.CopilotModel()
			} else {
				model = "copilot-sonnet"
			}
		}

		if aType == agent.AgentOpenHands && !sandbox {
			if _, oErr := exec.LookPath("openhands"); oErr != nil {
				sandbox = true
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

		var runner agent.Runner
		if sandbox {
			runner = agent.NewSandboxedRunner(nil, nil)
		} else {
			runner = agent.NewHostRunner(false)
		}
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
			"status":     summary.Status,
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
}

// registerTelemetryAndInferenceTools registers dendrite_telemetry, dendrite_inference_list and dendrite_inference_probe.
func registerTelemetryAndInferenceTools(s *Server, deps Dependencies) {
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
}
