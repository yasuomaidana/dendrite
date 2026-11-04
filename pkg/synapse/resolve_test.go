package synapse

import (
	"Dendrite/pkg/projectid"
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

func TestMemberStatusHealthy(t *testing.T) {
	if !StatusOK.Healthy() {
		t.Error("expected StatusOK to be healthy")
	}
	if !StatusRemote.Healthy() {
		t.Error("expected StatusRemote to be healthy")
	}
	if StatusMoved.Healthy() {
		t.Error("expected StatusMoved to be unhealthy")
	}
	if StatusMissing.Healthy() {
		t.Error("expected StatusMissing to be unhealthy")
	}
}

func TestResolveRemoteMember(t *testing.T) {
	m := Member{
		ProjectID: "dpid_0123456789abcdef0123456789abcdef",
		Alias:     "remote-repo",
		Source:    SourceRemote,
		PathHint:  "/nonexistent/remote/path",
	}

	status := StatusOf(m)
	if status != StatusRemote {
		t.Errorf("expected StatusRemote, got %s", status)
	}
}

func TestResolveViaProjectIndex(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("DENDRITE_HOME", tmpHome)

	targetDir := t.TempDir()
	pid := "dpid_1234567890abcdef1234567890abcdef"
	writeTestProjectID(t, targetDir, pid)

	// Write global index referencing targetDir
	idxJSON := `{"version": 1, "projects": {"` + pid + `": {"id": "` + pid + `", "path": "` + targetDir + `", "last_seen": "2026-10-18T00:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(tmpHome, "projects.json"), []byte(idxJSON), 0644); err != nil {
		t.Fatalf("failed to write projects.json: %v", err)
	}

	// PathHint is missing, but member should resolve via index
	m := Member{
		ProjectID: pid,
		Alias:     "indexed-repo",
		PathHint:  filepath.Join(t.TempDir(), "nonexistent"),
	}

	resolved, err := Resolve(m)
	if err != nil {
		t.Fatalf("expected Resolve to find directory via index: %v", err)
	}
	if resolved != targetDir {
		t.Errorf("expected resolved path %s, got %s", targetDir, resolved)
	}
	if st := StatusOf(m); st != StatusOK {
		t.Errorf("expected StatusOK, got %s", st)
	}

	// Empty project ID handling
	if got := indexedPath(""); got != "" {
		t.Errorf("expected empty string for empty projectID, got %q", got)
	}
}

func TestNormalizePath(t *testing.T) {
	if got := normalizePath(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := normalizePath("  "); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if got := normalizePath("~"); got != home {
			t.Errorf("expected %s, got %s", home, got)
		}
		if got := normalizePath("~/dendrite"); got != filepath.Join(home, "dendrite") {
			t.Errorf("expected %s, got %s", filepath.Join(home, "dendrite"), got)
		}
	}
}

// project makes a directory with a project.id registered in the Dendrite home's index.
func project(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := projectid.DefaultIndexPath()
	if err != nil {
		t.Fatal(err)
	}
	ix, err := projectid.LoadIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	e, err := ix.Register(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	return e.ID
}

func TestResolveFindsAMovedMemberThroughTheProjectIndex(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	root := t.TempDir()
	id := project(t, filepath.Join(root, "now-here"))

	m := Member{Alias: "api", ProjectID: id, PathHint: filepath.Join(root, "was-here")}
	got, err := Resolve(m)
	if err != nil || got != filepath.Join(root, "now-here") {
		t.Fatalf("Resolve = %q, %v: want the indexed location", got, err)
	}
	if StatusOf(m) != StatusOK {
		t.Errorf("status = %s, want OK", StatusOf(m))
	}
	// A hint that exists but is another project does not mask the right one.
	other := filepath.Join(root, "other")
	project(t, other)
	m.PathHint = other
	if got, err := Resolve(m); err != nil || got != filepath.Join(root, "now-here") {
		t.Errorf("with a wrong-project hint: %q, %v", got, err)
	}
}

func TestResolveExpandsTildeAndRelativeHints(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	id := project(t, filepath.Join(home, "work", "web"))
	got, err := Resolve(Member{ProjectID: id, PathHint: "~/work/web"})
	if err != nil || got != filepath.Join(home, "work", "web") {
		t.Fatalf("~ hint: %q, %v", got, err)
	}
	t.Chdir(filepath.Join(home, "work"))
	if got, err := Resolve(Member{ProjectID: id, PathHint: "web"}); err != nil || got != filepath.Join(home, "work", "web") {
		t.Errorf("relative hint: %q, %v", got, err)
	}
}

func TestRemoteMembersWithoutACheckoutAreHealthyAndLocalOnesAreNot(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	gone := filepath.Join(t.TempDir(), "gone")
	remote := Member{Alias: "lib", ProjectID: "dpid_remote", PathHint: gone, Source: SourceRemote}
	if s := StatusOf(remote); s != StatusRemote || !s.Healthy() {
		t.Errorf("remote member status = %s healthy=%v, want REMOTE and healthy", s, s.Healthy())
	}
	local := Member{Alias: "app", ProjectID: "dpid_local", PathHint: gone}
	if s := StatusOf(local); s != StatusMissing || s.Healthy() {
		t.Errorf("missing local member status = %s healthy=%v", s, s.Healthy())
	}
	if _, err := Resolve(local); !errors.Is(err, ErrMemberMissing) {
		t.Errorf("err = %v, want ErrMemberMissing", err)
	}
	// A directory that exists but is a different project is MOVED, not missing.
	other := t.TempDir()
	project(t, other)
	if s := StatusOf(Member{ProjectID: "dpid_someone_else", PathHint: other}); s != StatusMoved {
		t.Errorf("wrong project at the hint: status = %s, want MOVED", s)
	}
	s := &Synapse{Members: []Member{remote}}
	if h := s.Health(); !h["lib"].Healthy() {
		t.Errorf("a synapse of one remote member must read healthy: %v", h)
	}
}
