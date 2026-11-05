package governor

import (
	"context"
	"errors"
	"testing"

	"Dendrite/pkg/store/segmented"
)

func TestGovernor_SaveConfigFile_EmptyPath(t *testing.T) {
	g := NewQuotaGovernor(GovernorConfig{ConfigFile: ""}, nil, nil)
	g.cfg.ConfigFile = "" // clear defaulted path
	if err := g.saveConfigFile(); err != nil {
		t.Errorf("expected nil error on empty ConfigFile, got: %v", err)
	}
}

func TestGovernor_InternalPauseAndResumeEdgeCases(t *testing.T) {
	ctx := context.Background()
	g := NewQuotaGovernor(GovernorConfig{ConfigFile: ""}, nil, nil)

	// Pause when already in ModePaused: covers false branch of `if g.cfg.Mode != ModePaused`
	g.cfg.Mode = ModePaused
	if err := g.Pause(ctx, "", ""); err != nil {
		t.Fatalf("unexpected error pausing when already paused: %v", err)
	}

	// Resume when previousMode is ModePaused
	g.previousMode = ModePaused
	if err := g.Resume(ctx, "", ""); err != nil {
		t.Fatalf("unexpected error resuming with ModePaused previousMode: %v", err)
	}
	if g.cfg.Mode != ModePolicy {
		t.Errorf("expected ModePolicy, got %s", g.cfg.Mode)
	}

	// Resume when previousMode is empty
	g.previousMode = ""
	if err := g.Resume(ctx, "proj-1", "chunk-1"); err != nil {
		t.Fatalf("unexpected error resuming with empty previousMode: %v", err)
	}
	if g.cfg.Mode != ModePolicy {
		t.Errorf("expected ModePolicy, got %s", g.cfg.Mode)
	}
	if g.activeProject != "proj-1" || g.activeChunkID != "chunk-1" {
		t.Errorf("expected active project/chunk to be updated")
	}
}

func TestGovernor_CheckCloudExecution_NilStoreAtFloor(t *testing.T) {
	ctx := context.Background()
	g := NewQuotaGovernor(GovernorConfig{ConfigFile: ""}, nil, nil)
	g.SetRemainingQuota(0.10)
	g.cfg.AllowZeroDrain = false

	allowed, err := g.CheckCloudExecution(ctx, "proj", "")
	if allowed || !errors.Is(err, ErrQuotaFloorBlocked) {
		t.Errorf("expected ErrQuotaFloorBlocked, got allowed=%v, err=%v", allowed, err)
	}
}

func TestGovernor_StoreMissingChunk(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	store, err := segmented.NewSegmentedStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create segmented store: %v", err)
	}

	g := NewQuotaGovernor(GovernorConfig{ConfigFile: ""}, store, nil)

	// Pause with nonexistent chunk in store: exercises cp == nil branch
	if err := g.Pause(ctx, "proj", "nonexistent-chunk"); err != nil {
		t.Fatalf("unexpected error on Pause with missing chunk: %v", err)
	}

	// Resume with nonexistent chunk in store
	if err := g.Resume(ctx, "proj", "nonexistent-chunk"); err != nil {
		t.Fatalf("unexpected error on Resume with missing chunk: %v", err)
	}
}
