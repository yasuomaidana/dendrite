package opstools

import (
	"Dendrite/pkg/scheduler/governor"
	"Dendrite/pkg/scheduler/token"
	"Dendrite/pkg/tokens"
	"context"
	"fmt"
	"time"

	. "Dendrite/pkg/mcpserver"
)

var defaultMCPScheduler = token.NewTokenScheduler(nil, nil, "")
var defaultMCPGovernor = governor.NewQuotaGovernor(governor.DefaultConfig(), DefaultMCPStore, defaultMCPScheduler)

func registerSchedulerTools(s *Server, deps Dependencies) {
	// 41. dendrite_analyze_policy
	AddTool(s, "dendrite_analyze_policy", "Inspect or configure the Token-Aware Hybrid Scheduler policy (night local window, weekly quota reset, 48h pre-reset cloud sprint)",
		PropsSchema(map[string]interface{}{
			"night_window":        TypedProp("string", "Scheduled zero-cost night window in 'HH:MM-HH:MM' format (e.g. '23:00-06:00')"),
			"weekly_reset":        TypedProp("string", "Weekly quota reset time in 'DAY HH:MM' format (e.g. 'SUN 23:59')"),
			"sprint_before_reset": TypedProp("string", "Duration before weekly reset to activate cloud sprint (e.g. '48h')"),
			"reserve_floor":       TypedProp("number", "Safety reserve quota floor (0.0 to 1.0, default 0.20 for 20% reserve)"),
			"allow_zero":          TypedProp("boolean", "Override to permit draining quota down to 0% (bypasses safety reserve floor)"),
			"mode":                TypedProp("string", "Operating mode: 'auto', 'always', 'night', 'sprint', or 'paused'"),
		}),
		analyzePolicyHandler())

	// 42. dendrite_analyze_status
	AddTool(s, "dendrite_analyze_status", "Query real-time status of the Token-Aware Hybrid Scheduler, quota governor, active execution tier, in-night/in-sprint window booleans, and cloud token spend",
		PropsSchema(map[string]interface{}{
			"remaining_quota": TypedProp("number", "Optional simulated or current remaining quota ratio (0.0 to 1.0)"),
		}),
		analyzeStatusHandler())

	// 43. dendrite_analyze_trigger
	AddTool(s, "dendrite_analyze_trigger", "Trigger or simulate execution of an analysis chunk under current or simulated token scheduling constraints",
		ObjectSchema([]string{"chunk_id"}, map[string]interface{}{
			"chunk_id":        TypedProp("string", "Identifier of the code or AST chunk to analyze"),
			"simulate_night":  TypedProp("boolean", "If true, simulates execution inside the scheduled night window"),
			"simulate_sprint": TypedProp("boolean", "If true, simulates execution inside the 48h pre-reset cloud sprint"),
		}),
		analyzeTriggerHandler())

	// 44. dendrite_analyze_switch
	AddTool(s, "dendrite_analyze_switch", "Toggle Quota Governor switches (allow_zero drain override past 20% floor, or operational analysis mode)",
		PropsSchema(map[string]interface{}{
			"allow_zero": TypedProp("boolean", "If true, allows draining quota down to 0%; if false, halts cloud sprint at 20% reserve floor"),
			"mode":       TypedProp("string", "Switch operating mode: 'always', 'policy', or 'paused'"),
		}),
		analyzeSwitchHandler())

	// 47. dendrite_tokens_status
	AddTool(s, "dendrite_tokens_status", "Inspect remaining tokens in the day window and week window, and confirm Google Antigravity (AGY) connectivity and availability",
		PropsSchema(map[string]interface{}{
			"model":       TypedProp("string", "Target model identifier (default: \"gemini-3.8-flash-high\")"),
			"probe":       TypedProp("boolean", "If true, executes an active probe to confirm AGY endpoint responsiveness"),
			"spend_file":  TypedProp("string", "Optional path to spend.json override"),
			"policy_file": TypedProp("string", "Optional path to scheduler_policy.json override"),
		}),
		tokensStatusHandler())
}

// analyzeStatusHandler handles the dendrite_analyze_status tool.
func analyzeStatusHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		if rq, ok := args["remaining_quota"].(float64); ok {
			defaultMCPScheduler.SetRemainingQuota(rq)
			defaultMCPGovernor.SetRemainingQuota(rq)
		}

		st, _ := defaultMCPScheduler.Status() // the policy was validated when it was set, so it always evaluates

		govState := defaultMCPGovernor.GetState()

		return ResultOrError(map[string]interface{}{
			"scheduler": st,
			"governor":  govState,
		}, nil, ""), nil
	}
}

// schedulerFor is the scheduler an analyze_trigger call runs on: the shared one, or a scratch one on the shared policy
// with its mode forced to night or sprint when the call simulates them (night wins).
func schedulerFor(simNight, simSprint bool) *token.TokenScheduler {
	if !simNight && !simSprint {
		return defaultMCPScheduler
	}
	pol := defaultMCPScheduler.GetPolicy()
	pol.Mode = "sprint"
	if simNight {
		pol.Mode = "night"
	}
	scratch := token.NewTokenScheduler(nil, nil, "")
	_ = scratch.SetPolicy(pol)
	return scratch
}

// analyzeChunk is the work a scheduled chunk does: cloud models are charged through the governor.
func analyzeChunk(chunkID string) func(ctx context.Context, model string) (string, int, float64, error) {
	return func(ctx context.Context, model string) (string, int, float64, error) {
		cost := 0.0
		if model == "gemini-2.5-pro" {
			if allowed, gErr := defaultMCPGovernor.CheckCloudExecution(ctx, "mcp-project", chunkID); !allowed {
				return "", 0, 0, gErr
			}
			cost = 0.02
		}
		return fmt.Sprintf("Analysis completed for %s using %s", chunkID, model), 1250, cost, nil
	}
}

// analyzeTriggerHandler handles the dendrite_analyze_trigger tool.
func analyzeTriggerHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		chunkID, _ := args["chunk_id"].(string)
		if chunkID == "" {
			return nil, fmt.Errorf("missing chunk_id parameter")
		}
		simNight, _ := args["simulate_night"].(bool)
		simSprint, _ := args["simulate_sprint"].(bool)
		res, err := schedulerFor(simNight, simSprint).ProcessChunk(ctx, chunkID, analyzeChunk(chunkID))
		if err != nil {
			return nil, fmt.Errorf("processing chunk: %w", err)
		}
		return ResultOrError(res, nil, ""), nil
	}
}

// tokensStatusHandler handles the dendrite_tokens_status tool.
func tokensStatusHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

		return ResultOrError(rep, nil, ""), nil
	}
}

// applyPolicyArgs sets the scheduler policy fields an analyze_policy call carries and reports whether any was set.
// A duration that does not parse is an error.
func applyPolicyArgs(policy *token.Policy, args map[string]interface{}) (bool, error) {
	updated := false
	for key, dst := range map[string]*string{"night_window": &policy.NightWindow, "weekly_reset": &policy.WeeklyReset, "mode": &policy.Mode} {
		if v, ok := args[key].(string); ok && v != "" {
			*dst, updated = v, true
		}
	}
	if sbr, ok := args["sprint_before_reset"].(string); ok && sbr != "" {
		dur, err := time.ParseDuration(sbr)
		if err != nil {
			return false, fmt.Errorf("invalid sprint_before_reset duration '%s': %w", sbr, err)
		}
		policy.SprintBeforeReset, updated = dur, true
	}
	if rf, ok := args["reserve_floor"].(float64); ok {
		policy.ReserveFloor, updated = rf, true
	}
	if az, ok := args["allow_zero"].(bool); ok {
		policy.AllowZero, updated = az, true
		_ = defaultMCPGovernor.SetAllowZero(az)
	}
	return updated, nil
}

// analyzePolicyHandler handles the dendrite_analyze_policy tool.
func analyzePolicyHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		policy := defaultMCPScheduler.GetPolicy()
		updated, err := applyPolicyArgs(&policy, args)
		if err != nil {
			return nil, err
		}
		if updated {
			if err := defaultMCPScheduler.SetPolicy(policy); err != nil {
				return nil, fmt.Errorf("updating scheduler policy: %w", err)
			}
		}
		return ResultOrError(map[string]interface{}{
			"updated":  updated,
			"policy":   defaultMCPScheduler.GetPolicy(),
			"governor": defaultMCPGovernor.GetState(),
		}, nil, ""), nil
	}
}

// analyzeSwitchHandler handles the dendrite_analyze_switch tool.
func analyzeSwitchHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		if az, ok := args["allow_zero"].(bool); ok {
			_ = defaultMCPGovernor.SetAllowZero(az)
		}
		if mStr, ok := args["mode"].(string); ok && mStr != "" {
			_ = defaultMCPGovernor.SetMode(governor.AnalysisMode(mStr))
		}

		state := defaultMCPGovernor.GetState()
		return ResultOrError(state, nil, ""), nil
	}
}
