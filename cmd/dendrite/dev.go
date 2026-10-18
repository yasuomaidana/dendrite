package main

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

func handleDevCommand(ctx context.Context, sub string, args []string) {
	switch sub {
	case "audit-changes":
		handleAuditChangesCommand(ctx, args)
		return

	case "ollama":
		endpoint := ollama.DefaultBaseURL
		if len(args) > 1 && args[0] == "probe" {
			endpoint = args[1]
		}
		fmt.Printf("[+] Probing Ollama at %s...\n", endpoint)
		status, models, err := devtool.ProbeOllama(ctx, endpoint)
		if err != nil {
			fmt.Printf("[-] Probe failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] Status: Reachable (Latency: %v)\n", status.Latency)
		fmt.Printf("[✓] Models (%d):\n", len(models))
		for _, m := range models {
			fmt.Printf("    - %s\n", m.Name)
		}

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
		if len(args) > 0 && (args[0] == "advisor" || args[0] == "advise") {
			runTopologyAdvisor(ctx, args[1:])
			return
		}
		scenarioStr := "local-hybrid"
		if len(args) > 0 {
			scenarioStr = args[0]
		}
		runTopologyInspect(ctx, scenarioStr)

	case "gateway":
		if len(args) == 0 {
			handleGatewayCommand(ctx, []string{"local-hybrid"})
			return
		}
		handleGatewayCommand(ctx, args)

	case "inference":
		if len(args) == 0 {
			handleInferenceCommand(ctx, []string{"list"})
			return
		}
		handleInferenceCommand(ctx, args)

	case "audit-prompt":
		if len(args) == 0 {
			fmt.Println("[-] Please provide a prompt string to audit")
			return
		}
		input := strings.Join(args, " ")
		metrics := devtool.AuditPrompt(input)
		fmt.Printf("[+] Prompt Character Count:  %d\n", metrics.CharacterCount)
		fmt.Printf("[+] Prompt Word Count:       %d\n", metrics.WordCount)
		fmt.Printf("[+] Estimated Token Volume:  %d tokens (~4 chars/token)\n", metrics.EstimatedTokens)

	default:
		fmt.Printf("Unknown dev subcommand: %s\n", sub)
	}
}
