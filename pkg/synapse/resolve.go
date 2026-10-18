package synapse

import (
	"Dendrite/pkg/projectid"
	"errors"
	"os"
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
)

// Resolve verifies that the member repository exists at PathHint and has matching project.id.
// Returns PathHint when projectid.Read(m.PathHint) == m.ProjectID.
// Returns ErrMemberMissing if m.PathHint does not exist.
// Returns ErrMemberMoved if m.PathHint exists but project.id differs or is absent.
func Resolve(m Member) (string, error) {
	if m.PathHint == "" {
		return "", ErrMemberMissing
	}

	_, err := os.Stat(m.PathHint)
	if err != nil {
		return "", ErrMemberMissing
	}

	id, err := projectid.Read(m.PathHint)
	if err != nil || id != m.ProjectID {
		return "", ErrMemberMoved
	}

	return m.PathHint, nil
}

// StatusOf evaluates the health status of a member repository.
func StatusOf(m Member) MemberStatus {
	_, err := Resolve(m)
	if err == nil {
		return StatusOK
	}
	if errors.Is(err, ErrMemberMoved) {
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
