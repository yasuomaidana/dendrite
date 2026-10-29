package dag

import (
	"testing"
	"time"

	"Dendrite/pkg/tracker"
)

func TestGraph_AddNode_EmptyAndPreinitialized(t *testing.T) {
	g := NewGraph()
	// Node with empty ID should be ignored
	g.AddNode(&Node{ID: ""})
	if len(g.Nodes()) != 0 {
		t.Fatalf("expected 0 nodes after empty ID AddNode, got %d", len(g.Nodes()))
	}

	// Node with pre-initialized BlockedBy and Blocks
	pre := &Node{
		ID:        "DEN-PRE",
		BlockedBy: []string{"DEN-X"},
		Blocks:    []string{"DEN-Y"},
	}
	g.AddNode(pre)
	got := g.GetNode("DEN-PRE")
	if len(got.BlockedBy) != 1 || len(got.Blocks) != 1 {
		t.Errorf("pre-initialized slices were not preserved: %+v", got)
	}
}

func TestGraph_TopologicalSort_And_Gantt_WithCycle(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{ID: "C1"})
	g.AddNode(&Node{ID: "C2"})
	_ = g.AddEdge("C1", "C2")
	_ = g.AddEdge("C2", "C1")

	if _, err := g.TopologicalSort(); err == nil {
		t.Errorf("expected TopologicalSort to fail on cycle")
	}

	if _, err := g.GenerateGanttPlan(time.Now()); err == nil {
		t.Errorf("expected GenerateGanttPlan to fail on cycle")
	}
}

func TestGraph_GenerateGanttPlan_MilestoneStartDateUpdate(t *testing.T) {
	g := NewGraph()
	anchor := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	// T1 is blocked by B, so T1 starts later
	g.AddNode(&Node{ID: "B", Milestone: "M-Other", EstimatePoints: 10})
	g.AddNode(&Node{ID: "T1", Milestone: "M-Shared", EstimatePoints: 2})
	_ = g.AddEdge("B", "T1")

	// T2 has no blocker, so T2 starts at anchor date (earlier than T1)
	g.AddNode(&Node{ID: "T2", Milestone: "M-Shared", EstimatePoints: 2})

	plan, err := g.GenerateGanttPlan(anchor)
	if err != nil {
		t.Fatalf("GenerateGanttPlan failed: %v", err)
	}

	ms, ok := plan.Milestones["M-Shared"]
	if !ok {
		t.Fatalf("expected milestone M-Shared")
	}
	t2 := plan.Tickets["T2"]
	if ms.StartDate != t2.StartDate {
		t.Errorf("expected milestone start date to match earlier T2 start %s, got %s", t2.StartDate, ms.StartDate)
	}
}

func TestGraph_SmartSelect_ExtendedBranches(t *testing.T) {
	// 1. Multiple in-progress tickets sorted by TotalScore
	g1 := NewGraph()
	g1.AddNode(&Node{ID: "IP-1", Status: tracker.StatusInProgress, Priority: PriorityLow})
	g1.AddNode(&Node{ID: "IP-2", Status: tracker.StatusInProgress, Priority: PriorityUrgent})
	res1, err := g1.SmartSelect("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res1.SelectedTicket.ID != "IP-2" {
		t.Errorf("expected highest score in-progress IP-2, got %s", res1.SelectedTicket.ID)
	}

	// 2. Actionable candidates tie-breaking by SequenceID
	g2 := NewGraph()
	g2.AddNode(&Node{ID: "T-B", SequenceID: 20, Status: tracker.StatusTodo, Priority: PriorityHigh})
	g2.AddNode(&Node{ID: "T-A", SequenceID: 10, Status: tracker.StatusTodo, Priority: PriorityHigh})
	res2, err := g2.SmartSelect("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.SelectedTicket.ID != "T-A" {
		t.Errorf("expected tie-breaker to pick lower SequenceID T-A, got %s", res2.SelectedTicket.ID)
	}

	// 3. User override on ticket with blocker missing from g.nodes
	g3 := NewGraph()
	g3.AddNode(&Node{ID: "TARGET", Status: tracker.StatusTodo, BlockedBy: []string{"GHOST"}})
	res3, err := g3.SmartSelect("TARGET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res3.IsBlocked || len(res3.BlockingTickets) != 1 || res3.BlockingTickets[0] != "GHOST" {
		t.Errorf("expected TARGET to be blocked by missing GHOST, got %+v", res3)
	}

	// 4. Candidate blocked by tickets with StatusInReview and StatusCancelled (resolved)
	// and skipped statuses (StatusBlocked and StatusCancelled)
	g4 := NewGraph()
	g4.AddNode(&Node{ID: "REV", Status: tracker.StatusInReview})
	g4.AddNode(&Node{ID: "CAN", Status: tracker.StatusCancelled})
	g4.AddNode(&Node{ID: "BLOCKED-SKIPPED", Status: tracker.StatusBlocked})
	g4.AddNode(&Node{ID: "CANDIDATE", Status: tracker.StatusTodo, BlockedBy: []string{"REV", "CAN"}})
	res4, err := g4.SmartSelect("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res4.SelectedTicket.ID != "CANDIDATE" {
		t.Errorf("expected CANDIDATE to be unblocked by review/cancelled blockers, got %s", res4.SelectedTicket.ID)
	}

	// 5. Multiple blocked candidates sorted by len(BlockedBy)
	g5 := NewGraph()
	g5.AddNode(&Node{ID: "BLOCKER-1", Status: tracker.StatusTodo})
	g5.AddNode(&Node{ID: "BLOCKER-2", Status: tracker.StatusTodo})
	g5.AddNode(&Node{ID: "STUCK-MANY", Status: tracker.StatusTodo, BlockedBy: []string{"BLOCKER-1", "BLOCKER-2"}})
	g5.AddNode(&Node{ID: "STUCK-ONE", Status: tracker.StatusTodo, BlockedBy: []string{"BLOCKER-1"}})
	// Remove blockers from actionable by making them blocked or non-existent in graph
	g5.nodes["BLOCKER-1"].Status = tracker.StatusBlocked
	g5.nodes["BLOCKER-2"].Status = tracker.StatusBlocked
	res5, err := g5.SmartSelect("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res5.SelectedTicket.ID != "STUCK-ONE" {
		t.Errorf("expected STUCK-ONE with fewest blockers to be selected, got %s", res5.SelectedTicket.ID)
	}
}
