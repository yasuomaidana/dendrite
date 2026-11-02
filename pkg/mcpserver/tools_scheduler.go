package mcpserver

import (
	"context"
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
	addTool(s, "dendrite_analyze_policy", "Inspect or configure the Token-Aware Hybrid Scheduler policy (night local window, weekly quota reset, 48h pre-reset cloud sprint)",
		propsSchema(map[string]interface{}{
			"night_window":        typedProp("string", "Scheduled zero-cost night window in 'HH:MM-HH:MM' format (e.g. '23:00-06:00')"),
			"weekly_reset":        typedProp("string", "Weekly quota reset time in 'DAY HH:MM' format (e.g. 'SUN 23:59')"),
			"sprint_before_reset": typedProp("string", "Duration before weekly reset to activate cloud sprint (e.g. '48h')"),
			"reserve_floor":       typedProp("number", "Safety reserve quota floor (0.0 to 1.0, default 0.20 for 20% reserve)"),
			"allow_zero":          typedProp("boolean", "Override to permit draining quota down to 0% (bypasses safety reserve floor)"),
			"mode":                typedProp("string", "Operating mode: 'auto', 'always', 'night', 'sprint', or 'paused'"),
		}),
		func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

			return resultOrError(map[string]interface{}{
				"updated":  updated,
				"policy":   defaultMCPScheduler.GetPolicy(),
				"governor": defaultMCPGovernor.GetState(),
			}, nil, ""), nil
		})

	// 42. dendrite_analyze_status
	addTool(s, "dendrite_analyze_status", "Query real-time status of the Token-Aware Hybrid Scheduler, quota governor, active execution tier, in-night/in-sprint window booleans, and cloud token spend",
		propsSchema(map[string]interface{}{
			"remaining_quota": typedProp("number", "Optional simulated or current remaining quota ratio (0.0 to 1.0)"),
		}),
		func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
			if rq, ok := args["remaining_quota"].(float64); ok {
				defaultMCPScheduler.SetRemainingQuota(rq)
				defaultMCPGovernor.SetRemainingQuota(rq)
			}

			st, _ := defaultMCPScheduler.Status() // the policy was validated when it was set, so it always evaluates

			govState := defaultMCPGovernor.GetState()

			return resultOrError(map[string]interface{}{
				"scheduler": st,
				"governor":  govState,
			}, nil, ""), nil
		})

	// 43. dendrite_analyze_trigger
	addTool(s, "dendrite_analyze_trigger", "Trigger or simulate execution of an analysis chunk under current or simulated token scheduling constraints",
		objectSchema([]string{"chunk_id"}, map[string]interface{}{
			"chunk_id":        typedProp("string", "Identifier of the code or AST chunk to analyze"),
			"simulate_night":  typedProp("boolean", "If true, simulates execution inside the scheduled night window"),
			"simulate_sprint": typedProp("boolean", "If true, simulates execution inside the 48h pre-reset cloud sprint"),
		}),
		func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

			return resultOrError(res, nil, ""), nil
		})

	// 44. dendrite_analyze_switch
	addTool(s, "dendrite_analyze_switch", "Toggle Quota Governor switches (allow_zero drain override past 20% floor, or operational analysis mode)",
		propsSchema(map[string]interface{}{
			"allow_zero": typedProp("boolean", "If true, allows draining quota down to 0%; if false, halts cloud sprint at 20% reserve floor"),
			"mode":       typedProp("string", "Switch operating mode: 'always', 'policy', or 'paused'"),
		}),
		func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
			if az, ok := args["allow_zero"].(bool); ok {
				_ = defaultMCPGovernor.SetAllowZero(az)
			}
			if mStr, ok := args["mode"].(string); ok && mStr != "" {
				_ = defaultMCPGovernor.SetMode(governor.AnalysisMode(mStr))
			}

			state := defaultMCPGovernor.GetState()
			return resultOrError(state, nil, ""), nil
		})

	// 47. dendrite_tokens_status
	addTool(s, "dendrite_tokens_status", "Inspect remaining tokens in the day window and week window, and confirm Google Antigravity (AGY) connectivity and availability",
		propsSchema(map[string]interface{}{
			"model":       typedProp("string", "Target model identifier (default: \"gemini-3.8-flash-high\")"),
			"probe":       typedProp("boolean", "If true, executes an active probe to confirm AGY endpoint responsiveness"),
			"spend_file":  typedProp("string", "Optional path to spend.json override"),
			"policy_file": typedProp("string", "Optional path to scheduler_policy.json override"),
		}),
		func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
			rep, _ := eval.Evaluate(ctx, cfg, probe) // Evaluate reports what it could read and never fails

			return resultOrError(rep, nil, ""), nil
		})
}
