package synapse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readOnly makes dir unwritable for the rest of the test.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// registeredClone sets up a project with a root commit and a remote, clones it elsewhere (a clone
// has no identity file) and removes the original, so the index entry is stale and findable only by
// its root commit. It returns the project ID and the clone's directory.
func (e *healEnv) registeredClone(name string) (id, clone string) {
	e.t.Helper()
	dir, id := e.project("ws/" + name)
	gitInit(e.t, dir, "git@github.com:acme/"+name+".git")
	gitCommit(e.t, dir)
	if _, err := e.index.Register(dir); err != nil {
		e.t.Fatal(err)
	}
	if err := e.index.Save(); err != nil {
		e.t.Fatal(err)
	}
	clone = filepath.Join(e.root, "clones", name)
	gitClone(e.t, dir, clone, "https://github.com/acme/"+name)
	if err := os.RemoveAll(dir); err != nil {
		e.t.Fatal(err)
	}
	return id, clone
}

func (e *healEnv) refresh(apply, prune bool) ([]RefreshRow, error) {
	return RefreshIndex(e.store, e.index, RefreshOptions{Roots: []string{e.root}, Apply: apply, Prune: prune, NoDefaultRoots: true})
}

func TestPeersOfReportsAnUnreadableStore(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synapses")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PeersOf(NewStore(file), nil, "dpid_00000000000000000000000000000000"); err == nil {
		t.Fatal("expected the store error")
	}
}

func TestRefreshIndexReportsFailures(t *testing.T) {
	t.Run("backfilling the plane binding cannot save the index", func(t *testing.T) {
		e := newHealEnv(t)
		dir, _ := e.project("ws/bound")
		if err := os.MkdirAll(filepath.Join(dir, ".dendrite"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".dendrite", "plane.json"), []byte(`{"identifier":"DEN"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := e.index.Save(); err != nil {
			t.Fatal(err)
		}
		readOnly(t, filepath.Dir(e.index.Path()))
		if _, err := e.refresh(true, false); err == nil || !strings.Contains(err.Error(), "saving project index") {
			t.Fatalf("got %v, want the index save error", err)
		}
	})

	t.Run("the synapse store cannot be listed", func(t *testing.T) {
		e := newHealEnv(t)
		dir, _ := e.project("ws/gone")
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(e.root, "synapses-file")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		e.store = NewStore(file)
		if _, err := e.refresh(true, false); err == nil {
			t.Fatal("expected the store error")
		}
	})

	t.Run("a clone whose identity cannot be restored", func(t *testing.T) {
		e := newHealEnv(t)
		_, clone := e.registeredClone("locked")
		readOnly(t, clone)
		if _, err := e.refresh(true, false); err == nil {
			t.Fatal("expected the identity write error")
		}
	})

	t.Run("pruning cannot save the index", func(t *testing.T) {
		e := newHealEnv(t)
		dir, _ := e.project("ws/vanished")
		if err := e.index.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		readOnly(t, filepath.Dir(e.index.Path()))
		if _, err := e.refresh(true, true); err == nil || !strings.Contains(err.Error(), "saving project index") {
			t.Fatalf("got %v, want the index save error", err)
		}
	})
}

func TestRefreshIgnoresDirectoriesThatAlreadyHaveAnIdentity(t *testing.T) {
	e := newHealEnv(t)
	id, clone := e.registeredClone("shared")

	// A different project's directory carries its own ID, so it can never be this entry's new home.
	other, _ := e.project("ws/other")
	gitInit(t, other, "https://github.com/acme/shared")

	rows, err := e.refresh(false, false)
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, id)
	if r.Status != RefreshRelocated || r.NewPath != clone {
		t.Fatalf("row = %+v, want it relocated to the clone %s", r, clone)
	}
}

func TestRefreshReportsSeveralCandidatesWithoutGuessing(t *testing.T) {
	e := newHealEnv(t)
	id, clone := e.registeredClone("twin")
	twin := filepath.Join(e.root, "clones", "twin-copy")
	gitClone(t, clone, twin, "https://github.com/acme/twin")

	rows, err := e.refresh(false, false)
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, id)
	if r.Status != RefreshAmbiguous || len(r.Candidates) != 2 || r.NewPath != "" {
		t.Fatalf("row = %+v, want two candidates and no new path", r)
	}
}

func TestNormalizePathKeepsTheInputWhenTheWorkingDirectoryIsGone(t *testing.T) {
	old := absPath
	absPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { absPath = old })
	if got := normalizePath("rel/dir"); got != "rel/dir" {
		t.Fatalf("normalizePath = %q, want the input back", got)
	}
}

func TestIndexedPathIsEmptyWithoutAHomeDirectory(t *testing.T) {
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if got := indexedPath("dpid_00000000000000000000000000000000"); got != "" {
		t.Fatalf("indexedPath = %q, want empty", got)
	}
}

func TestIndexedPathIsEmptyWhenTheIndexIsCorrupt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "projects.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := indexedPath("dpid_00000000000000000000000000000000"); got != "" {
		t.Fatalf("indexedPath = %q, want empty", got)
	}
}
