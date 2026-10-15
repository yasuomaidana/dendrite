package dag

import (
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/tracker"
)

func TestGraph_CycleDetection(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{ID: "DEN-001", Title: "Task 1"})
	g.AddNode(&Node{ID: "DEN-002", Title: "Task 2"})
	g.AddNode(&Node{ID: "DEN-003", Title: "Task 3"})

	_ = g.AddEdge("DEN-001", "DEN-002") // 1 blocks 2
	_ = g.AddEdge("DEN-002", "DEN-003") // 2 blocks 3
	_ = g.AddEdge("DEN-003", "DEN-001") // 3 blocks 1 -> Cycle!

	cycle, err := g.DetectCycles()
	if err == nil {
		t.Fatalf("expected cycle error, got nil")
	}
	if len(cycle) == 0 {
		t.Errorf("expected non-empty cycle path")
	}
}

func TestGraph_SelfCycle(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{ID: "DEN-001", Title: "Task 1"})
	err := g.AddEdge("DEN-001", "DEN-001")
	if err == nil {
		t.Fatalf("expected error on self dependency")
	}
}

func TestGraph_MissingNodesEdge(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{ID: "DEN-001", Title: "Task 1"})
	if err := g.AddEdge("DEN-MISSING", "DEN-001"); err == nil {
		t.Errorf("expected error for missing blocking node")
	}
	if err := g.AddEdge("DEN-001", "DEN-MISSING"); err == nil {
		t.Errorf("expected error for missing blocked node")
	}
	// duplicate edge check
	g.AddNode(&Node{ID: "DEN-002", Title: "Task 2"})
	_ = g.AddEdge("DEN-001", "DEN-002")
	if err := g.AddEdge("DEN-001", "DEN-002"); err != nil {
		t.Errorf("expected duplicate edge to return nil error, got %v", err)
	}
}

func TestGraph_GetNodeAndNodes(t *testing.T) {
	g := NewGraph()
	g.AddNode(nil) // should not panic
	g.AddNode(&Node{ID: "DEN-001", Title: "Task 1", Milestone: "M1"})
	g.AddNode(&Node{ID: "DEN-001", Title: "Task 1", Milestone: "M1"}) // duplicate milestone branch

	if g.GetNode("DEN-001") == nil {
		t.Errorf("expected node DEN-001 to exist")
	}
	if len(g.Nodes()) != 1 {
		t.Errorf("expected 1 node, got %d", len(g.Nodes()))
	}
}

func TestGraph_TopologicalSort(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{ID: "DEN-001", Title: "A"})
	g.AddNode(&Node{ID: "DEN-002", Title: "B"})
	g.AddNode(&Node{ID: "DEN-003", Title: "C"})

	_ = g.AddEdge("DEN-001", "DEN-002")
	_ = g.AddEdge("DEN-002", "DEN-003")

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"DEN-001", "DEN-002", "DEN-003"}
	for i, id := range expected {
		if order[i] != id {
			t.Errorf("expected %s at pos %d, got %s", id, i, order[i])
		}
	}
}

func TestEstimateDays_And_PriorityScores(t *testing.T) {
	if days := EstimateDays(1, 0); days != 2 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(5, 0); days != 4 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(8, 0); days != 7 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(13, 0); days != 12 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(21, 0); days != 15 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(0, 2.5); days != 2 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(0, 5.0); days != 4 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(0, 7.5); days != 7 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(0, 9.5); days != 12 {
		t.Errorf("unexpected days: %d", days)
	}
	if days := EstimateDays(0, 0); days != 3 {
		t.Errorf("unexpected default days: %d", days)
	}

	if BasePriorityScore(PriorityUrgent) != 1000.0 {
		t.Errorf("unexpected urgent score")
	}
	if BasePriorityScore(PriorityHigh) != 500.0 {
		t.Errorf("unexpected high score")
	}
	if BasePriorityScore(PriorityMedium) != 200.0 {
		t.Errorf("unexpected medium score")
	}
	if BasePriorityScore(PriorityLow) != 50.0 {
		t.Errorf("unexpected low score")
	}
	if BasePriorityScore(PriorityNone) != 10.0 {
		t.Errorf("unexpected none score")
	}
	if BasePriorityScore("unknown") != 10.0 {
		t.Errorf("unexpected unknown score")
	}
}

func TestGraph_GanttPlan(t *testing.T) {
	g := NewGraph()
	anchor := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

	g.AddNode(&Node{
		ID:             "DEN-010",
		Title:          "Foundation Core",
		Milestone:      "Milestone 1",
		EstimatePoints: 3,
		Priority:       PriorityMedium,
		Status:         tracker.StatusDone,
	})
	g.AddNode(&Node{
		ID:             "DEN-011",
		Title:          "API Layer",
		Milestone:      "Milestone 1",
		EstimatePoints: 5,
		Priority:       PriorityHigh,
	})

	_ = g.AddEdge("DEN-010", "DEN-011") // DEN-010 blocks DEN-011

	plan, err := g.GenerateGanttPlan(anchor)
	if err != nil {
		t.Fatalf("failed to generate Gantt plan: %v", err)
	}

	t10 := plan.Tickets["DEN-010"]
	t11 := plan.Tickets["DEN-011"]

	if t10.StartDate != "2026-10-15" {
		t.Errorf("unexpected t10 start date: %s", t10.StartDate)
	}

	// DEN-011 must start on or after DEN-010 target date
	if t11.StartDate <= t10.StartDate {
		t.Errorf("expected t11 to start after t10, got t11=%s, t10=%s", t11.StartDate, t10.StartDate)
	}

	ms, ok := plan.Milestones["Milestone 1"]
	if !ok {
		t.Fatalf("expected Milestone 1 in plan")
	}
	if ms.StartDate != t10.StartDate {
		t.Errorf("milestone start should match first ticket, got %s", ms.StartDate)
	}
	if ms.TargetDate != t11.TargetDate {
		t.Errorf("milestone target should match last ticket, got %s", ms.TargetDate)
	}
	if ms.CompletedCount != 1 || ms.TicketCount != 2 {
		t.Errorf("unexpected milestone counts: completed=%d, total=%d", ms.CompletedCount, ms.TicketCount)
	}
}

func TestGraph_SmartSelect_PriorityAndBlockers(t *testing.T) {
	g := NewGraph()

	// Scenario:
	// Ticket A (Low Priority, Foundation, TODO) blocks Ticket B (Urgent, Feature, TODO)
	// Ticket C (High Priority, Independent, TODO)
	g.AddNode(&Node{
		ID:         "DEN-A",
		SequenceID: 1,
		Title:      "Low Priority Foundation",
		Status:     tracker.StatusTodo,
		Priority:   PriorityLow,
	})
	g.AddNode(&Node{
		ID:         "DEN-B",
		SequenceID: 2,
		Title:      "Urgent Downstream Feature",
		Status:     tracker.StatusTodo,
		Priority:   PriorityUrgent,
	})
	g.AddNode(&Node{
		ID:         "DEN-C",
		SequenceID: 3,
		Title:      "High Priority Independent Feature",
		Status:     tracker.StatusTodo,
		Priority:   PriorityHigh,
	})

	_ = g.AddEdge("DEN-A", "DEN-B") // A blocks B

	res, err := g.SmartSelect("")
	if err != nil {
		t.Fatalf("SmartSelect error: %v", err)
	}

	if res.SelectedTicket.ID != "DEN-A" {
		t.Fatalf("expected DEN-A to be selected due to unblock boost for urgent B, got %s", res.SelectedTicket.ID)
	}
	if res.IsBlocked {
		t.Errorf("DEN-A should not be marked blocked")
	}

	// Now mark DEN-A as DONE
	g.nodes["DEN-A"].Status = tracker.StatusDone

	// Now B is unblocked, and B is URGENT (1000) vs C HIGH (500).
	res2, err := g.SmartSelect("")
	if err != nil {
		t.Fatalf("SmartSelect error: %v", err)
	}
	if res2.SelectedTicket.ID != "DEN-B" {
		t.Fatalf("expected DEN-B (Urgent, now unblocked) to be selected, got %s", res2.SelectedTicket.ID)
	}

	// Now mark DEN-B as DONE
	g.nodes["DEN-B"].Status = tracker.StatusDone

	// Now C should be selected
	res3, err := g.SmartSelect("")
	if err != nil {
		t.Fatalf("SmartSelect error: %v", err)
	}
	if res3.SelectedTicket.ID != "DEN-C" {
		t.Fatalf("expected DEN-C to be selected, got %s", res3.SelectedTicket.ID)
	}

	// Mark DEN-C as DONE -> all done
	g.nodes["DEN-C"].Status = tracker.StatusDone
	if _, err := g.SmartSelect(""); err == nil {
		t.Errorf("expected error when all tickets completed")
	}
}

func TestGraph_SmartSelect_ActiveInProgress(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{
		ID:       "DEN-001",
		Title:    "Ticket 1",
		Status:   tracker.StatusInProgress,
		Priority: PriorityHigh,
	})
	g.AddNode(&Node{
		ID:       "DEN-002",
		Title:    "Ticket 2",
		Status:   tracker.StatusTodo,
		Priority: PriorityUrgent,
	})

	res, err := g.SmartSelect("")
	if err != nil {
		t.Fatalf("SmartSelect error: %v", err)
	}
	if res.SelectedTicket.ID != "DEN-001" {
		t.Errorf("expected already IN_PROGRESS ticket to be selected, got %s", res.SelectedTicket.ID)
	}
	if !res.IsActive {
		t.Errorf("expected IsActive to be true")
	}
}

func TestGraph_SmartSelect_AllBlockedCandidates(t *testing.T) {
	g := NewGraph()
	// Both tickets block each other or waiting on external blocked item
	g.AddNode(&Node{
		ID:        "DEN-X",
		Title:     "X",
		Status:    tracker.StatusTodo,
		BlockedBy: []string{"DEN-EXT"},
	})
	res, err := g.SmartSelect("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsBlocked {
		t.Errorf("expected ticket to be marked blocked")
	}
}

func TestGraph_SmartSelect_UserOverride(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{
		ID:       "DEN-001",
		Title:    "Blocker",
		Status:   tracker.StatusTodo,
		Priority: PriorityLow,
	})
	g.AddNode(&Node{
		ID:       "DEN-002",
		Title:    "Dependent",
		Status:   tracker.StatusTodo,
		Priority: PriorityUrgent,
	})
	_ = g.AddEdge("DEN-001", "DEN-002")

	// Missing ticket override
	if _, err := g.SmartSelect("DEN-999"); err == nil {
		t.Errorf("expected error for non-existent ticket")
	}

	// User explicitly requests DEN-002
	res, err := g.SmartSelect("DEN-002")
	if err != nil {
		t.Fatalf("SmartSelect error: %v", err)
	}
	if res.SelectedTicket.ID != "DEN-002" {
		t.Errorf("expected DEN-002, got %s", res.SelectedTicket.ID)
	}
	if !res.IsBlocked {
		t.Errorf("expected warning IsBlocked=true because DEN-001 is not done")
	}
	if len(res.BlockingTickets) != 1 || res.BlockingTickets[0] != "DEN-001" {
		t.Errorf("expected BlockingTickets to contain DEN-001, got %v", res.BlockingTickets)
	}
	if !strings.Contains(res.Reason, "WARNING") {
		t.Errorf("expected WARNING in reason, got %s", res.Reason)
	}

	// Now mark DEN-001 done and select DEN-002
	g.nodes["DEN-001"].Status = tracker.StatusDone
	resDone, err := g.SmartSelect("002") // fuzzy match
	if err != nil {
		t.Fatalf("fuzzy match error: %v", err)
	}
	if resDone.IsBlocked {
		t.Errorf("DEN-002 should no longer be blocked")
	}
}
