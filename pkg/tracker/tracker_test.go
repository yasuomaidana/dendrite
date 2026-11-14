package tracker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var sampleMarkdown = "# Dendrite Status\n\n" +
	"### Milestone 1: Core Foundation\n" +
	"- [x] **DEN-001**: Initialize Status (`pkg/tracker`)\n" +
	"- [ ] **DEN-002**: Ollama Client (`pkg/ollama`) *(IN PROGRESS)*\n" +
	"- [ ] **DEN-003**: CLI Tools (`cmd/dendrite`)\n"

func TestMemoryTracker_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	mdPath := filepath.Join(tmpDir, "DEV_STATUS.md")
	if err := os.WriteFile(mdPath, []byte(sampleMarkdown), 0644); err != nil {
		t.Fatalf("failed to write test md: %v", err)
	}

	tr := NewMemoryTracker(mdPath)
	if err := tr.LoadFromMarkdown(mdPath); err != nil {
		t.Fatalf("failed to load markdown: %v", err)
	}

	ctx := context.Background()

	// 1. List tickets
	tickets, err := tr.ListTickets(ctx)
	if err != nil {
		t.Fatalf("unexpected error listing tickets: %v", err)
	}
	if len(tickets) != 3 {
		t.Fatalf("expected 3 tickets, got %d", len(tickets))
	}

	// 2. Verify ticket 1
	t1, err := tr.GetTicket(ctx, "DEN-001")
	if err != nil {
		t.Fatalf("expected DEN-001 to exist: %v", err)
	}
	if t1.Status != StatusDone {
		t.Errorf("expected DEN-001 to be DONE, got %s", t1.Status)
	}

	// 3. Verify ticket 2 (IN PROGRESS)
	t2, err := tr.GetTicket(ctx, "DEN-002")
	if err != nil {
		t.Fatalf("expected DEN-002 to exist: %v", err)
	}
	if t2.Status != StatusInProgress {
		t.Errorf("expected DEN-002 to be IN_PROGRESS, got %s", t2.Status)
	}

	// 4. Update status of DEN-002 to DONE
	if err := tr.UpdateStatus(ctx, "DEN-002", StatusDone); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// 5. Create a new ticket
	newTicket := Ticket{
		ID:            "DEN-004",
		Title:         "New Ticket",
		Status:        StatusTodo,
		TargetPackage: "pkg/new",
	}
	if err := tr.CreateTicket(ctx, newTicket); err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}

	// Attempt duplicate create
	if err := tr.CreateTicket(ctx, newTicket); err == nil {
		t.Error("expected error creating duplicate ticket, got nil")
	}

	// 6. Sync back to markdown
	if err := tr.Sync(ctx); err != nil {
		t.Fatalf("failed to sync markdown: %v", err)
	}

	// 7. Re-read markdown and assert DEN-002 is marked [x]
	reloaded := NewMemoryTracker(mdPath)
	if err := reloaded.LoadFromMarkdown(mdPath); err != nil {
		t.Fatalf("failed to reload markdown: %v", err)
	}

	reloadedT2, err := reloaded.GetTicket(ctx, "DEN-002")
	if err != nil {
		t.Fatalf("failed to get reloaded T2: %v", err)
	}
	if reloadedT2.Status != StatusDone {
		t.Errorf("expected reloaded T2 to be DONE, got %s", reloadedT2.Status)
	}

	// 8. Non-existent ticket
	_, err = tr.GetTicket(ctx, "NON-EXISTENT")
	if err == nil {
		t.Error("expected error for non-existent ticket, got nil")
	}
	if err.Error() == "" {
		t.Error("expected error message")
	}

	// 9. Update non-existent ticket
	err = tr.UpdateStatus(ctx, "NON-EXISTENT", StatusDone)
	if err == nil {
		t.Error("expected error updating non-existent ticket, got nil")
	}
}

func TestMemoryTracker_Errors(t *testing.T) {
	tr := NewMemoryTracker("")
	ctx := context.Background()
	if err := tr.Sync(ctx); err == nil {
		t.Error("expected sync error with empty path, got nil")
	}

	if err := tr.LoadFromMarkdown("nonexistent-path.md"); err == nil {
		t.Error("expected load error with bad path, got nil")
	}
}

func TestRewriteStatus_StateMarkerAndLegacy(t *testing.T) {
	// 1. With state marker
	lineWithMarker := "- [ ] **DEN-101**: Title [State: To Do, Priority: High]"
	doneLine := rewriteStatus(lineWithMarker, StatusDone)
	if doneLine != "- [x] **DEN-101**: Title [State: Done, Priority: High]" {
		t.Errorf("unexpected rewritten done line: %s", doneLine)
	}

	reviewLine := rewriteStatus(lineWithMarker, StatusInReview)
	if reviewLine != "- [ ] **DEN-101**: Title [State: To Verify, Priority: High]" {
		t.Errorf("unexpected rewritten review line: %s", reviewLine)
	}

	// 2. Legacy status suffix
	legacyLine := "- [ ] **DEN-102**: Legacy Title *(IN PROGRESS)*"
	todoLine := rewriteStatus(legacyLine, StatusTodo)
	if todoLine != "- [ ] **DEN-102**: Legacy Title" {
		t.Errorf("unexpected rewritten todo line: %s", todoLine)
	}

	inProgLine := rewriteStatus("- [ ] **DEN-103**: New Task", StatusInProgress)
	if inProgLine != "- [ ] **DEN-103**: New Task *(IN PROGRESS)*" {
		t.Errorf("unexpected in-progress line: %s", inProgLine)
	}
}

func TestMemoryTracker_DuplicateAndNonZeroCreatedAt(t *testing.T) {
	mdWithDup := "# Status\n" +
		"- [x] **DEN-001**: First Version (`pkg/tracker`)\n" +
		"- [ ] **DEN-001**: Duplicate Version (`pkg/other`)\n" +
		"- [ ] **DEN-002**: Plain Ticket Without Target Package\n"

	tmpDir := t.TempDir()
	mdPath := filepath.Join(tmpDir, "DEV_STATUS.md")
	if err := os.WriteFile(mdPath, []byte(mdWithDup), 0644); err != nil {
		t.Fatalf("failed to write md: %v", err)
	}

	tr := NewMemoryTracker(mdPath)
	if err := tr.LoadFromMarkdown(mdPath); err != nil {
		t.Fatalf("LoadFromMarkdown failed: %v", err)
	}

	ctx := context.Background()
	t1, err := tr.GetTicket(ctx, "DEN-001")
	if err != nil {
		t.Fatalf("expected DEN-001 to exist: %v", err)
	}
	if t1.Title != "First Version" {
		t.Errorf("expected duplicate to preserve first version, got %q", t1.Title)
	}

	t2, err := tr.GetTicket(ctx, "DEN-002")
	if err != nil {
		t.Fatalf("expected DEN-002 to exist: %v", err)
	}
	if t2.TargetPackage != "" {
		t.Errorf("expected empty target package for DEN-002, got %q", t2.TargetPackage)
	}

	// CreateTicket with explicitly set CreatedAt
	customTime := t1.CreatedAt.Add(-time.Hour)
	newT := Ticket{
		ID:        "DEN-005",
		Title:     "Explicit Time",
		Status:    StatusTodo,
		CreatedAt: customTime,
	}
	if err := tr.CreateTicket(ctx, newT); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	gotT, _ := tr.GetTicket(ctx, "DEN-005")
	if !gotT.CreatedAt.Equal(customTime) {
		t.Errorf("expected CreatedAt preserved: %v vs %v", customTime, gotT.CreatedAt)
	}

	// Sync failure when file is deleted
	_ = os.Remove(mdPath)
	_ = tr.UpdateStatus(ctx, "DEN-001", StatusInProgress)
	if err := tr.Sync(ctx); err == nil {
		t.Error("expected Sync error when file was deleted, got nil")
	}
}
