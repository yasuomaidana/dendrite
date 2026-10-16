package dag

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
)

// Graph represents a directed acyclic graph of tickets and dependencies.
type Graph struct {
	nodes          map[string]*Node
	edges          map[string][]string // blockingID -> []blockedID (u blocks v)
	revEdges       map[string][]string // blockedID -> []blockingID (v blocked by u)
	milestoneOrder []string
}

// NewGraph creates an empty dependency graph.
func NewGraph() *Graph {
	return &Graph{
		nodes:          make(map[string]*Node),
		edges:          make(map[string][]string),
		revEdges:       make(map[string][]string),
		milestoneOrder: make([]string, 0),
	}
}

// AddNode registers a ticket node into the graph.
func (g *Graph) AddNode(n *Node) {
	if n == nil || n.ID == "" {
		return
	}
	if n.Priority == "" {
		n.Priority = PriorityMedium
	}
	if n.BlockedBy == nil {
		n.BlockedBy = make([]string, 0)
	}
	if n.Blocks == nil {
		n.Blocks = make([]string, 0)
	}

	g.nodes[n.ID] = n

	if n.Milestone != "" {
		found := false
		for _, m := range g.milestoneOrder {
			if m == n.Milestone {
				found = true
				break
			}
		}
		if !found {
			g.milestoneOrder = append(g.milestoneOrder, n.Milestone)
		}
	}
}

// AddEdge registers a dependency: blockingID blocks blockedID.
func (g *Graph) AddEdge(blockingID, blockedID string) error {
	if blockingID == blockedID {
		return fmt.Errorf("self-dependency cycle detected on %s", blockingID)
	}
	blocking, ok := g.nodes[blockingID]
	if !ok {
		return fmt.Errorf("blocking node %s not found in graph", blockingID)
	}
	blocked, ok := g.nodes[blockedID]
	if !ok {
		return fmt.Errorf("blocked node %s not found in graph", blockedID)
	}

	// Avoid duplicates in edges
	for _, id := range g.edges[blockingID] {
		if id == blockedID {
			return nil
		}
	}

	g.edges[blockingID] = append(g.edges[blockingID], blockedID)
	g.revEdges[blockedID] = append(g.revEdges[blockedID], blockingID)

	// Update node lists
	blocking.Blocks = append(blocking.Blocks, blockedID)
	blocked.BlockedBy = append(blocked.BlockedBy, blockingID)

	return nil
}

// GetNode retrieves a node by ID.
func (g *Graph) GetNode(id string) *Node {
	return g.nodes[id]
}

// Nodes returns all nodes in the graph.
func (g *Graph) Nodes() map[string]*Node {
	return g.nodes
}

// DetectCycles returns a circular path if a dependency cycle is found.
func (g *Graph) DetectCycles() ([]string, error) {
	visited := make(map[string]int) // 0: unvisited, 1: visiting, 2: visited
	var cycle []string

	var dfs func(u string, path []string) bool
	dfs = func(u string, path []string) bool {
		visited[u] = 1
		currentPath := append(path, u)

		for _, v := range g.edges[u] {
			if visited[v] == 1 {
				// Cycle detected
				cycleStart := -1
				for i, node := range currentPath {
					if node == v {
						cycleStart = i
						break
					}
				}
				if cycleStart != -1 {
					cycle = append(currentPath[cycleStart:], v)
				} else {
					cycle = append(currentPath, v)
				}
				return true
			}
			if visited[v] == 0 {
				if dfs(v, currentPath) {
					return true
				}
			}
		}

		visited[u] = 2
		return false
	}

	for id := range g.nodes {
		if visited[id] == 0 {
			if dfs(id, nil) {
				return cycle, fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
			}
		}
	}

	return nil, nil
}

// TopologicalSort returns nodes in topological dependency order (Kahn's algorithm).
func (g *Graph) TopologicalSort() ([]string, error) {
	if cycle, err := g.DetectCycles(); err != nil {
		return nil, fmt.Errorf("%w: cycle path: %v", err, cycle)
	}

	inDegree := make(map[string]int)
	for id := range g.nodes {
		inDegree[id] = 0
	}
	for _, dependents := range g.edges {
		for _, v := range dependents {
			inDegree[v]++
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	// Deterministic ordering for items with same degree
	sort.Strings(queue)

	var sorted []string
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		sorted = append(sorted, u)

		for _, v := range g.edges[u] {
			inDegree[v]--
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
		sort.Strings(queue)
	}

	return sorted, nil
}

// BasePriorityScore maps priority levels to numerical weights.
func BasePriorityScore(priority string) float64 {
	switch strings.ToLower(priority) {
	case PriorityUrgent:
		return 1000.0
	case PriorityHigh:
		return 500.0
	case PriorityMedium:
		return 200.0
	case PriorityLow:
		return 50.0
	case PriorityNone:
		fallthrough
	default:
		return 10.0
	}
}

// CalculateUnblockWeights calculates downstream critical-path unblock impact.
func (g *Graph) CalculateUnblockWeights() {
	for _, node := range g.nodes {
		node.PriorityScore = BasePriorityScore(node.Priority)

		// BFS to find all reachable descendants with distance decay
		type queueItem struct {
			id    string
			depth int
		}
		queue := []queueItem{{id: node.ID, depth: 0}}
		visited := map[string]bool{node.ID: true}
		var downstreamWeight float64

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]

			for _, nextID := range g.edges[curr.id] {
				if !visited[nextID] {
					visited[nextID] = true
					nextDepth := curr.depth + 1
					nextWeight := BasePriorityScore(g.nodes[nextID].Priority) / float64(nextDepth)
					downstreamWeight += nextWeight
					queue = append(queue, queueItem{id: nextID, depth: nextDepth})
				}
			}
		}

		node.UnblockWeight = downstreamWeight
		node.TotalScore = node.PriorityScore + node.UnblockWeight
	}
}

// EstimateDays calculates days for a ticket based on story points or complexity.
func EstimateDays(estimate int, complexity float64) int {
	if estimate > 0 {
		switch {
		case estimate <= 2:
			return 2
		case estimate <= 5:
			return 4
		case estimate <= 8:
			return 7
		case estimate <= 13:
			return 12
		default:
			return 15
		}
	}
	if complexity > 0 {
		switch {
		case complexity <= 3.0:
			return 2
		case complexity <= 6.0:
			return 4
		case complexity <= 8.0:
			return 7
		default:
			return 12
		}
	}
	return 3
}

// GenerateGanttPlan calculates start and target dates for all tickets and milestones.
func (g *Graph) GenerateGanttPlan(anchorDate time.Time) (*GanttPlan, error) {
	sortedOrder, err := g.TopologicalSort()
	if err != nil {
		return nil, err
	}

	plan := &GanttPlan{
		AnchorDate: anchorDate,
		Milestones: make(map[string]*MilestoneSchedule),
		Tickets:    g.nodes,
		Order:      sortedOrder,
	}

	ticketTargetTimes := make(map[string]time.Time)

	for _, id := range sortedOrder {
		node := g.nodes[id]

		// Earliest start date is anchorDate or 1 day after the latest blocker completes
		earliestStart := anchorDate
		for _, blockerID := range node.BlockedBy {
			if target, ok := ticketTargetTimes[blockerID]; ok {
				nextDay := target.AddDate(0, 0, 1)
				if nextDay.After(earliestStart) {
					earliestStart = nextDay
				}
			}
		}

		days := EstimateDays(node.EstimatePoints, node.Complexity)
		targetDate := earliestStart.AddDate(0, 0, days)

		node.StartDate = earliestStart.Format("2006-01-02")
		node.TargetDate = targetDate.Format("2006-01-02")
		ticketTargetTimes[id] = targetDate

		// Update milestone schedule
		if node.Milestone != "" {
			ms, ok := plan.Milestones[node.Milestone]
			if !ok {
				ms = &MilestoneSchedule{
					Name:       node.Milestone,
					StartDate:  node.StartDate,
					TargetDate: node.TargetDate,
					Tickets:    []string{id},
				}
				plan.Milestones[node.Milestone] = ms
			} else {
				ms.Tickets = append(ms.Tickets, id)
				if node.StartDate < ms.StartDate {
					ms.StartDate = node.StartDate
				}
				if node.TargetDate > ms.TargetDate {
					ms.TargetDate = node.TargetDate
				}
			}
			ms.TicketCount++
			if node.Status == tracker.StatusDone {
				ms.CompletedCount++
			}
		}
	}

	return plan, nil
}

// SmartSelect evaluates priority and blocker relationships to pick the next ticket.
func (g *Graph) SmartSelect(preferredTicketID string) (*SelectionResult, error) {
	g.CalculateUnblockWeights()

	// 1. Explicit ticket override
	if preferredTicketID != "" {
		node, ok := g.nodes[preferredTicketID]
		if !ok {
			// Try fuzzy match on numeric ID e.g. "36" -> "DEN-036"
			for id, n := range g.nodes {
				if strings.EqualFold(id, preferredTicketID) || strings.HasSuffix(id, preferredTicketID) {
					node = n
					ok = true
					break
				}
			}
		}
		if !ok {
			return nil, fmt.Errorf("requested ticket '%s' not found in graph", preferredTicketID)
		}

		var blockingUnfinished []string
		for _, bID := range node.BlockedBy {
			if bNode, exists := g.nodes[bID]; exists {
				if bNode.Status != tracker.StatusDone && bNode.Status != tracker.StatusInReview {
					blockingUnfinished = append(blockingUnfinished, bID)
				}
			} else {
				blockingUnfinished = append(blockingUnfinished, bID)
			}
		}

		isBlocked := len(blockingUnfinished) > 0
		reason := fmt.Sprintf("Explicitly requested ticket '%s' selected", node.ID)
		if isBlocked {
			reason = fmt.Sprintf("Explicitly requested ticket '%s' selected, but WARNING: blocked by unfinished dependencies: %s",
				node.ID, strings.Join(blockingUnfinished, ", "))
		}

		return &SelectionResult{
			SelectedTicket:      node,
			IsActive:            node.Status == tracker.StatusInProgress,
			IsBlocked:           isBlocked,
			BlockingTickets:     blockingUnfinished,
			UnblockedDownstream: node.Blocks,
			TotalScore:          node.TotalScore,
			Reason:              reason,
			Milestone:           node.Milestone,
			GanttStartDate:      node.StartDate,
			GanttTargetDate:     node.TargetDate,
		}, nil
	}

	// 2. Check for an already active IN_PROGRESS ticket
	var inProgressTickets []*Node
	for _, node := range g.nodes {
		if node.Status == tracker.StatusInProgress {
			inProgressTickets = append(inProgressTickets, node)
		}
	}
	if len(inProgressTickets) > 0 {
		// Pick highest priority in-progress ticket
		sort.Slice(inProgressTickets, func(i, j int) bool {
			return inProgressTickets[i].TotalScore > inProgressTickets[j].TotalScore
		})
		active := inProgressTickets[0]
		return &SelectionResult{
			SelectedTicket:      active,
			IsActive:            true,
			IsBlocked:           false,
			UnblockedDownstream: active.Blocks,
			TotalScore:          active.TotalScore,
			Reason:              fmt.Sprintf("Active ticket '%s' is already IN_PROGRESS", active.ID),
			Milestone:           active.Milestone,
			GanttStartDate:      active.StartDate,
			GanttTargetDate:     active.TargetDate,
		}, nil
	}

	// 3. Find candidates: Unfinished tickets (TODO/BACKLOG) that are NOT blocked by any unfinished tickets
	var actionableCandidates []*Node
	var blockedCandidates []*Node

	for _, node := range g.nodes {
		if node.Status == tracker.StatusDone || node.Status == tracker.StatusInReview {
			continue
		}

		isBlocked := false
		for _, bID := range node.BlockedBy {
			if bNode, exists := g.nodes[bID]; exists {
				if bNode.Status != tracker.StatusDone && bNode.Status != tracker.StatusInReview {
					isBlocked = true
					break
				}
			} else {
				isBlocked = true
				break
			}
		}

		if !isBlocked {
			actionableCandidates = append(actionableCandidates, node)
		} else {
			blockedCandidates = append(blockedCandidates, node)
		}
	}

	if len(actionableCandidates) == 0 {
		if len(blockedCandidates) == 0 {
			return nil, fmt.Errorf("no pending tickets found in graph; all tickets are completed")
		}
		// All candidates are blocked: choose the blocked candidate with fewest unresolved blockers
		sort.Slice(blockedCandidates, func(i, j int) bool {
			return len(blockedCandidates[i].BlockedBy) < len(blockedCandidates[j].BlockedBy)
		})
		stuck := blockedCandidates[0]
		return &SelectionResult{
			SelectedTicket:      stuck,
			IsActive:            false,
			IsBlocked:           true,
			BlockingTickets:     stuck.BlockedBy,
			UnblockedDownstream: stuck.Blocks,
			TotalScore:          stuck.TotalScore,
			Reason:              fmt.Sprintf("All remaining tickets are blocked. Next in queue '%s' is blocked by: %s", stuck.ID, strings.Join(stuck.BlockedBy, ", ")),
			Milestone:           stuck.Milestone,
			GanttStartDate:      stuck.StartDate,
			GanttTargetDate:     stuck.TargetDate,
		}, nil
	}

	// Sort actionable candidates by:
	// a. TotalScore descending (Priority + Critical Path Unblock Weight)
	// b. SequenceID / Ticket ID ascending (tie-breaker)
	sort.Slice(actionableCandidates, func(i, j int) bool {
		if actionableCandidates[i].TotalScore != actionableCandidates[j].TotalScore {
			return actionableCandidates[i].TotalScore > actionableCandidates[j].TotalScore
		}
		return actionableCandidates[i].SequenceID < actionableCandidates[j].SequenceID
	})

	optimal := actionableCandidates[0]
	return &SelectionResult{
		SelectedTicket:      optimal,
		IsActive:            false,
		IsBlocked:           false,
		UnblockedDownstream: optimal.Blocks,
		TotalScore:          optimal.TotalScore,
		Reason: fmt.Sprintf("Selected highest priority unblocked ticket '%s' (Priority: %s, Score: %.1f, Unblocks: %d downstream tickets)",
			optimal.ID, strings.ToUpper(optimal.Priority), optimal.TotalScore, len(optimal.Blocks)),
		Milestone:       optimal.Milestone,
		GanttStartDate:  optimal.StartDate,
		GanttTargetDate: optimal.TargetDate,
	}, nil
}
