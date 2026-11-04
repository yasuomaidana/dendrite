package planetools

import (
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
	"context"
	"fmt"
	"os"
	"strings"

	. "Dendrite/pkg/mcpserver"
	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/jetbrains"
)

// registerPlaneSyncAndIDETools registers dendrite_plane_replicate, dendrite_plane_push_status, dendrite_plane_doc_sync and dendrite_jetbrains_setup.
func registerPlaneSyncAndIDETools(s *Server, deps Dependencies) {
	// 12. dendrite_plane_replicate
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_replicate",
		Description: "Replicate DEV_STATUS.md directly from Plane service state (Plane owns status)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file":    map[string]interface{}{"type": "string", "description": "Target markdown file path (default: DEV_STATUS.md)"},
				"dry_run": map[string]interface{}{"type": "boolean", "description": "If true, simulates replication without modifying disk"},
			},
		},
	}, planeReplicateHandler(deps))

	// 13. dendrite_plane_push_status
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_push_status",
		Description: "Move local DEV_STATUS.md content (tickets, milestones, descriptions) to Plane service",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file": map[string]interface{}{"type": "string", "description": "Source markdown file path (default: DEV_STATUS.md)"},
			},
		},
	}, planePushStatusHandler(deps))

	// 14. dendrite_plane_doc_sync
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_doc_sync",
		Description: "Synchronize documentation between Plane pages and local repository (direction: to-plane, from-plane, bidirectional)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"direction": map[string]interface{}{"type": "string", "description": "Sync direction: 'to-plane', 'from-plane', or 'bidirectional' (default: bidirectional)"},
				"dir":       map[string]interface{}{"type": "string", "description": "Documentation directory path (default: docs)"},
			},
		},
	}, planeDocSyncHandler(deps))

	// 15. dendrite_jetbrains_setup
	s.RegisterTool(Tool{
		Name:        "dendrite_jetbrains_setup",
		Description: "Configure JetBrains IDE toolset with Dendrite MCP servers (.ai/mcp/mcp.json), agent run configurations, and external tools",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_dir":  map[string]interface{}{"type": "string", "description": "Project root directory (default: current working directory)"},
				"project_only": map[string]interface{}{"type": "boolean", "description": "Configure only project-level files (.ai/mcp/mcp.json and .idea)"},
				"global_only":  map[string]interface{}{"type": "boolean", "description": "Configure only user-level global files (~/.ai/mcp/mcp.json)"},
			},
		},
	}, jetbrainsSetupHandler())
}

// registerCommitTool registers dendrite_commit.
func registerCommitTool(s *Server, deps Dependencies) {
	// 46. dendrite_commit
	s.RegisterTool(Tool{
		Name:        "dendrite_commit",
		Description: "Autonomous Git commit engine with automatic local model delegation for conventional commit message synthesis and ticket provenance",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "Target ticket ID (e.g. DEN-098)",
				},
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Explicit commit message. If omitted, synthesized via local model",
				},
				"files": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "Specific file paths to stage (default: stage all)",
				},
				"stage_all": map[string]interface{}{
					"type":        "boolean",
					"description": "Stage all modified and untracked files (default: true)",
				},
				"use_local_model": map[string]interface{}{
					"type":        "boolean",
					"description": "Delegate commit message synthesis to local model (default: true)",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Local model name (default: qwen3.8:27b)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "Simulate staging and message synthesis without modifying git history",
				},
			},
		},
	}, commitHandler())
}

// planeReplicateHandler handles the dendrite_plane_replicate tool.
func planeReplicateHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		client, err := PlaneClientOf(deps)
		if err != nil {
			return nil, err
		}
		targetFile := "DEV_STATUS.md"
		if f, ok := args["file"].(string); ok && f != "" {
			targetFile = f
		}
		dryRun, _ := args["dry_run"].(bool)

		rendered, report, err := plane.ReplicateStatusFromPlane(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("replication failed: %w", err)
		}

		if !dryRun {
			if err := os.WriteFile(targetFile, []byte(rendered), 0644); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", targetFile, err)
			}
		}

		res := map[string]interface{}{
			"status":    "success",
			"ownership": "Plane Service",
			"file":      targetFile,
			"dry_run":   dryRun,
			"report":    report,
		}
		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}

// planePushStatusHandler handles the dendrite_plane_push_status tool.
func planePushStatusHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		client, err := PlaneClientOf(deps)
		if err != nil {
			return nil, err
		}
		sourceFile := "DEV_STATUS.md"
		if f, ok := args["file"].(string); ok && f != "" {
			sourceFile = f
		}
		content, err := os.ReadFile(sourceFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", sourceFile, err)
		}

		report, err := plane.PushStatusToPlane(ctx, client, string(content))
		if err != nil {
			return nil, fmt.Errorf("push to plane failed: %w", err)
		}

		res := map[string]interface{}{
			"status": "success",
			"file":   sourceFile,
			"report": report,
		}
		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}

// planeDocSyncHandler handles the dendrite_plane_doc_sync tool.
func planeDocSyncHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		client, err := PlaneClientOf(deps)
		if err != nil {
			return nil, err
		}
		dir := "docs"
		if d, ok := args["dir"].(string); ok && d != "" {
			dir = d
		}
		direction := plane.DirectionBidirectional
		if dirStr, ok := args["direction"].(string); ok && dirStr != "" {
			switch strings.ToLower(dirStr) {
			case "to-plane", "push":
				direction = plane.DirectionToPlane
			case "from-plane", "pull":
				direction = plane.DirectionFromPlane
			default:
				direction = plane.DirectionBidirectional
			}
		}

		report, err := plane.SyncDocs(ctx, client, dir, direction)
		if err != nil {
			return nil, fmt.Errorf("documentation sync failed: %w", err)
		}

		data := IndentJSON(report)
		return TextResult(string(data)), nil
	}
}

// jetbrainsSetupHandler handles the dendrite_jetbrains_setup tool.
func jetbrainsSetupHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		pDir, _ := args["project_dir"].(string)
		projectOnly, _ := args["project_only"].(bool)
		globalOnly, _ := args["global_only"].(bool)

		opts := jetbrains.SetupOptions{
			ProjectDir:             pDir,
			ConfigureProjectMCP:    !globalOnly,
			ConfigureGlobalMCP:     !projectOnly,
			ConfigureRunConfigs:    !globalOnly,
			ConfigureExternalTools: !globalOnly,
		}

		res, err := jetbrains.SetupJetBrainsToolset(opts)
		if err != nil {
			return nil, fmt.Errorf("jetbrains setup failed: %w", err)
		}

		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}

// commitHandler handles the dendrite_commit tool.
func commitHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		ticketID, _ := args["ticket_id"].(string)
		msg, _ := args["message"].(string)
		model, _ := args["model"].(string)
		dryRun, _ := args["dry_run"].(bool)
		res, err := devtool.ExecuteCommit(ctx, devtool.CommitOptions{
			TicketID:      ticketID,
			Message:       msg,
			Files:         NonEmptyStrings(args["files"]),
			StageAll:      BoolArgOr(args, "stage_all", true),
			UseLocalModel: BoolArgOr(args, "use_local_model", true),
			Model:         model,
			DryRun:        dryRun,
		})
		if err != nil {
			return nil, fmt.Errorf("commit failed: %w", err)
		}
		return TextResult(string(IndentJSON(res))), nil
	}
}

// RegisterPlaneTicketSetTool registers dendrite_plane_ticket_set, the MCP way to move a ticket
// through its lifecycle. Plane owns ticket state, so it never touches DEV_STATUS.md.
func RegisterPlaneTicketSetTool(s *Server, deps Dependencies) {
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
	}, planeTicketSetHandler(deps))
}

// planeTicketSetHandler handles the dendrite_plane_ticket_set tool.
func planeTicketSetHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
		if _, err := planesvc.Default().SetStatus(ctx, client, tracker.TicketID(strings.TrimSpace(ticket)), st); err != nil {
			return nil, fmt.Errorf("set %s to %s in Plane: %w", ticket, st, err)
		}
		data := IndentJSON(map[string]string{"ticket": ticket, "status": string(st), "source": "plane"})
		return TextResult(string(data)), nil
	}
}
