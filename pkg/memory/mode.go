package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"Dendrite/pkg/fsutil"
)

// Mode says which memory stores are written to: Claude Code's own file memory, Engram, both or neither.
type Mode string

// The memory modes. Both is the default: the two run side by side until a measurement says otherwise.
const (
	Both       Mode = "both"
	EngramOnly Mode = "engram"
	ClaudeOnly Mode = "claude"
	Off        Mode = "off"
)

// ParseMode reads a mode name; "auto" and "" mean no override and parse to the empty mode.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(s); m {
	case Both, EngramOnly, ClaudeOnly, Off, "":
		return m, nil
	}
	if s == "auto" {
		return "", nil
	}
	return "", fmt.Errorf("memory: unknown mode %q (both, engram, claude, off, auto)", s)
}

// Engram reports whether this mode writes to Engram.
func (m Mode) Engram() bool { return m == Both || m == EngramOnly }

// Claude reports whether this mode keeps Claude Code's file memory in use.
func (m Mode) Claude() bool { return m == Both || m == ClaudeOnly }

// State is what is stored about the mode: a hand-set override and the mode the last measurement computed.
type State struct {
	Override Mode   `json:"override,omitempty"`
	Computed Mode   `json:"computed,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Resolve is the mode in force: the override, else the computed mode, else Both.
func (s State) Resolve() Mode {
	for _, m := range []Mode{s.Override, s.Computed} {
		if m != "" {
			return m
		}
	}
	return Both
}

// StatePath is where the state of the Dendrite home lives.
func StatePath(home string) string { return filepath.Join(home, "memory", "state.json") }

// LoadState reads the state; a missing or unreadable file is the empty state, never an error, so a broken
// file cannot stop a session from starting.
func LoadState(home string) State {
	var s State
	if data, err := os.ReadFile(StatePath(home)); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

// SaveState stores the state atomically.
func SaveState(home string, s State) error {
	data, _ := json.MarshalIndent(s, "", "  ")
	return fsutil.WriteFileAtomic(StatePath(home), data)
}

// SetOverride sets the hand-chosen mode ("" removes it) and keeps the computed one.
func SetOverride(home string, m Mode) error {
	s := LoadState(home)
	s.Override = m
	return SaveState(home, s)
}
