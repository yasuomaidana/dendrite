package fork

import (
	"context"
	"sync"
	"time"
)

// ForkStatus denotes the lifecycle state of a speculative story fork.
type ForkStatus string

const (
	StatusActive      ForkStatus = "ACTIVE"
	StatusBenchmarked ForkStatus = "BENCHMARKED"
	StatusResolved    ForkStatus = "RESOLVED"
	StatusAborted     ForkStatus = "ABORTED"
)

// Variant represents a speculative branch implementation worktree.
type Variant struct {
	Name         string    `json:"name"`
	Branch       string    `json:"branch"`
	WorktreePath string    `json:"worktree_path"`
	CreatedAt    time.Time `json:"created_at"`
}

// VariantMetrics captures performance, memory, and CPU metrics for a variant.
type VariantMetrics struct {
	Variant    string        `json:"variant"`
	TTFT       time.Duration `json:"ttft"`
	TPS        float64       `json:"tps"`
	MemoryMB   float64       `json:"memory_mb"`
	CPUTime    time.Duration `json:"cpu_time"`
	CPUPercent float64       `json:"cpu_percent"`
	Throughput float64       `json:"throughput"`
	Error      string        `json:"error,omitempty"`
}

// ComparativeScorecard aggregates benchmark metrics for all variants of a story.
type ComparativeScorecard struct {
	StoryID           string                     `json:"story_id"`
	BaseCommit        string                     `json:"base_commit"`
	Timestamp         time.Time                  `json:"timestamp"`
	Metrics           map[string]*VariantMetrics `json:"metrics"`
	RecommendedWinner string                     `json:"recommended_winner"`
	Reason            string                     `json:"reason"`
}

// ForkManifest persists active worktree configuration and state on disk.
type ForkManifest struct {
	StoryID    string                `json:"story_id"`
	BaseBranch string                `json:"base_branch"`
	BaseCommit string                `json:"base_commit"`
	Variants   []*Variant            `json:"variants"`
	Status     ForkStatus            `json:"status"`
	Winner     string                `json:"winner,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
	Scorecard  *ComparativeScorecard `json:"scorecard,omitempty"`
}

// ResolutionResult encapsulates the outcome of a story resolution.
type ResolutionResult struct {
	StoryID        string    `json:"story_id"`
	Winner         string    `json:"winner"`
	MergedBranch   string    `json:"merged_branch"`
	PurgedVariants []string  `json:"purged_variants"`
	StoreKey       string    `json:"store_key"`
	ResolvedAt     time.Time `json:"resolved_at"`
}

// BenchmarkOptions specifies configurable parameters for variant benchmarking.
type BenchmarkOptions struct {
	Iterations   int           `json:"iterations"`
	Duration     time.Duration `json:"duration"`
	VirtualUsers int           `json:"virtual_users"`
	TargetFile   string        `json:"target_file,omitempty"`
}

// WorktreeGuard provides RAII cleanup semantics to prevent orphan worktrees or locked git indexes.
type WorktreeGuard struct {
	controller *ForkController
	storyID    string
	variants   []string
	dismissed  bool
	mu         sync.Mutex
}

// NewWorktreeGuard instantiates a WorktreeGuard for a story.
func NewWorktreeGuard(controller *ForkController, storyID string, variants []string) *WorktreeGuard {
	return &WorktreeGuard{
		controller: controller,
		storyID:    storyID,
		variants:   variants,
	}
}

// Dismiss disarms the guard, indicating successful completion and that automatic rollback is not needed.
func (g *WorktreeGuard) Dismiss() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dismissed = true
}

// Rollback triggers cleanup of any active worktrees and unlocks git state if the guard was not dismissed.
func (g *WorktreeGuard) Rollback(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.dismissed {
		return nil
	}
	return g.controller.Abort(ctx, g.storyID)
}

// Close implements io.Closer for RAII defer usage (e.g. defer guard.Close()).
func (g *WorktreeGuard) Close() error {
	return g.Rollback(context.Background())
}
