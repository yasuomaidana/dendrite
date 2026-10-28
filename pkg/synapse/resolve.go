package synapse

import (
	"Dendrite/pkg/projectid"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Standard typed errors for member resolution.
var (
	ErrMemberMoved   = errors.New("synapse: member moved")
	ErrMemberMissing = errors.New("synapse: member missing")
)

// MemberStatus describes the accessibility and identity verification state of a member repository.
type MemberStatus string

const (
	StatusOK      MemberStatus = "OK"
	StatusMoved   MemberStatus = "MOVED"
	StatusMissing MemberStatus = "MISSING"
	// StatusRemote is a member consumed from its remote: it has no local checkout, and needs none.
	StatusRemote MemberStatus = "REMOTE"
)

// Healthy reports whether the status needs no attention: the member resolves, or it is deliberately
// consumed from its remote.
func (s MemberStatus) Healthy() bool { return s == StatusOK || s == StatusRemote }

// Resolve finds the member's local repository. It tries PathHint (with ~ expanded and made absolute)
// and, when that is missing or holds another project, the path the global project index records for
// the member's project ID, so a repository that moved is found without running heal. A candidate
// counts only when its project.id matches the member's.
// Returns ErrMemberMissing when no candidate exists and ErrMemberMoved when one exists but is
// another project.
func Resolve(m Member) (string, error) {
	hint := normalizePath(m.PathHint)
	var candidates []string
	if hint != "" {
		candidates = append(candidates, hint)
	}
	if p := indexedPath(m.ProjectID); p != "" && p != hint {
		candidates = append(candidates, p)
	}
	if len(candidates) == 0 {
		return "", ErrMemberMissing
	}

	sawProject := false
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		sawProject = true
		if id, err := projectid.Read(dir); err == nil && id == m.ProjectID {
			return dir, nil
		}
	}
	if sawProject {
		return "", ErrMemberMoved
	}
	return "", ErrMemberMissing
}

// normalizePath expands a leading ~ and makes the path absolute.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// indexedPath is where the global project index says the project lives ("" when unknown).
func indexedPath(projectID string) string {
	path, err := projectid.DefaultIndexPath()
	if err != nil || projectID == "" {
		return ""
	}
	ix, err := projectid.LoadIndex(path)
	if err != nil {
		return ""
	}
	if e, ok := ix.Lookup(projectID); ok {
		return e.Path
	}
	return ""
}

// StatusOf evaluates the health status of a member repository.
func StatusOf(m Member) MemberStatus {
	_, err := Resolve(m)
	switch {
	case err == nil:
		return StatusOK
	case m.IsRemote():
		return StatusRemote
	case errors.Is(err, ErrMemberMoved):
		return StatusMoved
	}
	return StatusMissing
}

// Health evaluates each member in the Synapse and returns a mapping from alias to MemberStatus.
func (s *Synapse) Health() map[string]MemberStatus {
	if s == nil {
		return nil
	}
	health := make(map[string]MemberStatus, len(s.Members))
	for _, m := range s.Members {
		health[m.Alias] = StatusOf(m)
	}
	return health
}
