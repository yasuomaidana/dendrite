package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/ui/penpot"
)

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
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		file, _ := args["file"].(string)
		if strings.TrimSpace(file) == "" {
			return nil, fmt.Errorf("file argument is required")
		}
		ticketID, _ := args["ticket_id"].(string)
		if ticketID == "" {
			ticketID = "DEN-810"
		}
		endpoint, _ := args["penpot_endpoint"].(string)
		if endpoint == "" {
			endpoint = "http://localhost:9000"
		}
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

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})
}
