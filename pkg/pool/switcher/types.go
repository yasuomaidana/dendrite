package switcher

import (
	"errors"
	"fmt"
	"time"
)

// DependencyMode represents the operational linking mode of a dependency.
type DependencyMode string

const (
	ModePublished DependencyMode = "published"
	ModeBranch    DependencyMode = "branch"
	ModeLocal     DependencyMode = "local"
	ModeFork      DependencyMode = "fork"
)

// Standard typed errors.
var (
	ErrInvalidMode           = errors.New("invalid dependency mode")
	ErrMissingParameter      = errors.New("missing required mode parameter")
	ErrManifestNotFound      = errors.New("no recognized manifest found in project directory")
	ErrDependencyNotFound    = errors.New("target dependency not found in manifest")
	ErrTransactionalRollback = errors.New("transaction failed and original files were rolled back")
)

// Typestate transitions contract.
type DependencyState interface {
	Mode() DependencyMode
	Validate() error
}

// PublishedState guards published registry mode invariants.
type PublishedState struct {
	Version string
}

func (s PublishedState) Mode() DependencyMode { return ModePublished }
func (s PublishedState) Validate() error {
	if s.Version == "" {
		return fmt.Errorf("%w: version is required for published mode", ErrMissingParameter)
	}
	return nil
}

// BranchState guards experimental branch mode invariants.
type BranchState struct {
	BranchName string
	CommitSHA  string
}

func (s BranchState) Mode() DependencyMode { return ModeBranch }
func (s BranchState) Validate() error {
	if s.BranchName == "" && s.CommitSHA == "" {
		return fmt.Errorf("%w: branch name or commit SHA is required for branch mode", ErrMissingParameter)
	}
	return nil
}

// LocalState guards zero-latency local linking mode invariants.
type LocalState struct {
	LocalPath string
}

func (s LocalState) Mode() DependencyMode { return ModeLocal }
func (s LocalState) Validate() error {
	if s.LocalPath == "" {
		return fmt.Errorf("%w: local path is required for local mode", ErrMissingParameter)
	}
	return nil
}

// ForkState guards remote fork mode invariants.
type ForkState struct {
	ForkURL    string
	ForkBranch string
}

func (s ForkState) Mode() DependencyMode { return ModeFork }
func (s ForkState) Validate() error {
	if s.ForkURL == "" {
		return fmt.Errorf("%w: fork URL is required for fork mode", ErrMissingParameter)
	}
	return nil
}

// SwitchRequest encapsulates parameters for a mode transition.
type SwitchRequest struct {
	TargetProjectDir string         `json:"target_project_dir"`
	DependencyName   string         `json:"dependency_name"`
	Mode             DependencyMode `json:"mode"`
	Version          string         `json:"version,omitempty"`
	BranchName       string         `json:"branch_name,omitempty"`
	CommitSHA        string         `json:"commit_sha,omitempty"`
	LocalPath        string         `json:"local_path,omitempty"`
	ForkURL          string         `json:"fork_url,omitempty"`
	ForkBranch       string         `json:"fork_branch,omitempty"`
	SyncIDE          bool           `json:"sync_ide"`
}

// SwitchResult summarizes the result of an atomic dependency mode switch.
type SwitchResult struct {
	DependencyName   string         `json:"dependency_name"`
	TargetProject    string         `json:"target_project"`
	PreviousMode     DependencyMode `json:"previous_mode"`
	NewMode          DependencyMode `json:"new_mode"`
	ModifiedFiles    []string       `json:"modified_files"`
	IDEScopesUpdated bool           `json:"ide_scopes_updated"`
	Timestamp        time.Time      `json:"timestamp"`
}
