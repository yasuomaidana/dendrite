package synapse

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTestProjectID(t *testing.T, dir, projectID string) {
	t.Helper()
	dendriteDir := filepath.Join(dir, ".dendrite")
	if err := os.MkdirAll(dendriteDir, 0755); err != nil {
		t.Fatalf("failed to create .dendrite dir: %v", err)
	}
	idFile := filepath.Join(dendriteDir, "project.id")
	if err := os.WriteFile(idFile, []byte(projectID+"\n"), 0644); err != nil {
		t.Fatalf("failed to write project.id file: %v", err)
	}
}

func TestResolveOK(t *testing.T) {
	dir := t.TempDir()
	pid := "dpid_0123456789abcdef0123456789abcdef"
	writeTestProjectID(t, dir, pid)

	m := Member{
		ProjectID: pid,
		Alias:     "core-repo",
		Role:      RoleProducer,
		PathHint:  dir,
	}

	path, err := Resolve(m)
	if err != nil {
		t.Fatalf("expected Resolve to succeed, got error: %v", err)
	}
	if path != dir {
		t.Fatalf("expected resolved path %s, got %s", dir, path)
	}

	status := StatusOf(m)
	if status != StatusOK {
		t.Fatalf("expected status %s, got %s", StatusOK, status)
	}
}

func TestResolveMovedDifferentID(t *testing.T) {
	dir := t.TempDir()
	storedPID := "dpid_0123456789abcdef0123456789abcdef"
	expectedPID := "dpid_fedcba9876543210fedcba9876543210"
	writeTestProjectID(t, dir, storedPID)

	m := Member{
		ProjectID: expectedPID,
		Alias:     "core-repo",
		Role:      RoleProducer,
		PathHint:  dir,
	}

	_, err := Resolve(m)
	if err == nil {
		t.Fatalf("expected Resolve to return error for mismatched project ID, got nil")
	}
	if !errors.Is(err, ErrMemberMoved) {
		t.Fatalf("expected ErrMemberMoved, got %v", err)
	}

	status := StatusOf(m)
	if status != StatusMoved {
		t.Fatalf("expected status %s, got %s", StatusMoved, status)
	}
}

func TestResolveMovedAbsentProjectIDFile(t *testing.T) {
	dir := t.TempDir() // Directory exists, but no .dendrite/project.id
	expectedPID := "dpid_0123456789abcdef0123456789abcdef"

	m := Member{
		ProjectID: expectedPID,
		Alias:     "core-repo",
		Role:      RoleProducer,
		PathHint:  dir,
	}

	_, err := Resolve(m)
	if err == nil {
		t.Fatalf("expected Resolve to fail when project.id is absent, got nil")
	}
	if !errors.Is(err, ErrMemberMoved) {
		t.Fatalf("expected ErrMemberMoved, got %v", err)
	}

	status := StatusOf(m)
	if status != StatusMoved {
		t.Fatalf("expected status %s, got %s", StatusMoved, status)
	}
}

func TestResolveMissingPath(t *testing.T) {
	nonExistentDir := filepath.Join(t.TempDir(), "does-not-exist")
	expectedPID := "dpid_0123456789abcdef0123456789abcdef"

	m := Member{
		ProjectID: expectedPID,
		Alias:     "core-repo",
		Role:      RoleProducer,
		PathHint:  nonExistentDir,
	}

	_, err := Resolve(m)
	if err == nil {
		t.Fatalf("expected Resolve to fail for non-existent path, got nil")
	}
	if !errors.Is(err, ErrMemberMissing) {
		t.Fatalf("expected ErrMemberMissing, got %v", err)
	}

	status := StatusOf(m)
	if status != StatusMissing {
		t.Fatalf("expected status %s, got %s", StatusMissing, status)
	}

	// Empty PathHint
	mEmpty := Member{
		ProjectID: expectedPID,
		Alias:     "empty-repo",
		Role:      RolePeer,
		PathHint:  "",
	}
	_, err = Resolve(mEmpty)
	if !errors.Is(err, ErrMemberMissing) {
		t.Fatalf("expected ErrMemberMissing for empty PathHint, got %v", err)
	}
	if StatusOf(mEmpty) != StatusMissing {
		t.Fatalf("expected StatusMissing for empty PathHint")
	}
}

func TestSynapseHealth(t *testing.T) {
	dirOK := t.TempDir()
	pidOK := "dpid_11111111111111111111111111111111"
	writeTestProjectID(t, dirOK, pidOK)

	dirMoved := t.TempDir()
	pidMovedExpected := "dpid_22222222222222222222222222222222"
	// dirMoved has no project.id file -> MOVED

	dirMissing := filepath.Join(t.TempDir(), "nonexistent")
	pidMissing := "dpid_33333333333333333333333333333333"

	s := &Synapse{
		ID:    "mesh-synapse",
		Name:  "Mesh Synapse",
		State: StateActive,
		Members: []Member{
			{ProjectID: pidOK, Alias: "ok-repo", Role: RoleProducer, PathHint: dirOK},
			{ProjectID: pidMovedExpected, Alias: "moved-repo", Role: RoleConsumer, PathHint: dirMoved},
			{ProjectID: pidMissing, Alias: "missing-repo", Role: RolePeer, PathHint: dirMissing},
		},
	}

	health := s.Health()
	if len(health) != 3 {
		t.Fatalf("expected health matrix with 3 entries, got %d", len(health))
	}

	if health["ok-repo"] != StatusOK {
		t.Fatalf("expected ok-repo to have StatusOK, got %s", health["ok-repo"])
	}
	if health["moved-repo"] != StatusMoved {
		t.Fatalf("expected moved-repo to have StatusMoved, got %s", health["moved-repo"])
	}
	if health["missing-repo"] != StatusMissing {
		t.Fatalf("expected missing-repo to have StatusMissing, got %s", health["missing-repo"])
	}

	// Nil synapse
	var nilSynapse *Synapse
	if h := nilSynapse.Health(); h != nil {
		t.Fatalf("expected nil health matrix for nil synapse, got %+v", h)
	}
}
