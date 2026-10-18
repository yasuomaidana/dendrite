package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"Dendrite/pkg/scheduler/governor"
	"Dendrite/pkg/scheduler/token"
	"Dendrite/pkg/tokens"
)

var defaultMCPScheduler = token.NewTokenScheduler(nil, nil, "")
var defaultMCPGovernor = governor.NewQuotaGovernor(governor.DefaultConfig(), defaultMCPStore, defaultMCPScheduler)

func registerSchedulerTools(s *Server, deps Dependencies) {
	// 41. dendrite_analyze_policy
	s.RegisterTool(Tool{
		Name:        "dendrite_analyze_policy",
		Description: "Inspect or configure the Token-Aware Hybrid Scheduler policy (night local window, weekly quota reset, 48h pre-reset cloud sprint)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"night_window": map[string]interface{}{
					"type":        "string",
					"description": "Scheduled zero-cost night window in 'HH:MM-HH:MM' format (e.g. '23:00-06:00')",
				},
				"weekly_reset": map[string]interface{}{
					"type":        "string",
					"description": "Weekly quota reset time in 'DAY HH:MM' format (e.g. 'SUN 23:59')",
				},
				"sprint_before_reset": map[string]interface{}{
					"type":        "string",
					"description": "Duration before weekly reset to activate cloud sprint (e.g. '48h')",
				},
				"reserve_floor": map[string]interface{}{
					"type":        "number",
					"description": "Safety reserve quota floor (0.0 to 1.0, default 0.20 for 20% reserve)",
				},
				"allow_zero": map[string]interface{}{
					"type":        "boolean",
					"description": "Override to permit draining quota down to 0% (bypasses safety reserve floor)",
				},
				"mode": map[string]interface{}{
					"type":        "string",
					"description": "Operating mode: 'auto', 'always', 'night', 'sprint', or 'paused'",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		currentPolicy := defaultMCPScheduler.GetPolicy()
		updated := false

		if nw, ok := args["night_window"].(string); ok && nw != "" {
			currentPolicy.NightWindow = nw
			updated = true
		}
		if wr, ok := args["weekly_reset"].(string); ok && wr != "" {
			currentPolicy.WeeklyReset = wr
			updated = true
		}
		if sbr, ok := args["sprint_before_reset"].(string); ok && sbr != "" {
			dur, err := time.ParseDuration(sbr)
			if err != nil {
				return nil, fmt.Errorf("invalid sprint_before_reset duration '%s': %w", sbr, err)
			}
			currentPolicy.SprintBeforeReset = dur
			updated = true
		}
		if rf, ok := args["reserve_floor"].(float64); ok {
			currentPolicy.ReserveFloor = rf
			updated = true
		}
		if az, ok := args["allow_zero"].(bool); ok {
			currentPolicy.AllowZero = az
			_ = defaultMCPGovernor.SetAllowZero(az)
			updated = true
		}
		if mode, ok := args["mode"].(string); ok && mode != "" {
			currentPolicy.Mode = mode
			updated = true
		}

		if updated {
			if err := defaultMCPScheduler.SetPolicy(currentPolicy); err != nil {
				return nil, fmt.Errorf("updating scheduler policy: %w", err)
			}
		}

		bytes, err := json.MarshalIndent(map[string]interface{}{
			"updated":  updated,
			"policy":   defaultMCPScheduler.GetPolicy(),
			"governor": defaultMCPGovernor.GetState(),
		}, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(bytes),
				},
			},
		}, nil
	})

	// 42. dendrite_analyze_status
	s.RegisterTool(Tool{
		Name:        "dendrite_analyze_status",
		Description: "Query real-time status of the Token-Aware Hybrid Scheduler, quota governor, active execution tier, in-night/in-sprint window booleans, and cloud token spend",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"remaining_quota": map[string]interface{}{
					"type":        "number",
					"description": "Optional simulated or current remaining quota ratio (0.0 to 1.0)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		if rq, ok := args["remaining_quota"].(float64); ok {
			defaultMCPScheduler.SetRemainingQuota(rq)
			defaultMCPGovernor.SetRemainingQuota(rq)
		}

		st, err := defaultMCPScheduler.Status()
		if err != nil {
			return nil, fmt.Errorf("evaluating scheduler status: %w", err)
		}

		govState := defaultMCPGovernor.GetState()

		bytes, err := json.MarshalIndent(map[string]interface{}{
			"scheduler": st,
			"governor":  govState,
		}, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(bytes),
				},
			},
		}, nil
	})

	// 43. dendrite_analyze_trigger
	s.RegisterTool(Tool{
		Name:        "dendrite_analyze_trigger",
		Description: "Trigger or simulate execution of an analysis chunk under current or simulated token scheduling constraints",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"chunk_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the code or AST chunk to analyze",
				},
				"simulate_night": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, simulates execution inside the scheduled night window",
				},
				"simulate_sprint": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, simulates execution inside the 48h pre-reset cloud sprint",
				},
			},
			"required": []interface{}{"chunk_id"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		chunkID, _ := args["chunk_id"].(string)
		if chunkID == "" {
			return nil, fmt.Errorf("missing chunk_id parameter")
		}

		simNight, _ := args["simulate_night"].(bool)
		simSprint, _ := args["simulate_sprint"].(bool)

		sched := defaultMCPScheduler
		if simNight || simSprint {
			pol := sched.GetPolicy()
			if simNight {
				pol.Mode = "night"
			} else if simSprint {
				pol.Mode = "sprint"
			}
			tempSched := token.NewTokenScheduler(nil, nil, "")
			_ = tempSched.SetPolicy(pol)
			sched = tempSched
		}

		res, err := sched.ProcessChunk(ctx, chunkID, func(ctx context.Context, model string) (string, int, float64, error) {
			cost := 0.0
			if model == "gemini-2.5-pro" {
				allowed, gErr := defaultMCPGovernor.CheckCloudExecution(ctx, "mcp-project", chunkID)
				if !allowed {
					return "", 0, 0, gErr
				}
				cost = 0.02
			}
			return fmt.Sprintf("Analysis completed for %s using %s", chunkID, model), 1250, cost, nil
		})
		if err != nil {
			return nil, fmt.Errorf("processing chunk: %w", err)
		}

		bytes, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(bytes),
				},
			},
		}, nil
	})

	// 44. dendrite_analyze_switch
	s.RegisterTool(Tool{
		Name:        "dendrite_analyze_switch",
		Description: "Toggle Quota Governor switches (allow_zero drain override past 20% floor, or operational analysis mode)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"allow_zero": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, allows draining quota down to 0%; if false, halts cloud sprint at 20% reserve floor",
				},
				"mode": map[string]interface{}{
					"type":        "string",
					"description": "Switch operating mode: 'always', 'policy', or 'paused'",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		if az, ok := args["allow_zero"].(bool); ok {
			_ = defaultMCPGovernor.SetAllowZero(az)
		}
		if mStr, ok := args["mode"].(string); ok && mStr != "" {
			_ = defaultMCPGovernor.SetMode(governor.AnalysisMode(mStr))
		}

		state := defaultMCPGovernor.GetState()
		bytes, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(bytes),
				},
			},
		}, nil
	})

	// 47. dendrite_tokens_status
	s.RegisterTool(Tool{
		Name:        "dendrite_tokens_status",
		Description: "Inspect remaining tokens in the day window and week window, and confirm Google Antigravity (AGY) connectivity and availability",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Target model identifier (default: \"gemini-3.8-flash-high\")",
				},
				"probe": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, executes an active probe to confirm AGY endpoint responsiveness",
				},
				"spend_file": map[string]interface{}{
					"type":        "string",
					"description": "Optional path to spend.json override",
				},
				"policy_file": map[string]interface{}{
					"type":        "string",
					"description": "Optional path to scheduler_policy.json override",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		cfg := tokens.DefaultConfig()
		if m, ok := args["model"].(string); ok && m != "" {
			cfg.Model = m
		}
		if sf, ok := args["spend_file"].(string); ok && sf != "" {
			cfg.SpendFile = sf
		}
		if pf, ok := args["policy_file"].(string); ok && pf != "" {
			cfg.PolicyFile = pf
		}
		probe, _ := args["probe"].(bool)

		eval := tokens.NewEvaluator(nil, nil)
		rep, err := eval.Evaluate(ctx, cfg, probe)
		if err != nil {
			return nil, fmt.Errorf("evaluating token status: %w", err)
		}

		bytes, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(bytes),
				},
			},
		}, nil
	})
}
