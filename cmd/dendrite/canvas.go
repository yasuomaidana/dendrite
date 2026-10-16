package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/personalcontext/canvas"
	"Dendrite/pkg/tracker/plane"
)

func handleCanvasCommand(ctx context.Context, args []string) {
	if len(args) < 1 {
		printCanvasHelp()
		return
	}

	subcommand := args[0]
	subArgs := args[1:]

	planeClient, _ := plane.NewClientFromEnv()
	bridge := canvas.NewPenpotUIBridge(planeClient, "http://localhost:9000")
	registry := canvas.NewDefaultRegistry(bridge)

	switch subcommand {
	case "actions", "list-actions":
		actions := registry.ListActions()
		fmt.Printf("[✓] Supported Canvas Creative Actions (%d total):\n", len(actions))
		for _, a := range actions {
			prefix := "  • [Text Writing] "
			if strings.HasPrefix(string(a), "DESIGN_") || strings.HasPrefix(string(a), "SYNC_") ||
				strings.HasPrefix(string(a), "RELATE_") || strings.HasPrefix(string(a), "UPDATE_") {
				prefix = "  • [UI Creative] "
			}
			fmt.Printf("%s%s\n", prefix, a)
		}

	case "dispatch", "run":
		fs := flag.NewFlagSet("dispatch", flag.ContinueOnError)
		action := fs.String("action", "WRITE_PLAN", "Canvas Action ID")
		mode := fs.String("mode", "DRAFT", "Writing mode (DRAFT, REFINE, EXPAND, CONDENSE, CHANGE_TONE, CRITIQUE, FORMAT)")
		title := fs.String("title", "", "Artifact title")
		prompt := fs.String("prompt", "", "Generation prompt or acceptance criteria")
		content := fs.String("content", "", "Existing content for refinement or critique")
		framework := fs.String("framework", "svelte", "Frontend framework (svelte, react, html)")
		componentName := fs.String("component", "CanvasComponent", "Component name for UI generation")
		ticketID := fs.String("ticket", "", "Plane ticket ID")
		filePath := fs.String("file", "", "Target UI component file path")
		asJSON := fs.Bool("json", false, "Output results in JSON")

		if err := fs.Parse(subArgs); err != nil {
			return
		}

		if *prompt == "" && *content == "" {
			fmt.Println("[-] Error: --prompt or --content is required")
			return
		}

		params := map[string]interface{}{
			"framework":      *framework,
			"component_name": *componentName,
			"ticket_id":      *ticketID,
			"file_path":      *filePath,
		}

		req := canvas.ActionRequest{
			Action:         canvas.ActionID(*action),
			Mode:           canvas.WritingMode(*mode),
			Title:          *title,
			Prompt:         *prompt,
			CurrentContent: *content,
			Parameters:     params,
		}

		resp, err := registry.Dispatch(ctx, req)
		if err != nil {
			fmt.Printf("[-] Dispatch error: %v\n", err)
			return
		}

		if *asJSON {
			data, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("[✓] Action %s [%s] executed successfully\n", resp.Action, resp.Mode)
			if resp.Title != "" {
				fmt.Printf("Title: %s\n", resp.Title)
			}
			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Println(resp.Content)
			fmt.Println("--------------------------------------------------------------------------------")
			for _, n := range resp.Notes {
				fmt.Printf("  • %s\n", n)
			}
		}

	case "relate":
		fs := flag.NewFlagSet("relate", flag.ContinueOnError)
		ticketID := fs.String("ticket", "", "Plane ticket ID (e.g. DEN-807)")
		featureName := fs.String("feature", "", "Feature or functionality name")
		componentPath := fs.String("component", "", "Component file path")
		frameID := fs.String("frame", "", "Penpot frame ID or slug")
		desc := fs.String("desc", "", "Description or acceptance context")
		asJSON := fs.Bool("json", false, "Output results in JSON")

		if err := fs.Parse(subArgs); err != nil {
			return
		}

		if *ticketID == "" || *featureName == "" {
			fmt.Println("[-] Error: --ticket and --feature are required")
			return
		}

		rel := canvas.TicketFeatureRelation{
			TicketID:      *ticketID,
			FeatureName:   *featureName,
			ComponentPath: *componentPath,
			PenpotFrameID: *frameID,
			Description:   *desc,
		}

		if err := bridge.RelateTicketToFeature(ctx, rel); err != nil {
			fmt.Printf("[-] Failed to relate ticket: %v\n", err)
			return
		}

		saved, _ := bridge.GetTicketFeatureRelation(*ticketID)
		if *asJSON {
			data, _ := json.MarshalIndent(saved, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("[✓] Bound Plane ticket '%s' to feature '%s'\n", saved.TicketID, saved.FeatureName)
			fmt.Printf("  Penpot Frame URL: %s\n", saved.PenpotFrameURL)
			if saved.ComponentPath != "" {
				fmt.Printf("  Component Path:   %s\n", saved.ComponentPath)
			}
		}

	case "sync":
		fs := flag.NewFlagSet("sync", flag.ContinueOnError)
		filePath := fs.String("file", "", "Path to UI component file")
		ticketID := fs.String("ticket", "", "Target Plane ticket ID")
		dryRun := fs.Bool("dry-run", false, "Simulate sync without updating Plane")
		asJSON := fs.Bool("json", false, "Output results in JSON")

		if err := fs.Parse(subArgs); err != nil {
			return
		}

		if *filePath == "" {
			fmt.Println("[-] Error: --file is required")
			return
		}

		res, err := bridge.SyncComponentWithPenpot(ctx, *ticketID, *filePath, *dryRun)
		if err != nil {
			fmt.Printf("[-] Sync error: %v\n", err)
			return
		}

		if *asJSON {
			data, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("[✓] Synchronized component '%s' with Penpot Frame\n", res.FeatureName)
			fmt.Printf("  Frame URL:     %s\n", res.PenpotFrameURL)
			fmt.Printf("  Tokens Found:  %d design tokens\n", len(res.Tokens))
			fmt.Printf("  Plane Updated: %v\n", res.PlaneUpdated)
			fmt.Printf("  Details:       %s\n", res.Details)
		}

	case "update-state":
		fs := flag.NewFlagSet("update-state", flag.ContinueOnError)
		ticketID := fs.String("ticket", "", "Plane ticket ID")
		targetState := fs.String("state", "", "Target state (In Progress, To Verify, Done)")
		comment := fs.String("comment", "", "Status note or verification remark")
		penpotURL := fs.String("penpot-url", "", "Penpot frame deep-link URL")
		noPlane := fs.Bool("no-plane", false, "Skip synchronizing with Plane v2")
		asJSON := fs.Bool("json", false, "Output results in JSON")

		if err := fs.Parse(subArgs); err != nil {
			return
		}

		if *ticketID == "" || *targetState == "" {
			fmt.Println("[-] Error: --ticket and --state are required")
			return
		}

		req := canvas.ProjectStateUpdateRequest{
			TicketID:       *ticketID,
			TargetState:    *targetState,
			Comment:        *comment,
			PenpotFrameURL: *penpotURL,
			PushToPlane:    !*noPlane,
		}

		res, err := bridge.UpdateProjectState(ctx, req)
		if err != nil {
			fmt.Printf("[-] Update state error: %v\n", err)
			return
		}

		if *asJSON {
			data, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("[✓] %s\n", res.Message)
			fmt.Printf("  Ticket ID:    %s\n", res.TicketID)
			fmt.Printf("  New State:    %s (Previous: %s)\n", res.NewState, res.PreviousState)
			fmt.Printf("  Plane Synced: %v\n", res.PlaneSynced)
		}

	case "list":
		relations := bridge.ListTicketRelations()
		fmt.Printf("[✓] Registered Ticket-Feature Relations (%d total):\n", len(relations))
		for _, r := range relations {
			fmt.Printf("  • [%s] %s -> %s (State: %s)\n", r.TicketID, r.FeatureName, r.PenpotFrameURL, r.CurrentState)
		}

	default:
		fmt.Printf("[-] Unknown canvas command: %s\n", subcommand)
		printCanvasHelp()
		os.Exit(1)
	}
}

func printCanvasHelp() {
	fmt.Println("Dendrite Canvas Interactive Writing & Penpot UI Creative Bridge")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  dendrite canvas actions                                     List all 19 supported Canvas actions")
	fmt.Println("  dendrite canvas dispatch --action <ID> --prompt <P> [flags]  Execute Canvas writing or UI creative action")
	fmt.Println("  dendrite canvas relate --ticket <ID> --feature <NAME>       Relate Plane ticket to UI feature & Penpot frame")
	fmt.Println("  dendrite canvas sync --file <PATH> [--ticket <ID>]          Sync UI component with Penpot frame & Plane ticket")
	fmt.Println("  dendrite canvas update-state --ticket <ID> --state <STATE>  Transition project state bidirectionally")
	fmt.Println("  dendrite canvas list                                        List all active ticket-to-feature relations")
}
