package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/personalcontext/greeting"
)

func (s *Server) registerGreetingTools(deps Dependencies) {
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
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		uCtx := greeting.UserJourneyContext{}

		if v, ok := args["topic"].(string); ok {
			uCtx.Topic = v
		}
		if v, ok := args["decision"].(string); ok {
			uCtx.Decision = v
		}
		if v, ok := args["milestone"].(string); ok {
			uCtx.Milestone = v
		}
		if v, ok := args["milestone_achieved"].(bool); ok {
			uCtx.MilestoneAchieved = v
		}
		if v, ok := args["event"].(string); ok {
			uCtx.Event = v
		}
		if v, ok := args["days_until_event"].(float64); ok {
			uCtx.DaysUntilEvent = int(v)
		}
		if v, ok := args["unfinished_step"].(string); ok {
			uCtx.UnfinishedStep = v
		}
		if v, ok := args["specific_challenge"].(string); ok {
			uCtx.SpecificChallenge = v
		}
		if v, ok := args["curiosity_angle"].(string); ok {
			uCtx.CuriosityAngle = v
		}
		if v, ok := args["days_since_last_active"].(float64); ok {
			uCtx.DaysSinceLastActive = int(v)
		}
		if v, ok := args["turn_count"].(float64); ok {
			uCtx.TurnCount = int(v)
		}
		if v, ok := args["high_stress"].(bool); ok {
			uCtx.HighStress = v
		}
		if v, ok := args["low_pressure_hobby"].(bool); ok {
			uCtx.LowPressureHobby = v
		}

		if rawOpts, ok := args["options"].([]interface{}); ok {
			for _, o := range rawOpts {
				if s, ok := o.(string); ok {
					uCtx.Options = append(uCtx.Options, s)
				}
			}
		}

		var recentStyles []greeting.StyleID
		if rawStyles, ok := args["recent_styles"].([]interface{}); ok {
			for _, r := range rawStyles {
				if s, ok := r.(string); ok {
					recentStyles = append(recentStyles, greeting.StyleID(s))
				}
			}
		}

		eng := greeting.NewEngine()
		res, err := eng.Generate(ctx, uCtx, recentStyles)
		if err != nil {
			return nil, fmt.Errorf("failed to generate greeting: %w", err)
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
