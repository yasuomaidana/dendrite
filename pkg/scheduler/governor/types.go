package governor

import (
	"errors"

	"Dendrite/pkg/store/segmented"
)

var (
	// ErrQuotaFloorBlocked is returned when remaining quota hits the reserve floor without override.
	ErrQuotaFloorBlocked = errors.New("cloud inference blocked: remaining quota is at or below the 20% safety floor (enable --allow-zero to override)")
	// ErrGovernorPaused is returned when trying to execute analysis while the governor is paused.
	ErrGovernorPaused = errors.New("governor is paused")
	// ErrInvalidMode is returned when an unrecognized operating mode is specified.
	ErrInvalidMode = errors.New("invalid governor mode (allowed: always, policy, paused)")
)

// AnalysisMode defines the operational regime of the analysis engine.
type AnalysisMode string

const (
	// ModePolicy runs analysis according to time windows (local night + 48h pre-reset sprint).
	ModePolicy AnalysisMode = "policy"
	// ModeAlways runs analysis continuously until paused, respecting quota limits.
	ModeAlways AnalysisMode = "always"
	// ModePaused suspends analysis runs and preserves checkpointed state.
	ModePaused AnalysisMode = "paused"
)

// DefaultConfigFile is the default path for governor settings.
const DefaultConfigFile = ".dendrite/governor_config.json"

// GovernorConfig specifies safety thresholds, override switches, and operating modes.
type GovernorConfig struct {
	ReserveFloor   float64      `json:"reserve_floor"`    // default 0.20 (20% safety floor)
	AllowZeroDrain bool         `json:"allow_zero_drain"` // allow draining to 0%
	Mode           AnalysisMode `json:"mode"`             // "policy", "always", "paused"
	ConfigFile     string       `json:"config_file,omitempty"`
}

// DefaultConfig returns standard safety configurations.
func DefaultConfig() GovernorConfig {
	return GovernorConfig{
		ReserveFloor:   0.20,
		AllowZeroDrain: false,
		Mode:           ModePolicy,
		ConfigFile:     DefaultConfigFile,
	}
}

// GovernorState summarizes the live evaluation of the quota governor.
type GovernorState struct {
	Mode                AnalysisMode         `json:"mode"`
	RemainingQuota      float64              `json:"remaining_quota"`
	ReserveFloor        float64              `json:"reserve_floor"`
	AllowZeroDrain      bool                 `json:"allow_zero_drain"`
	IsCloudBlocked      bool                 `json:"is_cloud_blocked"`
	ActiveProject       string               `json:"active_project,omitempty"`
	ActiveChunkID       string               `json:"active_chunk_id,omitempty"`
	LastCheckpointState segmented.ChunkState `json:"last_checkpoint_state,omitempty"`
	Reason              string               `json:"reason"`
}
