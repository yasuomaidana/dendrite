package plane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/projectid"
)

// LinkFileName is the local file, under a repository's .dendrite/ directory (never committed),
// that names the Plane project tracking that repository's work.
const LinkFileName = "plane.json"

// ErrNoRepoRoot is returned when a directory is not inside a git repository.
var ErrNoRepoRoot = errors.New("not inside a git repository")

// ProjectLink records which Plane project tracks a repository.
type ProjectLink struct {
	Identifier string `json:"identifier"`
	ProjectID  string `json:"project_id"`
	Name       string `json:"name,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
}

var identifierCleanRegex = regexp.MustCompile(`[^A-Z0-9]`)

// LinkPath returns the link file location for a repository root; a linked worktree
// resolves to its main checkout's file.
func LinkPath(root string) string {
	return filepath.Join(projectid.StateRoot(root), ".dendrite", LinkFileName)
}

// FindRepoRoot walks up from dir to the nearest directory holding .git
// (a directory for normal checkouts, a file for worktrees).
func FindRepoRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", ErrNoRepoRoot
		}
		abs = parent
	}
}

// ReadLink loads the link file from a repository root.
func ReadLink(root string) (*ProjectLink, error) {
	data, err := os.ReadFile(LinkPath(root))
	if err != nil {
		return nil, err
	}
	var l ProjectLink
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse %s: %w", LinkPath(root), err)
	}
	if strings.TrimSpace(l.ProjectID) == "" {
		return nil, fmt.Errorf("%s has no project_id", LinkPath(root))
	}
	return &l, nil
}

// WriteLink stores the link file in a repository root.
func WriteLink(root string, l ProjectLink) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(LinkPath(root), append(data, '\n'))
}

// indexedLink returns the Plane binding the global index keeps for the repository at root.
// A fresh clone has no .dendrite/plane.json (it is not committed) but is recognised by its
// project ID once `dendrite chronos project refresh` or `ensure` has run in it.
func indexedLink(root string) *ProjectLink {
	id, err := projectid.Read(root)
	if err != nil {
		return nil
	}
	path, err := projectid.DefaultIndexPath()
	if err != nil {
		return nil
	}
	ix, err := projectid.LoadIndex(path)
	if err != nil {
		return nil
	}
	e, ok := ix.Lookup(id)
	if !ok || e.Plane == nil || strings.TrimSpace(e.Plane.ProjectID) == "" {
		return nil
	}
	return &ProjectLink{Identifier: e.Plane.Identifier, ProjectID: e.Plane.ProjectID, Name: e.Plane.Name, Workspace: e.Plane.Workspace}
}

// LinkForDir returns the link of the repository containing dir.
func LinkForDir(dir string) (*ProjectLink, string, error) {
	root, err := FindRepoRoot(dir)
	if err != nil {
		return nil, "", err
	}
	l, err := ReadLink(root)
	if err != nil {
		if fallback := indexedLink(root); fallback != nil {
			return fallback, root, nil
		}
		return nil, root, err
	}
	return l, root, nil
}

// SuggestIdentifier derives a Plane identifier (A-Z0-9, at most 10 characters) from a name.
func SuggestIdentifier(name string) string {
	id := identifierCleanRegex.ReplaceAllString(strings.ToUpper(name), "")
	if len(id) > 10 {
		id = id[:10]
	}
	return id
}

// FindProject returns the workspace project with the given identifier or UUID, or nil when none exists.
func (c *Client) FindProject(ctx context.Context, ref string) (*PlaneProject, error) {
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].ID == ref || strings.EqualFold(projects[i].Identifier, ref) {
			return &projects[i], nil
		}
	}
	return nil, nil
}

// EnsureProject adopts the project with the given identifier, or creates it, and converges
// its workflow states onto CanonicalStates. created reports whether it was new.
func (c *Client) EnsureProject(ctx context.Context, name, identifier, description string) (proj *PlaneProject, created bool, err error) {
	proj, err = c.FindProject(ctx, identifier)
	if err != nil {
		return nil, false, err
	}
	if proj == nil {
		if proj, err = c.CreateProject(ctx, name, identifier, description); err != nil {
			return nil, false, err
		}
		created = true
	}
	if _, err := c.EnsureLifecycleStates(ctx, proj.ID); err != nil {
		return proj, created, fmt.Errorf("ensure lifecycle states: %w", err)
	}
	return proj, created, nil
}
