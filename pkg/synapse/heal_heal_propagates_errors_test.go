package synapse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
)

func TestHealPropagatesErrors(t *testing.T) {
	t.Run("store list", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "synapses")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Heal(NewStore(file), HealOptions{NoDefaultRoots: true}); err == nil {
			t.Fatal("expected list error")
		}
	})

	setup := func(t *testing.T) (*healEnv, string) {
		e := newHealEnv(t)
		aDir, aID := e.project("a")
		bDir, bID := e.project("b")
		e.synapse("ab", Member{ProjectID: aID, Alias: "a", Role: RolePeer, PathHint: aDir}, Member{ProjectID: bID, Alias: "b", Role: RolePeer, PathHint: bDir})
		move(t, aDir, filepath.Join(e.root, "moved", "a"))
		return e, aID
	}

	t.Run("manifest save", func(t *testing.T) {
		e, _ := setup(t)
		if err := os.Chmod(e.store.Dir(), 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(e.store.Dir(), 0o755) })
		if _, err := Heal(e.store, e.opts(true)); err == nil || !strings.Contains(err.Error(), "saving synapse") {
			t.Fatalf("got %v, want manifest save error", err)
		}
	})

	t.Run("index save", func(t *testing.T) {
		e, _ := setup(t)
		home := filepath.Dir(e.store.Dir())
		if err := os.Chmod(home, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(home, 0o755) })
		// Manifests live one level deeper and stay writable.
		if _, err := Heal(e.store, e.opts(true)); err == nil || !strings.Contains(err.Error(), "project index") {
			t.Fatalf("got %v, want index save error", err)
		}
	})

	t.Run("restoring identity of a clone", func(t *testing.T) {
		e := newHealEnv(t)
		apiDir, apiID := e.project("api")
		gitInit(t, apiDir, "git@github.com:acme/api.git")
		_, _ = e.index.Register(apiDir)
		webDir, webID := e.project("web")
		e.synapse("web-api", Member{ProjectID: apiID, Alias: "api", Role: RoleProducer, PathHint: apiDir}, Member{ProjectID: webID, Alias: "web", Role: RoleConsumer, PathHint: webDir})
		if err := os.RemoveAll(apiDir); err != nil {
			t.Fatal(err)
		}
		clone := filepath.Join(e.root, "clone", "api")
		gitInit(t, clone, "git@github.com:acme/api.git")
		if err := os.Chmod(clone, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(clone, 0o755) })
		if _, err := Heal(e.store, e.opts(true)); err == nil {
			t.Fatal("expected error writing project.id into a read-only clone")
		}
	})
}

func TestHealWithoutIndex(t *testing.T) {
	e := newHealEnv(t)
	aDir, aID := e.project("a")
	bDir, bID := e.project("b")
	e.synapse("ab", Member{ProjectID: aID, Alias: "a", Role: RolePeer, PathHint: aDir}, Member{ProjectID: bID, Alias: "b", Role: RolePeer, PathHint: bDir})
	dst := filepath.Join(e.root, "moved", "a")
	move(t, aDir, dst)
	rows, err := Heal(e.store, HealOptions{Roots: []string{e.root}, Apply: true, NoDefaultRoots: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := rowFor(t, rows, "a"); !r.Applied || r.NewPath != dst {
		t.Fatalf("row = %+v", r)
	}
}

func TestHealSharedMemberAndUnindexedProject(t *testing.T) {
	e := newHealEnv(t)
	coreDir, coreID := e.project("core")
	aDir, aID := e.project("a")
	bDir, bID := e.project("b")
	e.synapse("core-a", Member{ProjectID: coreID, Alias: "core", Role: RoleProducer, PathHint: coreDir}, Member{ProjectID: aID, Alias: "a", Role: RoleConsumer, PathHint: aDir})
	e.synapse("core-b", Member{ProjectID: coreID, Alias: "core", Role: RoleProducer, PathHint: coreDir}, Member{ProjectID: bID, Alias: "b", Role: RoleConsumer, PathHint: bDir})
	// A member with no path hint at all is simply missing.
	e.synapse("ghost", Member{ProjectID: "dpid_99999999999999999999999999999999", Alias: "ghost", Role: RolePeer}, Member{ProjectID: aID, Alias: "a", Role: RolePeer, PathHint: aDir})

	dst := filepath.Join(e.root, "moved", "core")
	move(t, coreDir, dst)
	// Fresh index: the moved project is unknown to it and gets a new entry on apply.
	fresh, err := projectid.LoadIndex(filepath.Join(e.root, "fresh.json"))
	if err != nil {
		t.Fatal(err)
	}
	opts := e.opts(true)
	opts.Index = fresh
	rows, err := Heal(e.store, opts)
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	for _, r := range rows {
		if r.Alias == "core" && r.Applied && r.NewPath == dst {
			applied++
		}
		if r.Alias == "ghost" && r.Status != StatusMissing {
			t.Fatalf("ghost row = %+v", r)
		}
	}
	if applied != 2 {
		t.Fatalf("core healed in %d synapses, want 2: %+v", applied, rows)
	}
	if got, ok := fresh.Lookup(coreID); !ok || got.Path != dst || got.Name != "core" {
		t.Fatalf("fresh index entry = %+v, %v", got, ok)
	}
}

func TestSearchRootsEnvOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rr := filepath.Join(home, "RustRoverProjects")
	custom := filepath.Join(home, "work")
	for _, d := range []string{rr, custom, filepath.Join(home, "GoLandProjects")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(EnvSearchRoots, "~/work"+string(os.PathListSeparator)+rr)
	roots := searchRoots(Member{}, HealOptions{})
	if len(roots) != 2 || roots[0] != custom || roots[1] != rr {
		t.Fatalf("roots = %v, want [%s %s] (GoLandProjects replaced by override)", roots, custom, rr)
	}
}

// worktreeOf turns dir into a linked worktree checkout carrying the same project id as main.
func worktreeOf(t *testing.T, main, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".dendrite"), 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := projectid.Read(main)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeProjectID(dir, id); err != nil {
		t.Fatal(err)
	}
	gitdir := "gitdir: " + filepath.Join(main, ".git", "worktrees", filepath.Base(dir)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte(gitdir), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocatePrefersMainCheckoutOverWorktree(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("a/proj")
	gitInit(t, dir, "https://example.com/proj.git")
	// The worktree sorts before the main checkout so a first-hit scan would pick it.
	worktreeOf(t, dir, filepath.Join(e.root, "a", "0-proj-feature"))

	moved := filepath.Join(e.root, "b", "proj")
	move(t, dir, moved)
	loc := Locate(Member{ProjectID: id, Alias: "proj", PathHint: dir}, e.opts(false))
	if loc.Path != moved || loc.Match != MatchID {
		t.Fatalf("Locate = %+v, want main checkout %s", loc, moved)
	}
}

func TestLocateFallsBackToSoleWorktree(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("a/proj")
	wt := filepath.Join(e.root, "c", "proj-wt")
	worktreeOf(t, dir, wt)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	loc := Locate(Member{ProjectID: id, Alias: "proj", PathHint: dir}, e.opts(false))
	if loc.Path != wt {
		t.Fatalf("Locate = %+v, want worktree %s", loc, wt)
	}
}

func TestLocateDuplicateIDIsAmbiguous(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("a/proj")
	clone := filepath.Join(e.root, "b", "proj")
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectID(clone, id); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(e.root, "c", "proj")
	move(t, dir, moved)

	other, otherID := e.project("other")
	m := Member{ProjectID: id, Alias: "proj", Role: RolePeer, PathHint: dir}
	e.synapse("dup", m, Member{ProjectID: otherID, Alias: "other", Role: RolePeer, PathHint: other})
	loc := Locate(m, e.opts(false))
	if loc.Path != "" || len(loc.Candidates) != 2 || loc.Match != MatchID {
		t.Fatalf("Locate = %+v, want two id candidates", loc)
	}
	rows, err := Heal(e.store, e.opts(true))
	if err != nil {
		t.Fatal(err)
	}
	if r := rowFor(t, rows, "proj"); r.Status != StatusAmbiguous || r.Applied {
		t.Fatalf("row = %+v, want unapplied AMBIGUOUS", r)
	}
}

func TestIsLinkedWorktree(t *testing.T) {
	dir := t.TempDir()
	if isLinkedWorktree(dir) {
		t.Fatal("dir without .git reported as worktree")
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /x/.git/modules/sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isLinkedWorktree(dir) {
		t.Fatal("submodule gitdir reported as worktree")
	}
}
