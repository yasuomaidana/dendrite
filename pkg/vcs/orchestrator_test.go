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

func TestStackedBranchOrchestrator_ValidationErrors_And_EdgeCases(t *testing.T) {
	repoDir, _ := setupRealRepo(t)
	ctx := context.Background()

	guard := NewGitGuard(repoDir, ModeDegradedBudget)
	manifestPath := filepath.Join(repoDir, ".dendrite", "audit_queue.json")
	orch := NewStackedBranchOrchestrator(guard, manifestPath)

	// 1. Corrupt manifest unmarshal error
	_ = os.MkdirAll(filepath.Dir(manifestPath), 0755)
	_ = os.WriteFile(manifestPath, []byte("bad-json"), 0644)
	_, err := orch.LoadManifest()
	if err == nil {
		t.Fatal("expected LoadManifest error on corrupt JSON, got nil")
	}

	// 2. Empty module slug error
	_, err = orch.StartModuleBranch(ctx, "", "")
	if err == nil {
		t.Fatal("expected error on empty module slug, got nil")
	}

	// 3. RecordTicketCommit with nil ticket
	err = orch.RecordTicketCommit(ctx, "mod", nil, "")
	if err == nil {
		t.Fatal("expected error on nil ticket, got nil")
	}

	// 4. FinalizeModuleForAudit with empty slug or non-existent module
	_ = os.WriteFile(manifestPath, []byte("[]"), 0644)
	err = orch.FinalizeModuleForAudit(ctx, "")
	if err == nil {
		t.Fatal("expected error on empty slug for finalize")
	}
	err = orch.FinalizeModuleForAudit(ctx, "nonexistent-module")
	if err != nil {
		t.Fatalf("expected nonexistent module to be appended to manifest: %v", err)
	}

	// 5. Existing branch re-checkout in StartModuleBranch
	runGitCmd(t, repoDir, "checkout", "-b", "local/mod-exist", "develop")
	runGitCmd(t, repoDir, "checkout", "develop")
	_ = os.WriteFile(manifestPath, []byte(`[{"module":"mod-exist","branch":"local/mod-exist","base_branch":"develop","status":"IN_PROGRESS"}]`), 0644)
	b, err := orch.StartModuleBranch(ctx, "mod-exist", "")
	if err != nil || b != "local/mod-exist" {
		t.Fatalf("expected re-checkout of existing branch to succeed, got %s, err: %v", b, err)
	}

	// 6. RecordTicketCommit duplicate ticket handling
	tkt := &tracker.RoutingTicket{ID: "DEN-DUPE", Title: "Dupe"}
	_ = orch.RecordTicketCommit(ctx, "mod-exist", tkt, "feat: commit 1")
	_ = orch.RecordTicketCommit(ctx, "mod-exist", tkt, "feat: commit 2")
	items, _ := orch.LoadManifest()
	for _, it := range items {
		if it.Module == "mod-exist" {
			count := 0
			for _, id := range it.Tickets {
				if id == "DEN-DUPE" {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("expected duplicate ticket not to be appended twice, got count %d", count)
			}
		}
	}
}

func TestRemoteSyncWatcher_AdditionalBranches(t *testing.T) {
	repoDir, originDir := setupRealRepo(t)
	ctx := context.Background()

	guard := NewGitGuard(repoDir, ModeStandard)

	// Default orchestrator fallback test (orch == nil) and branch == ""
	wDefault := NewRemoteSyncWatcher(guard, nil)
	statusDefault, err := wDefault.FetchAndDetect(ctx, "")
	if err != nil {
		t.Fatalf("fetch and detect failed with empty branch: %v", err)
	}
	if statusDefault.Branch != "develop" {
		t.Fatalf("expected develop branch, got %s", statusDefault.Branch)
	}

	// Up to date sync (Behind == 0)
	w := NewRemoteSyncWatcher(guard, nil)
	status, err := w.SyncRemoteFixes(ctx, "develop")
	if err != nil {
		t.Fatalf("unexpected error on up-to-date sync: %v", err)
	}
	if status.Updated || status.Behind != 0 {
		t.Fatalf("expected 0 behind commits when up to date, got %+v", status)
	}

	// Remote commits detection
	tmp := t.TempDir()
	secondClone := filepath.Join(tmp, "second")
	runGitCmd(t, tmp, "clone", originDir, secondClone)
	runGitCmd(t, secondClone, "config", "user.name", "Second")
	runGitCmd(t, secondClone, "config", "user.email", "second@dendrite.ai")
	rf := filepath.Join(secondClone, "remote_file.txt")
	_ = os.WriteFile(rf, []byte("remote fix\n"), 0644)
	runGitCmd(t, secondClone, "add", "remote_file.txt")
	runGitCmd(t, secondClone, "commit", "-m", "[DEN-001] fix: remote fix")
	runGitCmd(t, secondClone, "push", "origin", "develop")

	statusBehind, err := w.FetchAndDetect(ctx, "develop")
	if err != nil {
		t.Fatalf("fetch and detect failed: %v", err)
	}
	if statusBehind.Behind != 1 {
		t.Fatalf("expected 1 behind commit, got %+v", statusBehind)
	}
}
