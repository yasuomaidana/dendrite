package plane

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
)

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
	sb.WriteString(fmt.Sprintf("- **Summary**: Backlog: %d | To Do: %d | In Progress: %d | To Verify: %d | Done: %d (Total: %d)\n",
		report.BacklogTickets, report.TodoTickets, report.InProgressTickets, report.ToVerifyTickets, report.DoneTickets, report.TotalTickets))
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

	itemByID := make(map[string]ticketItem)
	for _, it := range items {
		itemByID[it.id] = it
	}

	if it, ok := itemByID["DEN-036"]; ok {
		statusCol := "**CURRENT FOCUS [ ]**"
		if it.status == tracker.StatusDone {
			statusCol = "**COMPLETED [x]**"
		}
		sb.WriteString(fmt.Sprintf("|      **6**      | **DEN-036** | Milestone 17 | FEAT-021   |     8 SP     |     7.5 / 10     | `pkg/perf`, `cmd/dendrite`, `tests/perf`                     | %s |\n", statusCol))
	}
	if it, ok := itemByID["DEN-037"]; ok {
		statusCol := "**QUEUED [ ]**"
		if it.status == tracker.StatusDone {
			statusCol = "**COMPLETED [x]**"
		}
		sb.WriteString(fmt.Sprintf("|      **8**      | **DEN-037** | Milestone 18 | FEAT-022   |    13 SP     |     8.5 / 10     | `pkg/agentsearch`, `cmd/dendrite`                            |    %s     |\n", statusCol))
	}
	if it, ok := itemByID["DEN-038"]; ok {
		statusCol := "**QUEUED [ ]**"
		if it.status == tracker.StatusDone {
			statusCol = "**COMPLETED [x]**"
		}
		sb.WriteString(fmt.Sprintf("|      **9**      | **DEN-038** | Milestone 19 | FEAT-023   |    13 SP     |     9.0 / 10     | `pkg/testgen`, `pkg/uitest`, `cmd/dendrite`                  |    %s     |\n", statusCol))
	}
	if it, ok := itemByID["DEN-039"]; ok {
		statusCol := "**QUEUED [ ]**"
		if it.status == tracker.StatusDone {
			statusCol = "**COMPLETED [x]**"
		}
		sb.WriteString(fmt.Sprintf("|      **7**      | **DEN-039** | Milestone 20 | FEAT-024   |     8 SP     |     8.0 / 10     | `pkg/rea`, `pkg/ai/import`, `tests/showcase`, `cmd/dendrite` |    %s     |\n", statusCol))
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("## Milestone Execution Pipeline\n\n")

	mainItems := make([]ticketItem, 0)
	otherProjectItems := make(map[string][]ticketItem)

	if report != nil && len(report.OtherProjects) > 0 {
		for _, op := range report.OtherProjects {
			otherProjectItems[op] = make([]ticketItem, 0)
		}
	}

	for _, it := range items {
		proj := strings.ToUpper(strings.TrimSpace(it.project))
		if proj == "" || proj == "DEN" {
			mainItems = append(mainItems, it)
		} else {
			otherProjectItems[proj] = append(otherProjectItems[proj], it)
		}
	}

	milestoneMap := make(map[string][]ticketItem)
	for _, it := range mainItems {
		ms := strings.TrimSpace(it.milestone)
		if ms == "" {
			ms = "General"
		}
		milestoneMap[ms] = append(milestoneMap[ms], it)
	}

	var milestoneKeys []string
	for k := range milestoneMap {
		milestoneKeys = append(milestoneKeys, k)
	}

	sort.Slice(milestoneKeys, func(i, j int) bool {
		numI := extractMilestoneNumber(milestoneKeys[i])
		numJ := extractMilestoneNumber(milestoneKeys[j])
		if numI != numJ {
			return numI < numJ
		}
		return milestoneKeys[i] < milestoneKeys[j]
	})

	for _, mKey := range milestoneKeys {
		mTickets := milestoneMap[mKey]
		sort.Slice(mTickets, func(i, j int) bool {
			if mTickets[i].num != mTickets[j].num {
				return mTickets[i].num < mTickets[j].num
			}
			return mTickets[i].id < mTickets[j].id
		})

		sb.WriteString(fmt.Sprintf("### %s\n\n", mKey))
		writeDevStatusTickets(&sb, mTickets)
		sb.WriteString("\n")
	}

	if len(otherProjectItems) > 0 {
		sb.WriteString("---\n\n## Other Plane Projects\n\n")

		var otherProjKeys []string
		for k := range otherProjectItems {
			otherProjKeys = append(otherProjKeys, k)
		}
		sort.Strings(otherProjKeys)

		for _, pKey := range otherProjKeys {
			pTickets := otherProjectItems[pKey]
			sort.Slice(pTickets, func(i, j int) bool {
				if pTickets[i].num != pTickets[j].num {
					return pTickets[i].num < pTickets[j].num
				}
				return pTickets[i].id < pTickets[j].id
			})

			sb.WriteString(fmt.Sprintf("### %s\n\n", pKey))
			if len(pTickets) == 0 {
				sb.WriteString("*(No active tickets)*\n\n")
			} else {
				writeDevStatusTickets(&sb, pTickets)
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

func writeDevStatusTickets(sb *strings.Builder, tickets []ticketItem) {
	for _, it := range tickets {
		check := " "
		if it.status == tracker.StatusDone {
			check = "x"
		}
		suffix := ""
		if it.targetPkg != "" {
			suffix = fmt.Sprintf(" (`%s`)", it.targetPkg)
		}
		prio := it.priority
		if prio == "" {
			prio = "Medium"
		}
		sb.WriteString(fmt.Sprintf("- [%s] **%s**: %s%s [State: %s, Priority: %s]\n",
			check, it.id, it.title, suffix, formatStatusName(it.status), prio))
	}
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
	if sb.Len() == len("<div>") {
		sb.WriteString(fmt.Sprintf("<p>Context and implementation requirements for %s: %s</p>", t.ID, t.Title))
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
				switch t.Status {
				case tracker.StatusDone:
					stBadge = "[DONE]"
				case tracker.StatusInProgress:
					stBadge = "[IN PROGRESS]"
				case tracker.StatusInReview:
					stBadge = "[TO VERIFY]"
				case tracker.StatusBacklog:
					stBadge = "[BACKLOG]"
				default:
					stBadge = "[TODO]"
				}
				sb.WriteString(fmt.Sprintf("<li><strong>%s</strong> %s: %s</li>", stBadge, t.ID, t.Title))
			}
		}
		sb.WriteString("</ul>")
	}
	return sb.String()
}

func extractTargetPackage(content string) string {
	return extractRegexField(content, `(?:Target Package|package):\s*`+"`?"+`([^`+"`"+`\n<]+)`)
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
