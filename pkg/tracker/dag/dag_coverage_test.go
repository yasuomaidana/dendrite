package dag

import (
	"testing"
	"time"

	"Dendrite/pkg/tracker"
)

func newCyclicGraph() *Graph {
	g := NewGraph()
	g.AddNode(&Node{ID: "A"})
	g.AddNode(&Node{ID: "B"})
	_ = g.AddEdge("A", "B")
	_ = g.AddEdge("B", "A")
	return g
}

func TestGraph_AdditionalCoverage(t *testing.T) {
	t.Run("AddNode_EmptyID", func(t *testing.T) {
		g := NewGraph()
		g.AddNode(&Node{ID: "", Title: "No ID"})
		if len(g.Nodes()) != 0 {
			t.Fatalf("expected empty graph when adding node with empty ID")
		}
	})

	cycleTests := []struct {
		name string
		op   func(*Graph) error
	}{
		{"TopologicalSort_CycleError", func(g *Graph) error {
			_, err := g.TopologicalSort()
			return err
		}},
		{"GenerateGanttPlan_CycleError", func(g *Graph) error {
			_, err := g.GenerateGanttPlan(time.Now())
			return err
		}},
	}
	for _, tc := range cycleTests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.op(newCyclicGraph()); err == nil {
				t.Fatalf("expected cycle error from %s", tc.name)
			}
		})
	}

	t.Run("GenerateGanttPlan_MilestoneStartDateUpdate", func(t *testing.T) {
		g := NewGraph()
		anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		// Kahn queue sorts degree 0 alphabetically: "A", "Z"
		// Processing "A" unblocks "B" (milestone M1). B start date = anchor + days(A).
		// When "B" is processed, milestone M1 is created with start date 2026-01-06.
		// Then "Z" (milestone M1) is popped with start date anchor (2026-01-01).
		// This triggers: node.StartDate < ms.StartDate.
		g.AddNode(&Node{
			ID:             "A",
			Title:          "First",
			EstimatePoints: 5, // 5 days
			Status:         tracker.StatusDone,
		})
		g.AddNode(&Node{
			ID:             "B",
			Title:          "Dependent",
			Milestone:      "M1",
			EstimatePoints: 1,
			Status:         tracker.StatusDone,
		})
		g.AddNode(&Node{
			ID:             "Z",
			Title:          "Independent Later Alphabetically",
			Milestone:      "M1",
			EstimatePoints: 1,
			Status:         tracker.StatusTodo,
		})

		_ = g.AddEdge("A", "B")

		plan, err := g.GenerateGanttPlan(anchor)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ms := plan.Milestones["M1"]
		if ms == nil {
			t.Fatalf("expected milestone M1")
		}
		if ms.StartDate != "2026-01-01" {
			t.Errorf("expected M1 start date to be updated to 2026-01-01, got %s", ms.StartDate)
		}
	})

	t.Run("CalculateUnblockWeights_BFS_Decay", func(t *testing.T) {
		g := NewGraph()
		g.AddNode(&Node{ID: "N1", Priority: PriorityLow})
		g.AddNode(&Node{ID: "N2", Priority: PriorityMedium})
		g.AddNode(&Node{ID: "N3", Priority: PriorityHigh})
		_ = g.AddEdge("N1", "N2")
		_ = g.AddEdge("N2", "N3")

		g.CalculateUnblockWeights()
		n1 := g.GetNode("N1")
		// N2 is depth 1: 200/1 = 200. N3 is depth 2: 500/2 = 250.
		// downstream = 450. PriorityScore = 50. TotalScore = 500.
		if n1.UnblockWeight != 450.0 {
			t.Errorf("expected unblock weight 450.0, got %f", n1.UnblockWeight)
		}
	})
}

func TestGraph_SmartSelect_ExtendedCoverage(t *testing.T) {
	cases := []struct {
		name      string
		nodes     []*Node
		preferred string
		assert    func(t *testing.T, res *SelectionResult, err error)
	}{
		{
			name: "PreferredTicket_ActiveAndExternalBlocker",
			nodes: []*Node{
				{ID: "ACTIVE", Status: tracker.StatusInProgress, BlockedBy: []string{"EXTERNAL_TICKET"}},
			},
			preferred: "ACTIVE",
			assert: func(t *testing.T, res *SelectionResult, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !res.IsActive {
					t.Errorf("expected IsActive to be true")
				}
				if !res.IsBlocked {
					t.Errorf("expected IsBlocked to be true due to external blocker")
				}
				if len(res.BlockingTickets) != 1 || res.BlockingTickets[0] != "EXTERNAL_TICKET" {
					t.Errorf("unexpected blocking tickets: %v", res.BlockingTickets)
				}
			},
		},
		{
			name: "PreferredTicket_ResolvedBlockers",
			nodes: []*Node{
				{ID: "REV", Status: tracker.StatusInReview},
				{ID: "CANC", Status: tracker.StatusCancelled},
				{ID: "CHILD", Status: tracker.StatusTodo, BlockedBy: []string{"REV", "CANC"}},
			},
			preferred: "CHILD",
			assert: func(t *testing.T, res *SelectionResult, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.IsBlocked {
					t.Errorf("expected child not to be blocked by review/cancelled tickets")
				}
			},
		},
		{
			name: "InProgressTicket_AutoSelect",
			nodes: []*Node{
				{ID: "IP1", Status: tracker.StatusInProgress, Priority: PriorityLow},
				{ID: "IP2", Status: tracker.StatusInProgress, Priority: PriorityUrgent},
				{ID: "TODO1", Status: tracker.StatusTodo, Priority: PriorityUrgent},
			},
			assert: func(t *testing.T, res *SelectionResult, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !res.IsActive || res.SelectedTicket.ID != "IP2" {
					t.Errorf("expected highest priority in-progress ticket IP2, got %v", res.SelectedTicket.ID)
				}
			},
		},
		{
			name: "CandidateFilter_And_TieBreaker",
			nodes: []*Node{
				{ID: "D1", Status: tracker.StatusDone},
				{ID: "IR1", Status: tracker.StatusInReview},
				{ID: "BL1", Status: tracker.StatusBlocked},
				{ID: "CA1", Status: tracker.StatusCancelled},
				{ID: "ACT1", SequenceID: 10, Status: tracker.StatusTodo, Priority: PriorityUrgent, BlockedBy: []string{"IR1", "CA1"}},
				{ID: "ACT2", SequenceID: 5, Status: tracker.StatusTodo, Priority: PriorityUrgent},
			},
			assert: func(t *testing.T, res *SelectionResult, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.SelectedTicket.ID != "ACT2" {
					t.Errorf("expected tie-breaker to select ACT2, got %s", res.SelectedTicket.ID)
				}
			},
		},
		{
			name: "AllBlockedCandidates_SortedByBlockerCount",
			nodes: []*Node{
				{ID: "BLOCKED_HEAVY", Status: tracker.StatusTodo, BlockedBy: []string{"X1", "X2", "X3"}},
				{ID: "BLOCKED_LIGHT", Status: tracker.StatusTodo, BlockedBy: []string{"X1"}},
			},
			assert: func(t *testing.T, res *SelectionResult, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.SelectedTicket.ID != "BLOCKED_LIGHT" {
					t.Errorf("expected BLOCKED_LIGHT with fewest blockers, got %s", res.SelectedTicket.ID)
				}
				if !res.IsBlocked {
					t.Errorf("expected IsBlocked to be true")
				}
			},
		},
		{
			name: "AllCompleted_Error",
			nodes: []*Node{
				{ID: "D1", Status: tracker.StatusDone},
				{ID: "D2", Status: tracker.StatusInReview},
				{ID: "D3", Status: tracker.StatusCancelled},
			},
			assert: func(t *testing.T, res *SelectionResult, err error) {
				if err == nil {
					t.Fatalf("expected error when all tickets are completed/unselectable")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGraph()
			for _, n := range tc.nodes {
				g.AddNode(n)
			}
			res, err := g.SmartSelect(tc.preferred)
			tc.assert(t, res, err)
		})
	}
}
