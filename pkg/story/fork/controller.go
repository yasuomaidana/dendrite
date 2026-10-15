package fork

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PersistentStore models the contract for persisting benchmark results in the Dendrite store.
type PersistentStore interface {
	Put(key string, data []byte) error
}

// ForkController coordinates speculative story branching, worktree lifecycles, and differential benchmarks.
type ForkController struct {
	repoDir          string
	worktreesBaseDir string
	executor         GitExecutor
	harness          BenchmarkHarness
	store            PersistentStore
	mu               sync.RWMutex
}

// NewForkController constructs a new ForkController.
func NewForkController(
	repoDir string,
	worktreesBaseDir string,
	executor GitExecutor,
	harness BenchmarkHarness,
	store PersistentStore,
) *ForkController {
	if repoDir == "" {
		repoDir = "."
	}
	if worktreesBaseDir == "" {
		worktreesBaseDir = ".dendrite/worktrees"
	}
	if executor == nil {
		executor = NewOSGitExecutor()
	}
	if harness == nil {
		harness = NewDefaultBenchmarkHarness()
	}

	return &ForkController{
		repoDir:          repoDir,
		worktreesBaseDir: worktreesBaseDir,
		executor:         executor,
		harness:          harness,
		store:            store,
	}
}

// Fork creates isolated Git worktrees and branches for competing story implementation variants.
func (c *ForkController) Fork(
	ctx context.Context,
	storyID string,
	variants []string,
	baseCommit string,
) (*ForkManifest, *WorktreeGuard, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cleanedStoryID := strings.TrimSpace(storyID)
	if cleanedStoryID == "" {
		return nil, nil, fmt.Errorf("story ID cannot be empty")
	}

	// Deduplicate and validate variants
	cleanedVariants := make([]string, 0, len(variants))
	seen := make(map[string]bool)
	for _, v := range variants {
		item := strings.TrimSpace(v)
		if item != "" && !seen[item] {
			seen[item] = true
			cleanedVariants = append(cleanedVariants, item)
		}
	}

	if len(cleanedVariants) < 2 {
		return nil, nil, fmt.Errorf("at least 2 distinct variants are required to fork a story, got %d", len(cleanedVariants))
	}

	// Check if story already has active worktrees
	manifestPath := c.manifestPath(cleanedStoryID)
	if _, err := os.Stat(manifestPath); err == nil {
		existing, readErr := c.loadManifest(cleanedStoryID)
		if readErr == nil && existing.Status == StatusActive {
			return nil, nil, fmt.Errorf("story '%s' already has an active fork", cleanedStoryID)
		}
	}

	// Resolve base branch and commit
	baseBranch, err := c.executor.GetCurrentBranch(ctx, c.repoDir)
	if err != nil {
		baseBranch = "main"
	}
	if baseCommit == "" {
		baseCommit, err = c.executor.GetHeadCommit(ctx, c.repoDir)
		if err != nil {
			baseCommit = "HEAD"
		}
	}

	storyWorktreesDir := filepath.Join(c.worktreesBaseDir, cleanedStoryID)
	if err := os.MkdirAll(storyWorktreesDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed creating worktrees directory: %w", err)
	}

	manifest := &ForkManifest{
		StoryID:    cleanedStoryID,
		BaseBranch: baseBranch,
		BaseCommit: baseCommit,
		Variants:   make([]*Variant, 0, len(cleanedVariants)),
		Status:     StatusActive,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	guard := NewWorktreeGuard(c, cleanedStoryID, cleanedVariants)

	for _, vName := range cleanedVariants {
		branchName := fmt.Sprintf("story/%s/%s", cleanedStoryID, vName)
		worktreePath := filepath.Join(storyWorktreesDir, vName)

		// Create worktree via git executor
		if err := c.executor.WorktreeAdd(ctx, c.repoDir, branchName, worktreePath, baseCommit); err != nil {
			_ = guard.Rollback(ctx)
			return nil, nil, fmt.Errorf("failed adding worktree for variant '%s': %w", vName, err)
		}

		manifest.Variants = append(manifest.Variants, &Variant{
			Name:         vName,
			Branch:       branchName,
			WorktreePath: worktreePath,
			CreatedAt:    time.Now().UTC(),
		})
	}

	if err := c.saveManifestLocked(manifest); err != nil {
		_ = guard.Rollback(ctx)
		return nil, nil, fmt.Errorf("failed saving manifest: %w", err)
	}

	return manifest, guard, nil
}

// Benchmark executes differential performance benchmarks across all variants of a story.
func (c *ForkController) Benchmark(
	ctx context.Context,
	storyID string,
	opts BenchmarkOptions,
) (*ComparativeScorecard, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	manifest, err := c.loadManifest(storyID)
	if err != nil {
		return nil, fmt.Errorf("failed loading manifest for story '%s': %w", storyID, err)
	}

	if manifest.Status != StatusActive && manifest.Status != StatusBenchmarked {
		return nil, fmt.Errorf("story '%s' is in state %s; cannot benchmark", storyID, manifest.Status)
	}

	metricsMap := make(map[string]*VariantMetrics)
	for _, v := range manifest.Variants {
		metrics, benchErr := c.harness.BenchmarkVariant(ctx, v, opts)
		if benchErr != nil {
			metricsMap[v.Name] = &VariantMetrics{
				Variant: v.Name,
				Error:   benchErr.Error(),
			}
		} else {
			metricsMap[v.Name] = metrics
		}
	}

	scorecard := EvaluateScorecard(storyID, manifest.BaseCommit, metricsMap)
	manifest.Scorecard = scorecard
	manifest.Status = StatusBenchmarked
	manifest.UpdatedAt = time.Now().UTC()

	if err := c.saveManifestLocked(manifest); err != nil {
		return nil, fmt.Errorf("failed updating manifest with scorecard: %w", err)
	}

	return scorecard, nil
}

// Resolve merges the winning variant, persists benchmark metrics in store, and purges alternates.
func (c *ForkController) Resolve(
	ctx context.Context,
	storyID string,
	winner string,
) (*ResolutionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cleanedStoryID := strings.TrimSpace(storyID)
	cleanedWinner := strings.TrimSpace(winner)

	manifest, err := c.loadManifest(cleanedStoryID)
	if err != nil {
		return nil, fmt.Errorf("failed loading manifest for story '%s': %w", cleanedStoryID, err)
	}

	if manifest.Status == StatusResolved {
		return nil, fmt.Errorf("story '%s' has already been resolved to winner '%s'", cleanedStoryID, manifest.Winner)
	}

	var winningVariant *Variant
	alternateVariants := make([]*Variant, 0, len(manifest.Variants))
	for _, v := range manifest.Variants {
		if v.Name == cleanedWinner {
			winningVariant = v
		} else {
			alternateVariants = append(alternateVariants, v)
		}
	}

	if winningVariant == nil {
		return nil, fmt.Errorf("specified winner '%s' not found among variants for story '%s'", cleanedWinner, cleanedStoryID)
	}

	// 1. Merge winning variant branch into current branch
	if err := c.executor.MergeBranch(ctx, c.repoDir, winningVariant.Branch); err != nil {
		return nil, fmt.Errorf("failed merging winning branch '%s': %w", winningVariant.Branch, err)
	}

	// 2. Persist benchmark metrics in Dendrite store
	storeKey := fmt.Sprintf("story-fork:%s:benchmark", cleanedStoryID)
	if c.store != nil {
		storePayload, jsonErr := json.MarshalIndent(manifest, "", "  ")
		if jsonErr == nil {
			_ = c.store.Put(storeKey, storePayload)
		}
	}

	// 3. Purge alternate worktrees and branches
	purged := make([]string, 0, len(alternateVariants))
	for _, alt := range alternateVariants {
		_ = c.executor.WorktreeRemove(ctx, c.repoDir, alt.WorktreePath, true)
		_ = c.executor.BranchDelete(ctx, c.repoDir, alt.Branch, true)
		_ = os.RemoveAll(alt.WorktreePath)
		purged = append(purged, alt.Name)
	}

	// Remove winning worktree directory since its branch has been merged into base
	_ = c.executor.WorktreeRemove(ctx, c.repoDir, winningVariant.WorktreePath, true)
	_ = os.RemoveAll(winningVariant.WorktreePath)

	// Clean up git metadata and locks
	_ = c.executor.WorktreePrune(ctx, c.repoDir)
	_ = c.executor.CleanLocks(ctx, c.repoDir)

	manifest.Status = StatusResolved
	manifest.Winner = cleanedWinner
	manifest.UpdatedAt = time.Now().UTC()
	_ = c.saveManifestLocked(manifest)

	return &ResolutionResult{
		StoryID:        cleanedStoryID,
		Winner:         cleanedWinner,
		MergedBranch:   winningVariant.Branch,
		PurgedVariants: purged,
		StoreKey:       storeKey,
		ResolvedAt:     time.Now().UTC(),
	}, nil
}

// Abort cleans up all worktrees and branches for a story without merging.
func (c *ForkController) Abort(ctx context.Context, storyID string) error {
	cleanedStoryID := strings.TrimSpace(storyID)
	manifest, err := c.loadManifest(cleanedStoryID)
	if err != nil {
		// Clean directory directly if manifest missing
		storyDir := filepath.Join(c.worktreesBaseDir, cleanedStoryID)
		_ = os.RemoveAll(storyDir)
		_ = c.executor.WorktreePrune(ctx, c.repoDir)
		_ = c.executor.CleanLocks(ctx, c.repoDir)
		return nil
	}

	for _, v := range manifest.Variants {
		_ = c.executor.WorktreeRemove(ctx, c.repoDir, v.WorktreePath, true)
		_ = c.executor.BranchDelete(ctx, c.repoDir, v.Branch, true)
		_ = os.RemoveAll(v.WorktreePath)
	}

	_ = c.executor.WorktreePrune(ctx, c.repoDir)
	_ = c.executor.CleanLocks(ctx, c.repoDir)

	manifest.Status = StatusAborted
	manifest.UpdatedAt = time.Now().UTC()
	_ = c.saveManifestLocked(manifest)

	storyDir := filepath.Join(c.worktreesBaseDir, cleanedStoryID)
	return os.RemoveAll(storyDir)
}

// GetManifest retrieves the manifest for a story.
func (c *ForkController) GetManifest(storyID string) (*ForkManifest, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loadManifest(storyID)
}

func (c *ForkController) manifestPath(storyID string) string {
	return filepath.Join(c.worktreesBaseDir, storyID, "manifest.json")
}

func (c *ForkController) loadManifest(storyID string) (*ForkManifest, error) {
	path := c.manifestPath(storyID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest ForkManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (c *ForkController) saveManifestLocked(manifest *ForkManifest) error {
	path := c.manifestPath(manifest.StoryID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
