package vcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/tracker"
)

func TestIsDegradedBudget(t *testing.T) {
	// Force local
	if !IsDegradedBudget(100, 100, true) {
		t.Fatal("expected true when forceLocal is true")
	}

	// 15% or below triggers degraded mode
	if !IsDegradedBudget(150, 1000, false) {
		t.Fatal("expected true for 15% budget")
	}
	if !IsDegradedBudget(100, 1000, false) {
		t.Fatal("expected true for 10% budget")
	}

	// Above 15% does not trigger degraded mode
	if IsDegradedBudget(200, 1000, false) {
		t.Fatal("expected false for 20% budget")
	}
}

func TestStackedBranchOrchestrator_Manifest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "audit-manifest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	manifestPath := filepath.Join(tmpDir, "audit_queue.json")
	guard := NewGitGuard(tmpDir, ModeDegradedBudget)
	orch := NewStackedBranchOrchestrator(guard, manifestPath)

	// Initially empty
	items, err := orch.LoadManifest()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty items, got %d", len(items))
	}

	// Save items
	items = append(items, &AuditQueueItem{
		Branch:  "local/module-a",
		Module:  "module-a",
		Status:  StatusAwaitingAudit,
		Tickets: []string{"DEN-701"},
	})
	if err := orch.SaveManifest(items); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Re-load and verify
	loaded, err := orch.LoadManifest()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Branch != "local/module-a" {
		t.Fatalf("unexpected loaded items: %+v", loaded)
	}
}

func TestStackedBranchOrchestrator_Lifecycle(t *testing.T) {
	mock := newMockRunner()
	mock.responses["checkout develop"] = ""
	mock.responses["show-ref --verify --quiet refs/heads/local/module-a"] = "error"
	mock.errors["show-ref --verify --quiet refs/heads/local/module-a"] = os.ErrNotExist
	mock.responses["checkout -b local/module-a develop"] = ""
	mock.responses["rev-parse develop"] = "abc123sha"
	mock.responses["rev-parse local/module-a"] = "def456sha"
	mock.responses["symbolic-ref --short HEAD"] = "local/module-a"
	mock.responses["status --porcelain"] = "M file.go"
	mock.responses["add -A"] = ""
	mock.responses["commit -m feat(module-a): complete #42 - Feature X"] = ""
	mock.responses["push --set-upstream origin local/module-a"] = ""
	mock.responses["rev-parse HEAD"] = "def456sha"

	tmpDir, _ := os.MkdirTemp("", "orch-test-*")
	defer os.RemoveAll(tmpDir)

	guard := NewGitGuard(tmpDir, ModeDegradedBudget).WithRunner(mock)
	orch := NewStackedBranchOrchestrator(guard, filepath.Join(tmpDir, "queue.json"))

	ctx := context.Background()
	branch, err := orch.StartModuleBranch(ctx, "module-a", "")
	if err != nil {
		t.Fatalf("start module branch failed: %v", err)
	}
	if branch != "local/module-a" {
		t.Fatalf("expected 'local/module-a', got '%s'", branch)
	}

	// Record commit
	tkt := &tracker.RoutingTicket{ID: "DEN-701", Title: "Feature X", GitHubIssueNum: 42}
	if err := orch.RecordTicketCommit(ctx, "module-a", tkt, ""); err != nil {
		t.Fatalf("record ticket commit failed: %v", err)
	}

	// Finalize module
	if err := orch.FinalizeModuleForAudit(ctx, "module-a"); err != nil {
		t.Fatalf("finalize failed: %v", err)
	}

	// Verify state in manifest
	items, err := orch.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != StatusAwaitingAudit {
		t.Fatalf("expected status AWAITING_AUDIT, got %+v", items)
	}
	if len(items[0].Tickets) != 1 || items[0].Tickets[0] != "DEN-701" {
		t.Fatalf("expected ticket DEN-701 in queue item, got %+v", items[0].Tickets)
	}
}
