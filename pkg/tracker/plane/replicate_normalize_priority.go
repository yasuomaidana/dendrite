package plane

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
)

func normalizePriority(prio string) string {
	p := strings.ToLower(strings.TrimSpace(prio))
	switch p {
	case "urgent":
		return "Urgent"
	case "high":
		return "High"
	case "medium":
		return "Medium"
	case "low":
		return "Low"
	default:
		return "Medium"
	}
}

var knownMilestoneDescriptions = map[int]string{
	1:  "Milestone 1: Core Foundation, Developer Tools & Ollama Engine",
	11: "Milestone 11: Multi-Agent Topology & Distributed Mesh Routing (FEAT-015)",
	13: "Milestone 13: Topology Placement Advisor & Cloud-Burst Engine (FEAT-017)",
	14: "Milestone 14: Real-Time Telemetry Exporter & Distributed Trace Dashboard (FEAT-018)",
	15: "Milestone 15: Elastic Node Discovery & Dynamic Mesh Interconnect (FEAT-019)",
	16: "Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)",
	17: "Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)",
	18: "Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine (FEAT-022)",
	19: "Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite (FEAT-023)",
	20: "Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite (FEAT-024)",
	21: "Milestone 21: Model Context Protocol (MCP) Server & Tool Registry Bridge (FEAT-025)",
	22: "Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)",
}

var defaultTicketMilestones = map[string]string{
	"DEN-001": "Milestone 1: Core Foundation, Developer Tools & Ollama Engine",
	"DEN-030": "Milestone 11: Multi-Agent Topology & Distributed Mesh Routing (FEAT-015)",
	"DEN-032": "Milestone 13: Topology Placement Advisor & Cloud-Burst Engine (FEAT-017)",
	"DEN-033": "Milestone 14: Real-Time Telemetry Exporter & Distributed Trace Dashboard (FEAT-018)",
	"DEN-034": "Milestone 15: Elastic Node Discovery & Dynamic Mesh Interconnect (FEAT-019)",
	"DEN-035": "Milestone 16: Pluggable Multi-Provider Inference Engines (FEAT-020)",
	"DEN-036": "Milestone 17: Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)",
	"DEN-037": "Milestone 18: Agent Search, Capability Discovery & Semantic Routing Engine (FEAT-022)",
	"DEN-038": "Milestone 19: Developer Test Automation, Self-Healing Harness & UI Integration Suite (FEAT-023)",
	"DEN-039": "Milestone 20: Open-Source REA Showcase Pipeline & Cleanroom Parity Verification Suite (FEAT-024)",
	"DEN-040": "Milestone 21: Model Context Protocol (MCP) Server & Tool Registry Bridge (FEAT-025)",
	"DEN-041": "Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)",
	"DEN-042": "Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)",
	"DEN-043": "Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)",
	"DEN-044": "Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)",
	"DEN-045": "Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)",
	"DEN-046": "Milestone 22: Distributed Codebase Ecosystem, Modular REA & Cross-Project Interoperability Coordinator (FEAT-026)",
}

func normalizeMilestoneName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "General"
	}
	num := extractMilestoneNumber(name)
	if full, ok := knownMilestoneDescriptions[num]; ok {
		if len(name) < len(full) && !strings.Contains(name, ":") {
			return full
		}
	}
	return name
}

func extractRegexField(content, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(content)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractMilestone(content string) string {
	return extractRegexField(content, `(?:<strong>)?Milestone:?(?:</strong>)?\s*`+"`?"+`([^`+"`"+`\n<]+)`)
}

func extractProjectIdentifier(ticketID string) string {
	parts := strings.Split(ticketID, "-")
	if len(parts) >= 2 {
		return strings.ToUpper(parts[0])
	}
	return "DEN"
}

// milestoneFor names the milestone of an issue: its first module, a module that lists it, a
// "Milestone:" line in its description, then the built-in default for the ticket id ("" when none).
func milestoneFor(iss PlaneIssue, id string, moduleNames map[string]string, moduleOf map[string]string) string {
	if len(iss.ModuleIDs) > 0 && moduleNames[iss.ModuleIDs[0]] != "" {
		return moduleNames[iss.ModuleIDs[0]]
	}
	if moduleOf[iss.ID] != "" {
		return moduleOf[iss.ID]
	}
	if em := extractMilestone(iss.DescriptionHTML + " " + iss.DescriptionStripped); em != "" {
		return em
	}
	return defaultTicketMilestones[id]
}

// PullStatusFromPlane recreates DEV_STATUS.md directly from Plane service state.
func PullStatusFromPlane(ctx context.Context, client *Client) (string, *StatusReplicationReport, error) {
	start := time.Now()

	mainIssues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to query issues from plane: %w", err)
	}
	states, err := client.ListStates(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to query states from plane: %w", err)
	}
	modules, _ := client.ListModules(ctx)

	// DEV_STATUS.md describes the project it lives in: the workspace holds other projects (peers,
	// unrelated repositories) and each one replicates its own file from its own board.
	projectIdent, workspaceProjects, err := projectIdentifier(ctx, client)
	if err != nil {
		return "", nil, err
	}

	stateNameLookup := make(map[string]tracker.Status)
	for _, s := range states {
		stateNameLookup[s.ID] = resolvePlaneState(s.Group, s.Name)
	}
	moduleNameLookup, moduleIssuesMap := loadModuleNames(ctx, client, modules)

	report := &StatusReplicationReport{ModulesCount: len(modules), ProjectIdentifier: projectIdent}
	var items []ticketItem
	for _, iss := range mainIssues {
		item, ok := buildTicketItem(iss, projectIdent, workspaceProjects)
		if !ok {
			continue
		}
		item.status = issueStatus(iss, stateNameLookup)
		item.milestone = milestoneOf(iss, item, moduleNameLookup, moduleIssuesMap)
		report.countStatus(item.status)
		items = append(items, item)
	}

	report.TotalTickets = len(items)
	if report.TotalTickets > 0 {
		report.OverallProgress = float64(report.CompletedTickets) / float64(report.TotalTickets) * 100.0
	}
	report.CurrentFocus = currentFocusOf(items)
	if projectIdent == "DEN" {
		report.ActiveMilestone = "Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)"
	}

	rendered := renderDevStatusMarkdown(items, report)
	report.Duration = time.Since(start)

	return rendered, report, nil
}

// loadModuleNames maps module ids to names and each issue to the module that lists it.
func loadModuleNames(ctx context.Context, client *Client, modules []PlaneModule) (names, issueModule map[string]string) {
	names = make(map[string]string)
	issueModule = make(map[string]string)
	// Each module's issue list is independent: fetch them together, then merge in module order so a
	// ticket in several modules keeps the same (last) module as before.
	moduleIssues := make([][]PlaneIssue, len(modules))
	fanOut(len(modules), pullWorkers, func(i int) {
		if mIssues, err := client.ListModuleIssues(ctx, modules[i].ID); err == nil {
			moduleIssues[i] = mIssues
		}
	})
	for i, mod := range modules {
		names[mod.ID] = mod.Name
		for _, mi := range moduleIssues[i] {
			issueModule[mi.ID] = mod.Name
		}
	}
	return names, issueModule
}

// buildTicketItem names one issue of the project and says whether it belongs in its file.
func buildTicketItem(iss PlaneIssue, projectIdent string, workspaceProjects map[string]bool) (ticketItem, bool) {
	rawName := iss.Name
	id := fmt.Sprintf("%s-%03d", projectIdent, iss.SequenceID)
	title := rawName
	// Only the leading ID is stripped; IDs referenced later in the title stay.
	if loc := ticketIdPrefixRegex.FindStringSubmatchIndex(rawName); loc != nil {
		id = rawName[loc[2]:loc[3]]
		title = strings.TrimSpace(rawName[:loc[0]] + rawName[loc[1]:])
		title = strings.TrimSpace(strings.TrimPrefix(title, ":"))
	}

	proj := extractProjectIdentifier(id)
	if proj != projectIdent && proj != "DEN" && proj != "DEND" && workspaceProjects[proj] {
		// The title names another project of the workspace: that ticket belongs to the other
		// project's file. Copies relocated from DEN (DEN-*) and legacy DEND-* are kept.
		return ticketItem{}, false
	}
	if projectIdent == "DEN" {
		// Legacy DEND-* tickets live in DEN and belong to its milestones.
		proj = "DEN"
	} else if proj != projectIdent {
		// Relocated copies keep their DEN title; list them under their own key.
		id = fmt.Sprintf("%s-%d", projectIdent, iss.SequenceID)
		title = strings.TrimSpace(rawName)
		proj = projectIdent
	}

	return ticketItem{
		id:         id,
		num:        extractTicketNumber(id),
		title:      title,
		desc:       iss.DescriptionStripped,
		sequenceID: iss.SequenceID,
		targetPkg:  extractTargetPackage(iss.DescriptionHTML + " " + iss.DescriptionStripped),
		priority:   normalizePriority(iss.Priority),
		project:    proj,
	}, true
}

// issueStatus resolves an issue's state from the project's state ids, then from its embedded detail.
func issueStatus(iss PlaneIssue, stateNameLookup map[string]tracker.Status) tracker.Status {
	if st := stateNameLookup[iss.State]; st != "" {
		return st
	}
	return resolvePlaneState(iss.StateDetail.Group, iss.StateDetail.Name)
}

// milestoneOf names the normalized milestone of a ticket; DEN tickets without one go to "General".
func milestoneOf(iss PlaneIssue, item ticketItem, moduleNames, moduleOf map[string]string) string {
	milestone := milestoneFor(iss, item.id, moduleNames, moduleOf)
	if milestone == "" && item.project == "DEN" {
		milestone = "General"
	}
	return normalizeMilestoneName(milestone)
}

// countStatus adds one ticket to the per-state counters of the summary.
func (r *StatusReplicationReport) countStatus(st tracker.Status) {
	switch st {
	case tracker.StatusDone:
		r.CompletedTickets++
		r.DoneTickets++
	case tracker.StatusInReview:
		r.ToVerifyTickets++
	case tracker.StatusInProgress:
		r.InProgressTickets++
	case tracker.StatusTodo:
		r.TodoTickets++
	default: // backlog, blocked, cancelled and anything else counts as backlog
		r.BacklogTickets++
	}
}

// currentFocusOf is the first ticket in progress, else the first one to do ("" when neither exists).
func currentFocusOf(items []ticketItem) string {
	for _, want := range []tracker.Status{tracker.StatusInProgress, tracker.StatusTodo} {
		for _, item := range items {
			if item.status == want {
				return fmt.Sprintf("`%s: %s`", item.id, item.title)
			}
		}
	}
	return ""
}

// ReplicateStatusFromPlane recreates DEV_STATUS.md directly from Plane service state.
func ReplicateStatusFromPlane(ctx context.Context, client *Client) (string, *StatusReplicationReport, error) {
	return PullStatusFromPlane(ctx, client)
}

// projectIdentifier returns the upper-case identifier (DEN, DCORE...) of the client's project and
// the set of identifiers of every project of the workspace. A workspace that does not list the
// project is an error: guessing "DEN" would write a peer's board into the wrong file.
func projectIdentifier(ctx context.Context, client *Client) (string, map[string]bool, error) {
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to query projects from plane: %w", err)
	}
	own := ""
	known := make(map[string]bool, len(projects))
	for _, p := range projects {
		ident := strings.ToUpper(strings.TrimSpace(p.Identifier))
		if ident == "" {
			continue
		}
		known[ident] = true
		if p.ID == client.projectID {
			own = ident
		}
	}
	if own == "" {
		return "", nil, fmt.Errorf("project %s is not in the workspace project list or has no identifier", client.projectID)
	}
	return own, known, nil
}
