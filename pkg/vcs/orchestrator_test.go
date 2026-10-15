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
	tmpDir := t.TempDir()
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

func TestStackedBranchOrchestrator_Lifecycle_RealGit(t *testing.T) {
	repoDir, originDir := setupRealRepo(t)
	ctx := context.Background()

	guard := NewGitGuard(repoDir, ModeDegradedBudget)
	manifestPath := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	orch := NewStackedBranchOrchestrator(guard, manifestPath)

	// 1. Start module branch
	branch, err := orch.StartModuleBranch(ctx, "module-a", "")
	if err != nil {
		t.Fatalf("start module branch failed: %v", err)
	}
	if branch != "local/module-a" {
		t.Fatalf("expected 'local/module-a', got '%s'", branch)
	}

	// 2. Add real file and record commit
	testFile := filepath.Join(repoDir, "module_code.go")
	_ = os.WriteFile(testFile, []byte("package module_a\n"), 0644)

	tkt := &tracker.RoutingTicket{ID: "DEN-704", Title: "Speculative Branching", GitHubIssueNum: 4}
	if err := orch.RecordTicketCommit(ctx, "module-a", tkt, ""); err != nil {
		t.Fatalf("record ticket commit failed: %v", err)
	}

	// Verify commit exists on remote origin on branch local/module-a
	logOut := runGitCmd(t, originDir, "log", "-n", "1", "--oneline", "local/module-a")
	if !containsStr(logOut, "feat(module-a): complete #4 - Speculative Branching") {
		t.Fatalf("commit not pushed to origin/local/module-a: %s", logOut)
	}

	// 3. Finalize module
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
	if len(items[0].Tickets) != 1 || items[0].Tickets[0] != "DEN-704" {
		t.Fatalf("expected ticket DEN-704 in queue item, got %+v", items[0].Tickets)
	}
}

func TestStackedBranchOrchestrator_StackedOnParent(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	guard := NewGitGuard(repoDir, ModeDegradedBudget)
	orch := NewStackedBranchOrchestrator(guard, "")

	// 1. Start parent branch
	b1, err := orch.StartModuleBranch(ctx, "module-a", "")
	if err != nil || b1 != "local/module-a" {
		t.Fatalf("module-a failed: %v", err)
	}

	// 2. Start child branch stacked on parent
	b2, err := orch.StartModuleBranch(ctx, "module-b", "module-a")
	if err != nil || b2 != "local/module-b" {
		t.Fatalf("module-b failed: %v", err)
	}

	// 3. Test branch mismatch error in RecordTicketCommit
	tkt := &tracker.RoutingTicket{ID: "DEN-999", Title: "Mismatch"}
	err = orch.RecordTicketCommit(ctx, "wrong-module", tkt, "")
	if err == nil {
		t.Fatal("expected error on branch mismatch")
	}

	// Test IsDegradedBudget edge case: totalTokens <= 0
	if IsDegradedBudget(10, 0, false) {
		t.Fatal("expected false when totalTokens <= 0")
	}
}
