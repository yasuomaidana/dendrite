package main

import (
	"fmt"
	"strings"

	"Dendrite/pkg/personalcontext/canvas"

	"github.com/spf13/cobra"
)

// newCanvasBridge connects the canvas to Penpot and, when configured, Plane.
func newCanvasBridge() *canvas.PenpotUIBridge {
	planeClient, _ := newPlaneClient()
	return canvas.NewPenpotUIBridge(planeClient, defaultPenpotEndpoint)
}

// newCortexCanvasCmd builds `dendrite cortex canvas`: writing actions and the Penpot UI creative bridge.
func newCortexCanvasCmd() *cobra.Command {
	cmd := newGroupCmd("canvas", "Interactive writing canvas and Penpot UI creative action bridge", "",
		"creative-canvas")
	cmd.Example = "dendrite cortex canvas actions\n" +
		"dendrite cortex canvas dispatch --action WRITE_PLAN --prompt \"Q4 roadmap\"\n" +
		"dendrite cortex canvas relate --ticket DEN-807 --feature login --frame login-v2\n" +
		"dendrite cortex canvas update-state --ticket DEN-807 --state \"To Verify\""
	cmd.AddCommand(
		newCanvasActionsCmd(),
		newCanvasDispatchCmd(),
		newCanvasRelateCmd(),
		newCanvasSyncCmd(),
		newCanvasUpdateStateCmd(),
		newCanvasListCmd(),
	)
	return cmd
}

func newCanvasActionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "actions",
		Aliases: []string{"list-actions"},
		Short:   "List the supported canvas actions",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			actions := canvas.NewDefaultRegistry(newCanvasBridge()).ListActions()
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, actions)
			}
			fmt.Fprintf(out, "[✓] Supported Canvas Creative Actions (%d total):\n", len(actions))
			for _, a := range actions {
				kind := "Text Writing"
				for _, p := range []string{"DESIGN_", "SYNC_", "RELATE_", "UPDATE_"} {
					if strings.HasPrefix(string(a), p) {
						kind = "UI Creative"
					}
				}
				fmt.Fprintf(out, "  • [%s] %s\n", kind, a)
			}
			return nil
		},
	}
}

func newCanvasDispatchCmd() *cobra.Command {
	var action, mode, framework, component, ticket, file string
	req := canvas.ActionRequest{}
	cmd := &cobra.Command{
		Use:     "dispatch",
		Aliases: []string{"run"},
		Short:   "Run one canvas writing or UI creative action",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if req.Prompt == "" && req.CurrentContent == "" {
				return usageErrorf("give --prompt or --content")
			}
			req.Action, req.Mode = canvas.ActionID(action), canvas.WritingMode(mode)
			req.Parameters = map[string]any{
				"framework":      framework,
				"component_name": component,
				"ticket_id":      ticket,
				"file_path":      file,
			}
			resp, err := canvas.NewDefaultRegistry(newCanvasBridge()).Dispatch(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("dispatch %s: %w", action, err)
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, resp)
			}
			fmt.Fprintf(out, "[✓] Action %s [%s] executed successfully\n", resp.Action, resp.Mode)
			if resp.Title != "" {
				fmt.Fprintf(out, "Title: %s\n", resp.Title)
			}
			rule := strings.Repeat("-", 80)
			fmt.Fprintln(out, rule)
			fmt.Fprintln(out, resp.Content)
			fmt.Fprintln(out, rule)
			for _, n := range resp.Notes {
				fmt.Fprintf(out, "  • %s\n", n)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&action, "action", "WRITE_PLAN", "Canvas action ID (see 'canvas actions')")
	f.StringVar(&mode, "mode", "DRAFT", "Writing mode: DRAFT, REFINE, EXPAND, CONDENSE, CHANGE_TONE, CRITIQUE or FORMAT")
	f.StringVar(&req.Title, "title", "", "Artifact title")
	f.StringVar(&req.Prompt, "prompt", "", "Generation prompt or acceptance criteria")
	f.StringVar(&req.CurrentContent, "content", "", "Existing content to refine or critique")
	f.StringVar(&framework, "framework", "svelte", "Frontend framework: svelte, react or html")
	f.StringVar(&component, "component", "CanvasComponent", "Component name for UI generation")
	addTicketFlag(cmd, &ticket, "")
	addFileFlag(cmd, &file, "", "Target UI component file")
	return cmd
}

func newCanvasRelateCmd() *cobra.Command {
	rel := canvas.TicketFeatureRelation{}
	cmd := &cobra.Command{
		Use:   "relate",
		Short: "Bind a Plane ticket to a UI feature and Penpot frame",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			bridge := newCanvasBridge()
			if err := bridge.RelateTicketToFeature(cmd.Context(), rel); err != nil {
				return fmt.Errorf("relate ticket: %w", err)
			}
			saved, ok := bridge.GetTicketFeatureRelation(rel.TicketID)
			if !ok {
				return fmt.Errorf("relation for %s was not saved", rel.TicketID)
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, saved)
			}
			fmt.Fprintf(out, "[✓] Bound Plane ticket '%s' to feature '%s'\n", saved.TicketID, saved.FeatureName)
			fmt.Fprintf(out, "  Penpot Frame URL: %s\n", saved.PenpotFrameURL)
			if saved.ComponentPath != "" {
				fmt.Fprintf(out, "  Component Path:   %s\n", saved.ComponentPath)
			}
			return nil
		},
	}
	f := cmd.Flags()
	addTicketFlag(cmd, &rel.TicketID, "")
	f.StringVar(&rel.FeatureName, "feature", "", "Feature name")
	f.StringVar(&rel.ComponentPath, "component", "", "Component file path")
	f.StringVar(&rel.PenpotFrameID, "frame", "", "Penpot frame ID or slug")
	f.StringVar(&rel.Description, "desc", "", "Description or acceptance context")
	requireFlags(cmd, "ticket", "feature")
	return cmd
}

func newCanvasSyncCmd() *cobra.Command {
	var file, ticket string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync a UI component with its Penpot frame and Plane ticket",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := newCanvasBridge().SyncComponentWithPenpot(cmd.Context(), ticket, file, dryRun)
			if err != nil {
				return fmt.Errorf("sync component: %w", err)
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, res)
			}
			fmt.Fprintf(out, "[✓] Synchronized component '%s' with Penpot Frame\n", res.FeatureName)
			fmt.Fprintf(out, "  Frame URL:     %s\n", res.PenpotFrameURL)
			fmt.Fprintf(out, "  Tokens Found:  %d design tokens\n", len(res.Tokens))
			fmt.Fprintf(out, "  Plane Updated: %v\n", res.PlaneUpdated)
			fmt.Fprintf(out, "  Details:       %s\n", res.Details)
			return nil
		},
	}
	addFileFlag(cmd, &file, "", "UI component file")
	addTicketFlag(cmd, &ticket, "")
	addDryRunFlag(cmd, &dryRun)
	requireFlags(cmd, "file")
	return cmd
}

func newCanvasUpdateStateCmd() *cobra.Command {
	req := canvas.ProjectStateUpdateRequest{}
	var noPlane bool
	cmd := &cobra.Command{
		Use:   "update-state",
		Short: "Move a ticket to a new state and record a note",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			req.PushToPlane = !noPlane
			res, err := newCanvasBridge().UpdateProjectState(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("update state: %w", err)
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, res)
			}
			fmt.Fprintf(out, "[✓] %s\n", res.Message)
			fmt.Fprintf(out, "  Ticket ID:    %s\n", res.TicketID)
			fmt.Fprintf(out, "  New State:    %s (Previous: %s)\n", res.NewState, res.PreviousState)
			fmt.Fprintf(out, "  Plane Synced: %v\n", res.PlaneSynced)
			return nil
		},
	}
	f := cmd.Flags()
	addTicketFlag(cmd, &req.TicketID, "")
	f.StringVar(&req.TargetState, "state", "", "Target state (In Progress, To Verify, Done)")
	f.StringVar(&req.Comment, "comment", "", "Status note or verification remark")
	f.StringVar(&req.PenpotFrameURL, "penpot-url", "", "Penpot frame deep link")
	f.BoolVar(&noPlane, "no-plane", false, "Do not push the change to Plane")
	requireFlags(cmd, "ticket", "state")
	return cmd
}

func newCanvasListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List ticket-to-feature relations",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			relations := newCanvasBridge().ListTicketRelations()
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, relations)
			}
			fmt.Fprintf(out, "[✓] Registered Ticket-Feature Relations (%d total):\n", len(relations))
			for _, r := range relations {
				fmt.Fprintf(out, "  • [%s] %s -> %s (State: %s)\n", r.TicketID, r.FeatureName, r.PenpotFrameURL, r.CurrentState)
			}
			return nil
		},
	}
}
