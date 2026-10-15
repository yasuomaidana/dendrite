package governor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"Dendrite/pkg/scheduler/token"
	"Dendrite/pkg/store/segmented"
)

// QuotaGovernor manages safety reserves, 0% drain overrides, and continuous analysis pause engines.
type QuotaGovernor struct {
	mu             sync.RWMutex
	cfg            GovernorConfig
	store          *segmented.SegmentedStore
	scheduler      *token.TokenScheduler
	remainingQuota float64
	activeProject  string
	activeChunkID  string
	lastState      segmented.ChunkState
	previousMode   AnalysisMode
}

// NewQuotaGovernor initializes the quota governor with store and scheduler attachments.
func NewQuotaGovernor(cfg GovernorConfig, store *segmented.SegmentedStore, sched *token.TokenScheduler) *QuotaGovernor {
	if cfg.ReserveFloor <= 0.0 || cfg.ReserveFloor > 1.0 {
		cfg.ReserveFloor = 0.20
	}
	if cfg.Mode == "" {
		cfg.Mode = ModePolicy
	}
	if cfg.ConfigFile == "" {
		cfg.ConfigFile = DefaultConfigFile
	}

	loaded, err := loadConfigFile(cfg.ConfigFile)
	if err == nil && loaded != nil {
		cfg.ReserveFloor = loaded.ReserveFloor
		cfg.AllowZeroDrain = loaded.AllowZeroDrain
		cfg.Mode = loaded.Mode
	}

	g := &QuotaGovernor{
		cfg:            cfg,
		store:          store,
		scheduler:      sched,
		remainingQuota: 1.0,
		previousMode:   ModePolicy,
	}

	return g
}

// SetAllowZero sets the drain override to allow consuming remaining quota down to 0%.
func (g *QuotaGovernor) SetAllowZero(enabled bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.cfg.AllowZeroDrain = enabled
	_ = g.saveConfigFile()

	if g.scheduler != nil {
		p := g.scheduler.GetPolicy()
		p.AllowZero = enabled
		_ = g.scheduler.SetPolicy(p)
	}

	return nil
}

// SetMode transitions between "always", "policy", and "paused" operating regimes.
func (g *QuotaGovernor) SetMode(mode AnalysisMode) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	switch mode {
	case ModePolicy, ModeAlways, ModePaused:
		// valid
	default:
		return fmt.Errorf("%w: '%s'", ErrInvalidMode, mode)
	}

	if g.cfg.Mode != ModePaused {
		g.previousMode = g.cfg.Mode
	}
	g.cfg.Mode = mode
	_ = g.saveConfigFile()

	if g.scheduler != nil {
		p := g.scheduler.GetPolicy()
		p.Mode = string(mode)
		_ = g.scheduler.SetPolicy(p)
	}

	return nil
}

// SetRemainingQuota adjusts the tracked quota ratio (clamped to [0.0, 1.0]).
func (g *QuotaGovernor) SetRemainingQuota(ratio float64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if ratio < 0.0 {
		ratio = 0.0
	} else if ratio > 1.0 {
		ratio = 1.0
	}

	g.remainingQuota = ratio
	if g.scheduler != nil {
		g.scheduler.SetRemainingQuota(ratio)
	}
}

// GetState returns an atomic snapshot of governor diagnostics.
func (g *QuotaGovernor) GetState() GovernorState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	blocked := false
	reason := "Cloud inference permitted under active schedule"

	if g.cfg.Mode == ModePaused {
		blocked = true
		reason = "Analysis suspended by user pause directive"
	} else if g.remainingQuota <= 0.0 {
		blocked = true
		reason = "Weekly quota completely exhausted (0.0% remaining)"
	} else if g.remainingQuota <= g.cfg.ReserveFloor && !g.cfg.AllowZeroDrain {
		blocked = true
		reason = fmt.Sprintf("Remaining quota reached safety floor (%.1f%% <= %.1f%% floor): cloud calls halted to preserve safety reserve", g.remainingQuota*100, g.cfg.ReserveFloor*100)
	}

	return GovernorState{
		Mode:                g.cfg.Mode,
		RemainingQuota:      g.remainingQuota,
		ReserveFloor:        g.cfg.ReserveFloor,
		AllowZeroDrain:      g.cfg.AllowZeroDrain,
		IsCloudBlocked:      blocked,
		ActiveProject:       g.activeProject,
		ActiveChunkID:       g.activeChunkID,
		LastCheckpointState: g.lastState,
		Reason:              reason,
	}
}

// CheckCloudExecution evaluates whether a cloud call is permitted, updating checkpoint state on boundary hit.
func (g *QuotaGovernor) CheckCloudExecution(ctx context.Context, project, chunkID string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.activeProject = project
	g.activeChunkID = chunkID

	if g.cfg.Mode == ModePaused {
		return false, ErrGovernorPaused
	}

	if g.remainingQuota <= 0.0 {
		return false, ErrQuotaFloorBlocked
	}

	// 20% floor guard
	if g.remainingQuota <= g.cfg.ReserveFloor && !g.cfg.AllowZeroDrain {
		g.lastState = segmented.StatePausedQuotaFloor
		if g.store != nil && chunkID != "" {
			_, _ = g.store.UpdateCheckpointState(ctx, chunkID, segmented.StatePausedQuotaFloor)
		}
		return false, ErrQuotaFloorBlocked
	}

	return true, nil
}

// Pause suspends execution, persists active chunk progress, and transitions mode to paused.
func (g *QuotaGovernor) Pause(ctx context.Context, project, chunkID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.cfg.Mode != ModePaused {
		g.previousMode = g.cfg.Mode
	}
	g.cfg.Mode = ModePaused

	if project != "" {
		g.activeProject = project
	}
	if chunkID != "" {
		g.activeChunkID = chunkID
	}

	if g.store != nil && g.activeChunkID != "" {
		cp, err := g.store.GetCheckpoint(ctx, g.activeChunkID)
		if err == nil && cp != nil {
			cp.UpdatedAt = time.Now().UTC()
			_, _ = g.store.UpdateCheckpointState(ctx, g.activeChunkID, cp.State)
			g.lastState = cp.State
		}
	}

	if g.scheduler != nil {
		g.scheduler.Pause()
	}

	_ = g.saveConfigFile()
	return nil
}

// Resume restores operating mode and reactivates analysis passes.
func (g *QuotaGovernor) Resume(ctx context.Context, project, chunkID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	targetMode := g.previousMode
	if targetMode == "" || targetMode == ModePaused {
		targetMode = ModePolicy
	}
	g.cfg.Mode = targetMode

	if project != "" {
		g.activeProject = project
	}
	if chunkID != "" {
		g.activeChunkID = chunkID
	}

	if g.store != nil && g.activeChunkID != "" {
		cp, err := g.store.GetCheckpoint(ctx, g.activeChunkID)
		if err == nil && cp != nil {
			// If previously paused at floor but now eligible, update checkpoint
			if cp.State == segmented.StatePausedQuotaFloor && (g.remainingQuota > g.cfg.ReserveFloor || g.cfg.AllowZeroDrain) {
				_, _ = g.store.UpdateCheckpointState(ctx, g.activeChunkID, segmented.StateAnalyzingPartial)
				g.lastState = segmented.StateAnalyzingPartial
			} else {
				g.lastState = cp.State
			}
		}
	}

	if g.scheduler != nil {
		g.scheduler.Resume()
	}

	_ = g.saveConfigFile()
	return nil
}

func (g *QuotaGovernor) saveConfigFile() error {
	if g.cfg.ConfigFile == "" {
		return nil
	}
	dir := filepath.Dir(g.cfg.ConfigFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(g.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(g.cfg.ConfigFile, data, 0644)
}

func loadConfigFile(path string) (*GovernorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg GovernorConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
