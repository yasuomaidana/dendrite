package plane

import (
	"context"
	"fmt"
	"regexp"
	"sort"
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

func extractMilestone(content string) string {
	mRegex := regexp.MustCompile(`(?:<strong>)?Milestone:?(?:</strong>)?\s*` + "`?" + `([^` + "`" + `\n<]+)`)
	m := mRegex.FindStringSubmatch(content)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractProjectIdentifier(ticketID string) string {
	parts := strings.Split(ticketID, "-")
	if len(parts) >= 2 {
		return strings.ToUpper(parts[0])
	}
	return "DEN"
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

	stateNameLookup := make(map[string]tracker.Status)
	for _, s := range states {
		stateNameLookup[s.ID] = resolvePlaneState(s.Group, s.Name)
	}

	moduleNameLookup := make(map[string]string)
	moduleIssuesMap := make(map[string]string)
	for _, mod := range modules {
		moduleNameLookup[mod.ID] = mod.Name
		mIssues, err := client.ListModuleIssues(ctx, mod.ID)
		if err == nil {
			for _, mi := range mIssues {
				moduleIssuesMap[mi.ID] = mod.Name
			}
		}
	}

	// Discover any other projects in Plane workspace (e.g. DCORE, DRAG, DGIT, DMCP)
	allProjects, projErr := client.ListProjects(ctx)
	otherProjectsMap := make(map[string]PlaneProject)
	var otherProjectNames []string
	if projErr == nil {
		for _, p := range allProjects {
			ident := strings.ToUpper(strings.TrimSpace(p.Identifier))
			if p.ID != client.projectID && ident != "DEN" && ident != "" {
				otherProjectsMap[ident] = p
				otherProjectNames = append(otherProjectNames, ident)
			}
		}
		sort.Strings(otherProjectNames)
	}

	type projectIssuePair struct {
		projIdent string
		issue     PlaneIssue
	}
	var otherProjectIssues []projectIssuePair

	// State UUIDs are per project, so each sub-project needs its own lookup.
	projectStateLookup := map[string]map[string]tracker.Status{"DEN": stateNameLookup}
	for _, ident := range otherProjectNames {
		p := otherProjectsMap[ident]
		lookup := make(map[string]tracker.Status)
		if pStates, err := client.ListStatesForProject(ctx, p.ID); err == nil {
			for _, s := range pStates {
				lookup[s.ID] = resolvePlaneState(s.Group, s.Name)
			}
		}
		projectStateLookup[ident] = lookup
		pIssues, err := client.ListPlaneIssuesForProject(ctx, p.ID)
		if err == nil {
			for _, pi := range pIssues {
				otherProjectIssues = append(otherProjectIssues, projectIssuePair{
					projIdent: ident,
					issue:     pi,
				})
			}
		}
	}

	var allTicketItems []ticketItem
	report := &StatusReplicationReport{
		ModulesCount:  len(modules),
		OtherProjects: otherProjectNames,
	}
	seenIDs := make(map[string]bool)

	processIssue := func(iss PlaneIssue, defaultProj string) {
		if iss.ID != "" && seenIDs[iss.ID] {
			return
		}
		if iss.ID != "" {
			seenIDs[iss.ID] = true
		}

		st := projectStateLookup[defaultProj][iss.State]
		if st == "" {
			st = resolvePlaneState(iss.StateDetail.Group, iss.StateDetail.Name)
		}

		switch st {
		case tracker.StatusDone:
			report.CompletedTickets++
			report.DoneTickets++
		case tracker.StatusInReview:
			report.ToVerifyTickets++
		case tracker.StatusInProgress:
			report.InProgressTickets++
		case tracker.StatusTodo:
			report.TodoTickets++
		case tracker.StatusBacklog:
			report.BacklogTickets++
		default:
			report.BacklogTickets++
		}

		rawName := iss.Name
		id := fmt.Sprintf("%s-%03d", defaultProj, iss.SequenceID)
		title := rawName
		// Only the leading ID is stripped; IDs referenced later in the title stay.
		if loc := ticketIdPrefixRegex.FindStringSubmatchIndex(rawName); loc != nil {
			id = rawName[loc[2]:loc[3]]
			title = strings.TrimSpace(rawName[:loc[0]] + rawName[loc[1]:])
			title = strings.TrimPrefix(title, ":")
			title = strings.TrimSpace(title)
		}

		proj := extractProjectIdentifier(id)
		if defaultProj == "DEN" {
			// Legacy DEND-* tickets live in DEN and belong to its milestones.
			proj = "DEN"
		} else if proj != defaultProj {
			// Relocated copies keep their DEN title; list them under their own key.
			id = fmt.Sprintf("%s-%d", defaultProj, iss.SequenceID)
			title = strings.TrimSpace(rawName)
			proj = defaultProj
		}

		targetPkg := extractTargetPackage(iss.DescriptionHTML + " " + iss.DescriptionStripped)
		priority := normalizePriority(iss.Priority)

		milestone := ""
		if len(iss.ModuleIDs) > 0 && moduleNameLookup[iss.ModuleIDs[0]] != "" {
			milestone = moduleNameLookup[iss.ModuleIDs[0]]
		} else if moduleIssuesMap[iss.ID] != "" {
			milestone = moduleIssuesMap[iss.ID]
		} else if em := extractMilestone(iss.DescriptionHTML + " " + iss.DescriptionStripped); em != "" {
			milestone = em
		} else if dm, ok := defaultTicketMilestones[id]; ok {
			milestone = dm
		}

		if milestone == "" && proj == "DEN" {
			milestone = "General"
		}
		milestone = normalizeMilestoneName(milestone)

		allTicketItems = append(allTicketItems, ticketItem{
			id:         id,
			num:        extractTicketNumber(id),
			title:      title,
			status:     st,
			desc:       iss.DescriptionStripped,
			sequenceID: iss.SequenceID,
			targetPkg:  targetPkg,
			priority:   priority,
			milestone:  milestone,
			project:    proj,
		})
	}

	for _, iss := range mainIssues {
		processIssue(iss, "DEN")
	}

	for _, pair := range otherProjectIssues {
		processIssue(pair.issue, pair.projIdent)
	}

	report.TotalTickets = len(allTicketItems)
	if report.TotalTickets > 0 {
		report.OverallProgress = float64(report.CompletedTickets) / float64(report.TotalTickets) * 100.0
	}

	var currentFocus string
	for _, item := range allTicketItems {
		if item.status == tracker.StatusInProgress {
			currentFocus = fmt.Sprintf("`%s: %s`", item.id, item.title)
			break
		}
	}
	if currentFocus == "" {
		for _, item := range allTicketItems {
			if item.status == tracker.StatusTodo {
				currentFocus = fmt.Sprintf("`%s: %s`", item.id, item.title)
				break
			}
		}
	}
	report.CurrentFocus = currentFocus
	report.ActiveMilestone = "Milestone 17 — Performance Testing, Inference Benchmarking & Gateway Load Harness (FEAT-021)"

	rendered := renderDevStatusMarkdown(allTicketItems, report)
	report.Duration = time.Since(start)

	return rendered, report, nil
}

// ReplicateStatusFromPlane recreates DEV_STATUS.md directly from Plane service state.
func ReplicateStatusFromPlane(ctx context.Context, client *Client) (string, *StatusReplicationReport, error) {
	return PullStatusFromPlane(ctx, client)
}
