package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/personalcontext/canvas"
	"Dendrite/pkg/tracker/plane"
)

func registerCanvasTools(s *Server, deps Dependencies) {
	// Initialize PenpotUIBridge with PlaneClient if available
	bridge := canvas.NewPenpotUIBridge(deps.PlaneClient, "http://localhost:9000")
	registry := canvas.NewDefaultRegistry(bridge)

	// 1. dendrite_canvas_action_dispatch
	s.RegisterTool(Tool{
		Name:        "dendrite_canvas_action_dispatch",
		Description: "Execute a Canvas interactive writing action (15 modes) or UI creative action handler with full context assembly and tone/diff formatting",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Canvas Action ID (e.g. WRITE_EMAIL, WRITE_PLAN, WRITE_REPORT, DESIGN_UI_COMPONENT, SYNC_PENPOT_FRAME, RELATE_TICKET_FEATURE, UPDATE_PROJECT_STATE)",
				},
				"mode": map[string]interface{}{
					"type":        "string",
					"description": "Writing mode: DRAFT, REFINE, EXPAND, CONDENSE, CHANGE_TONE, CRITIQUE, FORMAT (default: DRAFT)",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Title or subject of the creative artifact",
				},
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "Directive, prompt, or acceptance criteria guiding content generation",
				},
				"current_content": map[string]interface{}{
					"type":        "string",
					"description": "Existing text or code for refinement, expansion, or diff critique",
				},
				"parameters": map[string]interface{}{
					"type":        "object",
					"description": "Optional parameters (framework, component_name, tone, ticket_id, file_path, target_state, dry_run)",
				},
			},
			"required": []interface{}{"action", "prompt"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		actionStr, _ := args["action"].(string)
		if strings.TrimSpace(actionStr) == "" {
			return nil, fmt.Errorf("action is required")
		}

		promptStr, _ := args["prompt"].(string)
		titleStr, _ := args["title"].(string)
		modeStr, _ := args["mode"].(string)
		currentContent, _ := args["current_content"].(string)

		params, _ := args["parameters"].(map[string]interface{})
		if params == nil {
			params = make(map[string]interface{})
		}

		req := canvas.ActionRequest{
			Action:         canvas.ActionID(strings.TrimSpace(actionStr)),
			Mode:           canvas.WritingMode(strings.TrimSpace(modeStr)),
			Title:          titleStr,
			Prompt:         promptStr,
			CurrentContent: currentContent,
			Parameters:     params,
		}

		resp, err := registry.Dispatch(ctx, req)
		if err != nil {
			return nil, err
		}

		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal canvas response: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(data),
				},
			},
		}, nil
	})

	// 2. dendrite_canvas_penpot_bridge
	s.RegisterTool(Tool{
		Name:        "dendrite_canvas_penpot_bridge",
		Description: "Relate Plane tickets and UI project functionalities to Penpot visual design frames and execute bidirectional project state synchronization",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"subaction": map[string]interface{}{
					"type":        "string",
					"description": "Operation to perform: 'relate', 'sync', 'update_state', or 'list'",
				},
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "Target Plane ticket ID (e.g. 'DEN-807')",
				},
				"feature_name": map[string]interface{}{
					"type":        "string",
					"description": "Project feature or functionality name (e.g. 'Canvas Workspace UI')",
				},
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to UI component file (e.g. 'web/src/Canvas.svelte')",
				},
				"target_state": map[string]interface{}{
					"type":        "string",
					"description": "Target project state for transition (e.g. 'In Progress', 'To Verify', 'Done')",
				},
				"penpot_frame_url": map[string]interface{}{
					"type":        "string",
					"description": "Custom Penpot frame URL or deep-link",
				},
				"comment": map[string]interface{}{
					"type":        "string",
					"description": "Optional comment or note to attach to Plane issue",
				},
				"push_to_plane": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, synchronizes state mutation directly to Plane v2 (default: true)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, simulates synchronization without modifying remote services",
				},
			},
			"required": []interface{}{"subaction"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		subaction, _ := args["subaction"].(string)
		ticketID, _ := args["ticket_id"].(string)
		featureName, _ := args["feature_name"].(string)
		filePath, _ := args["file_path"].(string)
		targetState, _ := args["target_state"].(string)
		penpotURL, _ := args["penpot_frame_url"].(string)
		comment, _ := args["comment"].(string)
		dryRun, _ := args["dry_run"].(bool)
		pushToPlane := true
		if p, ok := args["push_to_plane"].(bool); ok {
			pushToPlane = p
		}

		// Ensure planeClient is initialized from env if not in deps
		activeBridge := bridge
		if deps.PlaneClient == nil {
			if client, err := plane.NewClientFromEnv(); err == nil && client != nil {
				activeBridge = canvas.NewPenpotUIBridge(client, "http://localhost:9000")
			}
		}

		var result interface{}

		switch strings.ToLower(strings.TrimSpace(subaction)) {
		case "relate":
			rel := canvas.TicketFeatureRelation{
				TicketID:       ticketID,
				FeatureName:    featureName,
				ComponentPath:  filePath,
				PenpotFrameURL: penpotURL,
				Description:    comment,
			}
			if err := activeBridge.RelateTicketToFeature(ctx, rel); err != nil {
				return nil, err
			}
			saved, _ := activeBridge.GetTicketFeatureRelation(ticketID)
			result = map[string]interface{}{
				"status":   "success",
				"message":  fmt.Sprintf("Ticket '%s' bound to feature '%s'", ticketID, featureName),
				"relation": saved,
			}

		case "sync":
			res, err := activeBridge.SyncComponentWithPenpot(ctx, ticketID, filePath, dryRun)
			if err != nil {
				return nil, err
			}
			result = res

		case "update_state", "update-state", "transition":
			req := canvas.ProjectStateUpdateRequest{
				TicketID:       ticketID,
				TargetState:    targetState,
				Comment:        comment,
				PenpotFrameURL: penpotURL,
				PushToPlane:    pushToPlane,
			}
			res, err := activeBridge.UpdateProjectState(ctx, req)
			if err != nil {
				return nil, err
			}
			result = res

		case "list":
			relations := activeBridge.ListTicketRelations()
			result = map[string]interface{}{
				"count":     len(relations),
				"relations": relations,
			}

		default:
			return nil, fmt.Errorf("unknown subaction: %s (expected: 'relate', 'sync', 'update_state', or 'list')", subaction)
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal bridge result: %w", err)
		}

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(data),
				},
			},
		}, nil
	})
}
