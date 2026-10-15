package tracker

import (
	"testing"
)

func TestEnrichTicketClassification(t *testing.T) {
	// L1 should be local feasible and require audit
	t1 := &RoutingTicket{Difficulty: TierL1Trivial}
	EnrichTicketClassification(t1)
	if !t1.LocalFeasible {
		t.Fatalf("expected t1 to be local feasible")
	}
	if !t1.RequiresAudit || t1.AuditState != AuditNone {
		t.Fatalf("expected t1 to require audit with AuditNone state")
	}

	// L4 should NOT be local feasible
	t4 := &RoutingTicket{Difficulty: TierL4High}
	EnrichTicketClassification(t4)
	if t4.LocalFeasible {
		t.Fatalf("expected t4 to not be local feasible")
	}
}

func TestSelectInferenceEngine(t *testing.T) {
	// 1. Local feasible ticket uses local LLM
	lfTicket := &RoutingTicket{LocalFeasible: true, Difficulty: TierL2Low}
	engine, ok := lfTicket.SelectInferenceEngine(false)
	if !ok || engine != EngineLocalLLM {
		t.Fatalf("expected EngineLocalLLM, got %s, ok=%v", engine, ok)
	}

	// 2. Low budget mode: L3 is attempted locally
	l3Ticket := &RoutingTicket{Difficulty: TierL3Medium, LocalFeasible: false}
	engine, ok = l3Ticket.SelectInferenceEngine(true)
	if !ok || engine != EngineLocalLLM {
		t.Fatalf("expected EngineLocalLLM for L3 in low budget mode, got %s, ok=%v", engine, ok)
	}
	if !l3Ticket.RequiresAudit {
		t.Fatalf("expected L3 ticket in low budget mode to require audit")
	}

	// 3. Low budget mode: L4 is deferred
	l4Ticket := &RoutingTicket{Difficulty: TierL4High, LocalFeasible: false}
	engine, ok = l4Ticket.SelectInferenceEngine(true)
	if ok || engine != "" {
		t.Fatalf("expected L4 ticket in low budget mode to be deferred, got %s, ok=%v", engine, ok)
	}

	// 4. Standard mode: L4 uses Vendor Reasoning
	engine, ok = l4Ticket.SelectInferenceEngine(false)
	if !ok || engine != EngineVendorReasoning {
		t.Fatalf("expected EngineVendorReasoning for L4, got %s", engine)
	}

	// 5. Standard mode: L3 non-local uses Vendor Fast
	engine, ok = l3Ticket.SelectInferenceEngine(false)
	if !ok || engine != EngineVendorFast {
		t.Fatalf("expected EngineVendorFast for L3 in standard mode, got %s", engine)
	}
}

func TestSortForLowBudgetQueue(t *testing.T) {
	tickets := []*RoutingTicket{
		{ID: "T3", Priority: "medium", Difficulty: TierL3Medium, LocalFeasible: false, SequenceOrder: 3},
		{ID: "T2", Priority: "medium", Difficulty: TierL1Trivial, LocalFeasible: true, SequenceOrder: 2},
		{ID: "T1", Priority: "P0_TOP_BLOCKER", Difficulty: TierL2Low, LocalFeasible: true, SequenceOrder: 1},
		{ID: "T4", Priority: "P0_TOP_BLOCKER", Difficulty: TierL4High, LocalFeasible: false, SequenceOrder: 4},
	}

	SortForLowBudgetQueue(tickets)

	// Top blocker with LocalFeasible comes first (T1), then other top blocker (T4), then LocalFeasible T2, then T3
	if tickets[0].ID != "T1" {
		t.Fatalf("expected first ticket to be T1, got %s", tickets[0].ID)
	}
	if tickets[1].ID != "T4" {
		t.Fatalf("expected second ticket to be T4, got %s", tickets[1].ID)
	}
	if tickets[2].ID != "T2" {
		t.Fatalf("expected third ticket to be T2, got %s", tickets[2].ID)
	}
	if tickets[3].ID != "T3" {
		t.Fatalf("expected fourth ticket to be T3, got %s", tickets[3].ID)
	}
}
