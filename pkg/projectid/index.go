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

// Entry represents one project recorded in the global index.
type Entry struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Remote   string    `json:"remote,omitempty"` // normalized
	Name     string    `json:"name,omitempty"`
	LastSeen time.Time `json:"last_seen"`
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

// Register ensures a project ID exists for dir, then records it in the index.
// Register = Ensure(dir) + Upsert{ID, abs Path, NormalizeRemote(GitRemote(dir)), Name: filepath.Base(dir)}.
// It returns the newly registered or updated entry.
func (ix *Index) Register(dir string) (Entry, error) {
	id, err := Ensure(dir)
	if err != nil {
		return Entry{}, err
	}

	absPath, _ := filepath.Abs(dir) // Ensure already succeeded, so the directory exists.

	remote := NormalizeRemote(GitRemote(dir))
	ix.Upsert(Entry{ID: id, Path: absPath, Remote: remote, Name: filepath.Base(absPath)})

	stored, _ := ix.Lookup(id)
	return stored, nil
}
