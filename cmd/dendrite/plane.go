package main

import (
	"context"
	"fmt"
	"os"

	"Dendrite/pkg/tracker/plane"
)

func handlePlaneCommand(ctx context.Context, action string, args []string) {
	client, err := plane.NewClientFromEnv()
	if err != nil {
		fmt.Printf("[-] Failed to initialize Plane client: %v\n", err)
		return
	}

	switch action {
	case "status", "replicate", "pull-status":
		filePath := "DEV_STATUS.md"
		dryRun := false
		for i := 0; i < len(args); i++ {
			if (args[i] == "--file" || args[i] == "-f") && i+1 < len(args) {
				filePath = args[i+1]
				i++
			} else if args[i] == "--dry-run" {
				dryRun = true
			}
		}

		fmt.Printf("[+] Replicating DEV_STATUS from Plane service (%s)...\n", client.BaseURL())
		rendered, report, err := plane.ReplicateStatusFromPlane(ctx, client)
		if err != nil {
			fmt.Printf("[-] Replication failed: %v\n", err)
			return
		}

		if dryRun {
			fmt.Println("[+] Dry run mode: simulated replication output:")
			fmt.Println(rendered)
		} else {
			if err := os.WriteFile(filePath, []byte(rendered), 0644); err != nil {
				fmt.Printf("[-] Failed to write %s: %v\n", filePath, err)
				return
			}
			fmt.Printf("[✓] Successfully replicated %s from Plane (Ownership: Plane Service)\n", filePath)
		}

		fmt.Printf("    - Total Tickets:      %d\n", report.TotalTickets)
		fmt.Printf("    - Completed:          %d (%.1f%%)\n", report.CompletedTickets, report.OverallProgress)
		fmt.Printf("    - In Progress:        %d\n", report.InProgressTickets)
		fmt.Printf("    - Queued / To Do:     %d\n", report.TodoTickets)
		fmt.Printf("    - Active Milestone:   %s\n", report.ActiveMilestone)
		fmt.Printf("    - Current Focus:      %s\n", report.CurrentFocus)
		fmt.Printf("    - Sync Duration:      %v\n", report.Duration)

	case "push-status", "push":
		filePath := "DEV_STATUS.md"
		for i := 0; i < len(args); i++ {
			if (args[i] == "--file" || args[i] == "-f") && i+1 < len(args) {
				filePath = args[i+1]
				i++
			}
		}

		fmt.Printf("[+] Moving %s content to Plane service (%s)...\n", filePath, client.BaseURL())
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("[-] Failed to read %s: %v\n", filePath, err)
			return
		}

		report, err := plane.PushStatusToPlane(ctx, client, string(content))
		if err != nil {
			fmt.Printf("[-] Failed to push dev status to Plane: %v\n", err)
			return
		}

		fmt.Printf("[✓] Completed moving DEV_STATUS to Plane in %v\n", report.Duration)
		fmt.Printf("    - Tickets Synchronized: %d\n", report.TotalTickets)
		fmt.Printf("    - Completed Tickets:    %d\n", report.CompletedTickets)
		fmt.Printf("    - In Progress Tickets:  %d\n", report.InProgressTickets)
		fmt.Printf("    - Milestones (Modules): %d\n", report.ModulesCount)
		fmt.Printf("    - Roadmap Pages:        %d\n", report.PagesCount)
		fmt.Printf("    - Overall Progress:     %.1f%%\n", report.OverallProgress)

	case "doc", "docs":
		if len(args) == 0 {
			fmt.Println("Usage: dendrite plane doc <push|pull|sync> [--dir docs]")
			return
		}
		directionStr := args[0]
		docsDir := "docs"
		for i := 1; i < len(args); i++ {
			if (args[i] == "--dir" || args[i] == "-d") && i+1 < len(args) {
				docsDir = args[i+1]
				i++
			}
		}

		var direction plane.SyncDirection
		switch directionStr {
		case "push", "to-plane":
			direction = plane.DirectionToPlane
		case "pull", "from-plane":
			direction = plane.DirectionFromPlane
		case "sync", "both", "bidirectional":
			direction = plane.DirectionBidirectional
		default:
			fmt.Printf("[-] Unknown sync direction: %s. Use push, pull, or sync.\n", directionStr)
			return
		}

		fmt.Printf("[+] Synchronizing project documentation (%s) with Plane (%s)...\n", direction, docsDir)
		report, err := plane.SyncDocs(ctx, client, docsDir, direction)
		if err != nil {
			fmt.Printf("[-] Documentation sync encountered error: %v\n", err)
		}
		if report != nil {
			fmt.Printf("[✓] Documentation sync completed in %v\n", report.Duration)
			fmt.Printf("    - Files Pushed to Plane:   %d\n", report.FilesPushed)
			fmt.Printf("    - Files Pulled from Plane: %d\n", report.FilesPulled)
			fmt.Printf("    - Pages Created:           %d\n", report.PagesCreated)
			fmt.Printf("    - Pages Updated:           %d\n", report.PagesUpdated)
			if len(report.Errors) > 0 {
				fmt.Printf("    - Warnings/Errors (%d):\n", len(report.Errors))
				for _, e := range report.Errors {
					fmt.Printf("        * %s\n", e)
				}
			}
		}

	case "issues", "list":
		issues, err := client.ListPlaneIssues(ctx)
		if err != nil {
			fmt.Printf("[-] Failed to list Plane issues: %v\n", err)
			return
		}
		fmt.Printf("[+] Plane Issues (%d) for Project '%s':\n", len(issues), client.ProjectID())
		for _, iss := range issues {
			st := iss.StateDetail.Name
			if st == "" {
				st = iss.State
			}
			fmt.Printf("    [%s] #%d: %s\n", st, iss.SequenceID, iss.Name)
		}

	case "modules":
		modules, err := client.ListModules(ctx)
		if err != nil {
			fmt.Printf("[-] Failed to list Plane modules: %v\n", err)
			return
		}
		fmt.Printf("[+] Plane Modules / Milestones (%d):\n", len(modules))
		for _, mod := range modules {
			fmt.Printf("    - %s (Status: %s, ID: %s)\n", mod.Name, mod.Status, mod.ID)
		}

	case "pages":
		pages, err := client.ListPages(ctx)
		if err != nil {
			fmt.Printf("[-] Failed to list Plane pages: %v\n", err)
			return
		}
		fmt.Printf("[+] Plane Documentation Pages (%d):\n", len(pages))
		for _, p := range pages {
			fmt.Printf("    - %s (ID: %s, Updated: %s)\n", p.Name, p.ID, p.UpdatedAt.Format("2006-01-02 15:04"))
		}

	default:
		fmt.Printf("[-] Unknown plane action: %s\n", action)
		fmt.Println("Usage:")
		fmt.Println("  dendrite plane replicate [--file DEV_STATUS.md] [--dry-run]")
		fmt.Println("  dendrite plane push-status [--file DEV_STATUS.md]")
		fmt.Println("  dendrite plane doc <push|pull|sync> [--dir docs]")
		fmt.Println("  dendrite plane issues")
		fmt.Println("  dendrite plane modules")
		fmt.Println("  dendrite plane pages")
	}
}
