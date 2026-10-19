package main

import (
	"context"
	"fmt"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

func handleDevCommand(ctx context.Context, sub string, args []string) {
	switch sub {
	case "audit-changes":
		handleAuditChangesCommand(ctx, args)
		return

	case "ollama":
		probe := []string{"probe"}
		if len(args) > 1 && args[0] == "probe" {
			probe = append(probe, "--endpoint", args[1])
		}
		_ = invokeCmd(ctx, newSpikeCmd(), probe...)

	case "ticket":
		filePath := "DEV_STATUS.md"
		if len(args) == 0 || args[0] == "list" {
			tr := tracker.NewMemoryTracker(filePath)
			if err := tr.LoadFromMarkdown(filePath); err != nil {
				pClient, pErr := plane.NewClientFromEnv()
				if pErr == nil {
					issues, iErr := pClient.ListPlaneIssues(ctx)
					if iErr == nil && len(issues) > 0 {
						fmt.Printf("[+] Registered Tickets from Plane (%d):\n", len(issues))
						for _, iss := range issues {
							fmt.Printf("    [%s] %s\n", iss.State, iss.Name)
						}
						return
					}
				}
				fmt.Printf("[-] Failed to load tickets from %s: %v\n", filePath, err)
				return
			}
			tickets, _ := tr.ListTickets(ctx)
			fmt.Printf("[+] Registered Tickets (%d):\n", len(tickets))
			for _, t := range tickets {
				fmt.Printf("    [%s] %s: %s (%s)\n", t.Status, t.ID, t.Title, t.Milestone)
			}
		} else if args[0] == "set" && len(args) >= 3 {
			id := args[1]
			st, err := tracker.ParseStatus(args[2])
			if err != nil {
				fmt.Printf("[-] %v. Accepted: backlog, todo, in progress, to verify, done\n", err)
				return
			}
			pClient, pErr := plane.NewClientFromEnv()
			planeSynced := false
			if pErr == nil && pClient != nil {
				if err := pClient.UpdateStatus(ctx, tracker.TicketID(id), st); err != nil {
					fmt.Printf("[-] Failed to update ticket status in Plane: %v\n", err)
					return
				}
				planeSynced = true
			}
			mdErr := devtool.UpdateTicketInMarkdown(ctx, filePath, id, st)

			if mdErr != nil && !planeSynced {
				fmt.Printf("[-] Failed to update ticket: %v\n", mdErr)
				return
			}
			msg := fmt.Sprintf("[✓] Successfully updated ticket %s to %s in %s", id, st, filePath)
			if planeSynced {
				msg += " and reflected in Plane service"
			}
			fmt.Println(msg)
		} else if len(args) > 0 && args[0] == "next" {
			handlePlaneCommand(ctx, "next", args[1:])
		} else if len(args) > 0 && args[0] == "link" {
			handlePlaneCommand(ctx, "link", args[1:])
		} else if len(args) > 0 && (args[0] == "create" || args[0] == "add") {
			handlePlaneCommand(ctx, "create", args[1:])
		} else if len(args) > 0 && (args[0] == "sync-plane" || args[0] == "push-plane") {
			handlePlaneCommand(ctx, "push-status", args[1:])
		} else if len(args) > 0 && (args[0] == "replicate-plane" || args[0] == "pull-plane") {
			handlePlaneCommand(ctx, "replicate", args[1:])
		}

	case "topology":
		_ = invokeCmd(ctx, newAxonCmd(), args...)

	case "gateway":
		_ = invokeCmd(ctx, newCleftCmd(), args...)

	case "inference":
		_ = invokeCmd(ctx, newSpikeCmd(), args...)

	case "audit-prompt":
		_ = invokeCmd(ctx, newVesicleCmd(), append([]string{"estimate"}, args...)...)

	default:
		fmt.Printf("Unknown dev subcommand: %s\n", sub)
	}
}
