package opstools

import (
	"Dendrite/pkg/personalcontext/canvas"
	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/ui/penpot"
	"context"
	"fmt"
	"strings"

	. "Dendrite/pkg/mcpserver"
)

func RegisterCanvasTools(s *Server, deps Dependencies) {
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
	}, canvasActionDispatchHandler(registry))

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
	}, canvasPenpotBridgeHandler(deps, bridge))
}

// canvasActionDispatchHandler handles the dendrite_canvas_action_dispatch tool.
func canvasActionDispatchHandler(registry *canvas.Registry) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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

		data := IndentJSON(resp)

		return &CallToolResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: string(data),
				},
			},
		}, nil
	}
}

// penpotBridgeFor is the bridge a call uses: the registered one, or one on the Plane client of the environment when
// the server was given none.
func penpotBridgeFor(deps Dependencies, bridge *canvas.PenpotUIBridge) *canvas.PenpotUIBridge {
	if deps.PlaneClient == nil {
		if client, err := plane.NewClientFromEnv(); err == nil && client != nil {
			return canvas.NewPenpotUIBridge(client, "http://localhost:9000")
		}
	}
	return bridge
}

func penpotRelate(ctx context.Context, b *canvas.PenpotUIBridge, args map[string]interface{}) (interface{}, error) {
	ticketID, _ := args["ticket_id"].(string)
	featureName, _ := args["feature_name"].(string)
	filePath, _ := args["file_path"].(string)
	penpotURL, _ := args["penpot_frame_url"].(string)
	comment, _ := args["comment"].(string)
	rel := canvas.TicketFeatureRelation{TicketID: ticketID, FeatureName: featureName, ComponentPath: filePath, PenpotFrameURL: penpotURL, Description: comment}
	if err := b.RelateTicketToFeature(ctx, rel); err != nil {
		return nil, err
	}
	saved, _ := b.GetTicketFeatureRelation(ticketID)
	return map[string]interface{}{
		"status":   "success",
		"message":  fmt.Sprintf("Ticket '%s' bound to feature '%s'", ticketID, featureName),
		"relation": saved,
	}, nil
}

func penpotSync(ctx context.Context, b *canvas.PenpotUIBridge, args map[string]interface{}) (interface{}, error) {
	ticketID, _ := args["ticket_id"].(string)
	filePath, _ := args["file_path"].(string)
	dryRun, _ := args["dry_run"].(bool)
	return b.SyncComponentWithPenpot(ctx, ticketID, filePath, dryRun)
}

func penpotTransition(ctx context.Context, b *canvas.PenpotUIBridge, args map[string]interface{}) (interface{}, error) {
	ticketID, _ := args["ticket_id"].(string)
	targetState, _ := args["target_state"].(string)
	penpotURL, _ := args["penpot_frame_url"].(string)
	comment, _ := args["comment"].(string)
	return b.UpdateProjectState(ctx, canvas.ProjectStateUpdateRequest{
		TicketID:       ticketID,
		TargetState:    targetState,
		Comment:        comment,
		PenpotFrameURL: penpotURL,
		PushToPlane:    BoolArgOr(args, "push_to_plane", true),
	})
}

func penpotList(b *canvas.PenpotUIBridge) interface{} {
	relations := b.ListTicketRelations()
	return map[string]interface{}{"count": len(relations), "relations": relations}
}

// canvasPenpotBridgeHandler handles the dendrite_canvas_penpot_bridge tool.
func canvasPenpotBridgeHandler(deps Dependencies, bridge *canvas.PenpotUIBridge) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		subaction, _ := args["subaction"].(string)
		b := penpotBridgeFor(deps, bridge)
		var result interface{}
		var err error
		switch strings.ToLower(strings.TrimSpace(subaction)) {
		case "relate":
			result, err = penpotRelate(ctx, b, args)
		case "sync":
			result, err = penpotSync(ctx, b, args)
		case "update_state", "update-state", "transition":
			result, err = penpotTransition(ctx, b, args)
		case "list":
			result = penpotList(b)
		default:
			err = fmt.Errorf("unknown subaction: %s (expected: 'relate', 'sync', 'update_state', or 'list')", subaction)
		}
		if err != nil {
			return nil, err
		}
		return TextResult(string(IndentJSON(result))), nil
	}
}

func registerPenpotTools(s *Server, deps Dependencies) {
	// dendrite_ui_penpot_sync
	s.RegisterTool(Tool{
		Name:        "dendrite_ui_penpot_sync",
		Description: "Synchronize UI component development (.svelte, .tsx, .vue, .css) with Penpot design frames and post deep-links to Plane issues",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file": map[string]interface{}{
					"type":        "string",
					"description": "Path to UI component file (e.g. 'web/src/components/Button.svelte')",
				},
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "Target Plane ticket ID (e.g. 'DEN-810')",
				},
				"penpot_endpoint": map[string]interface{}{
					"type":        "string",
					"description": "Penpot service endpoint (default: 'http://localhost:9000')",
				},
				"comment_plane": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, posts deep-link comment to Plane issue (default: true)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, simulates synchronization without modifying Plane or Penpot",
				},
			},
			"required": []interface{}{"file"},
		},
	}, uiPenpotSyncHandler(deps))
}

// uiPenpotSyncHandler handles the dendrite_ui_penpot_sync tool.
func uiPenpotSyncHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		file, _ := args["file"].(string)
		if strings.TrimSpace(file) == "" {
			return nil, fmt.Errorf("file argument is required")
		}
		ticketID := StringArgOr(args, "ticket_id", "DEN-810")
		endpoint := StringArgOr(args, "penpot_endpoint", "http://localhost:9000")
		commentPlane := true
		if cp, ok := args["comment_plane"].(bool); ok {
			commentPlane = cp
		}
		dryRun, _ := args["dry_run"].(bool)

		syncer := penpot.NewSynchronizer(deps.PlaneClient)
		res, err := syncer.Sync(ctx, penpot.SyncOptions{
			TicketID:       ticketID,
			FilePath:       file,
			PenpotEndpoint: endpoint,
			DryRun:         dryRun,
			CommentPlane:   commentPlane,
		})
		if err != nil {
			return nil, err
		}

		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}
