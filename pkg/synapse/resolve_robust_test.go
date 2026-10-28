package synapse

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/projectid"
)

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
