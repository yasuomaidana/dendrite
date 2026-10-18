package synapse

import (
	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/projectid"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DirName defines the subdirectory name within Dendrite home where synapses reside.
const DirName = "synapses"

// ErrNotFound is returned when a requested synapse manifest cannot be found.
var ErrNotFound = errors.New("synapse: not found")

// ErrInvalidID is returned when an ID violates the required format or attempts directory traversal.
var ErrInvalidID = errors.New("synapse: invalid id")

// Store provides persistent file-backed CRUD operations for Synapse manifests.
type Store struct {
	dir string
}

// NewStore creates a new Store instance managing manifests in dir.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// DefaultStore returns a Store rooted under projectid.HomeDir()/synapses.
func DefaultStore() (*Store, error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return nil, err
	}
	return NewStore(filepath.Join(home, DirName)), nil
}

// Dir returns the directory managed by this store.
func (st *Store) Dir() string {
	return st.dir
}

// Save validates the Synapse, initializes CreatedAt if zero, sets UpdatedAt to now (UTC),
// ensures the store directory exists, and atomically writes <id>.toml via a temp file.
func (st *Store) Save(s *Synapse) error {
	if s == nil {
		return errors.New("synapse is nil")
	}

	now := time.Now().UTC().Truncate(time.Second)
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now

	if err := s.Validate(); err != nil {
		return err
	}

	data, _ := Marshal(s) // Marshal only fails on a nil synapse, rejected above.
	return fsutil.WriteFileAtomic(filepath.Join(st.dir, s.ID+".toml"), data)
}

// Load retrieves and decodes a Synapse manifest by ID.
// Rejects IDs failing the ID regex to prevent directory traversal attacks.
// Returns ErrNotFound if the file does not exist.
func (st *Store) Load(id string) (*Synapse, error) {
	if !idRegex.MatchString(id) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidID, id)
	}

	target := filepath.Join(st.dir, id+".toml")
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return Unmarshal(data)
}

// List returns all stored Synapse manifests sorted lexicographically by ID.
// Ignores non-.toml files and directories. Returns an error naming the bad file if parsing fails.
func (st *Store) List() ([]*Synapse, error) {
	entries, err := os.ReadDir(st.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Synapse{}, nil
		}
		return nil, err
	}

	var list []*Synapse
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}

		target := filepath.Join(st.dir, name)
		data, err := os.ReadFile(target)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", name, err)
		}

		s, err := Unmarshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", name, err)
		}
		list = append(list, s)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})

	return list, nil
}

// Delete removes the manifest file <id>.toml.
// Rejects IDs failing the ID regex. Returns ErrNotFound if the file does not exist.
func (st *Store) Delete(id string) error {
	if !idRegex.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}

	target := filepath.Join(st.dir, id+".toml")
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// ReferencingProject finds all Synapses in the store that include projectID as a member.
func (st *Store) ReferencingProject(projectID string) ([]*Synapse, error) {
	all, err := st.List()
	if err != nil {
		return nil, err
	}

	var matched []*Synapse
	for _, s := range all {
		if _, ok := s.Member(projectID); ok {
			matched = append(matched, s)
		}
	}
	return matched, nil
}
