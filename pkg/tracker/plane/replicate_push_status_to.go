package plane

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"Dendrite/pkg/quality"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
)

// PushStatusToPlaneIn is PushStatusToPlane with the zero-mock gate evaluated against repoDir,
// so a peer repository's DEV_STATUS is checked against that peer's packages.
func PushStatusToPlaneIn(ctx context.Context, client *Client, mdContent, repoDir string) (*StatusReplicationReport, error) {
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
		// Zero-Mock Policy Enforcement Gate:
		// Prohibit pushing any ticket as DONE or IN_REVIEW if associated packages contain mocks
		if t.Status == tracker.StatusDone || t.Status == tracker.StatusInReview {
			pkgs := quality.ExtractPackages(t.Title + " " + t.TargetPackage)
			if len(pkgs) > 0 {
				if hasMocks, _, _ := quality.ValidateZeroMockGate(repoDir, pkgs); hasMocks {
					t.Status = tracker.StatusTodo
				}
			}
		}

		switch t.Status {
		case tracker.StatusDone:
			report.CompletedTickets++
			report.DoneTickets++
		case tracker.StatusInReview:
			report.ToVerifyTickets++
		case tracker.StatusInProgress:
			report.InProgressTickets++
		case tracker.StatusBacklog:
			report.BacklogTickets++
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

// ListPlaneIssuesForProject queries all issues for a specific project across all pages.
func (c *Client) ListPlaneIssuesForProject(ctx context.Context, projectID string) ([]PlaneIssue, error) {
	if projectID == "" {
		projectID = c.projectID
	}
	var allIssues []PlaneIssue
	cursor := ""

	for {
		endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/", c.workspaceSlug, projectID)
		endpoint += "?per_page=100"
		if cursor != "" {
			endpoint = fmt.Sprintf("%s&cursor=%s", endpoint, cursor)
		}
		req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			return nil, ErrUnauthorized
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to list plane issues for project %s: status %d", projectID, resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		pageIssues, nextCursor, hasNext := parseIssuePage(bodyBytes)
		allIssues = append(allIssues, pageIssues...)
		if !hasNext {
			break
		}
		cursor = nextCursor
	}

	return dedupeIssues(allIssues), nil
}

// ListStatesForProject queries all states configured for a specific project.
func (c *Client) ListStatesForProject(ctx context.Context, projectID string) ([]PlaneState, error) {
	if projectID == "" {
		projectID = c.projectID
	}
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/states/", c.workspaceSlug, projectID)
	return fetchResourceList[PlaneState](ctx, c, endpoint, fmt.Sprintf("failed to list plane states for project %s", projectID))
}

func resolvePlaneState(group, name string) tracker.Status { return StatusFor(group, name) }

func formatStatusName(st tracker.Status) string {
	return tracker.StatusDisplayName(st)
}
