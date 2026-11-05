package projectid

import (
	"Dendrite/pkg/fsutil"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// PlaneBinding is the Plane project that tracks a repository. The global index keeps it
// so the binding survives a fresh clone, because .dendrite/ is never committed.
type PlaneBinding struct {
	Identifier string `json:"identifier"`
	ProjectID  string `json:"project_id"`
	Name       string `json:"name,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
}

// Entry represents one project recorded in the global index.
type Entry struct {
	ID         string        `json:"id"`
	Path       string        `json:"path"`
	Remote     string        `json:"remote,omitempty"` // normalized
	Name       string        `json:"name,omitempty"`
	RootCommit string        `json:"root_commit,omitempty"` // first commit of the repository: identity across clones
	Plane      *PlaneBinding `json:"plane,omitempty"`
	LastSeen   time.Time     `json:"last_seen"`
}

// Index tracks known Dendrite projects across the local machine.
type Index struct {
	path     string
	mu       sync.Mutex
	Version  int              `json:"version"`
	Projects map[string]Entry `json:"projects"`
}

// HomeDir returns $DENDRITE_HOME if set, else ~/.dendrite.
func HomeDir() (string, error) {
	if h := os.Getenv("DENDRITE_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("projectid: get user home directory: %w", err)
	}
	return filepath.Join(home, ".dendrite"), nil
}

// DefaultIndexPath returns HomeDir()/projects.json.
func DefaultIndexPath() (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "projects.json"), nil
}

// LoadIndex reads the index file from disk; a missing file yields an empty index (Version 1).
// path is remembered for subsequent calls to Save.
func LoadIndex(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Index{
				path:     path,
				Version:  1,
				Projects: make(map[string]Entry),
			}, nil
		}
		return nil, err
	}

	var ix Index
	if err := json.Unmarshal(data, &ix); err != nil {
		return nil, err
	}
	ix.path = path
	if ix.Projects == nil {
		ix.Projects = make(map[string]Entry)
	}
	if ix.Version == 0 {
		ix.Version = 1
	}
	return &ix, nil
}

// Path returns the file the index was loaded from and saves to.
func (ix *Index) Path() string {
	return ix.path
}

// Save writes the index atomically to disk using a temporary file and rename.
func (ix *Index) Save() error {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	// Entries hold only strings and times, so marshaling cannot fail.
	data, _ := json.MarshalIndent(ix, "", "  ")
	return fsutil.WriteFileAtomic(ix.path, append(data, '\n'))
}

// Upsert adds or updates an entry keyed by its ID.
// It sets LastSeen to time.Now().UTC() if zero, and normalizes Remote.
func (ix *Index) Upsert(e Entry) {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	if ix.Projects == nil {
		ix.Projects = make(map[string]Entry)
	}
	if e.LastSeen.IsZero() {
		e.LastSeen = time.Now().UTC()
	} else {
		e.LastSeen = e.LastSeen.UTC()
	}
	e.Remote = NormalizeRemote(e.Remote)
	ix.Projects[e.ID] = e
}

// Lookup finds an entry by ID.
func (ix *Index) Lookup(id string) (Entry, bool) {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	e, ok := ix.Projects[id]
	return e, ok
}

// LookupByRemote finds all entries matching the given remote (normalized), sorted by ID.
func (ix *Index) LookupByRemote(remote string) []Entry {
	norm := NormalizeRemote(remote)
	if norm == "" {
		return []Entry{}
	}

	ix.mu.Lock()
	defer ix.mu.Unlock()

	matches := make([]Entry, 0)
	for _, e := range ix.Projects {
		if e.Remote == norm {
			matches = append(matches, e)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].ID < matches[j].ID
	})
	return matches
}

// LookupByPath finds an entry by comparing cleaned file paths.
func (ix *Index) LookupByPath(path string) (Entry, bool) {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	cleanPath := filepath.Clean(path)
	absPath, err := filepath.Abs(path)

	for _, e := range ix.Projects {
		cleanEntryPath := filepath.Clean(e.Path)
		if cleanEntryPath == cleanPath || (err == nil && cleanEntryPath == filepath.Clean(absPath)) {
			return e, true
		}
	}
	return Entry{}, false
}

// Remove deletes an entry by ID, returning true if it existed.
func (ix *Index) Remove(id string) bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	if _, ok := ix.Projects[id]; ok {
		delete(ix.Projects, id)
		return true
	}
	return false
}

// List returns all entries in the index sorted by ID.
func (ix *Index) List() []Entry {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	list := make([]Entry, 0, len(ix.Projects))
	for _, e := range ix.Projects {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

// LookupByRootCommit finds the entry of a repository by its first commit. When the repository
// has a remote, the entry's remote must match (a fork has its own identity). The match must be
// unique; an ambiguous one returns false rather than guessing.
func (ix *Index) LookupByRootCommit(root, remote string) (Entry, bool) {
	remote = NormalizeRemote(remote)
	if root == "" {
		return Entry{}, false
	}

	ix.mu.Lock()
	defer ix.mu.Unlock()

	var found []Entry
	for _, e := range ix.Projects {
		if e.RootCommit == root && (remote == "" || e.Remote == remote) {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		return Entry{}, false
	}
	return found[0], true
}

// SetPlane records (or with nil clears) the Plane binding of an indexed project.
// It returns false when the project is not in the index.
func (ix *Index) SetPlane(id string, b *PlaneBinding) bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	e, ok := ix.Projects[id]
	if !ok {
		return false
	}
	e.Plane = b
	ix.Projects[id] = e
	return true
}

// Register ensures a project ID exists for dir, then records it in the index.
// A linked worktree registers its main checkout. A checkout without .dendrite/project.id
// (a fresh clone, as .dendrite/ is not committed) takes the ID of the indexed project with the
// same root commit and remote before a new ID is minted. The Plane binding already in the index
// is kept. It returns the newly registered or updated entry.
func (ix *Index) Register(dir string) (Entry, error) {
	dir = StateRoot(dir)
	root := GitRootCommit(dir)
	remote := NormalizeRemote(GitRemote(dir))

	id, err := Read(dir)
	switch {
	case err == nil:
	case errors.Is(err, ErrNoProjectID):
		if known, ok := ix.LookupByRootCommit(root, remote); ok {
			id = known.ID
			if err := fsutil.WriteFileAtomic(Path(dir), []byte(id+"\n")); err != nil {
				return Entry{}, fmt.Errorf("projectid: write %s: %w", Path(dir), err)
			}
		} else if id, err = Ensure(dir); err != nil {
			return Entry{}, err
		}
	default:
		return Entry{}, err
	}

	absPath, _ := filepath.Abs(dir) // the directory exists, as its ID was just read or written.

	prev, _ := ix.Lookup(id)
	ix.Upsert(Entry{ID: id, Path: absPath, Remote: remote, Name: filepath.Base(absPath), RootCommit: root, Plane: prev.Plane})

	stored, _ := ix.Lookup(id)
	return stored, nil
}
