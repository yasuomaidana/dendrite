package projectid

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"Dendrite/pkg/fsutil"
)

// UntrackedEntry records a repository that was deliberately detached from Plane.
type UntrackedEntry struct {
	Name      string    `json:"name"`
	Path      string    `json:"path,omitempty"`
	RemovedAt time.Time `json:"removed_at"`
}

// Untracked is the set of repositories that are intentionally not Plane projects, keyed by project
// ID, so the project audit does not report them as missing one.
type Untracked struct {
	path     string
	Projects map[string]UntrackedEntry `json:"projects"`
}

// DefaultUntrackedPath returns HomeDir()/untracked.json.
func DefaultUntrackedPath() (string, error) {
	home, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "untracked.json"), nil
}

// LoadUntracked reads the file; a missing file yields an empty set.
func LoadUntracked(path string) (*Untracked, error) {
	u := &Untracked{path: path, Projects: map[string]UntrackedEntry{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return u, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, u); err != nil {
		return nil, err
	}
	if u.Projects == nil {
		u.Projects = map[string]UntrackedEntry{}
	}
	return u, nil
}

// Add records the project as intentionally not tracked.
func (u *Untracked) Add(id, name, path string) {
	u.Projects[id] = UntrackedEntry{Name: name, Path: path, RemovedAt: time.Now().UTC()}
}

// Has reports whether the project ID was detached from Plane on purpose.
func (u *Untracked) Has(id string) bool {
	_, ok := u.Projects[id]
	return ok
}

// IDs returns the recorded project IDs, sorted.
func (u *Untracked) IDs() []string {
	ids := make([]string, 0, len(u.Projects))
	for id := range u.Projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Save writes the set atomically.
func (u *Untracked) Save() error {
	data, _ := json.MarshalIndent(u, "", "  ")
	return fsutil.WriteFileAtomic(u.path, append(data, '\n'))
}
