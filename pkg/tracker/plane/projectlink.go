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
)

// LinkFileName is the committed file, under a repository's .dendrite/ directory,
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

// LinkPath returns the link file location for a repository root.
func LinkPath(root string) string {
	return filepath.Join(root, ".dendrite", LinkFileName)
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

// LinkForDir returns the link of the repository containing dir.
func LinkForDir(dir string) (*ProjectLink, string, error) {
	root, err := FindRepoRoot(dir)
	if err != nil {
		return nil, "", err
	}
	l, err := ReadLink(root)
	if err != nil {
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

// WithProject returns a copy of the client scoped to another project of the same workspace.
func (c *Client) WithProject(projectID string) *Client {
	cp := *c
	cp.projectID = projectID
	return &cp
}

// FindProject returns the workspace project with the given identifier or name, or nil.
func (c *Client) FindProject(ctx context.Context, identifier, name string) (*PlaneProject, error) {
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if identifier != "" && strings.EqualFold(projects[i].Identifier, identifier) {
			return &projects[i], nil
		}
	}
	for i := range projects {
		if name != "" && strings.EqualFold(projects[i].Name, name) {
			return &projects[i], nil
		}
	}
	return nil, nil
}

// EnsureProject adopts the project matching identifier or name, or creates it,
// and provisions the lifecycle states. created reports whether it was new.
func (c *Client) EnsureProject(ctx context.Context, name, identifier, description string) (proj *PlaneProject, created bool, err error) {
	proj, err = c.FindProject(ctx, identifier, name)
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
