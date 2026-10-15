package plane

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
)

var (
	devTicketLineRegex        = regexp.MustCompile(`^-\s+\[([ xX])\]\s+\*\*([A-Za-z0-9-]+)\*\*:\s+(.*?)(?:\s+\(` + "`" + `([^` + "`" + `]+)` + "`" + `\))?(?:\s+\*\(([A-Z_ ]+)\)\*)?$`)
	milestoneHdrRegex         = regexp.MustCompile(`^###\s+(Milestone\s+\d+:.*?)(?:\s+\(([^)]+)\))?$`)
	ticketIdPrefixRegex       = regexp.MustCompile(`\[?(DEN-\d+)\]?`)
	complexityTableRowRegex   = regexp.MustCompile(`\|\s*(?:\*\*\d+\*\*|\d+)\s*\|\s*\*?\*?([A-Za-z0-9-]+)\*?\*?\s*\|\s*([^|]+)\|\s*([^|]+)\|\s*(\d+)\s*SP\s*\|\s*([0-9.]+)\s*/\s*10`)
)

// ParsedDevTicket encapsulates all extracted metadata for a ticket from DEV_STATUS.md.
type ticketItem struct {
	id         string
	num        int
	title      string
	status     tracker.Status
	desc       string
	stateName  string
	sequenceID int
	targetPkg  string
}

type ParsedDevTicket struct {
	ID                 string
	Number             int
	Title              string
	TargetPackage      string
	Milestone          string
	FeatureID          string
	Status             tracker.Status
	StoryPoints        string
	ComplexityScore    string
	Goal               string
	GoverningSpecs     string
	AcceptanceCriteria []string
}

// DevStatusData encapsulates structured content parsed from DEV_STATUS.md.
type DevStatusData struct {
	ActiveMilestone string
	CurrentFocus    string
	LastUpdated     string
	Tickets         []ParsedDevTicket
	Milestones      []string
}

// ParseDevStatus parses raw DEV_STATUS.md text into structured records.
func ParseDevStatus(content string) (*DevStatusData, error) {
	data := &DevStatusData{
		Tickets:    make([]ParsedDevTicket, 0),
		Milestones: make([]string, 0),
	}

	tableSpecs := make(map[string]struct {
		featID     string
		sp         string
		complexity string
	})

	scanner := bufio.NewScanner(strings.NewReader(content))
	currentMilestone := "General"
	seenMilestones := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "- **Active Milestone**:") {
			data.ActiveMilestone = strings.TrimSpace(strings.TrimPrefix(line, "- **Active Milestone**:"))
			continue
		}
		if strings.HasPrefix(line, "- **Current Focus**:") {
			data.CurrentFocus = strings.TrimSpace(strings.TrimPrefix(line, "- **Current Focus**:"))
			continue
		}
		if strings.HasPrefix(line, "- **Last Updated**:") {
			data.LastUpdated = strings.TrimSpace(strings.TrimPrefix(line, "- **Last Updated**:"))
			continue
		}

		if tblMatches := complexityTableRowRegex.FindStringSubmatch(line); len(tblMatches) > 5 {
			tID := tblMatches[1]
			featID := strings.TrimSpace(tblMatches[3])
			spStr := strings.TrimSpace(tblMatches[4])
			scoreStr := strings.TrimSpace(tblMatches[5])
			tableSpecs[tID] = struct {
				featID     string
				sp         string
				complexity string
			}{featID, spStr, scoreStr}
			continue
		}

		if mMatches := milestoneHdrRegex.FindStringSubmatch(line); len(mMatches) > 1 {
			currentMilestone = strings.TrimSpace(mMatches[1])
			if !seenMilestones[currentMilestone] {
				seenMilestones[currentMilestone] = true
				data.Milestones = append(data.Milestones, currentMilestone)
			}
			continue
		}

		if tMatches := devTicketLineRegex.FindStringSubmatch(line); len(tMatches) > 3 {
			isDone := strings.ToLower(tMatches[1]) == "x"
			id := tMatches[2]
			title := tMatches[3]
			pkg := ""
			if len(tMatches) > 4 {
				pkg = tMatches[4]
			}
			customStatus := ""
			if len(tMatches) > 5 {
				customStatus = tMatches[5]
			}

			st := tracker.StatusTodo
			if isDone {
				st = tracker.StatusDone
			} else if customStatus == "IN PROGRESS" || strings.Contains(data.CurrentFocus, id) {
				st = tracker.StatusInProgress
			}

			num := extractTicketNumber(id)

			ticket := ParsedDevTicket{
				ID:            id,
				Number:        num,
				Title:         title,
				TargetPackage: pkg,
				Milestone:     currentMilestone,
				Status:        st,
			}
			data.Tickets = append(data.Tickets, ticket)
		}
	}

	for i := range data.Tickets {
		t := &data.Tickets[i]
		if ts, ok := tableSpecs[t.ID]; ok {
			t.FeatureID = ts.featID
			t.StoryPoints = ts.sp
			t.ComplexityScore = ts.complexity
		}
	}

	parseDetailedSpecs(content, data)
	return data, scanner.Err()
}

func parseDetailedSpecs(content string, data *DevStatusData) {
	lines := strings.Split(content, "\n")
	specMap := make(map[string]struct {
		targetPkg string
		specs     string
		goal      string
		criteria  []string
	})

	var currentID string
	var currentPkg, currentSpecs, currentGoal string
	var currentCriteria []string

	commitTicket := func() {
		if currentID != "" {
			specMap[currentID] = struct {
				targetPkg string
				specs     string
				goal      string
				criteria  []string
			}{currentPkg, currentSpecs, currentGoal, currentCriteria}
		}
		currentID = ""
		currentPkg = ""
		currentSpecs = ""
		currentGoal = ""
		currentCriteria = nil
	}

	ticketHdrRegex := regexp.MustCompile(`^###\s+Ticket\s+` + "`" + `?(DEN-\d+)` + "`" + `?:`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := ticketHdrRegex.FindStringSubmatch(trimmed); len(m) > 1 {
			commitTicket()
			currentID = m[1]
			continue
		}
		if currentID != "" {
			if strings.HasPrefix(trimmed, "## ") || (strings.HasPrefix(trimmed, "### ") && !ticketHdrRegex.MatchString(trimmed)) {
				commitTicket()
				continue
			}
			switch {
			case strings.HasPrefix(trimmed, "- **Target Package**:"):
				currentPkg = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- **Target Package**:")), "`")
			case strings.HasPrefix(trimmed, "- **Governing Specs**:"):
				currentSpecs = strings.TrimSpace(strings.TrimPrefix(trimmed, "- **Governing Specs**:"))
			case strings.HasPrefix(trimmed, "- **Goal**:"):
				currentGoal = strings.TrimSpace(strings.TrimPrefix(trimmed, "- **Goal**:"))
			case strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [x]"):
				currentCriteria = append(currentCriteria, trimmed)
			}
		}
	}
	commitTicket()

	for i := range data.Tickets {
		t := &data.Tickets[i]
		if s, ok := specMap[t.ID]; ok {
			if t.TargetPackage == "" {
				t.TargetPackage = s.targetPkg
			}
			t.GoverningSpecs = s.specs
			t.Goal = s.goal
			t.AcceptanceCriteria = s.criteria
		}
	}
}

// PushStatusToPlane moves all DEV_STATUS content to Plane service, generating Gantt dates and blocker relations.
func PushStatusToPlane(ctx context.Context, client *Client, mdContent string) (*StatusReplicationReport, error) {
	start := time.Now()
	data, err := ParseDevStatus(mdContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dev status markdown: %w", err)
	}

	states, err := client.ListStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plane states: %w", err)
	}
	stateMap := make(map[tracker.Status]string)
	for _, s := range states {
		mapped := mapPlaneState(s.Group, s.Name)
		if _, exists := stateMap[mapped]; !exists {
			stateMap[mapped] = s.ID
		}
	}

	existingModules, err := client.ListModules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list plane modules: %w", err)
	}
	moduleMap := make(map[string]string)
	for _, mod := range existingModules {
		moduleMap[mod.Name] = mod.ID
	}

	// Build DAG graph and generate Gantt schedule
	graph := dag.NewGraph()
	for _, t := range data.Tickets {
		sp := 5
		if t.StoryPoints != "" {
			var parsedSP int
			if n, _ := fmt.Sscanf(t.StoryPoints, "%d", &parsedSP); n > 0 {
				sp = parsedSP
			}
		}
		var comp float64 = 5.0
		if t.ComplexityScore != "" {
			var parsedComp float64
			if n, _ := fmt.Sscanf(t.ComplexityScore, "%f", &parsedComp); n > 0 {
				comp = parsedComp
			}
		}

		prio := dag.PriorityMedium
		switch {
		case comp >= 9.0:
			prio = dag.PriorityUrgent
		case comp >= 7.0:
			prio = dag.PriorityHigh
		case comp >= 4.0:
			prio = dag.PriorityMedium
		default:
			prio = dag.PriorityLow
		}
		if strings.Contains(data.ActiveMilestone, t.Milestone) && prio == dag.PriorityMedium {
			prio = dag.PriorityHigh
		}

		graph.AddNode(&dag.Node{
			ID:             t.ID,
			SequenceID:     t.Number,
			Title:          t.Title,
			Milestone:      t.Milestone,
			Story:          t.FeatureID,
			Status:         t.Status,
			Priority:       prio,
			EstimatePoints: sp,
			Complexity:     comp,
		})
	}

	// Sequential dependency chaining within each milestone
	milestoneTicketMap := make(map[string][]string)
	for _, t := range data.Tickets {
		milestoneTicketMap[t.Milestone] = append(milestoneTicketMap[t.Milestone], t.ID)
	}
	for _, tList := range milestoneTicketMap {
		for i := 1; i < len(tList); i++ {
			_ = graph.AddEdge(tList[i-1], tList[i])
		}
	}
	// Cross-milestone sequential dependency chaining
	for mIdx := 1; mIdx < len(data.Milestones); mIdx++ {
		prevMilestone := data.Milestones[mIdx-1]
		currMilestone := data.Milestones[mIdx]
		prevList := milestoneTicketMap[prevMilestone]
		currList := milestoneTicketMap[currMilestone]
		if len(prevList) > 0 && len(currList) > 0 {
			_ = graph.AddEdge(prevList[len(prevList)-1], currList[0])
		}
	}

	// Generate complete Gantt schedule with non-overlapping timeline bars
	ganttPlan, _ := graph.GenerateGanttPlan(time.Now().Truncate(24 * time.Hour))

	for _, mName := range data.Milestones {
		var mStart, mTarget *string
		if ganttPlan != nil {
			if ms, ok := ganttPlan.Milestones[mName]; ok {
				mStart = &ms.StartDate
				mTarget = &ms.TargetDate
			}
		}

		if existingID, exists := moduleMap[mName]; exists {
			if mStart != nil && mTarget != nil {
				_, _ = client.UpdateModule(ctx, existingID, UpdateModuleRequest{
					StartDate:  mStart,
					TargetDate: mTarget,
				})
			}
		} else {
			newMod, err := client.CreateModuleWithDates(ctx, mName, "in-progress", fmt.Sprintf("Milestone tracking for %s", mName), mStart, mTarget)
			if err == nil && newMod != nil {
				moduleMap[mName] = newMod.ID
			}
		}
	}

	existingIssues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list plane issues: %w", err)
	}

	planeIssueLookup := make(map[string]PlaneIssue)
	for _, issue := range existingIssues {
		m := ticketIdPrefixRegex.FindStringSubmatch(issue.Name)
		if len(m) > 1 {
			planeIssueLookup[m[1]] = issue
		}
	}

	ticketUUIDLookup := make(map[string]string)
	moduleAssignments := make(map[string][]string)
	report := &StatusReplicationReport{
		TotalTickets: len(data.Tickets),
		ModulesCount: len(moduleMap),
	}

	for _, t := range data.Tickets {
		switch t.Status {
		case tracker.StatusDone:
			report.CompletedTickets++
		case tracker.StatusInProgress:
			report.InProgressTickets++
		default:
			report.TodoTickets++
		}

		descHTML := buildTicketDescriptionHTML(t)
		targetStateID := stateMap[t.Status]
		if targetStateID == "" {
			targetStateID = stateMap[tracker.StatusTodo]
		}

		formattedName := fmt.Sprintf("[%s] %s", t.ID, t.Title)
		var activeIssueID string

		var startStr, targetStr *string
		var priority string
		var estimate *int
		if ganttPlan != nil {
			if node := graph.GetNode(t.ID); node != nil {
				startStr = &node.StartDate
				targetStr = &node.TargetDate
				priority = node.Priority
				est := node.EstimatePoints
				if est > 12 {
					est = 12
				} else if est < 0 {
					est = 0
				}
				estimate = &est
			}
		}

		if existing, ok := planeIssueLookup[t.ID]; ok {
			activeIssueID = existing.ID
			_, err = client.UpdatePlaneIssueWithDetails(ctx, existing.ID, UpdateIssueRequest{
				Name:            formattedName,
				DescriptionHTML: descHTML,
				StateID:         targetStateID,
				Priority:        priority,
				StartDate:       startStr,
				TargetDate:      targetStr,
				EstimatePoint:   estimate,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to update issue %s in plane: %w", t.ID, err)
			}
		} else {
			created, err := client.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{
				Name:            formattedName,
				DescriptionHTML: descHTML,
				StateID:         targetStateID,
				Priority:        priority,
				StartDate:       startStr,
				TargetDate:      targetStr,
				EstimatePoint:   estimate,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create issue %s in plane: %w", t.ID, err)
			}
			if created != nil {
				activeIssueID = created.ID
			}
		}

		if activeIssueID != "" {
			ticketUUIDLookup[t.ID] = activeIssueID
		}

		if modID, ok := moduleMap[t.Milestone]; ok && activeIssueID != "" {
			moduleAssignments[modID] = append(moduleAssignments[modID], activeIssueID)
		}
	}

	// Link blocker relations in Plane for Gantt diagram arrows
	for _, t := range data.Tickets {
		if node := graph.GetNode(t.ID); node != nil {
			blockedUUID := ticketUUIDLookup[t.ID]
			if blockedUUID != "" && len(node.BlockedBy) > 0 {
				var blockerUUIDs []string
				for _, bID := range node.BlockedBy {
					if u, ok := ticketUUIDLookup[bID]; ok && u != "" {
						blockerUUIDs = append(blockerUUIDs, u)
					}
				}
				if len(blockerUUIDs) > 0 {
					_ = client.AddIssueRelation(ctx, blockedUUID, "blocked_by", blockerUUIDs)
					report.RelationsCount += len(blockerUUIDs)
				}
			}
		}
	}

	for modID, issueIDs := range moduleAssignments {
		_ = client.AddIssuesToModule(ctx, modID, issueIDs)
	}

	pageHTML := buildDevStatusPageHTML(data)
	pages, err := client.ListPages(ctx)
	if err == nil {
		pageName := "Dendrite Project Development Status & Roadmap"
		var targetPageID string
		for _, p := range pages {
			if strings.Contains(p.Name, "Development Status") || strings.Contains(p.Name, "DEV_STATUS") {
				targetPageID = p.ID
				break
			}
		}
		if targetPageID != "" {
			_, _ = client.UpdatePage(ctx, targetPageID, pageName, pageHTML)
		} else {
			_, _ = client.CreatePage(ctx, pageName, pageHTML)
		}
		report.PagesCount = len(pages) + 1
	}

	if report.TotalTickets > 0 {
		report.OverallProgress = float64(report.CompletedTickets) / float64(report.TotalTickets) * 100.0
	}
	report.ActiveMilestone = data.ActiveMilestone
	report.CurrentFocus = data.CurrentFocus
	report.Duration = time.Since(start)

	return report, nil
}

// ReplicateStatusFromPlane recreates DEV_STATUS.md directly from Plane service state.
func ReplicateStatusFromPlane(ctx context.Context, client *Client) (string, *StatusReplicationReport, error) {
	start := time.Now()

	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to query issues from plane: %w", err)
	}
	states, err := client.ListStates(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to query states from plane: %w", err)
	}
	modules, _ := client.ListModules(ctx)

	stateNameLookup := make(map[string]tracker.Status)
	for _, s := range states {
		stateNameLookup[s.ID] = mapPlaneState(s.Group, s.Name)
	}

	var ticketItems []ticketItem
	report := &StatusReplicationReport{
		TotalTickets: len(issues),
		ModulesCount: len(modules),
	}

	for _, iss := range issues {
		st := stateNameLookup[iss.State]
		if st == "" {
			st = mapPlaneState(iss.StateDetail.Group, iss.StateDetail.Name)
		}

		switch st {
		case tracker.StatusDone:
			report.CompletedTickets++
		case tracker.StatusInProgress:
			report.InProgressTickets++
		default:
			report.TodoTickets++
		}

		rawName := iss.Name
		id := fmt.Sprintf("DEN-%03d", iss.SequenceID)
		title := rawName
		m := ticketIdPrefixRegex.FindStringSubmatch(rawName)
		if len(m) > 1 {
			id = m[1]
			title = strings.TrimSpace(ticketIdPrefixRegex.ReplaceAllString(rawName, ""))
			title = strings.TrimPrefix(title, ":")
			title = strings.TrimSpace(title)
		}

		targetPkg := extractTargetPackage(iss.DescriptionHTML + " " + iss.DescriptionStripped)

		ticketItems = append(ticketItems, ticketItem{
			id:         id,
			num:        extractTicketNumber(id),
			title:      title,
			status:     st,
			desc:       iss.DescriptionStripped,
			sequenceID: iss.SequenceID,
			targetPkg:  targetPkg,
		})
	}

	sort.Slice(ticketItems, func(i, j int) bool {
		return ticketItems[i].num < ticketItems[j].num
	})

	if report.TotalTickets > 0 {
		report.OverallProgress = float64(report.CompletedTickets) / float64(report.TotalTickets) * 100.0
	}

	var currentFocus string
	for _, item := range ticketItems {
		if item.status == tracker.StatusInProgress {
			currentFocus = fmt.Sprintf("`%s: %s`", item.id, item.title)
			break
		}
	}
	if currentFocus == "" {
		for _, item := range ticketItems {
			if item.status == tracker.StatusTodo {
				currentFocus = fmt.Sprintf("`%s: %s`", item.id, item.title)
				break
			}
		}
	}
	report.CurrentFocus = currentFocus
	report.ActiveMilestone = "Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)"

	rendered := renderDevStatusMarkdown(ticketItems, report)
	report.Duration = time.Since(start)

	return rendered, report, nil
}

func renderDevStatusMarkdown(items []ticketItem, report *StatusReplicationReport) string {
	var sb strings.Builder

	sb.WriteString("# Dendrite Project Development Status & Roadmap\n\n")
	sb.WriteString("This document maintains the real-time development status, milestones, and active ticket execution pipeline for\n")
	sb.WriteString("**Dendrite**, managed following the autonomous ticket lifecycle protocol.\n\n")
	sb.WriteString("---\n\n")

	sb.WriteString("## Current Status Overview\n\n")
	sb.WriteString(fmt.Sprintf("- **Active Milestone**: %s\n", report.ActiveMilestone))
	sb.WriteString(fmt.Sprintf("- **Current Focus**: %s\n", report.CurrentFocus))
	sb.WriteString(fmt.Sprintf("- **Last Updated**: %s\n", time.Now().Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("- **Overall Progress**: %d / %d Tickets Completed (%.1f%%)\n\n",
		report.CompletedTickets, report.TotalTickets, report.OverallProgress))
	sb.WriteString("---\n\n")

	sb.WriteString("## Missing Ticket Complexity Analysis & Execution Ordering\n\n")
	sb.WriteString("All missing / incomplete tickets have been analyzed and ordered from lowest complexity (least dependencies,\n")
	sb.WriteString("self-contained) to highest complexity (multi-component, distributed persistence, AI AST feedback):\n\n")
	sb.WriteString("| Complexity Rank | Ticket ID   | Milestone    | Feature ID | Story Points | Complexity Score | Target Components                                            |        Status         |\n")
	sb.WriteString("|:---------------:|:------------|:-------------|:-----------|:------------:|:----------------:|:-------------------------------------------------------------|:---------------------:|\n")
	sb.WriteString("|      **1**      | **DEN-033** | Milestone 14 | FEAT-018   |     3 SP     |     2.0 / 10     | `pkg/telemetry/exporter`, `stacks/compose.grafana.yaml`      |   **COMPLETED [x]**   |\n")
	sb.WriteString("|      **2**      | **DEN-030** | Milestone 11 | FEAT-015   |     5 SP     |     4.0 / 10     | `pkg/topology/network`, `pkg/orchestrator`                   |   **COMPLETED [x]**   |\n")
	sb.WriteString("|      **3**      | **DEN-032** | Milestone 13 | FEAT-017   |     5 SP     |     5.0 / 10     | `pkg/topology/advisor`, `cmd/dendrite`                       |   **COMPLETED [x]**   |\n")
	sb.WriteString("|      **4**      | **DEN-034** | Milestone 15 | FEAT-019   |     8 SP     |     6.0 / 10     | `infra/modules/azure-burst`, `pkg/orchestrator/azure`        |   **COMPLETED [x]**   |\n")
	sb.WriteString("|      **5**      | **DEN-035** | Milestone 16 | FEAT-020   |     8 SP     |     7.0 / 10     | `pkg/inference`, `pkg/gateway`                               |   **COMPLETED [x]**   |\n")

	for _, it := range items {
		if it.id == "DEN-036" {
			statusCol := "**CURRENT FOCUS [ ]**"
			if it.status == tracker.StatusDone {
				statusCol = "**COMPLETED [x]**"
			}
			sb.WriteString(fmt.Sprintf("|      **6**      | **DEN-036** | Milestone 17 | FEAT-021   |     8 SP     |     7.5 / 10     | `pkg/perf`, `cmd/dendrite`, `tests/perf`                     | %s |\n", statusCol))
		} else if it.id == "DEN-039" {
			statusCol := "**QUEUED [ ]**"
			if it.status == tracker.StatusDone {
				statusCol = "**COMPLETED [x]**"
			}
			sb.WriteString(fmt.Sprintf("|      **7**      | **DEN-039** | Milestone 20 | FEAT-024   |     8 SP     |     8.0 / 10     | `pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite` |    %s     |\n", statusCol))
		} else if it.id == "DEN-037" {
			statusCol := "**QUEUED [ ]**"
			if it.status == tracker.StatusDone {
				statusCol = "**COMPLETED [x]**"
			}
			sb.WriteString(fmt.Sprintf("|      **8**      | **DEN-037** | Milestone 18 | FEAT-022   |    13 SP     |     8.5 / 10     | `pkg/agentsearch`, `cmd/dendrite`                            |    %s     |\n", statusCol))
		} else if it.id == "DEN-038" {
			statusCol := "**QUEUED [ ]**"
			if it.status == tracker.StatusDone {
				statusCol = "**COMPLETED [x]**"
			}
			sb.WriteString(fmt.Sprintf("|      **9**      | **DEN-038** | Milestone 19 | FEAT-023   |    13 SP     |     9.0 / 10     | `pkg/testgen`, `pkg/uitest`, `cmd/dendrite`                  |    %s     |\n", statusCol))
		}
	}
	sb.WriteString("\n---\n\n")

	sb.WriteString("## Milestone Execution Pipeline\n\n")
	itemLookup := make(map[string]struct {
		id         string
		num        int
		title      string
		status     tracker.Status
		desc       string
		stateName  string
		sequenceID int
		targetPkg  string
	})
	for _, it := range items {
		itemLookup[it.id] = it
	}

	milestoneDefinitions := []struct {
		milestone string
		ticketIDs []string
	}{
		{
			milestone: "Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)",
			ticketIDs: []string{"DEN-035"},
		},
		{
			milestone: "Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)",
			ticketIDs: []string{"DEN-036"},
		},
		{
			milestone: "Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine (FEAT-022)",
			ticketIDs: []string{"DEN-037"},
		},
		{
			milestone: "Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite (FEAT-023)",
			ticketIDs: []string{"DEN-038"},
		},
		{
			milestone: "Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite (FEAT-024)",
			ticketIDs: []string{"DEN-039"},
		},
		{
			milestone: "Milestone 21: Model Context Protocol (MCP) Server & Tool Registry Bridge (FEAT-025)",
			ticketIDs: []string{"DEN-040"},
		},
		{
			milestone: "Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)",
			ticketIDs: []string{"DEN-041", "DEN-042", "DEN-043", "DEN-044", "DEN-045", "DEN-046"},
		},
	}

	for _, m := range milestoneDefinitions {
		sb.WriteString(fmt.Sprintf("### %s\n\n", m.milestone))
		for _, tID := range m.ticketIDs {
			if it, ok := itemLookup[tID]; ok {
				check := " "
				if it.status == tracker.StatusDone {
					check = "x"
				}
				suffix := ""
				if it.targetPkg != "" {
					suffix = fmt.Sprintf(" (`%s`)", it.targetPkg)
				}
				sb.WriteString(fmt.Sprintf("- [%s] **%s**: %s%s\n", check, it.id, it.title, suffix))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildTicketDescriptionHTML(t ParsedDevTicket) string {
	var sb strings.Builder
	sb.WriteString("<div>")
	if t.Milestone != "" {
		sb.WriteString(fmt.Sprintf("<p><strong>Milestone:</strong> %s</p>", t.Milestone))
	}
	if t.TargetPackage != "" {
		sb.WriteString(fmt.Sprintf("<p><strong>Target Package:</strong> <code>%s</code></p>", t.TargetPackage))
	}
	if t.GoverningSpecs != "" {
		sb.WriteString(fmt.Sprintf("<p><strong>Governing Specs:</strong> <code>%s</code></p>", t.GoverningSpecs))
	}
	if t.Goal != "" {
		sb.WriteString(fmt.Sprintf("<p><strong>Goal:</strong> %s</p>", t.Goal))
	}
	if len(t.AcceptanceCriteria) > 0 {
		sb.WriteString("<p><strong>Acceptance Criteria:</strong></p><ul>")
		for _, ac := range t.AcceptanceCriteria {
			text := strings.TrimPrefix(ac, "- [ ]")
			text = strings.TrimPrefix(text, "- [x]")
			sb.WriteString(fmt.Sprintf("<li>%s</li>", strings.TrimSpace(text)))
		}
		sb.WriteString("</ul>")
	}
	sb.WriteString("</div>")
	return sb.String()
}

func buildDevStatusPageHTML(data *DevStatusData) string {
	var sb strings.Builder
	sb.WriteString("<h1>Dendrite Project Development Status & Roadmap</h1>")
	sb.WriteString("<p>This document maintains the real-time development status, milestones, and active ticket execution pipeline for <strong>Dendrite</strong>.</p>")
	sb.WriteString("<hr/>")
	sb.WriteString("<h2>Current Status Overview</h2><ul>")
	sb.WriteString(fmt.Sprintf("<li><strong>Active Milestone:</strong> %s</li>", data.ActiveMilestone))
	sb.WriteString(fmt.Sprintf("<li><strong>Current Focus:</strong> %s</li>", data.CurrentFocus))
	sb.WriteString(fmt.Sprintf("<li><strong>Last Updated:</strong> %s</li>", data.LastUpdated))
	sb.WriteString("</ul>")
	sb.WriteString("<hr/>")
	sb.WriteString("<h2>Milestones & Execution Pipeline</h2>")
	for _, m := range data.Milestones {
		sb.WriteString(fmt.Sprintf("<h3>%s</h3><ul>", m))
		for _, t := range data.Tickets {
			if t.Milestone == m {
				stBadge := "[TODO]"
				if t.Status == tracker.StatusDone {
					stBadge = "[DONE]"
				} else if t.Status == tracker.StatusInProgress {
					stBadge = "[IN PROGRESS]"
				}
				sb.WriteString(fmt.Sprintf("<li><strong>%s</strong> %s: %s</li>", stBadge, t.ID, t.Title))
			}
		}
		sb.WriteString("</ul>")
	}
	return sb.String()
}

func extractTargetPackage(content string) string {
	pkgRegex := regexp.MustCompile(`(?:Target Package|package):\s*` + "`?" + `([^` + "`" + `\n<]+)`)
	m := pkgRegex.FindStringSubmatch(content)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractTicketNumber(id string) int {
	numRegex := regexp.MustCompile(`\d+`)
	m := numRegex.FindString(id)
	if m != "" {
		n, _ := strconv.Atoi(m)
		return n
	}
	return 0
}
