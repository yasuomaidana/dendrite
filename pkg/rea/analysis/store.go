// Package analysis runs a reverse-engineering analysis of a binary in stages that survive a pause: the PE image is
// parsed by the dendrite-core sidecar, its structure is written as a small set of documents, and those documents
// are indexed by the RAG sidecar so an agent reads only the parts it needs. Every stage is checkpointed on disk,
// so `pause` and `resume` work across processes and `purge` removes everything.
package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/projectid"
)

// Stage is how far an analysis got. The stages run in this order.
type Stage string

const (
	StageNew     Stage = "new"     // created, nothing parsed yet
	StageParsed  Stage = "parsed"  // the inspected structure is saved in image.json
	StageWritten Stage = "written" // the workspace documents are written
	StageTriaged Stage = "triaged" // the functions are routed to a model tier (triage.json), when the target has any
	StageDone    Stage = "done"    // the workspace is indexed and searchable
)

// Errors of the analysis commands.
var (
	ErrNotFound   = errors.New("no such analysis")
	ErrBadID      = errors.New("an analysis id is 1 to 64 letters, digits, dots, dashes or underscores, starting with a letter or digit")
	ErrExists     = errors.New("an analysis with this id already exists")
	ErrPaused     = errors.New("the analysis is paused: resume it with `dendrite retro resume`")
	ErrNeedsForce = errors.New("purge deletes the analysis, its documents and its index: pass --force")
	ErrNotDone    = errors.New("the analysis is not finished: run `dendrite retro resume` first")
)

// State is the checkpoint of one analysis, kept in <root>/<id>/state.json.
type State struct {
	ID        string    `json:"id"`
	Binary    string    `json:"binary"`
	Format    string    `json:"format"`
	Kind      string    `json:"kind,omitempty"`
	Tier      string    `json:"tier"`
	Model     string    `json:"model,omitempty"`
	Stage     Stage     `json:"stage"`
	Paused    bool      `json:"paused"`
	Gate      string    `json:"gate,omitempty"`
	Triage    *Triage   `json:"triage,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidID reports whether id is safe to use as a directory name; purge deletes that directory.
func ValidID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrBadID, id)
	}
	return nil
}

// NewID names an analysis after its binary and the moment it was created.
func NewID(binary string, now time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", binary, now.UnixNano())))
	return "pe-" + hex.EncodeToString(sum[:4])
}

// Store keeps analyses under Root, one directory each.
type Store struct{ Root string }

// DefaultStore keeps analyses under <dendrite home>/rea.
func DefaultStore() (*Store, error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return nil, err
	}
	return &Store{Root: filepath.Join(home, "rea")}, nil
}

// Dir is the directory of analysis id; WorkspaceDir holds the documents that get indexed.
func (s *Store) Dir(id string) string          { return filepath.Join(s.Root, id) }
func (s *Store) WorkspaceDir(id string) string { return filepath.Join(s.Dir(id), "workspace") }
func (s *Store) statePath(id string) string    { return filepath.Join(s.Dir(id), "state.json") }

// Create records a new analysis; an id already in use is refused.
func (s *Store) Create(st State) error {
	if err := ValidID(st.ID); err != nil {
		return err
	}
	if _, err := os.Stat(s.statePath(st.ID)); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, st.ID)
	}
	return s.write(&st)
}

// Load reads the checkpoint of analysis id.
func (s *Store) Load(id string) (State, error) {
	if err := ValidID(id); err != nil {
		return State{}, err
	}
	var st State
	data, err := os.ReadFile(s.statePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return State{}, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("analysis %s: unreadable checkpoint: %w", id, err)
	}
	return st, nil
}

// Save writes the checkpoint of st.
func (s *Store) Save(st *State) error { return s.write(st) }

func (s *Store) write(st *State) error {
	data, _ := json.MarshalIndent(st, "", "  ") // a struct of strings and times always marshals
	return fsutil.WriteFileAtomic(s.statePath(st.ID), append(data, '\n'))
}

// Delete removes analysis id and everything under it.
func (s *Store) Delete(id string) error {
	if err := ValidID(id); err != nil {
		return err
	}
	return os.RemoveAll(s.Dir(id))
}
