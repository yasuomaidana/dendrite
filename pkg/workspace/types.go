package workspace

import (
	"errors"
	"time"
)

// State represents the lifecycle typestate of a workspace.
type State string

const (
	StateUnregistered State = "Unregistered"
	StateRegistered   State = "Registered"
	StateActive       State = "Active"
	StateSuspended    State = "Suspended"
)

// Standard typed errors for workspace registry operations.
var (
	ErrProjectNotFound        = errors.New("project not found in workspace registry")
	ErrProjectAlreadyExists   = errors.New("project already exists in workspace registry")
	ErrInvalidProjectName     = errors.New("invalid project name")
	ErrInvalidProjectPath     = errors.New("invalid or non-existent project path")
	ErrNoActiveProject        = errors.New("no active project selected")
	ErrIllegalStateTransition = errors.New("illegal workspace state transition")
)

// ProjectEntry represents an isolated project workspace registered within Dendrite.
type ProjectEntry struct {
	Name            string    `yaml:"name" json:"name"`
	Path            string    `yaml:"path" json:"path"`
	State           State     `yaml:"state" json:"state"`
	DataDir         string    `yaml:"data_dir" json:"data_dir"`
	GraphDBPath     string    `yaml:"graph_db_path" json:"graph_db_path"`
	SpendCachePath  string    `yaml:"spend_cache_path" json:"spend_cache_path"`
	ScratchDir      string    `yaml:"scratch_dir" json:"scratch_dir"`
	VirtualKeyAlias string    `yaml:"virtual_key_alias" json:"virtual_key_alias"`
	RegisteredAt    time.Time `yaml:"registered_at" json:"registered_at"`
	LastSwitchedAt  time.Time `yaml:"last_switched_at,omitempty" json:"last_switched_at,omitempty"`
}

// RegistryConfig models the persistent root configuration format.
type RegistryConfig struct {
	ActiveProject string                  `yaml:"active_project" json:"active_project"`
	Projects      map[string]ProjectEntry `yaml:"projects" json:"projects"`
}

// ValidateTransition asserts whether a state transition adheres to the typestate pattern.
func ValidateTransition(from, to State) error {
	switch from {
	case StateUnregistered:
		if to == StateRegistered {
			return nil
		}
	case StateRegistered:
		if to == StateActive || to == StateUnregistered {
			return nil
		}
	case StateActive:
		if to == StateSuspended || to == StateUnregistered {
			return nil
		}
	case StateSuspended:
		if to == StateActive || to == StateUnregistered {
			return nil
		}
	}
	return ErrIllegalStateTransition
}
