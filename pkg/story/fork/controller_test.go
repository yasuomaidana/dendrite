package fork

import (
	"Dendrite/pkg/statedir"
	"Dendrite/pkg/store"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	runCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (out: %s)", args, err, string(out))
		}
	}

	runCmd("init")
	runCmd("config", "user.name", "Dendrite Bot")
	runCmd("config", "user.email", "bot@dendrite.ai")

	initialFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(initialFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}
	runCmd("add", "README.md")
	runCmd("commit", "-m", "Initial commit")
	runCmd("checkout", "-B", "main")

	return repoDir
}

func TestForkController_DefaultsAndConfig(t *testing.T) {
	tmpDir := t.TempDir()
	worktreesDir := filepath.Join(tmpDir, "worktrees")
	executor := NewOSGitExecutor()
	harness := NewDefaultBenchmarkHarness()
	st := store.NewStore(filepath.Join(tmpDir, "store"), filepath.Join(tmpDir, "adr"))

	ctrlDefault := NewForkController("", "", nil, nil, nil)
	if ctrlDefault.repoDir != "." {
		t.Errorf("expected repoDir '.', got %s", ctrlDefault.repoDir)
	}
	if ctrlDefault.worktreesBaseDir != statedir.Resolve(".dendrite/worktrees") {
		t.Errorf("expected worktreesBaseDir %s, got %s", statedir.Resolve(".dendrite/worktrees"), ctrlDefault.worktreesBaseDir)
	}
	if ctrlDefault.executor == nil || ctrlDefault.harness == nil {
		t.Error("expected non-nil default executor and harness")
	}

	ctrlCustom := NewForkController(tmpDir, worktreesDir, executor, harness, st)
	if ctrlCustom.repoDir != tmpDir || ctrlCustom.worktreesBaseDir != worktreesDir {
		t.Errorf("unexpected custom controller configuration: %+v", ctrlCustom)
	}
	if ctrlCustom.store != st || ctrlCustom.executor != executor || ctrlCustom.harness != harness {
		t.Errorf("expected exact components assigned: %+v", ctrlCustom)
	}
}

func TestForkController_Validation(t *testing.T) {
	repoDir := initTestGitRepo(t)
	worktreesDir := filepath.Join(t.TempDir(), "worktrees")
	ctrl := NewForkController(repoDir, worktreesDir, nil, nil, nil)
	ctx := context.Background()

	// Empty story ID
	_, _, err := ctrl.Fork(ctx, "", []string{"v1", "v2"}, "")
	if err == nil {
		t.Fatal("expected error on empty story ID")
	}

	// Single variant
	_, _, err = ctrl.Fork(ctx, "story-val-1", []string{"v1"}, "")
	if err == nil {
		t.Fatal("expected error on single variant")
	}

	// Duplicate variants
	_, _, err = ctrl.Fork(ctx, "story-val-2", []string{"v1", "v1"}, "")
	if err == nil {
		t.Fatal("expected error on duplicate variants")
	}

	// Whitespace and empty variant filtering resulting in < 2
	_, _, err = ctrl.Fork(ctx, "story-val-3", []string{"   ", "v1"}, "")
	if err == nil {
		t.Fatal("expected error when distinct variant count is less than 2")
	}
}

func TestForkController_Lifecycle_RealGit(t *testing.T) {
	repoDir := initTestGitRepo(t)
	tmpDir := t.TempDir()
	worktreesDir := filepath.Join(tmpDir, "worktrees")
	storeDir := filepath.Join(tmpDir, "store")
	st := store.NewStore(storeDir, filepath.Join(tmpDir, "adr"))
	ctrl := NewForkController(repoDir, worktreesDir, nil, nil, st)
	ctx := context.Background()

	storyID := "story-lifecycle-101"
	variants := []string{"variant-alpha", "variant-beta"}

	// 1. Fork
	manifest, guard, err := ctrl.Fork(ctx, storyID, variants, "")
	if err != nil {
		t.Fatalf("Fork failed: %v", err)
	}
	defer guard.Close()

	if manifest.Status != StatusActive {
		t.Errorf("expected StatusActive, got %s", manifest.Status)
	}
	if len(manifest.Variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(manifest.Variants))
	}
	if manifest.BaseBranch != "main" {
		t.Errorf("expected base branch main, got %s", manifest.BaseBranch)
	}

	for _, v := range manifest.Variants {
		if _, err := os.Stat(v.WorktreePath); err != nil {
			t.Fatalf("worktree path %s does not exist: %v", v.WorktreePath, err)
		}
	}

	// 2. Reject duplicate active fork
	_, _, err = ctrl.Fork(ctx, storyID, variants, "")
	if err == nil {
		t.Fatal("expected error on already active fork")
	}

	// 3. Make change in variant-alpha
	varAlphaPath := filepath.Join(worktreesDir, storyID, "variant-alpha")
	codeFile := filepath.Join(varAlphaPath, "feature.txt")
	if err := os.WriteFile(codeFile, []byte("feature variant alpha"), 0644); err != nil {
		t.Fatalf("failed writing file in variant worktree: %v", err)
	}
	cmdAdd := exec.Command("git", "add", "feature.txt")
	cmdAdd.Dir = varAlphaPath
	if out, err := cmdAdd.CombinedOutput(); err != nil {
		t.Fatalf("git add in worktree failed: %v (out: %s)", err, string(out))
	}
	cmdCommit := exec.Command("git", "commit", "-m", "Commit variant alpha feature")
	cmdCommit.Dir = varAlphaPath
	if out, err := cmdCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit in worktree failed: %v (out: %s)", err, string(out))
	}

	// 4. Benchmark
	opts := BenchmarkOptions{Iterations: 10, VirtualUsers: 2}
	scorecard, err := ctrl.Benchmark(ctx, storyID, opts)
	if err != nil {
		t.Fatalf("Benchmark failed: %v", err)
	}
	if scorecard.RecommendedWinner == "" {
		t.Fatal("expected a recommended winner")
	}
	if len(scorecard.Metrics) != 2 {
		t.Errorf("expected 2 metrics, got %d", len(scorecard.Metrics))
	}

	loadedManifest, err := ctrl.GetManifest(storyID)
	if err != nil {
		t.Fatalf("failed getting manifest: %v", err)
	}
	if loadedManifest.Status != StatusBenchmarked {
		t.Errorf("expected StatusBenchmarked, got %s", loadedManifest.Status)
	}

	// Benchmark again on StatusBenchmarked should succeed
	scorecard2, err := ctrl.Benchmark(ctx, storyID, opts)
	if err != nil {
		t.Fatalf("second benchmark on StatusBenchmarked failed: %v", err)
	}
	if scorecard2 == nil {
		t.Fatal("expected non-nil scorecard from second benchmark")
	}

	// 5. Resolve - invalid winner
	_, err = ctrl.Resolve(ctx, storyID, "nonexistent-variant")
	if err == nil {
		t.Fatal("expected error on non-existent winner")
	}

	// 6. Resolve - success with variant-alpha
	res, err := ctrl.Resolve(ctx, storyID, "variant-alpha")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	guard.Dismiss()

	if res.Winner != "variant-alpha" {
		t.Errorf("expected winner variant-alpha, got %s", res.Winner)
	}
	if len(res.PurgedVariants) != 1 || res.PurgedVariants[0] != "variant-beta" {
		t.Errorf("expected purged variant-beta, got %v", res.PurgedVariants)
	}

	// Verify merged file exists in main repository
	mergedFile := filepath.Join(repoDir, "feature.txt")
	if _, err := os.Stat(mergedFile); err != nil {
		t.Errorf("expected merged file %s in root repo: %v", mergedFile, err)
	}

	// Verify persistence in real store
	storedData, err := st.Get(res.StoreKey)
	if err != nil || len(storedData) == 0 {
		t.Errorf("expected benchmark data persisted in store for key %s: %v", res.StoreKey, err)
	}

	// Verify manifest status is StatusResolved
	resolvedManifest, err := ctrl.GetManifest(storyID)
	if err != nil {
		t.Fatalf("failed loading resolved manifest: %v", err)
	}
	if resolvedManifest.Status != StatusResolved {
		t.Errorf("expected StatusResolved, got %s", resolvedManifest.Status)
	}

	// 7. Resolve already resolved
	_, err = ctrl.Resolve(ctx, storyID, "variant-alpha")
	if err == nil {
		t.Fatal("expected error when resolving already resolved story")
	}

	// 8. Benchmark already resolved
	_, err = ctrl.Benchmark(ctx, storyID, opts)
	if err == nil {
		t.Fatal("expected error when benchmarking already resolved story")
	}
}

func TestForkController_GuardAndRollback(t *testing.T) {
	repoDir := initTestGitRepo(t)
	tmpDir := t.TempDir()
	worktreesDir := filepath.Join(tmpDir, "worktrees")
	ctrl := NewForkController(repoDir, worktreesDir, nil, nil, nil)
	ctx := context.Background()

	// Case 1: Close without dismiss triggers rollback
	storyID := "story-rollback-test"
	manifest, guard, err := ctrl.Fork(ctx, storyID, []string{"v1", "v2"}, "")
	if err != nil {
		t.Fatalf("Fork failed: %v", err)
	}
	for _, v := range manifest.Variants {
		if _, err := os.Stat(v.WorktreePath); err != nil {
			t.Fatalf("worktree path %s should exist before close: %v", v.WorktreePath, err)
		}
	}

	if err := guard.Close(); err != nil {
		t.Fatalf("guard.Close failed: %v", err)
	}

	// After rollback, story worktree directory should be purged
	storyDir := filepath.Join(worktreesDir, storyID)
	if _, err := os.Stat(storyDir); !os.IsNotExist(err) {
		t.Errorf("expected story worktrees dir to be removed on rollback")
	}

	// Case 2: Guard dismissed does not rollback
	storyID2 := "story-dismiss-test"
	_, guard2, err := ctrl.Fork(ctx, storyID2, []string{"v1", "v2"}, "")
	if err != nil {
		t.Fatalf("Fork failed: %v", err)
	}
	guard2.Dismiss()
	if err := guard2.Close(); err != nil {
		t.Fatalf("guard2.Close failed: %v", err)
	}

	// Worktree directory should still exist
	storyDir2 := filepath.Join(worktreesDir, storyID2)
	if _, err := os.Stat(storyDir2); err != nil {
		t.Errorf("expected story worktrees dir to exist after dismissed close: %v", err)
	}
}

func TestForkController_AbortDirect(t *testing.T) {
	repoDir := initTestGitRepo(t)
	tmpDir := t.TempDir()
	worktreesDir := filepath.Join(tmpDir, "worktrees")
	ctrl := NewForkController(repoDir, worktreesDir, nil, nil, nil)
	ctx := context.Background()

	// Abort on nonexistent story cleans dir directly and returns nil
	if err := ctrl.Abort(ctx, "nonexistent-story-abort"); err != nil {
		t.Fatalf("unexpected error on abort nonexistent: %v", err)
	}

	// Abort on active story
	storyID := "story-abort-active"
	_, guard, err := ctrl.Fork(ctx, storyID, []string{"v1", "v2"}, "")
	if err != nil {
		t.Fatalf("Fork failed: %v", err)
	}
	guard.Dismiss()

	if err := ctrl.Abort(ctx, storyID); err != nil {
		t.Fatalf("Abort failed: %v", err)
	}

	storyDir := filepath.Join(worktreesDir, storyID)
	if _, err := os.Stat(storyDir); !os.IsNotExist(err) {
		t.Errorf("expected story directory to be removed after Abort")
	}
}
