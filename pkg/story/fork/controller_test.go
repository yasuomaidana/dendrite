package fork

import (
	"Dendrite/pkg/statedir"
	"Dendrite/pkg/store"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestForkController_ErrorPaths(t *testing.T) {
	repoDir := initTestGitRepo(t)
	tmpDir := t.TempDir()
	worktreesDir := filepath.Join(tmpDir, "worktrees")
	ctrl := NewForkController(repoDir, worktreesDir, nil, nil, nil)
	ctx := context.Background()

	// 1. MkdirAll fails when creating worktrees dir because a regular file exists
	blockedBase := filepath.Join(tmpDir, "blocked_base")
	if err := os.MkdirAll(blockedBase, 0755); err != nil {
		t.Fatal(err)
	}
	blockedFile := filepath.Join(blockedBase, "story-blocked")
	if err := os.WriteFile(blockedFile, []byte("blocking-file"), 0644); err != nil {
		t.Fatal(err)
	}
	ctrlBlocked := NewForkController(repoDir, blockedBase, nil, nil, nil)
	_, _, err := ctrlBlocked.Fork(ctx, "story-blocked", []string{"v1", "v2"}, "")
	if err == nil || !strings.Contains(err.Error(), "failed creating worktrees directory") {
		t.Fatalf("expected failed creating worktrees directory error, got: %v", err)
	}

	// 2. Fork in non-git directory triggers fallback for GetCurrentBranch and GetHeadCommit, then WorktreeAdd fails
	nonGitDir := t.TempDir()
	ctrlNonGit := NewForkController(nonGitDir, filepath.Join(tmpDir, "wt-nongit"), nil, nil, nil)
	_, _, err = ctrlNonGit.Fork(ctx, "story-nongit", []string{"v1", "v2"}, "")
	if err == nil {
		t.Fatal("expected Fork error in non-git directory")
	}

	// 3. Pre-existing branch causes WorktreeAdd failure and triggers rollback
	cmdBranch := exec.Command("git", "branch", "story/story-add-fail/v2")
	cmdBranch.Dir = repoDir
	if out, err := cmdBranch.CombinedOutput(); err != nil {
		t.Fatalf("failed creating pre-existing branch: %v (out: %s)", err, string(out))
	}
	_, _, err = ctrl.Fork(ctx, "story-add-fail", []string{"v1", "v2"}, "")
	if err == nil || !strings.Contains(err.Error(), "failed adding worktree for variant 'v2'") {
		t.Fatalf("expected worktree add failure for v2, got: %v", err)
	}

	// 4. Manifest directory collision causes saveManifestLocked failure and rollback
	saveFailStory := "story-save-fail"
	saveFailDir := filepath.Join(worktreesDir, saveFailStory)
	_ = os.MkdirAll(filepath.Join(saveFailDir, "manifest.json"), 0755)
	_, _, err = ctrl.Fork(ctx, saveFailStory, []string{"v1", "v2"}, "")
	if err == nil || !strings.Contains(err.Error(), "failed saving manifest") {
		t.Fatalf("expected failed saving manifest, got: %v", err)
	}

	// 5. Benchmark on missing story
	_, err = ctrl.Benchmark(ctx, "missing-story-bench", BenchmarkOptions{})
	if err == nil {
		t.Fatal("expected error benchmarking missing story")
	}

	// 6. Benchmark with canceled context captures variant error without aborting
	storyBenchCtx := "story-bench-ctx"
	_, guardCtx, err := ctrl.Fork(ctx, storyBenchCtx, []string{"v1", "v2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer guardCtx.Close()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	card, err := ctrl.Benchmark(canceledCtx, storyBenchCtx, BenchmarkOptions{})
	if err != nil {
		t.Fatalf("expected benchmark to capture variant errors without failing, got: %v", err)
	}
	if card.Reason != "Single candidate available" {
		t.Errorf("expected Single candidate available for all errored variants, got: %s", card.Reason)
	}

	// 7. Benchmark save manifest failure
	storyBenchSave := "story-bench-save"
	_, guardSave, err := ctrl.Fork(ctx, storyBenchSave, []string{"v1", "v2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer guardSave.Close()
	benchManifestPath := filepath.Join(worktreesDir, storyBenchSave, "manifest.json")
	_ = os.Chmod(benchManifestPath, 0444)
	_, err = ctrl.Benchmark(ctx, storyBenchSave, BenchmarkOptions{})
	_ = os.Chmod(benchManifestPath, 0644)
	if err == nil || !strings.Contains(err.Error(), "failed updating manifest with scorecard") {
		t.Fatalf("expected failed updating manifest with scorecard, got: %v", err)
	}

	// 8. Resolve on missing story
	_, err = ctrl.Resolve(ctx, "missing-story-resolve", "v1")
	if err == nil {
		t.Fatal("expected error resolving missing story")
	}

	// 9. Resolve with merge conflict
	storyConflict := "story-conflict-merge"
	manifestConflict, guardConflict, err := ctrl.Fork(ctx, storyConflict, []string{"va", "vb"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer guardConflict.Close()
	vaPath := manifestConflict.Variants[0].WorktreePath
	conflictFileVA := filepath.Join(vaPath, "conflict.txt")
	_ = os.WriteFile(conflictFileVA, []byte("variant content"), 0644)
	cmdA := exec.Command("git", "add", "conflict.txt")
	cmdA.Dir = vaPath
	_ = cmdA.Run()
	cmdAC := exec.Command("git", "commit", "-m", "conflict variant commit")
	cmdAC.Dir = vaPath
	_ = cmdAC.Run()

	conflictFileMain := filepath.Join(repoDir, "conflict.txt")
	_ = os.WriteFile(conflictFileMain, []byte("main content"), 0644)
	cmdM := exec.Command("git", "add", "conflict.txt")
	cmdM.Dir = repoDir
	_ = cmdM.Run()
	cmdMC := exec.Command("git", "commit", "-m", "conflict main commit")
	cmdMC.Dir = repoDir
	_ = cmdMC.Run()

	_, err = ctrl.Resolve(ctx, storyConflict, "va")
	if err == nil || !strings.Contains(err.Error(), "failed merging winning branch") {
		t.Fatalf("expected merge conflict error, got: %v", err)
	}
	cmdAbort := exec.Command("git", "merge", "--abort")
	cmdAbort.Dir = repoDir
	_ = cmdAbort.Run()

	// 10. Resolve with nil store
	storyNoStore := "story-no-store"
	ctrlNoStore := NewForkController(repoDir, worktreesDir, nil, nil, nil)
	_, guardNoStore, err := ctrlNoStore.Fork(ctx, storyNoStore, []string{"v1", "v2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer guardNoStore.Close()
	resNoStore, err := ctrlNoStore.Resolve(ctx, storyNoStore, "v1")
	if err != nil {
		t.Fatalf("Resolve with nil store failed: %v", err)
	}
	if resNoStore.Winner != "v1" {
		t.Errorf("expected winner v1, got %s", resNoStore.Winner)
	}
	guardNoStore.Dismiss()

	// 11. loadManifest with corrupt JSON
	corruptStory := "story-corrupt"
	corruptDir := filepath.Join(worktreesDir, corruptStory)
	_ = os.MkdirAll(corruptDir, 0755)
	_ = os.WriteFile(filepath.Join(corruptDir, "manifest.json"), []byte("{invalid-json"), 0644)
	_, err = ctrl.GetManifest(corruptStory)
	if err == nil {
		t.Fatal("expected unmarshal error on corrupt manifest")
	}

	// 12. saveManifestLocked directory creation error
	saveBlockedDir := filepath.Join(tmpDir, "save_blocked")
	_ = os.WriteFile(saveBlockedDir, []byte("blocking-file"), 0644)
	ctrlSaveBlocked := NewForkController(repoDir, saveBlockedDir, nil, nil, nil)
	err = ctrlSaveBlocked.saveManifestLocked(&ForkManifest{StoryID: "blocked-story"})
	if err == nil {
		t.Fatal("expected error when saving manifest to blocked directory")
	}
}

func TestBenchmarkHarness_Direct(t *testing.T) {
	ctx := context.Background()
	harness := NewDefaultBenchmarkHarness()

	// 1. Nil variant
	_, err := harness.BenchmarkVariant(ctx, nil, BenchmarkOptions{})
	if err == nil {
		t.Fatal("expected error on nil variant")
	}

	// 2. Nonexistent worktree path
	metrics, err := harness.BenchmarkVariant(ctx, &Variant{Name: "v-missing", WorktreePath: "/nonexistent/path/99999"}, BenchmarkOptions{})
	if err != nil || metrics == nil || metrics.Error == "" {
		t.Fatalf("expected metrics with error on missing path, got: %+v, err: %v", metrics, err)
	}

	// 3. Canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	tmpDir := t.TempDir()
	_, err = harness.BenchmarkVariant(canceledCtx, &Variant{Name: "v-cancel", WorktreePath: tmpDir}, BenchmarkOptions{})
	if err == nil {
		t.Fatal("expected error on canceled context")
	}

	// 4. Real path with files, default iterations and vus
	sampleFile := filepath.Join(tmpDir, "sample.txt")
	_ = os.WriteFile(sampleFile, []byte("hello benchmark world"), 0644)
	subDir := filepath.Join(tmpDir, "subdir")
	_ = os.MkdirAll(subDir, 0755)
	_ = os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested content"), 0644)

	m1, err := harness.BenchmarkVariant(ctx, &Variant{Name: "v-real", WorktreePath: tmpDir}, BenchmarkOptions{Iterations: 0, VirtualUsers: 0})
	if err != nil {
		t.Fatalf("BenchmarkVariant failed: %v", err)
	}
	if m1.Error != "" || m1.TPS <= 0 || m1.MemoryMB <= 0 {
		t.Fatalf("unexpected metrics values: %+v", m1)
	}

	// 5. Empty directory path (fileCount == 0)
	emptyDir := t.TempDir()
	m2, err := harness.BenchmarkVariant(ctx, &Variant{Name: "v-empty", WorktreePath: emptyDir}, BenchmarkOptions{Iterations: 5, VirtualUsers: 1})
	if err != nil {
		t.Fatalf("BenchmarkVariant on empty dir failed: %v", err)
	}
	if m2.Error != "" {
		t.Fatalf("expected no error on empty dir metrics: %+v", m2)
	}

	// 6. EvaluateScorecard empty metrics
	emptyCard := EvaluateScorecard("s-empty", "commit-1", nil)
	if emptyCard.Reason != "No variant metrics available" {
		t.Errorf("expected 'No variant metrics available', got: %s", emptyCard.Reason)
	}

	// 7. EvaluateScorecard all errored metrics
	errMetrics := map[string]*VariantMetrics{
		"v-err": {Variant: "v-err", Error: "some failure"},
	}
	errCard := EvaluateScorecard("s-err", "commit-1", errMetrics)
	if errCard.RecommendedWinner != "v-err" || errCard.Reason != "Single candidate available" {
		t.Errorf("unexpected fallback scorecard: %+v", errCard)
	}

	// 8. EvaluateScorecard with zero TTFT and MemoryMB bounds
	zeroBoundsMetrics := map[string]*VariantMetrics{
		"v-zero": {
			Variant:  "v-zero",
			TTFT:     0,
			MemoryMB: 0,
			TPS:      50.0,
		},
	}
	zeroCard := EvaluateScorecard("s-zero", "commit-1", zeroBoundsMetrics)
	if zeroCard.RecommendedWinner != "v-zero" {
		t.Errorf("expected winner v-zero, got %s", zeroCard.RecommendedWinner)
	}

	// 9. EvaluateScorecard competitive comparison
	compMetrics := map[string]*VariantMetrics{
		"v-low": {
			Variant:  "v-low",
			TTFT:     100 * time.Millisecond,
			MemoryMB: 50.0,
			TPS:      10.0,
		},
		"v-high": {
			Variant:  "v-high",
			TTFT:     5 * time.Millisecond,
			MemoryMB: 2.0,
			TPS:      500.0,
		},
	}
	compCard := EvaluateScorecard("s-comp", "commit-1", compMetrics)
	if compCard.RecommendedWinner != "v-high" {
		t.Errorf("expected winner v-high, got %s", compCard.RecommendedWinner)
	}
	if !strings.Contains(compCard.Reason, "Variant 'v-high' outperformed") {
		t.Errorf("unexpected scorecard reason: %s", compCard.Reason)
	}
}

func TestOSGitExecutor_Direct(t *testing.T) {
	repoDir := initTestGitRepo(t)
	executor := NewOSGitExecutor()
	ctx := context.Background()

	// 1. RunGit with empty directory and version check
	ver, err := executor.RunGit(ctx, "", "version")
	if err != nil || !strings.Contains(ver, "git version") {
		t.Fatalf("RunGit with empty dir failed: %v (out: %s)", err, ver)
	}

	// 2. RunGit invalid command
	_, err = executor.RunGit(ctx, repoDir, "invalid-command-xyz")
	if err == nil {
		t.Fatal("expected error on invalid git command")
	}

	// 3. GetCurrentBranch and GetHeadCommit
	branch, err := executor.GetCurrentBranch(ctx, repoDir)
	if err != nil || branch != "main" {
		t.Fatalf("GetCurrentBranch failed: %v (branch: %s)", err, branch)
	}
	commit, err := executor.GetHeadCommit(ctx, repoDir)
	if err != nil || len(commit) < 7 {
		t.Fatalf("GetHeadCommit failed: %v (commit: %s)", err, commit)
	}

	// Errors on non-git dir
	emptyDir := t.TempDir()
	if _, err := executor.GetCurrentBranch(ctx, emptyDir); err == nil {
		t.Fatal("expected error getting branch on empty non-git dir")
	}
	if _, err := executor.GetHeadCommit(ctx, emptyDir); err == nil {
		t.Fatal("expected error getting head commit on empty non-git dir")
	}

	// 4. WorktreeAdd variants
	wtBase := t.TempDir()
	// branch != "" && commitish != ""
	wtPath1 := filepath.Join(wtBase, "wt1")
	if err := executor.WorktreeAdd(ctx, repoDir, "branch-wt1", wtPath1, "HEAD"); err != nil {
		t.Fatalf("WorktreeAdd branch+commitish failed: %v", err)
	}

	// branch != "" && commitish == ""
	wtPath2 := filepath.Join(wtBase, "wt2")
	if err := executor.WorktreeAdd(ctx, repoDir, "branch-wt2", wtPath2, ""); err != nil {
		t.Fatalf("WorktreeAdd branch without commitish failed: %v", err)
	}

	// branch == "" && commitish != "" (checkout existing branch)
	cmdBr3 := exec.Command("git", "branch", "branch-wt3")
	cmdBr3.Dir = repoDir
	_ = cmdBr3.Run()
	wtPath3 := filepath.Join(wtBase, "wt3")
	if err := executor.WorktreeAdd(ctx, repoDir, "", wtPath3, "branch-wt3"); err != nil {
		t.Fatalf("WorktreeAdd commitish without branch flag failed: %v", err)
	}

	// branch == "" && commitish == "" (checkout branch named after directory)
	cmdBr4 := exec.Command("git", "branch", "branch-wt4")
	cmdBr4.Dir = repoDir
	_ = cmdBr4.Run()
	wtPath4 := filepath.Join(wtBase, "branch-wt4")
	if err := executor.WorktreeAdd(ctx, repoDir, "", wtPath4, ""); err != nil {
		t.Fatalf("WorktreeAdd without branch and commitish failed: %v", err)
	}

	// 5. WorktreeList
	wts, err := executor.WorktreeList(ctx, repoDir)
	if err != nil {
		t.Fatalf("WorktreeList failed: %v", err)
	}
	if len(wts) < 2 {
		t.Errorf("expected multiple worktrees, got %v", wts)
	}

	// WorktreeList error on invalid dir
	if _, err := executor.WorktreeList(ctx, emptyDir); err == nil {
		t.Fatal("expected error listing worktrees on non-git dir")
	}

	// 6. WorktreeRemove with force=false and force=true
	if err := executor.WorktreeRemove(ctx, repoDir, wtPath1, false); err != nil {
		t.Fatalf("WorktreeRemove force=false failed: %v", err)
	}
	if err := executor.WorktreeRemove(ctx, repoDir, wtPath2, true); err != nil {
		t.Fatalf("WorktreeRemove force=true failed: %v", err)
	}
	_ = executor.WorktreeRemove(ctx, repoDir, wtPath3, true)
	_ = executor.WorktreeRemove(ctx, repoDir, wtPath4, true)

	// 7. WorktreePrune
	if err := executor.WorktreePrune(ctx, repoDir); err != nil {
		t.Fatalf("WorktreePrune failed: %v", err)
	}

	// 8. BranchDelete with force=false and force=true
	if err := executor.BranchDelete(ctx, repoDir, "branch-wt1", false); err != nil {
		t.Fatalf("BranchDelete force=false failed: %v", err)
	}
	if err := executor.BranchDelete(ctx, repoDir, "branch-wt2", true); err != nil {
		t.Fatalf("BranchDelete force=true failed: %v", err)
	}
	_ = executor.BranchDelete(ctx, repoDir, "branch-wt3", true)
	_ = executor.BranchDelete(ctx, repoDir, "branch-wt4", true)

	// 9. MergeBranch
	cmdBrMerge := exec.Command("git", "checkout", "-b", "branch-to-merge")
	cmdBrMerge.Dir = repoDir
	_ = cmdBrMerge.Run()
	mergeFile := filepath.Join(repoDir, "merged_item.txt")
	_ = os.WriteFile(mergeFile, []byte("data to merge"), 0644)
	cmdAddMerge := exec.Command("git", "add", "merged_item.txt")
	cmdAddMerge.Dir = repoDir
	_ = cmdAddMerge.Run()
	cmdCommitMerge := exec.Command("git", "commit", "-m", "Commit merge item")
	cmdCommitMerge.Dir = repoDir
	_ = cmdCommitMerge.Run()

	cmdCheckoutMain := exec.Command("git", "checkout", "main")
	cmdCheckoutMain.Dir = repoDir
	_ = cmdCheckoutMain.Run()

	if err := executor.MergeBranch(ctx, repoDir, "branch-to-merge"); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if _, err := os.Stat(mergeFile); err != nil {
		t.Errorf("expected merged file to exist in main: %v", err)
	}

	// 10. CleanLocks
	gitDir := filepath.Join(repoDir, ".git")
	staleLock := filepath.Join(gitDir, "index.lock")
	_ = os.WriteFile(staleLock, []byte("lock content"), 0644)

	wtDir := filepath.Join(gitDir, "worktrees")
	subWtDir := filepath.Join(wtDir, "subwt1")
	_ = os.MkdirAll(subWtDir, 0755)
	subLocked := filepath.Join(subWtDir, "locked")
	_ = os.WriteFile(subLocked, []byte("sub lock"), 0644)
	subIdxLock := filepath.Join(subWtDir, "index.lock")
	_ = os.WriteFile(subIdxLock, []byte("sub idx lock"), 0644)

	_ = os.WriteFile(filepath.Join(wtDir, "regular_file"), []byte("non-dir"), 0644)

	if err := executor.CleanLocks(ctx, repoDir); err != nil {
		t.Fatalf("CleanLocks failed: %v", err)
	}

	if _, err := os.Stat(staleLock); err == nil {
		t.Errorf("expected index.lock to be removed by CleanLocks")
	}
	if _, err := os.Stat(subLocked); err == nil {
		t.Errorf("expected sub locked to be removed by CleanLocks")
	}
	if _, err := os.Stat(subIdxLock); err == nil {
		t.Errorf("expected sub index.lock to be removed by CleanLocks")
	}
}
