package mcpserver

import (
	"context"

	"Dendrite/pkg/personalcontext/greeting"
)

func registerGreetingTools(s *Server, deps Dependencies) {
	// dendrite_pcontext_greeting_generate
	s.RegisterTool(Tool{
		Name:        "dendrite_pcontext_greeting_generate",
		Description: "Generates a dynamic 10-style personalized headline greeting with strict cliché suppression, sliding history tracking, and journey context resolution",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"topic": map[string]interface{}{
					"type":        "string",
					"description": "User's current topic or project",
				},
				"decision": map[string]interface{}{
					"type":        "string",
					"description": "Decision point user is considering",
				},
				"milestone": map[string]interface{}{
					"type":        "string",
					"description": "Milestone title achieved or in-flight",
				},
				"milestone_achieved": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether the milestone was recently completed",
				},
				"event": map[string]interface{}{
					"type":        "string",
					"description": "Upcoming event or deadline",
				},
				"days_until_event": map[string]interface{}{
					"type":        "integer",
					"description": "Days remaining until event",
				},
				"options": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "Option alternatives user is weighing (e.g. Option A vs B)",
				},
				"unfinished_step": map[string]interface{}{
					"type":        "string",
					"description": "Pending task left from prior turn",
				},
				"specific_challenge": map[string]interface{}{
					"type":        "string",
					"description": "Complex or stressful blocker",
				},
				"curiosity_angle": map[string]interface{}{
					"type":        "string",
					"description": "Novel or unexplored angle for topic",
				},
				"days_since_last_active": map[string]interface{}{
					"type":        "integer",
					"description": "Days since the user last interacted",
				},
				"turn_count": map[string]interface{}{
					"type":        "integer",
					"description": "Number of turns in current conversation",
				},
				"high_stress": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether user context indicates high stress",
				},
				"low_pressure_hobby": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether topic is a low-stakes creative hobby",
				},
				"recent_styles": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "Recently generated style IDs to suppress (e.g. ['G-01', 'G-02'])",
				},
			},
		},
	}, pcontextGreetingGenerateHandler())
}

// pcontextGreetingGenerateHandler handles the dendrite_pcontext_greeting_generate tool.
func pcontextGreetingGenerateHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		var recentStyles []greeting.StyleID
		for _, s := range stringList(args["recent_styles"]) {
			recentStyles = append(recentStyles, greeting.StyleID(s))
		}
		res, err := greeting.NewEngine().Generate(ctx, journeyContext(args), recentStyles)
		if err != nil {
			return errorResult("generate greeting: %v", err), nil
		}
		return textResult(string(indentJSON(res))), nil
	}
}

// journeyContext reads the user-journey fields a greeting tool call carries.
func journeyContext(args map[string]interface{}) greeting.UserJourneyContext {
	u := greeting.UserJourneyContext{Options: stringList(args["options"])}
	for key, dst := range map[string]*string{
		"topic": &u.Topic, "decision": &u.Decision, "milestone": &u.Milestone, "event": &u.Event,
		"unfinished_step": &u.UnfinishedStep, "specific_challenge": &u.SpecificChallenge, "curiosity_angle": &u.CuriosityAngle,
	} {
		setString(args, key, dst)
	}
	for key, dst := range map[string]*bool{
		"milestone_achieved": &u.MilestoneAchieved, "high_stress": &u.HighStress, "low_pressure_hobby": &u.LowPressureHobby,
	} {
		setBool(args, key, dst)
	}
	for key, dst := range map[string]*int{
		"days_until_event": &u.DaysUntilEvent, "days_since_last_active": &u.DaysSinceLastActive, "turn_count": &u.TurnCount,
	} {
		setInt(args, key, dst)
	}
	return u
}

func setString(args map[string]interface{}, key string, dst *string) {
	if v, ok := args[key].(string); ok {
		*dst = v
	}
}

func setBool(args map[string]interface{}, key string, dst *bool) {
	if v, ok := args[key].(bool); ok {
		*dst = v
	}
}

func setInt(args map[string]interface{}, key string, dst *int) {
	if v, ok := args[key].(float64); ok {
		*dst = int(v)
	}
}

// stringList reads a JSON array of strings; items that are not text are skipped.
func stringList(raw interface{}) []string {
	var out []string
	items, _ := raw.([]interface{})
	for _, o := range items {
		if s, ok := o.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
