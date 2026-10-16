package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
	"Dendrite/pkg/tracker/plane"
)

var cliTicketPrefixRegex = regexp.MustCompile(`\[?(DEN-\d+)\]?`)

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

		fmt.Printf("[+] Moving %s content to Plane service (%s) with Gantt timelines & dependencies...\n", filePath, client.BaseURL())
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
		fmt.Printf("    - Gantt Dependencies:   %d\n", report.RelationsCount)
		fmt.Printf("    - Roadmap Pages:        %d\n", report.PagesCount)
		fmt.Printf("    - Overall Progress:     %.1f%%\n", report.OverallProgress)

	case "next", "smart-select":
		var requestedTicket string
		var milestoneFilter string
		jsonOutput := false

		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--ticket", "-t":
				if i+1 < len(args) {
					requestedTicket = args[i+1]
					i++
				}
			case "--milestone", "-m":
				if i+1 < len(args) {
					milestoneFilter = args[i+1]
					i++
				}
			case "--json":
				jsonOutput = true
			}
		}

		issues, err := client.ListPlaneIssues(ctx)
		if err != nil {
			fmt.Printf("[-] Failed to query Plane issues: %v\n", err)
			return
		}
		states, _ := client.ListStates(ctx)
		stateMap := make(map[string]tracker.Status)
		for _, st := range states {
			g := strings.ToLower(st.Group)
			n := strings.ToLower(st.Name)
			switch {
			case g == "completed" || strings.Contains(n, "done"):
				stateMap[st.ID] = tracker.StatusDone
			case strings.Contains(n, "review") || strings.Contains(n, "verify"):
				stateMap[st.ID] = tracker.StatusInReview
			case g == "started" || strings.Contains(n, "progress") || strings.Contains(n, "active"):
				stateMap[st.ID] = tracker.StatusInProgress
			case g == "unstarted" || strings.Contains(n, "todo") || strings.Contains(n, "to do"):
				stateMap[st.ID] = tracker.StatusTodo
			default:
				stateMap[st.ID] = tracker.StatusBacklog
			}
		}

		graph := dag.NewGraph()
		uuidToID := make(map[string]string)
		for _, iss := range issues {
			if milestoneFilter != "" && !strings.Contains(strings.ToLower(iss.Name), strings.ToLower(milestoneFilter)) {
				continue
			}
			id := fmt.Sprintf("DEN-%03d", iss.SequenceID)
			m := cliTicketPrefixRegex.FindStringSubmatch(iss.Name)
			if len(m) > 1 {
				id = m[1]
			}
			uuidToID[iss.ID] = id

			st := stateMap[iss.State]
			if st == "" {
				st = tracker.StatusTodo
			}

			prio := iss.Priority
			if prio == "" || prio == "none" {
				prio = dag.PriorityMedium
			}

			est := 5
			if iss.EstimatePoint != nil {
				est = *iss.EstimatePoint
			}

			sDate := ""
			if iss.StartDate != nil {
				sDate = *iss.StartDate
			}
			tDate := ""
			if iss.TargetDate != nil {
				tDate = *iss.TargetDate
			}

			graph.AddNode(&dag.Node{
				ID:             id,
				SequenceID:     iss.SequenceID,
				Title:          iss.Name,
				Status:         st,
				Priority:       prio,
				EstimatePoints: est,
				StartDate:      sDate,
				TargetDate:     tDate,
			})
		}

		// Link relations from Plane
		for _, iss := range issues {
			currID := uuidToID[iss.ID]
			rels, err := client.GetIssueRelations(ctx, iss.ID)
			if err == nil && rels != nil {
				for _, blocked := range rels.BlockedBy {
					bID := uuidToID[blocked.ID]
					if bID == "" {
						m := cliTicketPrefixRegex.FindStringSubmatch(blocked.Name)
						if len(m) > 1 {
							bID = m[1]
						}
					}
					if bID != "" && bID != currID {
						_ = graph.AddEdge(bID, currID)
					}
				}
			}
		}

		_, _ = graph.GenerateGanttPlan(time.Now().Truncate(24 * time.Hour))

		res, err := graph.SmartSelect(requestedTicket)
		if err != nil {
			fmt.Printf("[-] Smart selection error: %v\n", err)
			return
		}

		if jsonOutput {
			data, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Println("[+] Smart Ticket Prioritization & DAG Reachability (Source: Plane v2)")
		t := res.SelectedTicket
		fmt.Printf("    * Selected Ticket:    %s\n", t.ID)
		fmt.Printf("    * Title:              %s\n", t.Title)
		fmt.Printf("    * Priority:           %s (Base Score: %.0f, Total Score: %.1f)\n",
			strings.ToUpper(t.Priority), t.PriorityScore, res.TotalScore)
		fmt.Printf("    * Operational Status: %s\n", t.Status)
		if res.IsActive {
			fmt.Printf("    * Active State:       ALREADY IN PROGRESS\n")
		}
		if res.IsBlocked {
			fmt.Printf("    * Blocker Warning:    BLOCKED by unresolved prerequisites: %s\n", strings.Join(res.BlockingTickets, ", "))
		} else {
			fmt.Printf("    * Dependency Status:  UNBLOCKED (all prerequisites satisfied)\n")
		}
		if len(res.UnblockedDownstream) > 0 {
			fmt.Printf("    * Downstream Impact:  Unblocks %d dependent tickets: %s\n",
				len(res.UnblockedDownstream), strings.Join(res.UnblockedDownstream, ", "))
		}
		if t.StartDate != "" && t.TargetDate != "" {
			fmt.Printf("    * Gantt Schedule:     %s -> %s\n", t.StartDate, t.TargetDate)
		}
		fmt.Printf("    * Decision Reason:    %s\n", res.Reason)

	case "link":
		if len(args) < 3 {
			fmt.Println("Usage: dendrite plane link <ticket_a> <blocking|blocked_by|relates_to|duplicate> <ticket_b>")
			return
		}
		ticketA := args[0]
		relType := args[1]
		ticketB := args[2]

		uA, err := resolvePlaneTicketUUID(ctx, client, ticketA)
		if err != nil {
			fmt.Printf("[-] Error: %v\n", err)
			return
		}
		uB, err := resolvePlaneTicketUUID(ctx, client, ticketB)
		if err != nil {
			fmt.Printf("[-] Error: %v\n", err)
			return
		}

		if err := client.AddIssueRelation(ctx, uA, relType, []string{uB}); err != nil {
			fmt.Printf("[-] Failed to link issues in Plane: %v\n", err)
			return
		}

		fmt.Printf("[✓] Successfully linked %s -> %s (%s) for Plane Gantt dependency view\n", ticketA, ticketB, relType)

	case "relate", "ticket-relate":
		var ticketID string
		var priority string
		var blockedBy []string
		var blocking []string
		var relatesTo []string
		var reason string
		var startDate string
		var targetDate string

		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--ticket", "-t":
				if i+1 < len(args) {
					ticketID = args[i+1]
					i++
				}
			case "--priority", "-p":
				if i+1 < len(args) {
					priority = strings.ToLower(args[i+1])
					i++
				}
			case "--blocked-by", "-b":
				if i+1 < len(args) {
					parts := strings.Split(args[i+1], ",")
					for _, p := range parts {
						if tr := strings.TrimSpace(p); tr != "" {
							blockedBy = append(blockedBy, tr)
						}
					}
					i++
				}
			case "--blocking":
				if i+1 < len(args) {
					parts := strings.Split(args[i+1], ",")
					for _, p := range parts {
						if tr := strings.TrimSpace(p); tr != "" {
							blocking = append(blocking, tr)
						}
					}
					i++
				}
			case "--relates-to", "-r":
				if i+1 < len(args) {
					parts := strings.Split(args[i+1], ",")
					for _, p := range parts {
						if tr := strings.TrimSpace(p); tr != "" {
							relatesTo = append(relatesTo, tr)
						}
					}
					i++
				}
			case "--reason":
				if i+1 < len(args) {
					reason = args[i+1]
					i++
				}
			case "--start", "-s":
				if i+1 < len(args) {
					startDate = args[i+1]
					i++
				}
			case "--target":
				if i+1 < len(args) {
					targetDate = args[i+1]
					i++
				}
			}
		}

		if ticketID == "" {
			fmt.Println("Usage: dendrite plane relate --ticket <id> [--priority <prio>] [--blocked-by <ids>] [--blocking <ids>] [--relates-to <ids>] [--reason <reason>]")
			return
		}

		uTarget, err := resolvePlaneTicketUUID(ctx, client, ticketID)
		if err != nil {
			fmt.Printf("[-] Error resolving ticket %s: %v\n", ticketID, err)
			return
		}

		if priority != "" || startDate != "" || targetDate != "" {
			patchReq := plane.UpdateIssueRequest{}
			if priority != "" {
				patchReq.Priority = priority
			}
			if startDate != "" {
				patchReq.StartDate = &startDate
			}
			if targetDate != "" {
				patchReq.TargetDate = &targetDate
			}
			_, err := client.UpdatePlaneIssueWithDetails(ctx, uTarget, patchReq)
			if err != nil {
				fmt.Printf("[-] Failed to update ticket metadata: %v\n", err)
			} else {
				fmt.Printf("[✓] Updated ticket %s metadata (Priority: %s)\n", ticketID, strings.ToUpper(priority))
			}
		}

		for _, b := range blockedBy {
			uB, err := resolvePlaneTicketUUID(ctx, client, b)
			if err != nil {
				fmt.Printf("[-] Warning: could not resolve blocker %s: %v\n", b, err)
				continue
			}
			if err := client.AddIssueRelation(ctx, uTarget, "blocked_by", []string{uB}); err != nil {
				fmt.Printf("[-] Failed to link blocked_by %s: %v\n", b, err)
			} else {
				fmt.Printf("[✓] Linked: %s is blocked_by %s\n", ticketID, b)
			}
		}

		for _, blk := range blocking {
			uBlk, err := resolvePlaneTicketUUID(ctx, client, blk)
			if err != nil {
				fmt.Printf("[-] Warning: could not resolve blocking target %s: %v\n", blk, err)
				continue
			}
			if err := client.AddIssueRelation(ctx, uTarget, "blocking", []string{uBlk}); err != nil {
				fmt.Printf("[-] Failed to link blocking %s: %v\n", blk, err)
			} else {
				fmt.Printf("[✓] Linked: %s is blocking %s\n", ticketID, blk)
			}
		}

		for _, rel := range relatesTo {
			uRel, err := resolvePlaneTicketUUID(ctx, client, rel)
			if err != nil {
				fmt.Printf("[-] Warning: could not resolve relates_to %s: %v\n", rel, err)
				continue
			}
			if err := client.AddIssueRelation(ctx, uTarget, "relates_to", []string{uRel}); err != nil {
				fmt.Printf("[-] Failed to link relates_to %s: %v\n", rel, err)
			} else {
				fmt.Printf("[✓] Linked: %s relates_to %s\n", ticketID, rel)
			}
		}

		if reason != "" {
			fmt.Printf("    * Stored context: %s\n", reason)
		}

	case "create", "issue-create":
		var name string
		var desc string
		var milestone string
		var priority = "medium"
		var estimate *int
		var blockedBy []string

		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--name", "-n":
				if i+1 < len(args) {
					name = args[i+1]
					i++
				}
			case "--desc", "-d", "--context", "-c":
				if i+1 < len(args) {
					desc = args[i+1]
					i++
				}
			case "--milestone", "-m":
				if i+1 < len(args) {
					milestone = args[i+1]
					i++
				}
			case "--priority", "-p":
				if i+1 < len(args) {
					priority = args[i+1]
					i++
				}
			case "--estimate", "-e":
				if i+1 < len(args) {
					if n, err := strconv.Atoi(args[i+1]); err == nil {
						estimate = &n
					}
					i++
				}
			case "--blocked-by", "-b":
				if i+1 < len(args) {
					parts := strings.Split(args[i+1], ",")
					for _, p := range parts {
						item := strings.TrimSpace(p)
						if item != "" {
							blockedBy = append(blockedBy, item)
						}
					}
					i++
				}
			}
		}

		if strings.TrimSpace(name) == "" || plane.IsEmptyContext(desc) {
			fmt.Println("Usage: dendrite plane create --name \"[DEN-XXX] Title\" --desc \"Acceptance criteria and context\" [--milestone \"M1\"] [--priority high] [--estimate 5] [--blocked-by \"DEN-001,DEN-002\"]")
			if strings.TrimSpace(name) != "" && plane.IsEmptyContext(desc) {
				fmt.Println("[-] Error: ticket context/description is mandatory and cannot be empty. In theory and practice, empty tickets are forbidden. Provide --desc or --context with acceptance criteria and goal.")
			}
			return
		}

		now := time.Now().Truncate(24 * time.Hour)
		days := 4
		if estimate != nil {
			days = dag.EstimateDays(*estimate, 0)
		}
		sDate := now.Format("2006-01-02")
		tDate := now.AddDate(0, 0, days).Format("2006-01-02")

		var moduleID string
		if milestone != "" {
			modules, err := client.ListModules(ctx)
			if err == nil {
				for _, mod := range modules {
					if strings.Contains(strings.ToLower(mod.Name), strings.ToLower(milestone)) || mod.ID == milestone {
						moduleID = mod.ID
						break
					}
				}
			}
			if moduleID == "" {
				newMod, err := client.CreateModuleWithDates(ctx, milestone, "in-progress", fmt.Sprintf("Milestone tracking for %s", milestone), &sDate, &tDate)
				if err == nil && newMod != nil {
					moduleID = newMod.ID
				}
			}
		}

		var blockerUUIDs []string
		if len(blockedBy) > 0 {
			existingIssues, _ := client.ListPlaneIssues(ctx)
			for _, bStr := range blockedBy {
				if len(bStr) == 36 && strings.Count(bStr, "-") == 4 {
					blockerUUIDs = append(blockerUUIDs, bStr)
				} else {
					for _, iss := range existingIssues {
						if strings.Contains(iss.Name, "["+bStr+"]") || strings.Contains(iss.Name, bStr) {
							blockerUUIDs = append(blockerUUIDs, iss.ID)
							break
						}
					}
				}
			}
		}

		descHTML := fmt.Sprintf("<div><p>%s</p></div>", desc)
		created, err := client.CreatePlaneIssueWithDetails(ctx, plane.CreateIssueRequest{
			Name:            name,
			DescriptionHTML: descHTML,
			Priority:        priority,
			EstimatePoint:   estimate,
			StartDate:       &sDate,
			TargetDate:      &tDate,
			ModuleID:        moduleID,
			BlockedBy:       blockerUUIDs,
		})
		if err != nil {
			fmt.Printf("[-] Failed to create issue in Plane: %v\n", err)
			return
		}

		fmt.Printf("[✓] Successfully created ticket in Plane & placed on Gantt development plan:\n")
		fmt.Printf("    - ID:             %s (#%d)\n", created.ID, created.SequenceID)
		fmt.Printf("    - Name:           %s\n", created.Name)
		fmt.Printf("    - Priority:       %s\n", strings.ToUpper(priority))
		fmt.Printf("    - Gantt Timeline: %s -> %s\n", sDate, tDate)
		if moduleID != "" {
			fmt.Printf("    - Milestone:      %s\n", milestone)
		}
		if len(blockerUUIDs) > 0 {
			fmt.Printf("    - Dependencies:   Blocked by %d tickets\n", len(blockerUUIDs))
		}

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
			prio := iss.Priority
			if prio == "" {
				prio = "none"
			}
			dateSpan := ""
			if iss.StartDate != nil && iss.TargetDate != nil {
				dateSpan = fmt.Sprintf(" [%s -> %s]", *iss.StartDate, *iss.TargetDate)
			}
			fmt.Printf("    [%s | %s] #%d: %s%s\n", st, strings.ToUpper(prio), iss.SequenceID, iss.Name, dateSpan)
		}

	case "modules":
		modules, err := client.ListModules(ctx)
		if err != nil {
			fmt.Printf("[-] Failed to list Plane modules: %v\n", err)
			return
		}
		fmt.Printf("[+] Plane Modules / Milestones (%d):\n", len(modules))
		for _, mod := range modules {
			dateSpan := ""
			if mod.StartDate != nil && mod.TargetDate != nil {
				dateSpan = fmt.Sprintf(" (Dates: %s -> %s)", *mod.StartDate, *mod.TargetDate)
			}
			fmt.Printf("    - %s (Status: %s, ID: %s)%s\n", mod.Name, mod.Status, mod.ID, dateSpan)
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

	case "gantt-sync", "align-gantt":
		fmt.Printf("[+] Synchronizing Plane Gantt timelines, milestone dates, and dependency arrows...\n")
		rep, err := plane.SyncPlaneGanttAndRelations(ctx, client)
		if err != nil {
			fmt.Printf("[-] Gantt synchronization failed: %v\n", err)
			return
		}
		fmt.Printf("[✓] Plane Gantt schedule & dependency relations synchronized in %v:\n", rep.Duration)
		fmt.Printf("    - Milestones Scheduled:  %d (Total: %d)\n", rep.UpdatedModules, rep.TotalModules)
		fmt.Printf("    - Tickets Scheduled:     %d (Total: %d)\n", rep.UpdatedTickets, rep.TotalTickets)
		fmt.Printf("    - Dependency Arrows:     %d\n", rep.RelationsCreated)
		fmt.Printf("    - Live Gantt View URL:   %s/dendrite-ws/projects/%s/issues/ (Layout: Gantt)\n", client.BaseURL(), client.ProjectID())

	default:
		fmt.Printf("[-] Unknown plane action: %s\n", action)
		fmt.Println("Usage:")
		fmt.Println("  dendrite plane replicate [--file DEV_STATUS.md] [--dry-run]")
		fmt.Println("  dendrite plane push-status [--file DEV_STATUS.md]")
		fmt.Println("  dendrite plane next [--ticket <id>] [--milestone <name>] [--json]")
		fmt.Println("  dendrite plane link <ticket_a> <blocking|blocked_by|relates_to> <ticket_b>")
		fmt.Println("  dendrite plane relate --ticket <id> [--priority <prio>] [--blocked-by <ids>] [--blocking <ids>] [--relates-to <ids>]")
		fmt.Println("  dendrite plane gantt-sync (aligns all milestones, tickets, and Gantt dependency arrows)")
		fmt.Println("  dendrite plane create --name \"[DEN-XXX] Title\" --desc \"Acceptance criteria and context\" [--milestone \"M1\"] [--priority high] [--estimate 5] [--blocked-by \"DEN-001\"]")
		fmt.Println("  dendrite plane doc <push|pull|sync> [--dir docs]")
		fmt.Println("  dendrite plane issues")
		fmt.Println("  dendrite plane modules")
		fmt.Println("  dendrite plane pages")
	}
}

func resolvePlaneTicketUUID(ctx context.Context, client *plane.Client, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 36 && strings.Count(raw, "-") == 4 {
		return raw, nil
	}
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return "", err
	}
	if seq, err := strconv.Atoi(raw); err == nil {
		for _, iss := range issues {
			if iss.SequenceID == seq {
				return iss.ID, nil
			}
		}
	}
	for _, iss := range issues {
		if strings.Contains(strings.ToLower(iss.Name), strings.ToLower("["+raw+"]")) {
			return iss.ID, nil
		}
	}
	for _, iss := range issues {
		if strings.Contains(strings.ToLower(iss.Name), strings.ToLower(raw)) {
			return iss.ID, nil
		}
	}
	return "", fmt.Errorf("ticket '%s' not found in Plane", raw)
}
