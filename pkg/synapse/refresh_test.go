package synapse

import (
	"Dendrite/pkg/projectid"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func gitCommit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func gitClone(t *testing.T, src, dst, remote string) {
	t.Helper()
	cmd := exec.Command("git", "clone", "-q", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dst, "remote", "set-url", "origin", remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote set-url: %v\n%s", err, out)
	}
}

func gitWorktreeAdd(t *testing.T, dir, wtDir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "worktree", "add", "-b", "wt-branch", wtDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
}

func refreshRowFor(t *testing.T, rows []RefreshRow, id string) RefreshRow {
	return rowWhere(t, rows, id, func(r RefreshRow) bool { return r.ID == id })
}

func TestRefreshRelocatedByID(t *testing.T) {
	e := newHealEnv(t)
	pDir, pID := e.project("ws/repo")
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}

	newPath := filepath.Join(e.root, "ws", "moved_repo")
	move(t, pDir, newPath)

	// Apply: false reports RefreshRelocated and leaves the index file unchanged.
	rows, err := RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          false,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, pID)
	if r.Status != RefreshRelocated || r.NewPath != newPath || r.Applied {
		t.Fatalf("row = %+v, want %s with %s and Applied=false", r, RefreshRelocated, newPath)
	}
	after, err := os.ReadFile(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("index file changed when Apply: false")
	}

	// Apply: true updates the index entry to point at the new path and sets Applied to true.
	rows, err = RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          true,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r = refreshRowFor(t, rows, pID)
	if r.Status != RefreshRelocated || r.NewPath != newPath || !r.Applied {
		t.Fatalf("row = %+v, want %s with %s and Applied=true", r, RefreshRelocated, newPath)
	}

	reloaded, err := projectid.LoadIndex(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := reloaded.Lookup(pID)
	if !ok || entry.Path != newPath {
		t.Fatalf("index entry path = %q, want %q (found=%v)", entry.Path, newPath, ok)
	}
}

func TestRefreshSynapseFollows(t *testing.T) {
	e := newHealEnv(t)
	apiDir, apiID := e.project("ws/api")
	webDir, webID := e.project("ws/web")
	syn := e.synapse("web-api",
		Member{ProjectID: apiID, Alias: "api", Role: RoleProducer, PathHint: apiDir},
		Member{ProjectID: webID, Alias: "web", Role: RoleConsumer, PathHint: webDir},
	)
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}

	newAPI := filepath.Join(e.root, "ws", "services", "api")
	move(t, apiDir, newAPI)

	// Mark broken to simulate the state once a member directory is moved.
	syn.State = StateBroken
	if err := e.store.Save(syn); err != nil {
		t.Fatal(err)
	}

	rows, err := RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          true,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, apiID)
	if r.Status != RefreshRelocated || r.NewPath != newAPI || !r.Applied {
		t.Fatalf("row = %+v, want %s with %s and Applied=true", r, RefreshRelocated, newAPI)
	}

	loaded, err := e.store.Load("web-api")
	if err != nil {
		t.Fatal(err)
	}
	m, ok := loaded.Member("api")
	if !ok || m.PathHint != newAPI {
		t.Fatalf("member api PathHint = %q, want %q", m.PathHint, newAPI)
	}
	if loaded.State == StateBroken {
		t.Fatalf("synapse state = %s, want not broken", loaded.State)
	}
	if loaded.State != StateActive {
		t.Fatalf("synapse state = %s, want %s", loaded.State, StateActive)
	}
}

func TestRefreshHealthyIsOK(t *testing.T) {
	e := newHealEnv(t)
	pDir, pID := e.project("ws/healthy")
	_ = pDir
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}

	rows, err := RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          true,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, pID)
	if r.Status != RefreshOK || r.Applied {
		t.Fatalf("row = %+v, want %s and Applied=false", r, RefreshOK)
	}

	after, err := os.ReadFile(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("healthy entry was rewritten")
	}
}

func TestRefreshMissingAndPrune(t *testing.T) {
	e := newHealEnv(t)
	pDir, pID := e.project("ws/deleted_project")
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(pDir); err != nil {
		t.Fatal(err)
	}

	// Missing without Prune (Apply: false).
	rows, err := RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          false,
		Prune:          false,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, pID)
	if r.Status != RefreshMissing || r.Applied {
		t.Fatalf("row = %+v, want %s and Applied=false", r, RefreshMissing)
	}

	// Missing without Prune even with Apply: true.
	rows, err = RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          true,
		Prune:          false,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r = refreshRowFor(t, rows, pID)
	if r.Status != RefreshMissing || r.Applied {
		t.Fatalf("row = %+v, want %s and Applied=false", r, RefreshMissing)
	}
	if _, ok := e.index.Lookup(pID); !ok {
		t.Fatal("entry should still be in index when Prune is false")
	}

	// With Apply: true, Prune: true it is RefreshPruned and gone from ix.
	rows, err = RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          true,
		Prune:          true,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r = refreshRowFor(t, rows, pID)
	if r.Status != RefreshPruned || !r.Applied {
		t.Fatalf("row = %+v, want %s and Applied=true", r, RefreshPruned)
	}

	if _, ok := e.index.Lookup(pID); ok {
		t.Fatal("entry still in memory index after prune")
	}
	reloaded, err := projectid.LoadIndex(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Lookup(pID); ok {
		t.Fatal("entry still in disk index after prune")
	}
}

func TestRefreshAmbiguous(t *testing.T) {
	e := newHealEnv(t)
	origDir, pID := e.project("orig/repo")
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(origDir); err != nil {
		t.Fatal(err)
	}

	root1 := filepath.Join(e.root, "root1")
	root2 := filepath.Join(e.root, "root2")
	c1 := filepath.Join(root1, "repo")
	c2 := filepath.Join(root2, "repo")

	gitInit(t, c1, "git@github.com:acme/repo.git")
	gitCommit(t, c1)
	if err := writeProjectID(c1, pID); err != nil {
		t.Fatal(err)
	}

	gitInit(t, c2, "git@github.com:acme/repo.git")
	gitCommit(t, c2)
	if err := writeProjectID(c2, pID); err != nil {
		t.Fatal(err)
	}

	rows, err := RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{root1, root2},
		Apply:          true,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, pID)
	if r.Status != RefreshAmbiguous || r.Applied {
		t.Fatalf("row = %+v, want %s and Applied=false", r, RefreshAmbiguous)
	}

	expectedCandidates := []string{c1, c2}
	sort.Strings(expectedCandidates)
	if !reflect.DeepEqual(r.Candidates, expectedCandidates) {
		t.Fatalf("candidates = %v, want %v", r.Candidates, expectedCandidates)
	}

	reloaded, err := projectid.LoadIndex(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Lookup(pID)
	if !ok || got.Path != origDir {
		t.Fatalf("index entry changed: path = %q, want %q", got.Path, origDir)
	}
}

func TestRefreshLinkedWorktreeIsStale(t *testing.T) {
	e := newHealEnv(t)
	mainDir, pID := e.project("ws/main")
	gitInit(t, mainDir, "git@github.com:acme/main.git")
	gitCommit(t, mainDir)

	wtDir := filepath.Join(e.root, "ws", "wt")
	gitWorktreeAdd(t, mainDir, wtDir)

	// Point the index entry at the linked worktree.
	entry, ok := e.index.Lookup(pID)
	if !ok {
		t.Fatal("entry not found")
	}
	entry.Path = wtDir
	e.index.Upsert(entry)
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}

	rows, err := RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          true,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, pID)
	if r.Status != RefreshRelocated || r.OldPath != wtDir || r.NewPath != mainDir || !r.Applied {
		t.Fatalf("row = %+v, want %s relocating from %s to %s with Applied=true", r, RefreshRelocated, wtDir, mainDir)
	}
	if r.Match != MatchID {
		t.Fatalf("match = %s, want %s", r.Match, MatchID)
	}

	reloaded, err := projectid.LoadIndex(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Lookup(pID)
	if !ok || got.Path != mainDir {
		t.Fatalf("reloaded index path = %q, want %q", got.Path, mainDir)
	}
}

func TestRefreshFreshCloneByRootCommit(t *testing.T) {
	e := newHealEnv(t)
	pDir, pID := e.project("ws/proj")
	gitInit(t, pDir, "git@github.com:acme/proj.git")
	gitCommit(t, pDir)

	// Re-register to populate RootCommit and Remote in the index entry.
	entry, err := e.index.Register(pDir)
	if err != nil {
		t.Fatal(err)
	}
	if entry.RootCommit == "" {
		t.Fatal("entry.RootCommit should not be empty")
	}
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}

	cloneDir := filepath.Join(e.root, "clones", "proj-clone")
	gitClone(t, pDir, cloneDir, "https://github.com/acme/proj")

	if err := os.RemoveAll(pDir); err != nil {
		t.Fatal(err)
	}

	// Verify clone does not have .dendrite.
	if _, err := os.Stat(filepath.Join(cloneDir, ".dendrite")); !os.IsNotExist(err) {
		t.Fatalf("clone should not have .dendrite directory: %v", err)
	}

	rows, err := RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          true,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, pID)
	if r.Status != RefreshRelocated || r.NewPath != cloneDir || !r.Applied {
		t.Fatalf("row = %+v, want %s with %s and Applied=true", r, RefreshRelocated, cloneDir)
	}
	if r.Match != MatchRootCommit {
		t.Fatalf("row match = %s, want %s", r.Match, MatchRootCommit)
	}

	// After Apply, the clone has .dendrite/project.id equal to the entry's ID.
	gotID, err := projectid.Read(cloneDir)
	if err != nil || gotID != pID {
		t.Fatalf("clone project id = %q (err=%v), want %q", gotID, err, pID)
	}

	reloaded, err := projectid.LoadIndex(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Lookup(pID)
	if !ok || got.Path != cloneDir {
		t.Fatalf("reloaded index path = %q, want %q", got.Path, cloneDir)
	}
}

func TestRefreshRemoteOnly(t *testing.T) {
	e := newHealEnv(t)
	pDir, pID := e.project("ws/proj")
	gitInit(t, pDir, "git@github.com:acme/proj.git")
	gitCommit(t, pDir)

	entry, err := e.index.Register(pDir)
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly clear RootCommit to test remote-only match.
	entry.RootCommit = ""
	e.index.Upsert(entry)
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}

	cloneDir := filepath.Join(e.root, "clones", "proj-clone")
	gitClone(t, pDir, cloneDir, "https://github.com/acme/proj")

	if err := os.RemoveAll(pDir); err != nil {
		t.Fatal(err)
	}

	rows, err := RefreshIndex(e.store, e.index, RefreshOptions{
		Roots:          []string{e.root},
		Apply:          true,
		NoDefaultRoots: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := refreshRowFor(t, rows, pID)
	if r.Status != RefreshRelocated || r.NewPath != cloneDir || !r.Applied {
		t.Fatalf("row = %+v, want %s with %s and Applied=true", r, RefreshRelocated, cloneDir)
	}
	if r.Match != MatchRemote {
		t.Fatalf("row match = %s, want %s", r.Match, MatchRemote)
	}

	// After Apply, the clone has .dendrite/project.id equal to the entry's ID.
	gotID, err := projectid.Read(cloneDir)
	if err != nil || gotID != pID {
		t.Fatalf("clone project id = %q (err=%v), want %q", gotID, err, pID)
	}

	reloaded, err := projectid.LoadIndex(e.index.Path())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Lookup(pID)
	if !ok || got.Path != cloneDir {
		t.Fatalf("reloaded index path = %q, want %q", got.Path, cloneDir)
	}
}

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

func TestRefreshIndexReportsFailuresBackfillingThePlaneBindingCannotSaveTheIndex(t *testing.T) {
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
}

func TestRefreshIndexReportsFailuresTheSynapseStoreCannotBeListed(t *testing.T) {
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
}

func TestRefreshIndexReportsFailuresACloneWhoseIdentityCannotBeRestored(t *testing.T) {
	e := newHealEnv(t)
	_, clone := e.registeredClone("locked")
	readOnly(t, clone)
	if _, err := e.refresh(true, false); err == nil {
		t.Fatal("expected the identity write error")
	}
}

func TestRefreshIndexReportsFailuresPruningCannotSaveTheIndex(t *testing.T) {
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
