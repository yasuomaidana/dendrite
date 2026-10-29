package fork

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBenchmarkVariant_NilAndMissingPath(t *testing.T) {
	harness := NewDefaultBenchmarkHarness()
	ctx := context.Background()

	// Nil variant
	_, err := harness.BenchmarkVariant(ctx, nil, BenchmarkOptions{})
	if err == nil {
		t.Fatal("expected error on nil variant, got nil")
	}

	// Missing worktree path returns metric with Error
	missingVariant := &Variant{
		Name:         "missing-var",
		WorktreePath: filepath.Join(t.TempDir(), "nonexistent"),
	}
	res, err := harness.BenchmarkVariant(ctx, missingVariant, BenchmarkOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Error == "" {
		t.Fatal("expected metric Error for nonexistent worktree path")
	}
}

func TestBenchmarkVariant_ContextCancelled(t *testing.T) {
	harness := NewDefaultBenchmarkHarness()
	dir := t.TempDir()
	v := &Variant{Name: "cancel-var", WorktreePath: dir}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := harness.BenchmarkVariant(ctx, v, BenchmarkOptions{Iterations: 100})
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}

func TestEvaluateScorecard_EdgeCases(t *testing.T) {
	// Empty metrics
	scEmpty := EvaluateScorecard("story-empty", "c0", map[string]*VariantMetrics{})
	if scEmpty.Reason != "No variant metrics available" {
		t.Errorf("unexpected reason for empty metrics: %s", scEmpty.Reason)
	}

	// All errored metrics
	metricsAllError := map[string]*VariantMetrics{
		"v1": {Variant: "v1", Error: "timeout"},
		"v2": {Variant: "v2", Error: "crash"},
	}
	scAllError := EvaluateScorecard("story-err", "c0", metricsAllError)
	if scAllError.RecommendedWinner == "" {
		t.Fatal("expected fallback recommended winner when all variants error")
	}
	if scAllError.Reason != "Single candidate available" {
		t.Errorf("unexpected reason: %s", scAllError.Reason)
	}
}

func TestGitExecutor_CleanLocksAndCommands(t *testing.T) {
	repoDir := initTestGitRepo(t)
	executor := NewOSGitExecutor()
	ctx := context.Background()

	// 1. Create stale locks
	gitDir := filepath.Join(repoDir, ".git")
	indexLock := filepath.Join(gitDir, "index.lock")
	if err := os.WriteFile(indexLock, []byte("lock"), 0644); err != nil {
		t.Fatalf("failed to write index.lock: %v", err)
	}

	wtLocksDir := filepath.Join(gitDir, "worktrees", "wt1")
	if err := os.MkdirAll(wtLocksDir, 0755); err != nil {
		t.Fatalf("failed to mkdir wt1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtLocksDir, "locked"), []byte("locked"), 0644); err != nil {
		t.Fatalf("failed to write locked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtLocksDir, "index.lock"), []byte("sub-lock"), 0644); err != nil {
		t.Fatalf("failed to write wt1/index.lock: %v", err)
	}

	if err := executor.CleanLocks(ctx, repoDir); err != nil {
		t.Fatalf("CleanLocks failed: %v", err)
	}

	if _, err := os.Stat(indexLock); !os.IsNotExist(err) {
		t.Error("expected index.lock to be removed")
	}

	// 2. WorktreeList
	worktrees, err := executor.WorktreeList(ctx, repoDir)
	if err != nil {
		t.Fatalf("WorktreeList failed: %v", err)
	}
	if len(worktrees) == 0 {
		t.Fatal("expected at least 1 worktree from main repo")
	}

	// 3. BranchDelete non-force
	// Create branch first
	if _, err := executor.RunGit(ctx, repoDir, "branch", "branch-to-del"); err != nil {
		t.Fatalf("failed creating branch: %v", err)
	}
	if err := executor.BranchDelete(ctx, repoDir, "branch-to-del", false); err != nil {
		t.Fatalf("BranchDelete failed: %v", err)
	}

	// 4. RunGit error on unknown command
	if _, err := executor.RunGit(ctx, repoDir, "unsupported-command"); err == nil {
		t.Fatal("expected error on unsupported git command, got nil")
	}
}

func TestForkController_MissingStoryAndCorruptManifest(t *testing.T) {
	repoDir := initTestGitRepo(t)
	tmpDir := t.TempDir()
	worktreesDir := filepath.Join(tmpDir, "worktrees")
	ctrl := NewForkController(repoDir, worktreesDir, nil, nil, nil)
	ctx := context.Background()

	// Benchmark on non-existent story
	_, err := ctrl.Benchmark(ctx, "nonexistent-story", BenchmarkOptions{})
	if err == nil {
		t.Fatal("expected error benchmarking non-existent story")
	}

	// Resolve on non-existent story
	_, err = ctrl.Resolve(ctx, "nonexistent-story", "v1")
	if err == nil {
		t.Fatal("expected error resolving non-existent story")
	}

	// Corrupt manifest file
	storyID := "corrupt-story"
	manifestDir := filepath.Join(worktreesDir, storyID)
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatalf("failed creating manifest dir: %v", err)
	}
	manifestPath := filepath.Join(manifestDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("not-valid-json"), 0644); err != nil {
		t.Fatalf("failed writing corrupt manifest: %v", err)
	}

	if _, err := ctrl.GetManifest(storyID); err == nil {
		t.Fatal("expected error reading corrupt manifest, got nil")
	}
}

func TestForkController_BenchmarkWithFailingVariant(t *testing.T) {
	repoDir := initTestGitRepo(t)
	tmpDir := t.TempDir()
	worktreesDir := filepath.Join(tmpDir, "worktrees")
	ctrl := NewForkController(repoDir, worktreesDir, nil, nil, nil)
	ctx := context.Background()

	storyID := "story-bench-fail"
	manifest, guard, err := ctrl.Fork(ctx, storyID, []string{"var-good", "var-bad"}, "")
	if err != nil {
		t.Fatalf("Fork failed: %v", err)
	}
	defer guard.Close()

	// Delete var-bad directory to simulate benchmark probe failure on that variant
	for _, v := range manifest.Variants {
		if v.Name == "var-bad" {
			_ = os.RemoveAll(v.WorktreePath)
		}
	}

	opts := BenchmarkOptions{Iterations: 5, VirtualUsers: 1}
	scorecard, err := ctrl.Benchmark(ctx, storyID, opts)
	if err != nil {
		t.Fatalf("Benchmark should succeed even if a variant errors: %v", err)
	}

	if scorecard.RecommendedWinner != "var-good" {
		t.Errorf("expected var-good to win over failing variant, got %s", scorecard.RecommendedWinner)
	}
	if scorecard.Metrics["var-bad"] == nil || scorecard.Metrics["var-bad"].Error == "" {
		t.Error("expected var-bad to have recorded an error in scorecard")
	}
}
