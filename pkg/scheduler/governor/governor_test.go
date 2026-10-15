package governor_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/scheduler/governor"
	"Dendrite/pkg/scheduler/token"
	"Dendrite/pkg/store/segmented"
)

func TestGovernor_DefaultsAndConfigPersistence(t *testing.T) {
	// 1. Test DefaultConfig
	def := governor.DefaultConfig()
	if def.ReserveFloor != 0.20 || def.AllowZeroDrain || def.Mode != governor.ModePolicy {
		t.Fatalf("unexpected DefaultConfig: %+v", def)
	}

	tmpDir, err := os.MkdirTemp("", "governor_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgFile := filepath.Join(tmpDir, "cfg.json")

	// Test boundary floor correction
	badFloorCfg := governor.GovernorConfig{
		ReserveFloor: -0.5,
		ConfigFile:   cfgFile,
	}
	gBad := governor.NewQuotaGovernor(badFloorCfg, nil, nil)
	if gBad.GetState().ReserveFloor != 0.20 {
		t.Errorf("expected negative reserve floor to default to 0.20, got %f", gBad.GetState().ReserveFloor)
	}

	badFloorHigh := governor.GovernorConfig{
		ReserveFloor: 1.5,
		ConfigFile:   cfgFile,
	}
	gBadHigh := governor.NewQuotaGovernor(badFloorHigh, nil, nil)
	if gBadHigh.GetState().ReserveFloor != 0.20 {
		t.Errorf("expected >1.0 reserve floor to default to 0.20, got %f", gBadHigh.GetState().ReserveFloor)
	}

	// Test loading valid config
	initialCfg := governor.GovernorConfig{
		ReserveFloor:   0.25,
		AllowZeroDrain: true,
		Mode:           governor.ModeAlways,
		ConfigFile:     cfgFile,
	}
	g := governor.NewQuotaGovernor(initialCfg, nil, nil)
	if err := g.SetAllowZero(true); err != nil {
		t.Fatalf("failed to set allow zero: %v", err)
	}
	if err := g.SetMode(governor.ModeAlways); err != nil {
		t.Fatalf("failed to set mode: %v", err)
	}

	// Reload from file
	gReloaded := governor.NewQuotaGovernor(governor.GovernorConfig{ConfigFile: cfgFile}, nil, nil)
	st := gReloaded.GetState()
	if st.ReserveFloor != 0.25 || !st.AllowZeroDrain || st.Mode != governor.ModeAlways {
		t.Errorf("reloaded config mismatch: %+v", st)
	}

	// Empty config file path branch
	gNoFile := governor.NewQuotaGovernor(governor.GovernorConfig{ConfigFile: ""}, nil, nil)
	if err := gNoFile.SetAllowZero(false); err != nil {
		t.Errorf("unexpected error on empty config file: %v", err)
	}
}

func TestGovernor_ModesAndSwitches(t *testing.T) {
	sched := token.NewTokenScheduler(nil, nil, "")
	cfg := governor.DefaultConfig()
	cfg.ConfigFile = ""
	g := governor.NewQuotaGovernor(cfg, nil, sched)

	// Test SetAllowZero with scheduler attached
	if err := g.SetAllowZero(true); err != nil {
		t.Fatalf("failed SetAllowZero: %v", err)
	}
	if !g.GetState().AllowZeroDrain {
		t.Errorf("expected AllowZeroDrain to be true")
	}
	if !sched.GetPolicy().AllowZero {
		t.Errorf("expected scheduler AllowZero to be true")
	}

	// Test SetMode valid modes
	if err := g.SetMode(governor.ModeAlways); err != nil {
		t.Fatalf("failed to set ModeAlways: %v", err)
	}
	if g.GetState().Mode != governor.ModeAlways {
		t.Errorf("expected mode always, got %s", g.GetState().Mode)
	}
	if sched.GetPolicy().Mode != "always" {
		t.Errorf("expected scheduler mode always, got %s", sched.GetPolicy().Mode)
	}

	if err := g.SetMode(governor.ModePolicy); err != nil {
		t.Fatalf("failed to set ModePolicy: %v", err)
	}
	if g.GetState().Mode != governor.ModePolicy {
		t.Errorf("expected mode policy, got %s", g.GetState().Mode)
	}

	// Test SetMode invalid
	if err := g.SetMode("invalid-mode"); !errors.Is(err, governor.ErrInvalidMode) {
		t.Errorf("expected ErrInvalidMode, got %v", err)
	}

	// Test SetRemainingQuota clamping and scheduler sync
	g.SetRemainingQuota(0.85)
	if g.GetState().RemainingQuota != 0.85 {
		t.Errorf("expected 0.85, got %f", g.GetState().RemainingQuota)
	}

	g.SetRemainingQuota(-0.2)
	if g.GetState().RemainingQuota != 0.0 {
		t.Errorf("expected clamp to 0.0, got %f", g.GetState().RemainingQuota)
	}

	g.SetRemainingQuota(2.5)
	if g.GetState().RemainingQuota != 1.0 {
		t.Errorf("expected clamp to 1.0, got %f", g.GetState().RemainingQuota)
	}
}

func TestGovernor_EvaluationAndStateDiagnostics(t *testing.T) {
	cfg := governor.DefaultConfig()
	cfg.ConfigFile = ""
	g := governor.NewQuotaGovernor(cfg, nil, nil)

	// 1. Quota exhausted (0.0)
	g.SetRemainingQuota(0.0)
	st1 := g.GetState()
	if !st1.IsCloudBlocked {
		t.Errorf("expected cloud blocked at 0 quota")
	}

	// 2. Reserve floor reached (0.19 <= 0.20) with AllowZero = false
	g.SetRemainingQuota(0.19)
	_ = g.SetAllowZero(false)
	st2 := g.GetState()
	if !st2.IsCloudBlocked {
		t.Errorf("expected cloud blocked at reserve floor")
	}

	// 3. Reserve floor reached (0.19 <= 0.20) with AllowZero = true (override active)
	_ = g.SetAllowZero(true)
	st3 := g.GetState()
	if st3.IsCloudBlocked {
		t.Errorf("expected cloud permitted when AllowZero is true")
	}

	// 4. Paused mode
	_ = g.SetMode(governor.ModePaused)
	st4 := g.GetState()
	if !st4.IsCloudBlocked {
		t.Errorf("expected cloud blocked when paused")
	}
}

func TestGovernor_CheckCloudExecutionAndStoreIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "governor_store_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := segmented.NewSegmentedStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to initialize segmented store: %v", err)
	}

	ctx := context.Background()

	// Commit a preliminary chunk
	chunkID := "chunk-alpha"
	project := "dendrite-core"
	seg := segmented.Segment{
		Hash:       "hash-1234",
		Type:       segmented.SegmentFile,
		Identifier: "pkg/core.go",
		SizeBytes:  100,
	}
	_, err = store.CommitChunk(ctx, project, chunkID, []segmented.Segment{seg}, map[string][]byte{"hash-1234": []byte("content")}, false)
	if err != nil {
		t.Fatalf("failed to commit chunk: %v", err)
	}

	sched := token.NewTokenScheduler(nil, nil, "")
	cfg := governor.DefaultConfig()
	cfg.ConfigFile = filepath.Join(tmpDir, "test_gov.json")
	g := governor.NewQuotaGovernor(cfg, store, sched)

	// 1. Check with normal quota (> 20%)
	g.SetRemainingQuota(0.50)
	allowed, err := g.CheckCloudExecution(ctx, project, chunkID)
	if !allowed || err != nil {
		t.Fatalf("expected cloud execution allowed: %v", err)
	}

	// 2. Check when paused
	_ = g.SetMode(governor.ModePaused)
	allowedPaused, err := g.CheckCloudExecution(ctx, project, chunkID)
	if allowedPaused || !errors.Is(err, governor.ErrGovernorPaused) {
		t.Errorf("expected ErrGovernorPaused, got %v", err)
	}

	// Restore mode
	_ = g.SetMode(governor.ModePolicy)

	// 3. Check when quota <= 0.0
	g.SetRemainingQuota(0.0)
	allowedZero, err := g.CheckCloudExecution(ctx, project, chunkID)
	if allowedZero || !errors.Is(err, governor.ErrQuotaFloorBlocked) {
		t.Errorf("expected ErrQuotaFloorBlocked at 0.0 quota, got %v", err)
	}

	// 4. Check when remaining quota hits 20% floor (0.18) with AllowZero = false
	g.SetRemainingQuota(0.18)
	_ = g.SetAllowZero(false)
	allowedFloor, err := g.CheckCloudExecution(ctx, project, chunkID)
	if allowedFloor || !errors.Is(err, governor.ErrQuotaFloorBlocked) {
		t.Errorf("expected ErrQuotaFloorBlocked at floor, got %v", err)
	}

	// Verify that checkpoint state in SQLite was transitioned to PAUSED_QUOTA_FLOOR!
	cp, err := store.GetCheckpoint(ctx, chunkID)
	if err != nil {
		t.Fatalf("failed to get checkpoint: %v", err)
	}
	if cp.State != segmented.StatePausedQuotaFloor {
		t.Errorf("expected chunk checkpoint state PAUSED_QUOTA_FLOOR, got %s", cp.State)
	}
	if g.GetState().LastCheckpointState != segmented.StatePausedQuotaFloor {
		t.Errorf("expected governor last state PAUSED_QUOTA_FLOOR")
	}

	// 5. Test Pause and Resume with store
	if err := g.Pause(ctx, project, chunkID); err != nil {
		t.Fatalf("failed to pause: %v", err)
	}
	if g.GetState().Mode != governor.ModePaused {
		t.Errorf("expected paused mode")
	}

	// Resume after allowing zero drain
	_ = g.SetAllowZero(true)
	if err := g.Resume(ctx, project, chunkID); err != nil {
		t.Fatalf("failed to resume: %v", err)
	}
	if g.GetState().Mode != governor.ModePolicy {
		t.Errorf("expected resumed mode policy")
	}

	// Verify checkpoint transitioned back to ANALYZING_PARTIAL on resume with override
	cpResumed, err := store.GetCheckpoint(ctx, chunkID)
	if err != nil {
		t.Fatalf("failed to get resumed checkpoint: %v", err)
	}
	if cpResumed.State != segmented.StateAnalyzingPartial {
		t.Errorf("expected state ANALYZING_PARTIAL, got %s", cpResumed.State)
	}

	// Test Pause and Resume when previous mode was ModeAlways
	_ = g.SetMode(governor.ModeAlways)
	_ = g.Pause(ctx, project, chunkID)
	_ = g.Resume(ctx, project, chunkID)
	if g.GetState().Mode != governor.ModeAlways {
		t.Errorf("expected resumed mode to restore ModeAlways, got %s", g.GetState().Mode)
	}
}

func TestGovernor_EdgeBranches(t *testing.T) {
	// 1. loadConfigFile invalid JSON
	tmpDir, err := os.MkdirTemp("", "gov_edge_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	badJSONFile := filepath.Join(tmpDir, "bad.json")
	_ = os.WriteFile(badJSONFile, []byte("{corrupt-json"), 0644)
	gBad := governor.NewQuotaGovernor(governor.GovernorConfig{ConfigFile: badJSONFile}, nil, nil)
	if gBad.GetState().Mode != governor.ModePolicy {
		t.Errorf("expected default mode policy on corrupted config file")
	}

	// 2. saveConfigFile error on unwritable dir
	unwritableFile := "/root/impossible_dir/cfg.json"
	gUnwritable := governor.NewQuotaGovernor(governor.GovernorConfig{ConfigFile: unwritableFile}, nil, nil)
	_ = gUnwritable.SetAllowZero(true)

	// 3. Resume when previousMode was ModePaused
	gPaused := governor.NewQuotaGovernor(governor.GovernorConfig{ConfigFile: ""}, nil, nil)
	_ = gPaused.SetMode(governor.ModePaused)
	_ = gPaused.Resume(context.Background(), "", "")
	if gPaused.GetState().Mode != governor.ModePolicy {
		t.Errorf("expected Resume to fall back to ModePolicy when previousMode is empty/paused")
	}
}

func TestGovernor_ResumeElseBranch(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "gov_resume_*")
	defer os.RemoveAll(tmpDir)

	store, _ := segmented.NewSegmentedStore(tmpDir)
	ctx := context.Background()

	// Commit chunk as STORED (final)
	chunkID := "chunk-done"
	seg := segmented.Segment{Hash: "h-done", Type: segmented.SegmentFile, Identifier: "done.go", SizeBytes: 10}
	_, _ = store.CommitChunk(ctx, "p", chunkID, []segmented.Segment{seg}, map[string][]byte{"h-done": []byte("done")}, true)

	g := governor.NewQuotaGovernor(governor.GovernorConfig{ConfigFile: ""}, store, nil)
	_ = g.Resume(ctx, "p", chunkID)
	if g.GetState().LastCheckpointState != segmented.StateStored {
		t.Errorf("expected last state STORED, got %s", g.GetState().LastCheckpointState)
	}
}
