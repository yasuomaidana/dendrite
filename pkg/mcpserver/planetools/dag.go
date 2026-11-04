package planetools

import (
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
	"Dendrite/pkg/tracker/plane"
	"cmp"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	. "Dendrite/pkg/mcpserver"
)

var ticketNumRegex = regexp.MustCompile(`\[?(DEN-\d+)\]?`)

func registerPlaneDAGTools(s *Server, deps Dependencies) {
	handleSmartSelect := planeSmartSelectHandler(deps)

	// 1. dendrite_ticket_smart_select
	s.RegisterTool(Tool{
		Name:        "dendrite_ticket_smart_select",
		Description: "Smart selects the optimal unblocked ticket to be done based on priority (Urgent > High > Medium > Low), dependency relationships (DAG unblocked check), and downstream unblock impact (critical path weight). If ticket_id is specified by user, evaluates its blockers and selects it.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional specific ticket ID requested by user (e.g. 'DEN-036')",
				},
				"milestone": map[string]interface{}{
					"type":        "string",
					"description": "Optional milestone name or filter",
				},
				"file": map[string]interface{}{
					"type":        "string",
					"description": "Optional fallback source markdown file (default: DEV_STATUS.md)",
				},
			},
		},
	}, handleSmartSelect)

	// 2. dendrite_plane_ticket_smart_select (alias)
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_ticket_smart_select",
		Description: "Smart selects the optimal unblocked ticket from Plane v2 project management container respecting priority and blocker relationships.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional specific ticket ID requested by user (e.g. 'DEN-036')",
				},
				"milestone": map[string]interface{}{
					"type":        "string",
					"description": "Optional milestone filter",
				},
			},
		},
	}, handleSmartSelect)

	// 3. dendrite_plane_relation_link
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_relation_link",
		Description: "Create a dependency relationship between tickets in Plane (relation_type: 'blocking', 'blocked_by', 'relates_to', 'duplicate') for Gantt diagrams and DAG scheduling",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"issue_id": map[string]interface{}{
					"type":        "string",
					"description": "Source ticket ID or UUID (e.g. 'DEN-036' or issue UUID)",
				},
				"relation_type": map[string]interface{}{
					"type":        "string",
					"description": "Relation type: 'blocking', 'blocked_by', 'relates_to', or 'duplicate'",
				},
				"target_issue_id": map[string]interface{}{
					"type":        "string",
					"description": "Target ticket ID or UUID that is related/blocked",
				},
			},
			"required": []interface{}{"issue_id", "relation_type", "target_issue_id"},
		},
	}, planeRelationLinkHandler(deps))

	// 4. dendrite_plane_issue_create
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_issue_create",
		Description: "Create a new ticket in Plane, assign it to a milestone/module, set priority, schedule start and target dates for the Gantt timeline, and establish blocker relationships to place it correctly in the development plan",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Ticket title / name (e.g. '[DEN-101] Feature Name')",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Mandatory ticket description (HTML or markdown detailing goals, acceptance criteria, and DoD)",
				},
				"context": map[string]interface{}{
					"type":        "string",
					"description": "Alias for description",
				},
				"milestone": map[string]interface{}{
					"type":        "string",
					"description": "Milestone / Module name or UUID to assign this ticket to",
				},
				"priority": map[string]interface{}{
					"type":        "string",
					"description": "Priority: 'urgent', 'high', 'medium', 'low', or 'none' (default: 'medium')",
				},
				"estimate": map[string]interface{}{
					"type":        "integer",
					"description": "Story points estimate (e.g. 3, 5, 8, 13)",
				},
				"start_date": map[string]interface{}{
					"type":        "string",
					"description": "Optional start date in YYYY-MM-DD format (auto-calculated if omitted)",
				},
				"target_date": map[string]interface{}{
					"type":        "string",
					"description": "Optional target date in YYYY-MM-DD format (auto-calculated if omitted)",
				},
				"blocked_by": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of ticket IDs or UUIDs that block this ticket",
				},
			},
			"required": []interface{}{"name", "description"},
		},
	}, planeIssueCreateHandler(deps))
}

// resolveIssueUUID turns a ticket reference (DEN-036, a sequence number or a UUID) into the issue's UUID.
func resolveIssueUUID(ctx context.Context, client *plane.Client, raw string) (string, error) {
	if len(raw) == 36 && strings.Count(raw, "-") == 4 {
		return raw, nil
	}
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return "", err
	}
	if iss := plane.ResolveIssue(issues, raw); iss != nil {
		return iss.ID, nil
	}
	return "", fmt.Errorf("ticket '%s' not found in Plane", raw)
}

// planeRelationLinkHandler handles the dendrite_plane_relation_link tool.
func planeRelationLinkHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		issueID, _ := args["issue_id"].(string)
		relationType, _ := args["relation_type"].(string)
		targetIssueID, _ := args["target_issue_id"].(string)

		if issueID == "" || relationType == "" || targetIssueID == "" {
			return nil, fmt.Errorf("issue_id, relation_type, and target_issue_id are all required")
		}

		client, err := PlaneClientOf(deps)
		if err != nil {
			return nil, err
		}

		sourceUUID, err := resolveIssueUUID(ctx, client, issueID)
		if err != nil {
			return nil, err
		}
		targetUUID, err := resolveIssueUUID(ctx, client, targetIssueID)
		if err != nil {
			return nil, err
		}

		if err := client.AddIssueRelation(ctx, sourceUUID, relationType, []string{targetUUID}); err != nil {
			return nil, fmt.Errorf("failed to create relation in Plane: %w", err)
		}

		res := map[string]interface{}{
			"status":          "success",
			"relation_type":   relationType,
			"source_ticket":   issueID,
			"source_uuid":     sourceUUID,
			"target_ticket":   targetIssueID,
			"target_uuid":     targetUUID,
			"gantt_connected": true,
		}

		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}

// planeIssueCreateHandler handles the dendrite_plane_issue_create tool.
func planeIssueCreateHandler(deps Dependencies) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		name, _ := args["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("missing required 'name' parameter")
		}
		desc := StringArgOr(args, "description", StringArgOr(args, "context", ""))
		if plane.IsEmptyContext(desc) {
			return nil, fmt.Errorf("ticket context/description cannot be empty: acceptance criteria and architectural context are mandatory")
		}
		client, err := PlaneClientOf(deps)
		if err != nil {
			return nil, err
		}
		var estimate *int
		if estFloat, ok := args["estimate"].(float64); ok {
			e := int(estFloat)
			estimate = &e
		}
		sDate, tDate := issueDates(args, estimate)
		mName, _ := args["milestone"].(string)
		moduleID := resolveModuleID(ctx, client, mName, sDate, tDate)
		blockerUUIDs := resolveBlockers(ctx, client, args["blocked_by"])
		prio := StringArgOr(args, "priority", dag.PriorityMedium)

		created, _, err := planesvc.Default().CreateIssue(ctx, client, plane.CreateIssueRequest{
			Name:            name,
			DescriptionHTML: fmt.Sprintf("<div><p>%s</p></div>", desc),
			Priority:        prio,
			EstimatePoint:   estimate,
			StartDate:       &sDate,
			TargetDate:      &tDate,
			ModuleID:        moduleID,
			BlockedBy:       blockerUUIDs,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create issue in Plane: %w", err)
		}
		return TextResult(string(IndentJSON(map[string]interface{}{
			"status":            "success",
			"issue_id":          created.ID,
			"sequence_id":       created.SequenceID,
			"name":              created.Name,
			"priority":          prio,
			"start_date":        sDate,
			"target_date":       tDate,
			"module_id":         moduleID,
			"blockers_count":    len(blockerUUIDs),
			"gantt_plan_placed": true,
		}))), nil
	}
}

// issueDates are the start and target dates of a new ticket: the ones given, otherwise today and today plus the
// estimate's duration (four days without an estimate).
func issueDates(args map[string]interface{}, estimate *int) (start, target string) {
	start, _ = args["start_date"].(string)
	target, _ = args["target_date"].(string)
	if start != "" && target != "" {
		return start, target
	}
	now := time.Now().Truncate(24 * time.Hour)
	days := 4
	if estimate != nil {
		days = dag.EstimateDays(*estimate, 0)
	}
	if start == "" {
		start = now.Format("2006-01-02")
	}
	if target == "" {
		target = now.AddDate(0, 0, days).Format("2006-01-02")
	}
	return start, target
}

// resolveModuleID finds the module (milestone) whose name contains mName, or creates it; "" for no milestone.
func resolveModuleID(ctx context.Context, client *plane.Client, mName, sDate, tDate string) string {
	if mName == "" {
		return ""
	}
	if modules, err := client.ListModules(ctx); err == nil {
		for _, mod := range modules {
			if strings.Contains(strings.ToLower(mod.Name), strings.ToLower(mName)) || mod.ID == mName {
				return mod.ID
			}
		}
	}
	newMod, err := client.CreateModuleWithDates(ctx, mName, "in-progress", fmt.Sprintf("Milestone tracking for %s", mName), &sDate, &tDate)
	if err != nil || newMod == nil {
		return ""
	}
	return newMod.ID
}

// resolveBlockers turns the blocked_by argument (ticket ids or issue UUIDs) into issue UUIDs.
func resolveBlockers(ctx context.Context, client *plane.Client, raw interface{}) []string {
	rawBlockers, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	existingIssues, _ := client.ListPlaneIssues(ctx)
	var uuids []string
	for _, rb := range rawBlockers {
		bStr, ok := rb.(string)
		switch {
		case !ok || bStr == "":
		case len(bStr) == 36 && strings.Count(bStr, "-") == 4:
			uuids = append(uuids, bStr)
		default:
			for _, iss := range existingIssues {
				if strings.Contains(iss.Name, bStr) {
					uuids = append(uuids, iss.ID)
					break
				}
			}
		}
	}
	return uuids
}

// planeSmartSelectHandler answers dendrite_ticket_smart_select and its Plane alias.
func planeSmartSelectHandler(deps Dependencies) func(context.Context, map[string]interface{}) (*CallToolResult, error) {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		ticketID, _ := args["ticket_id"].(string)
		milestoneFilter, _ := args["milestone"].(string)
		sourceFile := StringArgOr(args, "file", deps.DevStatus)

		graph := dag.NewGraph()
		planeLoaded := loadPlaneGraph(ctx, deps, graph)
		if !planeLoaded { // fall back to the local markdown ledger when Plane is unavailable or empty
			if err := loadLedgerGraph(graph, sourceFile, milestoneFilter); err != nil {
				return nil, err
			}
		}

		// Ensure Gantt plan is generated
		_, _ = graph.GenerateGanttPlan(time.Now().Truncate(24 * time.Hour))

		res, err := graph.SmartSelect(ticketID)
		if err != nil {
			return nil, fmt.Errorf("smart selection failed: %w", err)
		}
		output := map[string]interface{}{
			"status":            "success",
			"authoritative_src": map[string]interface{}{"plane_online": planeLoaded},
			"selection":         res,
		}
		return TextResult(string(IndentJSON(output))), nil
	}
}

// loadPlaneGraph fills graph from the authoritative Plane board and reports whether Plane answered with issues.
func loadPlaneGraph(ctx context.Context, deps Dependencies, graph *dag.Graph) bool {
	client := deps.PlaneClient
	if client == nil {
		client, _ = plane.NewClientFromEnv()
	}
	if client == nil {
		return false
	}
	snap, _, err := planesvc.Default().Snapshot(ctx, client, false)
	if err != nil || len(snap.Issues) == 0 {
		return false
	}
	uuidToID := addPlaneNodes(graph, snap)
	addPlaneEdges(graph, snap, uuidToID)
	return true
}

// ticketIDOf is the DEN-n id an issue goes by: the one in its title, else its sequence number.
func ticketIDOf(iss plane.PlaneIssue) string {
	if m := ticketNumRegex.FindStringSubmatch(iss.Name); len(m) > 1 {
		return m[1]
	}
	return fmt.Sprintf("DEN-%03d", iss.SequenceID)
}

// planeNode is the graph node of an issue in the given status.
func planeNode(iss plane.PlaneIssue, id string, st tracker.Status) *dag.Node {
	prio := iss.Priority
	if prio == "" || prio == "none" {
		prio = dag.PriorityMedium
	}
	est := 5
	if iss.EstimatePoint != nil {
		est = *iss.EstimatePoint
	}
	return &dag.Node{
		ID:             id,
		SequenceID:     iss.SequenceID,
		Title:          iss.Name,
		Status:         st,
		Priority:       prio,
		EstimatePoints: est,
		StartDate:      deref(iss.StartDate),
		TargetDate:     deref(iss.TargetDate),
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// addPlaneNodes adds a node per issue and returns the issue UUID -> ticket id map.
func addPlaneNodes(graph *dag.Graph, snap *planesvc.Snapshot) map[string]string {
	uuidToID := make(map[string]string, len(snap.Issues))
	for _, iss := range snap.Issues {
		id := ticketIDOf(iss)
		uuidToID[iss.ID] = id
		graph.AddNode(planeNode(iss, id, cmp.Or(snap.Statuses[iss.ID], tracker.StatusTodo)))
	}
	return uuidToID
}

// addPlaneEdges adds an edge from each blocker to the issue it blocks.
func addPlaneEdges(graph *dag.Graph, snap *planesvc.Snapshot, uuidToID map[string]string) {
	for _, iss := range snap.Issues {
		currID := uuidToID[iss.ID]
		for _, blocked := range snap.Relations[iss.ID].BlockedBy {
			blockerID := uuidToID[blocked.ID]
			if blockerID == "" {
				if m := ticketNumRegex.FindStringSubmatch(blocked.Name); len(m) > 1 {
					blockerID = m[1]
				}
			}
			if blockerID != "" && blockerID != currID {
				_ = graph.AddEdge(blockerID, currID)
			}
		}
	}
}

// loadLedgerGraph fills graph from the DEV_STATUS markdown ledger, optionally for one milestone, chaining the
// tickets of a milestone in order.
func loadLedgerGraph(graph *dag.Graph, sourceFile, milestoneFilter string) error {
	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("failed to read %s and Plane is not available: %w", sourceFile, err)
	}
	data, err := plane.ParseDevStatus(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse dev status markdown: %w", err)
	}
	for _, t := range data.Tickets {
		if milestoneFilter != "" && !strings.Contains(strings.ToLower(t.Milestone), strings.ToLower(milestoneFilter)) {
			continue
		}
		sp, comp := plane.TicketScore(t)
		prio := plane.PriorityFor(comp, strings.Contains(data.ActiveMilestone, t.Milestone))
		graph.AddNode(&dag.Node{
			ID: t.ID, SequenceID: t.Number, Title: t.Title, Milestone: t.Milestone, Story: t.FeatureID,
			Status: t.Status, Priority: prio, EstimatePoints: sp, Complexity: comp,
		})
	}
	byMilestone := make(map[string][]string)
	for _, t := range data.Tickets {
		byMilestone[t.Milestone] = append(byMilestone[t.Milestone], t.ID)
	}
	for _, ids := range byMilestone {
		for i := 1; i < len(ids); i++ {
			_ = graph.AddEdge(ids[i-1], ids[i])
		}
	}
	return nil
}
