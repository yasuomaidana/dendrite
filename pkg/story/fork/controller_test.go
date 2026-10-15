package fork

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// MockStore implements PersistentStore in-memory for testing.
type MockStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func NewMockStore() *MockStore {
	return &MockStore{data: make(map[string][]byte)}
}

func (s *MockStore) Put(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = data
	return nil
}

func (s *MockStore) Get(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data[key]
	return d, ok
}

// MockGitExecutor implements GitExecutor with recorded calls and programmable responses.
type MockGitExecutor struct {
	mu               sync.Mutex
	branches         map[string]string
	worktrees        []string
	mergedBranch     string
	deletedBranches  []string
	removedWorktrees []string
	pruned           bool
	cleanedLocks     bool
	failOnOp         string
}

func NewMockGitExecutor() *MockGitExecutor {
	return &MockGitExecutor{
		branches: map[string]string{"HEAD": "commit-root-123", "main": "commit-root-123"},
	}
}

func (m *MockGitExecutor) RunGit(ctx context.Context, dir string, args ...string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOnOp == "run_git" {
		return "", fmt.Errorf("simulated run git error")
	}
	return "ok", nil
}

func (m *MockGitExecutor) WorktreeAdd(ctx context.Context, repoDir, branch, path, commitish string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOnOp == "worktree_add" {
		return fmt.Errorf("simulated worktree add error")
	}
	m.worktrees = append(m.worktrees, path)
	m.branches[branch] = commitish
	return os.MkdirAll(path, 0755)
}

func (m *MockGitExecutor) WorktreeRemove(ctx context.Context, repoDir, path string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOnOp == "worktree_remove" {
		return fmt.Errorf("simulated worktree remove error")
	}
	m.removedWorktrees = append(m.removedWorktrees, path)
	return os.RemoveAll(path)
}

func (m *MockGitExecutor) WorktreePrune(ctx context.Context, repoDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruned = true
	return nil
}

func (m *MockGitExecutor) WorktreeList(ctx context.Context, repoDir string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.worktrees, nil
}

func (m *MockGitExecutor) BranchDelete(ctx context.Context, repoDir, branch string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletedBranches = append(m.deletedBranches, branch)
	delete(m.branches, branch)
	return nil
}

func (m *MockGitExecutor) MergeBranch(ctx context.Context, repoDir, branch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOnOp == "merge_branch" {
		return fmt.Errorf("simulated merge conflict")
	}
	m.mergedBranch = branch
	return nil
}

func (m *MockGitExecutor) GetCurrentBranch(ctx context.Context, repoDir string) (string, error) {
	return "main", nil
}

func (m *MockGitExecutor) GetHeadCommit(ctx context.Context, repoDir string) (string, error) {
	return "commit-head-abc", nil
}

func (m *MockGitExecutor) CleanLocks(ctx context.Context, repoDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanedLocks = true
	return nil
}

// TestForkControllerUnit tests controller orchestration and edge cases with mocks.
func TestForkControllerUnit(t *testing.T) {
	tmpDir := t.TempDir()
	worktreesDir := filepath.Join(tmpDir, "worktrees")
	executor := NewMockGitExecutor()
	store := NewMockStore()
	harness := NewDefaultBenchmarkHarness()

	ctrl := NewForkController(tmpDir, worktreesDir, executor, harness, store)
	ctx := context.Background()

	// 1. Validation checks
	t.Run("Validation_EmptyStory", func(t *testing.T) {
		_, _, err := ctrl.Fork(ctx, "", []string{"v1", "v2"}, "")
		if err == nil {
			t.Fatal("expected error on empty story ID")
		}
	})

	t.Run("Validation_InsufficientVariants", func(t *testing.T) {
		_, _, err := ctrl.Fork(ctx, "story-1", []string{"v1"}, "")
		if err == nil {
			t.Fatal("expected error on single variant")
		}
		_, _, err = ctrl.Fork(ctx, "story-1", []string{"v1", "v1"}, "")
		if err == nil {
			t.Fatal("expected error on duplicate variants")
		}
	})

	// 2. Successful Fork
	t.Run("Fork_Success", func(t *testing.T) {
		manifest, guard, err := ctrl.Fork(ctx, "story-101", []string{"astar-bitvec", "bidirectional-dijkstra"}, "")
		if err != nil {
			t.Fatalf("unexpected fork error: %v", err)
		}
		if manifest.Status != StatusActive {
			t.Errorf("expected StatusActive, got %s", manifest.Status)
		}
		if len(manifest.Variants) != 2 {
			t.Fatalf("expected 2 variants, got %d", len(manifest.Variants))
		}
		if manifest.BaseBranch != "main" {
			t.Errorf("expected base branch main, got %s", manifest.BaseBranch)
		}
		guard.Dismiss() // Dismiss so it doesn't rollback
	})

	// 3. Duplicate active fork rejected
	t.Run("Fork_AlreadyActive", func(t *testing.T) {
		_, _, err := ctrl.Fork(ctx, "story-101", []string{"v1", "v2"}, "")
		if err == nil {
			t.Fatal("expected error on already active fork")
		}
	})

	// 4. Benchmark Execution
	t.Run("Benchmark_Success", func(t *testing.T) {
		opts := BenchmarkOptions{Iterations: 10, VirtualUsers: 2}
		scorecard, err := ctrl.Benchmark(ctx, "story-101", opts)
		if err != nil {
			t.Fatalf("benchmark failed: %v", err)
		}
		if scorecard.RecommendedWinner == "" {
			t.Fatal("expected a recommended winner")
		}
		if len(scorecard.Metrics) != 2 {
			t.Errorf("expected 2 variant metrics, got %d", len(scorecard.Metrics))
		}

		manifest, err := ctrl.GetManifest("story-101")
		if err != nil {
			t.Fatalf("failed getting manifest: %v", err)
		}
		if manifest.Status != StatusBenchmarked {
			t.Errorf("expected StatusBenchmarked, got %s", manifest.Status)
		}
	})

	// 5. Resolution
	t.Run("Resolve_InvalidWinner", func(t *testing.T) {
		_, err := ctrl.Resolve(ctx, "story-101", "non-existent")
		if err == nil {
			t.Fatal("expected error on non-existent winner")
		}
	})

	t.Run("Resolve_Success", func(t *testing.T) {
		res, err := ctrl.Resolve(ctx, "story-101", "astar-bitvec")
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if res.Winner != "astar-bitvec" {
			t.Errorf("expected winner astar-bitvec, got %s", res.Winner)
		}
		if len(res.PurgedVariants) != 1 || res.PurgedVariants[0] != "bidirectional-dijkstra" {
			t.Errorf("expected purged variants [bidirectional-dijkstra], got %v", res.PurgedVariants)
		}
		if executor.mergedBranch != "story/story-101/astar-bitvec" {
			t.Errorf("expected merged branch story/story-101/astar-bitvec, got %s", executor.mergedBranch)
		}

		// Check store persistence
		stored, ok := store.Get(res.StoreKey)
		if !ok || len(stored) == 0 {
			t.Error("expected benchmark data persisted in store")
		}

		// Check manifest updated
		manifest, err := ctrl.GetManifest("story-101")
		if err != nil {
			t.Fatalf("failed getting manifest: %v", err)
		}
		if manifest.Status != StatusResolved {
			t.Errorf("expected StatusResolved, got %s", manifest.Status)
		}
	})

	t.Run("Resolve_AlreadyResolved", func(t *testing.T) {
		_, err := ctrl.Resolve(ctx, "story-101", "astar-bitvec")
		if err == nil {
			t.Fatal("expected error on already resolved story")
		}
	})

	// 6. RAII Guard Rollback on failure
	t.Run("RAII_Guard_Rollback", func(t *testing.T) {
		_, guard, err := ctrl.Fork(ctx, "story-temp", []string{"v1", "v2"}, "")
		if err != nil {
			t.Fatalf("failed to fork: %v", err)
		}
		// Close without dismissing triggers rollback
		err = guard.Close()
		if err != nil {
			t.Fatalf("guard close failed: %v", err)
		}

		// Check manifest was aborted and cleaned
		manifest, _ := ctrl.GetManifest("story-temp")
		if manifest != nil && manifest.Status != StatusAborted {
			t.Errorf("expected aborted manifest, got %v", manifest)
		}
	})
}

// TestRealGitIntegration tests actual git worktree commands using OSGitExecutor on a temporary repository.
func TestRealGitIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not found in PATH; skipping live git test")
	}

	repoDir := t.TempDir()
	worktreesDir := filepath.Join(repoDir, ".dendrite", "worktrees")

	// 1. Initialize git repo
	runGitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (out: %s)", args, err, string(out))
		}
	}

	runGitCmd("init")
	runGitCmd("config", "user.name", "Dendrite Bot")
	runGitCmd("config", "user.email", "bot@dendrite.ai")

	// Create initial file & commit
	initialFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(initialFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitCmd("add", "README.md")
	runGitCmd("commit", "-m", "Initial commit")

	executor := NewOSGitExecutor()
	store := NewMockStore()
	harness := NewDefaultBenchmarkHarness()
	ctrl := NewForkController(repoDir, worktreesDir, executor, harness, store)
	ctx := context.Background()

	// 2. Fork into two variants
	storyID := "story-algo-opt"
	manifest, guard, err := ctrl.Fork(ctx, storyID, []string{"variant-a", "variant-b"}, "")
	if err != nil {
		t.Fatalf("failed to fork story in real git: %v", err)
	}
	defer guard.Close()

	if len(manifest.Variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(manifest.Variants))
	}

	// Verify worktree paths exist on filesystem
	for _, v := range manifest.Variants {
		if _, err := os.Stat(v.WorktreePath); err != nil {
			t.Fatalf("worktree path %s does not exist: %v", v.WorktreePath, err)
		}
	}

	// 3. Make a change in variant-a
	varAPath := filepath.Join(worktreesDir, storyID, "variant-a", "algo.txt")
	if err := os.WriteFile(varAPath, []byte("variant A algorithm"), 0644); err != nil {
		t.Fatal(err)
	}
	cmdA := exec.Command("git", "add", "algo.txt")
	cmdA.Dir = filepath.Join(worktreesDir, storyID, "variant-a")
	_ = cmdA.Run()
	cmdACommit := exec.Command("git", "commit", "-m", "Implement variant A")
	cmdACommit.Dir = filepath.Join(worktreesDir, storyID, "variant-a")
	_ = cmdACommit.Run()

	// 4. Run differential benchmark
	scorecard, err := ctrl.Benchmark(ctx, storyID, BenchmarkOptions{Iterations: 20})
	if err != nil {
		t.Fatalf("benchmark failed: %v", err)
	}
	if scorecard == nil || len(scorecard.Metrics) != 2 {
		t.Fatalf("invalid scorecard: %v", scorecard)
	}

	// 5. Test CleanLocks
	fakeLock := filepath.Join(repoDir, ".git", "index.lock")
	_ = os.WriteFile(fakeLock, []byte("lock"), 0644)
	if err := executor.CleanLocks(ctx, repoDir); err != nil {
		t.Errorf("clean locks failed: %v", err)
	}
	if _, err := os.Stat(fakeLock); err == nil {
		t.Error("expected fake lock file to be removed")
	}

	// 6. Resolve to variant-a
	res, err := ctrl.Resolve(ctx, storyID, "variant-a")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	guard.Dismiss()

	if res.Winner != "variant-a" {
		t.Errorf("expected winner variant-a, got %s", res.Winner)
	}

	// Verify merged file exists in main repo
	mergedFile := filepath.Join(repoDir, "algo.txt")
	if _, err := os.Stat(mergedFile); err != nil {
		t.Errorf("expected merged file %s in root repo: %v", mergedFile, err)
	}

	// 7. Test WorktreeList
	wts, err := executor.WorktreeList(ctx, repoDir)
	if err != nil {
		t.Fatalf("worktree list failed: %v", err)
	}
	if len(wts) == 0 {
		t.Error("expected at least main worktree in list")
	}
}

func TestControllerDefaultsAndEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// NewForkController defaults
	ctrlDefault := NewForkController("", "", nil, nil, nil)
	if ctrlDefault.repoDir != "." || ctrlDefault.worktreesBaseDir != ".dendrite/worktrees" {
		t.Errorf("unexpected defaults: %+v", ctrlDefault)
	}

	// BenchmarkVariant nil and invalid path
	harness := NewDefaultBenchmarkHarness()
	_, err := harness.BenchmarkVariant(ctx, nil, BenchmarkOptions{})
	if err == nil {
		t.Error("expected error for nil variant")
	}

	metrics, err := harness.BenchmarkVariant(ctx, &Variant{Name: "v-missing", WorktreePath: "/nonexistent/path/1234"}, BenchmarkOptions{})
	if err != nil || metrics.Error == "" {
		t.Errorf("expected error field in metrics for non-existent path, got: %+v, err: %v", metrics, err)
	}

	// EvaluateScorecard empty metrics and all errored
	emptyCard := EvaluateScorecard("s-empty", "commit-1", nil)
	if emptyCard.Reason != "No variant metrics available" {
		t.Errorf("expected reason for empty metrics, got: %s", emptyCard.Reason)
	}

	erroredMetrics := map[string]*VariantMetrics{
		"v-err": {Variant: "v-err", Error: "some failure"},
	}
	errCard := EvaluateScorecard("s-err", "commit-1", erroredMetrics)
	if errCard.RecommendedWinner != "v-err" {
		t.Errorf("expected fallback winner v-err, got %s", errCard.RecommendedWinner)
	}

	// Fork failure rollback
	execFail := NewMockGitExecutor()
	execFail.failOnOp = "worktree_add"
	ctrlFail := NewForkController(tmpDir, filepath.Join(tmpDir, "wt"), execFail, harness, nil)
	_, _, err = ctrlFail.Fork(ctx, "story-fail", []string{"v1", "v2"}, "")
	if err == nil {
		t.Fatal("expected fork error when worktree_add fails")
	}

	// Abort on non-existent manifest
	err = ctrlDefault.Abort(ctx, "nonexistent-story-xyz")
	if err != nil {
		t.Errorf("unexpected error on abort nonexistent: %v", err)
	}

	// Benchmark on resolved story
	execOk := NewMockGitExecutor()
	ctrlOk := NewForkController(tmpDir, filepath.Join(tmpDir, "wt"), execOk, harness, nil)
	manifest, guard, _ := ctrlOk.Fork(ctx, "story-bench-fail", []string{"v1", "v2"}, "")
	guard.Dismiss()
	_, _ = ctrlOk.Resolve(ctx, "story-bench-fail", "v1")
	_, err = ctrlOk.Benchmark(ctx, "story-bench-fail", BenchmarkOptions{})
	if err == nil {
		t.Error("expected benchmark error on resolved story")
	}

	// Benchmark on non-existent story
	_, err = ctrlOk.Benchmark(ctx, "missing-story", BenchmarkOptions{})
	if err == nil {
		t.Error("expected benchmark error on missing story")
	}
	_ = manifest
}
