package agent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/tracker"
)

func TestSessionLedger_BasicOperations(t *testing.T) {
	tmpDir := t.TempDir()
	ledgerPath := filepath.Join(tmpDir, "sessions.json")

	ledger := agent.NewSessionLedger(nil, ledgerPath)

	// 1. Error on nil session
	if err := ledger.RecordSession(context.Background(), nil); !errors.Is(err, agent.ErrNilSession) {
		t.Errorf("expected ErrNilSession for nil session, got: %v", err)
	}

	// 2. Record valid session
	s1 := &agent.SessionSummary{
		SessionID:        "sess-1",
		AgentType:        agent.AgentAider,
		TicketID:         "DEN-024",
		StartTime:        time.Now().Add(-10 * time.Minute),
		EndTime:          time.Now(),
		Duration:         10 * time.Minute,
		TokensConsumed:   2000,
		EstimatedCostUSD: 0.005,
		CommitsGenerated: []string{"[DEN-024] Feat (Agent: aider)"},
		Status:           "completed",
	}

	if err := ledger.RecordSession(context.Background(), s1); err != nil {
		t.Fatalf("unexpected error recording session: %v", err)
	}

	// 3. GetSession
	retrieved, ok := ledger.GetSession("sess-1")
	if !ok || retrieved.TicketID != "DEN-024" {
		t.Errorf("expected to retrieve sess-1, got ok=%v, session=%v", ok, retrieved)
	}

	_, okMissing := ledger.GetSession("missing")
	if okMissing {
		t.Errorf("expected false for missing session")
	}

	// 4. Record second session
	s2 := &agent.SessionSummary{
		SessionID:        "sess-2",
		AgentType:        agent.AgentOpenHands,
		TicketID:         "DEN-024",
		StartTime:        time.Now(),
		EndTime:          time.Now().Add(5 * time.Minute),
		Duration:         5 * time.Minute,
		TokensConsumed:   1000,
		EstimatedCostUSD: 0.002,
		Status:           "completed",
	}
	_ = ledger.RecordSession(context.Background(), s2)

	// 5. Record third session (AGY)
	s3 := &agent.SessionSummary{
		SessionID:        "sess-3",
		AgentType:        agent.AgentAGY,
		TicketID:         "DEN-028",
		StartTime:        time.Now(),
		EndTime:          time.Now().Add(3 * time.Minute),
		Duration:         3 * time.Minute,
		TokensConsumed:   1500,
		EstimatedCostUSD: 0.003,
		CommitsGenerated: []string{"[DEN-028] AGY support (Agent: agy)"},
		Status:           "completed",
	}
	_ = ledger.RecordSession(context.Background(), s3)

	// 6. Query lists and totals
	byTicket := ledger.ListSessionsByTicket("DEN-024")
	if len(byTicket) != 2 {
		t.Errorf("expected 2 sessions for DEN-024, got %d", len(byTicket))
	}

	byTicketAGY := ledger.ListSessionsByTicket("DEN-028")
	if len(byTicketAGY) != 1 {
		t.Errorf("expected 1 session for DEN-028, got %d", len(byTicketAGY))
	}

	byAgent := ledger.ListSessionsByAgent(agent.AgentAider)
	if len(byAgent) != 1 || byAgent[0].SessionID != "sess-1" {
		t.Errorf("expected 1 session for aider, got %d", len(byAgent))
	}

	byAgentAGY := ledger.ListSessionsByAgent(agent.AgentAGY)
	if len(byAgentAGY) != 1 || byAgentAGY[0].SessionID != "sess-3" {
		t.Errorf("expected 1 session for agy, got %d", len(byAgentAGY))
	}

	if ledger.TotalTokens() != 4500 {
		t.Errorf("expected 4500 total tokens, got %d", ledger.TotalTokens())
	}

	if ledger.TotalSpendUSD() != 0.010 {
		t.Errorf("expected $0.010 total spend, got %.4f", ledger.TotalSpendUSD())
	}
}

func TestSessionLedger_TrackerSync(t *testing.T) {
	tr := tracker.NewMemoryTracker("")
	_ = tr.CreateTicket(context.Background(), tracker.Ticket{
		ID:     "DEN-025",
		Title:  "Agent Sandbox Isolation",
		Status: tracker.StatusInProgress,
	})

	ledger := agent.NewSessionLedger(tr, "")
	ctx := context.Background()

	// Completed session should transition ticket to DONE
	sDone := &agent.SessionSummary{
		SessionID: "sess-done",
		AgentType: agent.AgentAider,
		TicketID:  "DEN-025",
		Status:    "completed",
	}
	_ = ledger.RecordSession(ctx, sDone)

	tkt, err := tr.GetTicket(ctx, "DEN-025")
	if err != nil || tkt.Status != tracker.StatusDone {
		t.Errorf("expected ticket status to be DONE after completed session, got %s, err: %v", tkt.Status, err)
	}

	// Failed session transitions ticket to IN_REVIEW
	sFailed := &agent.SessionSummary{
		SessionID: "sess-failed",
		AgentType: agent.AgentAider,
		TicketID:  "DEN-025",
		Status:    "failed",
	}
	_ = ledger.RecordSession(ctx, sFailed)

	tktReview, _ := tr.GetTicket(ctx, "DEN-025")
	if tktReview.Status != tracker.StatusInReview {
		t.Errorf("expected ticket status to be IN_REVIEW after failed session, got %s", tktReview.Status)
	}
}

func TestSessionLedger_JSONRoundtrip(t *testing.T) {
	ledger := agent.NewSessionLedger(nil, "")
	_ = ledger.RecordSession(context.Background(), &agent.SessionSummary{
		SessionID:      "sess-rt",
		AgentType:      agent.AgentOpenCode,
		TokensConsumed: 500,
	})

	data, err := ledger.ExportJSON()
	if err != nil {
		t.Fatalf("unexpected error exporting JSON: %v", err)
	}

	newLedger := agent.NewSessionLedger(nil, "")
	if err := newLedger.LoadJSON(data); err != nil {
		t.Fatalf("unexpected error loading JSON: %v", err)
	}

	if newLedger.TotalTokens() != 500 {
		t.Errorf("expected 500 total tokens in restored ledger, got %d", newLedger.TotalTokens())
	}

	// Test invalid JSON load
	if err := newLedger.LoadJSON([]byte("{invalid-json")); err == nil {
		t.Errorf("expected error loading invalid JSON")
	}

	// Verify ledger file on disk was written
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "disk_ledger.json")
	diskLedger := agent.NewSessionLedger(nil, filePath)
	_ = diskLedger.RecordSession(context.Background(), &agent.SessionSummary{
		SessionID: "sess-disk",
	})

	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("expected ledger file to exist on disk: %v", err)
	}
}
