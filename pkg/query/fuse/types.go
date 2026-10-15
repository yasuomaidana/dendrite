package fuse

import (
	"errors"
	"time"

	"Dendrite/pkg/ui/modal"
)

// Standard typed errors.
var (
	ErrInsufficientSources   = errors.New("fusion requires at least two source artifacts or projects")
	ErrFusionTargetRequired  = errors.New("fused target directory is required")
	ErrFusionPromptCancelled = errors.New("target directory selection prompt was cancelled by user")
)

// PromptHandler handles interactive directory confirmation.
type PromptHandler func(m *modal.DirectoryPickerModal) (string, error)

// FuseRequest specifies inputs for combining multiple artifacts into a single unified member.
type FuseRequest struct {
	Sources          []string      `json:"sources"`
	FusedMemberName  string        `json:"fused_member_name"`
	TargetLanguage   string        `json:"target_language"` // rust, go
	TargetDirectory  string        `json:"target_directory,omitempty"`
	RegisterInPool   bool          `json:"register_in_pool"`
	PoolConfigFile   string        `json:"pool_config_file,omitempty"`
	Prompt           PromptHandler `json:"-"`
}

// FuseResult encapsulates the outcome of architectural fusion.
type FuseResult struct {
	FusedMemberName       string    `json:"fused_member_name"`
	TargetDirectory       string    `json:"target_directory"`
	SourceArtifacts       []string  `json:"source_artifacts"`
	DeduplicatedLibraries []string  `json:"deduplicated_libraries"`
	ChannelMechanism      string    `json:"channel_mechanism"`
	EmittedFiles          []string  `json:"emitted_files"`
	PoolRegistered        bool      `json:"pool_registered"`
	Timestamp             time.Time `json:"timestamp"`
}
