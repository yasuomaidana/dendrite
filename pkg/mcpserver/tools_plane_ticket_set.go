package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// registerPlaneTicketSetTool registers dendrite_plane_ticket_set, the MCP way to move a ticket
// through its lifecycle. Plane owns ticket state, so it never touches DEV_STATUS.md.
func registerPlaneTicketSetTool(s *Server, deps Dependencies) {
	s.RegisterTool(Tool{
		Name: "dendrite_plane_ticket_set",
		Description: "Set a ticket's workflow state in Plane (backlog, todo, in progress, to verify, done). " +
			"Plane is the owner of ticket state; completed work goes to 'to verify' for human sign-off, not 'done'.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket": map[string]interface{}{"type": "string", "description": "Ticket reference: DEN-324, its Plane number (324) or the issue UUID"},
				"status": map[string]interface{}{"type": "string", "description": "backlog, todo, in progress, to verify or done"},
			},
			"required": []string{"ticket", "status"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		ticket, _ := args["ticket"].(string)
		statusArg, _ := args["status"].(string)
		if strings.TrimSpace(ticket) == "" || strings.TrimSpace(statusArg) == "" {
			return nil, fmt.Errorf("ticket and status are required")
		}
		st, err := tracker.ParseStatus(statusArg)
		if err != nil {
			return nil, err
		}
		client := deps.PlaneClient
		if client == nil {
			if client, err = plane.NewClientFromEnv(); err != nil {
				return nil, fmt.Errorf("plane client error: %w", err)
			}
		}
		if err := client.UpdateStatus(ctx, tracker.TicketID(strings.TrimSpace(ticket)), st); err != nil {
			return nil, fmt.Errorf("set %s to %s in Plane: %w", ticket, st, err)
		}
		data, _ := json.MarshalIndent(map[string]string{"ticket": ticket, "status": string(st), "source": "plane"}, "", "  ")
		return &CallToolResult{Content: []ContentItem{{Type: "text", Text: string(data)}}}, nil
	})
}
