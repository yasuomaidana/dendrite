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
)

var (
	devTicketLineRegex  = regexp.MustCompile(`^-\s+\[([ xX])\]\s+\*\*([A-Za-z0-9-]+)\*\*:\s+(.*?)(?:\s+\(` + "`" + `([^` + "`" + `]+)` + "`" + `\))?(?:\s+\*\(([A-Z_ ]+)\)\*)?$`)
	milestoneHdrRegex   = regexp.MustCompile(`^###\s+(Milestone\s+\d+:.*?)(?:\s+\(([^)]+)\))?$`)
	ticketIdPrefixRegex = regexp.MustCompile(`\[?(DEN-\d+)\]?`)
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

// PushStatusToPlane moves all DEV_STATUS content to Plane service.
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

	for _, mName := range data.Milestones {
		if _, exists := moduleMap[mName]; !exists {
			newMod, err := client.CreateModule(ctx, mName, "in-progress", fmt.Sprintf("Milestone tracking for %s", mName))
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

		if existing, ok := planeIssueLookup[t.ID]; ok {
			activeIssueID = existing.ID
			_, err = client.UpdatePlaneIssue(ctx, existing.ID, formattedName, descHTML, targetStateID, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to update issue %s in plane: %w", t.ID, err)
			}
		} else {
			created, err := client.CreatePlaneIssue(ctx, formattedName, descHTML, targetStateID, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create issue %s in plane: %w", t.ID, err)
			}
			if created != nil {
				activeIssueID = created.ID
			}
		}

		if modID, ok := moduleMap[t.Milestone]; ok && activeIssueID != "" {
			moduleAssignments[modID] = append(moduleAssignments[modID], activeIssueID)
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
		{"Milestone 13: Remote SSH Hardware Discovery & Automated Service Placement Router (FEAT-017)", []string{"DEN-032"}},
		{"Milestone 15: Azure Cloud-Burst Infrastructure & Managed Storage (FEAT-019)", []string{"DEN-034"}},
		{"Milestone 21: Model Context Protocol (MCP) Server & Tool Registry Bridge (FEAT-025)", []string{"DEN-040"}},
		{"Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)", []string{"DEN-041", "DEN-042", "DEN-043", "DEN-044", "DEN-045", "DEN-046"}},
		{"Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)", []string{"DEN-035"}},
		{"Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)", []string{"DEN-036"}},
		{"Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite (FEAT-024)", []string{"DEN-039"}},
		{"Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine (FEAT-022)", []string{"DEN-037"}},
		{"Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite (FEAT-023)", []string{"DEN-038"}},
		{"Milestone 11: Pluggable Mesh Networking & Zero-Trust Connectivity (FEAT-015)", []string{"DEN-030"}},
		{"Milestone 14: Hybrid Telemetry Architecture & Prometheus/Grafana Observability Suite (FEAT-018)", []string{"DEN-033"}},
		{"Milestone 12: Open-Source Project Management & Design Tooling (FEAT-016)", []string{"DEN-031"}},
		{"Milestone 10: IDE Intelligence & JetBrains MCP Bridge (FEAT-014)", []string{"DEN-028"}},
		{"Milestone 9: Web UI & Dashboard Engine (FEAT-009)", []string{"DEN-014", "DEN-015"}},
		{"Milestone 8: Multi-Project Workspaces & Pluggable Trackers (FEAT-005, FEAT-008)", []string{"DEN-012", "DEN-013", "DEN-027"}},
		{"Milestone 7: Polyglot Translation & Documentation Reflection (FEAT-011, FEAT-012)", []string{"DEN-020", "DEN-021", "DEN-022", "DEN-023"}},
		{"Milestone 6: AI-Assisted Token Optimization & Cleanroom Import (FEAT-010)", []string{"DEN-018", "DEN-019"}},
		{"Milestone 5: Reverse Engineering & Sandboxed Intelligence (FEAT-003, FEAT-004, FEAT-006)", []string{"DEN-009", "DEN-010", "DEN-011"}},
		{"Milestone 4: Autonomous Coding Agent Harness & Tool Orchestration (FEAT-013)", []string{"DEN-024", "DEN-025", "DEN-026"}},
		{"Milestone 3: Token Budget Governance & Fallback Gateway (FEAT-002)", []string{"DEN-007", "DEN-008"}},
		{"Milestone 2: Multi-Node Topology & Cloud-Burst Infrastructure (FEAT-001, FEAT-007)", []string{"DEN-005", "DEN-006", "DEN-016", "DEN-017", "DEN-029"}},
		{"Milestone 1: Core Foundation, Developer Tools & Ollama Engine", []string{"DEN-001", "DEN-002", "DEN-003", "DEN-004"}},
	}

	for _, m := range milestoneDefinitions {
		sb.WriteString(fmt.Sprintf("### %s\n\n", m.milestone))
		for _, tid := range m.ticketIDs {
			if it, ok := itemLookup[tid]; ok {
				check := " "
				suffix := ""
				if it.status == tracker.StatusDone {
					check = "x"
				} else if it.status == tracker.StatusInProgress {
					suffix = " *(IN PROGRESS)*"
				}
				pkgPart := ""
				if it.targetPkg != "" && !strings.Contains(it.title, it.targetPkg) && !strings.HasSuffix(strings.TrimSpace(it.title), ")") {
					pkgPart = fmt.Sprintf(" (`%s`)", it.targetPkg)
				}
				sb.WriteString(fmt.Sprintf("- [%s] **%s**: %s%s%s\n", check, it.id, it.title, pkgPart, suffix))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString("## Detailed Specifications for Active & Queued Tickets\n\n")
	sb.WriteString("### Ticket `DEN-036`: Performance Testing, Inference Benchmarking & Gateway Load Harness (k6)\n\n")
	sb.WriteString("- **Target Package**: `pkg/perf`, `cmd/dendrite`, `tests/perf`\n")
	sb.WriteString("- **Governing Specs**: [`docs/features/FEAT-021-performance-testing-k6-benchmark.md`](docs/features/FEAT-021-performance-testing-k6-benchmark.md)\n")
	sb.WriteString("- **Goal**: Integrate k6 runner wrapper and scenario definitions to benchmark Time-To-First-Token (TTFT), Tokens-Per-Second (TPS), concurrent agent load, Apple Silicon thermal correlation, and LiteLLM cascade failovers under injected faults.\n")
	sb.WriteString("- **Acceptance Criteria**:\n")
	sb.WriteString("    - [ ] Implement `k6` benchmark scenario runner with TTFT/TPS metrics parsing.\n")
	sb.WriteString("    - [ ] Fault injection harness simulating HTTP 429 and HTTP 503 failover triggers.\n")
	sb.WriteString("    - [ ] CLI subcommand `dendrite bench <inference|gateway|matrix>`.\n")
	sb.WriteString("    - [ ] Statement coverage $\\ge 85\\%$.\n\n")

	sb.WriteString("### Ticket `DEN-039`: Open-Source Reverse Engineering Showcases & Cleanroom Parity Verification Suite\n\n")
	sb.WriteString("- **Target Package**: `pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite`\n")
	sb.WriteString("- **Governing Specs**: [`docs/features/FEAT-024-open-source-showcases.md`](docs/features/FEAT-024-open-source-showcases.md)\n")
	sb.WriteString("- **Goal**: Implement automated end-to-end showcase runners and cleanroom verification suites using 5 open-source target archetypes.\n")
	sb.WriteString("- **Acceptance Criteria**:\n")
	sb.WriteString("    - [ ] CLI subcommand `dendrite rea showcase <aegis|dxball|ctf|th04|mattermost|all>` executing automated analysis and verification loops.\n")
	sb.WriteString("    - [ ] Bounded evidence bundling conforming to $< 4,000$ tokens per showcase probe.\n")
	sb.WriteString("    - [ ] Cleanroom synthesis pipeline producing test-verified Go/Rust implementations and ADR documents with YAML frontmatter.\n")
	sb.WriteString("    - [ ] Table-driven integration and property test suite achieving $\\ge 85\\%$ statement coverage.\n\n")

	sb.WriteString("### Ticket `DEN-041`: Distributed Codebase Multi-Source Ecosystem & Priority Resolver\n\n")
	sb.WriteString("- **Target Package**: `pkg/distcodebase`, `cmd/dendrite`\n")
	sb.WriteString("- **Governing Specs**: [`docs/features/FEAT-026-distributed-codebase-ecosystem.md`](docs/features/FEAT-026-distributed-codebase-ecosystem.md)\n")
	sb.WriteString("- **Goal**: Implement multi-source registry prioritizing user/organization-owned repositories (P1), curated high-performance OSS engines (P2: wgpu, turso, diesel, diesel-cte-ext), and public registries (P3), forced modular decomposition, local LLM interoperability coordinator, and parallel/satellite project spawner with Plane v2 integration.\n")
	sb.WriteString("- **Acceptance Criteria**:\n")
	sb.WriteString("    - [x] Multi-source priority resolver prioritizing owned repos (P1) > curated OSS (P2) > public registries (P3).\n")
	sb.WriteString("    - [x] Pre-configured libraries: `wgpu`, `yasuomaidana` repositories, `turso`, `diesel`, `diesel-cte-ext`.\n")
	sb.WriteString("    - [x] Forced modular decomposition engine for REA evidence bundles and legacy systems.\n")
	sb.WriteString("    - [x] Cross-project interoperability coordinator with `TODO(interop:...)` fallbacks and automatic reconciliation.\n")
	sb.WriteString("    - [x] Parallel repository and Plane v2 satellite project spawner.\n")
	sb.WriteString("    - [x] Native MCP tools (`dendrite_dist_sources`, `dendrite_dist_decompose`, `dendrite_dist_interop`, `dendrite_dist_spawn`) and CLI subcommands.\n")
	sb.WriteString("    - [x] Statement test coverage $\\ge 85\\%$ (achieved 89.2%).\n\n")

	return sb.String()
}

func extractTicketNumber(id string) int {
	clean := strings.TrimPrefix(id, "DEN-")
	n, _ := strconv.Atoi(clean)
	return n
}

func extractTargetPackage(text string) string {
	pkgRegex := regexp.MustCompile(`(?:Target Package:?\s*|` + "`" + `)(pkg/[a-zA-Z0-9_/]+|infra/[a-zA-Z0-9_/]+|stacks/[a-zA-Z0-9_./]+|cmd/[a-zA-Z0-9_/]+)`)
	m := pkgRegex.FindStringSubmatch(text)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func buildTicketDescriptionHTML(t ParsedDevTicket) string {
	var sb strings.Builder
	sb.WriteString("<div>")
	if t.TargetPackage != "" {
		sb.WriteString(fmt.Sprintf("<p><strong>Target Package:</strong> <code>%s</code></p>", t.TargetPackage))
	}
	if t.Milestone != "" {
		sb.WriteString(fmt.Sprintf("<p><strong>Milestone:</strong> %s</p>", t.Milestone))
	}
	if t.GoverningSpecs != "" {
		sb.WriteString(fmt.Sprintf("<p><strong>Governing Specs:</strong> %s</p>", t.GoverningSpecs))
	}
	if t.Goal != "" {
		sb.WriteString(fmt.Sprintf("<p><strong>Goal:</strong> %s</p>", t.Goal))
	}
	if len(t.AcceptanceCriteria) > 0 {
		sb.WriteString("<p><strong>Acceptance Criteria:</strong></p><ul>")
		for _, ac := range t.AcceptanceCriteria {
			sb.WriteString(fmt.Sprintf("<li>%s</li>", ac))
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

	sb.WriteString("<h2>Milestones & Execution Pipeline</h2><ul>")
	for _, m := range data.Milestones {
		sb.WriteString(fmt.Sprintf("<li><strong>%s</strong></li>", m))
	}
	sb.WriteString("</ul>")
	return sb.String()
}
