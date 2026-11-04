package mcpserver

import (
	"Dendrite/pkg/planesvc"
	"cmp"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
	"Dendrite/pkg/tracker/plane"
)

// planeSmartSelectHandler answers dendrite_ticket_smart_select and its Plane alias.
func planeSmartSelectHandler(deps Dependencies) func(context.Context, map[string]interface{}) (*CallToolResult, error) {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		ticketID, _ := args["ticket_id"].(string)
		milestoneFilter, _ := args["milestone"].(string)
		sourceFile := stringArgOr(args, "file", deps.DevStatus)

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
		return textResult(string(indentJSON(output))), nil
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
