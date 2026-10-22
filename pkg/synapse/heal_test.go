package synapse

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/projectid"
)

// healEnv is a self-contained workspace: a synapse store, a project index and member dirs.
type healEnv struct {
	t     *testing.T
	root  string
	store *Store
	index *projectid.Index
}

func newHealEnv(t *testing.T) *healEnv {
	t.Helper()
	root := t.TempDir()
	ix, err := projectid.LoadIndex(filepath.Join(root, "home", "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &healEnv{t: t, root: root, store: NewStore(filepath.Join(root, "home", "synapses")), index: ix}
}

// project creates a directory with a minted project ID and registers it in the index.
func (e *healEnv) project(rel string) (string, string) {
	e.t.Helper()
	dir := filepath.Join(e.root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	entry, err := e.index.Register(dir)
	if err != nil {
		e.t.Fatal(err)
	}
	return dir, entry.ID
}

func (e *healEnv) synapse(id string, members ...Member) *Synapse {
	e.t.Helper()
	s := &Synapse{ID: id, Name: id, State: StateActive, Members: members}
	if err := e.store.Save(s); err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *healEnv) opts(apply bool) HealOptions {
	return HealOptions{Roots: []string{e.root}, Apply: apply, Index: e.index, NoDefaultRoots: true}
}

func gitInit(t *testing.T, dir, remote string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q", dir}, {"-C", dir, "remote", "add", "origin", remote}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func move(t *testing.T, from, to string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

func rowFor(t *testing.T, rows []HealRow, alias string) HealRow {
	t.Helper()
	for _, r := range rows {
		if r.Alias == alias {
			return r
		}
	}
	t.Fatalf("no row for %s in %+v", alias, rows)
	return HealRow{}
}

func TestHealAppliesMovedMemberEverywhere(t *testing.T) {
	e := newHealEnv(t)
	apiDir, apiID := e.project("ws/api")
	webDir, webID := e.project("ws/web")
	e.synapse("web-api",
		Member{ProjectID: apiID, Alias: "api", Role: RoleProducer, PathHint: apiDir},
		Member{ProjectID: webID, Alias: "web", Role: RoleConsumer, PathHint: webDir},
	)
	if err := e.index.Save(); err != nil {
		t.Fatal(err)
	}

	// web's pool references api by path.
	pool := filepath.Join(webDir, filepath.FromSlash(dag.DefaultPoolConfigFile))
	if err := os.MkdirAll(filepath.Dir(pool), 0o755); err != nil {
		t.Fatal(err)
	}
	poolJSON, _ := json.Marshal(dag.PoolConfig{Version: "1.0.0", Projects: []dag.ProjectInfo{{Name: "api", Path: apiDir}}})
	if err := os.WriteFile(pool, poolJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	newAPI := filepath.Join(e.root, "ws", "services", "api")
	move(t, apiDir, newAPI)

	rows, err := Heal(e.store, e.opts(true))
	if err != nil {
		t.Fatal(err)
	}
	r := rowFor(t, rows, "api")
	if r.Status != StatusMoved || r.NewPath != newAPI || r.Match != MatchID || !r.Applied {
		t.Fatalf("api row = %+v", r)
	}
	if w := rowFor(t, rows, "web"); w.Status != StatusOK {
		t.Fatalf("web row = %+v", w)
	}

	s, err := e.store.Load("web-api")
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Member("api"); m.PathHint != newAPI {
		t.Fatalf("manifest path_hint = %s, want %s", m.PathHint, newAPI)
	}
	reloaded, err := projectid.LoadIndex(filepath.Join(e.root, "home", "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reloaded.Lookup(apiID); got.Path != newAPI {
		t.Fatalf("index path = %s, want %s", got.Path, newAPI)
	}
	if got := dag.NewPoolManager(pool).List(); got[0].Path != newAPI {
		t.Fatalf("pool path = %s, want %s", got[0].Path, newAPI)
	}

	// A second heal is a no-op: everything resolves.
	rows, err = Heal(e.store, e.opts(true))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Status != StatusOK || r.Applied {
			t.Fatalf("second heal row = %+v", r)
		}
	}
}

func TestHealWithoutApplyWritesNothing(t *testing.T) {
	e := newHealEnv(t)
	aDir, aID := e.project("a")
	bDir, bID := e.project("b")
	e.synapse("ab", Member{ProjectID: aID, Alias: "a", Role: RolePeer, PathHint: aDir}, Member{ProjectID: bID, Alias: "b", Role: RolePeer, PathHint: bDir})
	manifest := filepath.Join(e.store.Dir(), "ab.toml")
	before, _ := os.ReadFile(manifest)

	move(t, aDir, filepath.Join(e.root, "moved", "a"))
	rows, err := Heal(e.store, e.opts(false))
	if err != nil {
		t.Fatal(err)
	}
	if r := rowFor(t, rows, "a"); r.Status != StatusMoved || r.Applied {
		t.Fatalf("row = %+v, want MOVED and not applied", r)
	}
	after, _ := os.ReadFile(manifest)
	if string(before) != string(after) {
		t.Fatal("manifest changed without --auto")
	}
}

func TestHealRemoteMatch(t *testing.T) {
	t.Run("unique clone is adopted and its identity restored", func(t *testing.T) {
		e := newHealEnv(t)
		apiDir, apiID := e.project("api")
		gitInit(t, apiDir, "git@github.com:acme/api.git")
		webDir, webID := e.project("web")
		if _, err := e.index.Register(apiDir); err != nil { // refresh remote
			t.Fatal(err)
		}
		e.synapse("web-api", Member{ProjectID: apiID, Alias: "api", Role: RoleProducer, PathHint: apiDir}, Member{ProjectID: webID, Alias: "web", Role: RoleConsumer, PathHint: webDir})

		// The original is deleted and re-cloned elsewhere without .dendrite.
		if err := os.RemoveAll(apiDir); err != nil {
			t.Fatal(err)
		}
		clone := filepath.Join(e.root, "clones", "api-v2")
		gitInit(t, clone, "https://github.com/acme/api")

		rows, err := Heal(e.store, e.opts(true))
		if err != nil {
			t.Fatal(err)
		}
		r := rowFor(t, rows, "api")
		if r.Status != StatusMoved || r.Match != MatchRemote || r.NewPath != clone || !r.Applied {
			t.Fatalf("row = %+v", r)
		}
		if got, err := projectid.Read(clone); err != nil || got != apiID {
			t.Fatalf("clone identity = %q, %v; want %s", got, err, apiID)
		}
	})

	t.Run("two clones are ambiguous and never applied", func(t *testing.T) {
		e := newHealEnv(t)
		apiDir, apiID := e.project("api")
		gitInit(t, apiDir, "git@github.com:acme/api.git")
		webDir, webID := e.project("web")
		if _, err := e.index.Register(apiDir); err != nil {
			t.Fatal(err)
		}
		e.synapse("web-api", Member{ProjectID: apiID, Alias: "api", Role: RoleProducer, PathHint: apiDir}, Member{ProjectID: webID, Alias: "web", Role: RoleConsumer, PathHint: webDir})
		if err := os.RemoveAll(apiDir); err != nil {
			t.Fatal(err)
		}
		gitInit(t, filepath.Join(e.root, "c1", "api"), "https://github.com/acme/api.git")
		gitInit(t, filepath.Join(e.root, "c2", "api"), "ssh://git@github.com/acme/api")

		rows, err := Heal(e.store, e.opts(true))
		if err != nil {
			t.Fatal(err)
		}
		r := rowFor(t, rows, "api")
		if r.Status != StatusAmbiguous || r.Applied || len(r.Candidates) != 2 {
			t.Fatalf("row = %+v, want AMBIGUOUS with 2 candidates", r)
		}
		s, _ := e.store.Load("web-api")
		if s.State != StateBroken {
			t.Fatalf("state = %s, want broken while a member is unresolved", s.State)
		}
	})
}

func TestHealNameMatchIsOnlyAProposal(t *testing.T) {
	e := newHealEnv(t)
	apiDir, apiID := e.project("api")
	webDir, webID := e.project("web")
	e.synapse("web-api", Member{ProjectID: apiID, Alias: "api", Role: RoleProducer, PathHint: apiDir}, Member{ProjectID: webID, Alias: "web", Role: RoleConsumer, PathHint: webDir})
	if err := os.RemoveAll(apiDir); err != nil {
		t.Fatal(err)
	}
	// Same basename, no identity, no remote.
	gitInit(t, filepath.Join(e.root, "elsewhere", "api"), "")

	rows, err := Heal(e.store, e.opts(true))
	if err != nil {
		t.Fatal(err)
	}
	r := rowFor(t, rows, "api")
	if r.Status != StatusAmbiguous || r.Match != MatchName || r.Applied {
		t.Fatalf("row = %+v, want name-matched AMBIGUOUS proposal", r)
	}
}

func TestHealMissingThenRecovered(t *testing.T) {
	e := newHealEnv(t)
	// Nested deep enough that the parent-of-parent fallback roots stay inside e.root.
	aDir, aID := e.project("ws/deep/a")
	bDir, bID := e.project("b")
	e.synapse("ab", Member{ProjectID: aID, Alias: "a", Role: RolePeer, PathHint: aDir}, Member{ProjectID: bID, Alias: "b", Role: RolePeer, PathHint: bDir})

	// Move a outside every search root.
	outside := t.TempDir()
	move(t, aDir, filepath.Join(outside, "a"))
	rows, err := Heal(e.store, e.opts(true))
	if err != nil {
		t.Fatal(err)
	}
	if r := rowFor(t, rows, "a"); r.Status != StatusMissing {
		t.Fatalf("row = %+v, want MISSING", r)
	}
	if s, _ := e.store.Load("ab"); s.State != StateBroken {
		t.Fatalf("state = %s, want broken", s.State)
	}

	// A hint directory finds it via Locate; adding it as a root lets heal recover.
	m := Member{ProjectID: aID, Alias: "a", PathHint: aDir}
	if loc := Locate(m, HealOptions{NoDefaultRoots: true}, outside); loc.Path != filepath.Join(outside, "a") || loc.Match != MatchID {
		t.Fatalf("Locate with hint = %+v", loc)
	}
	opts := e.opts(true)
	opts.Roots = append(opts.Roots, outside)
	if _, err := Heal(e.store, opts); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.store.Load("ab"); s.State != StateActive {
		t.Fatalf("state = %s, want active after heal", s.State)
	}
}

func TestScanSkipsVendoredAndDeepDirs(t *testing.T) {
	e := newHealEnv(t)
	aDir, aID := e.project("a")
	m := Member{ProjectID: aID, Alias: "a", PathHint: filepath.Join(e.root, "gone")}

	for _, rel := range []string{
		filepath.Join("x", "node_modules", "a"),
		filepath.Join("x", "target", "a"),
		filepath.Join("x", ".cache", "a"),
		filepath.Join("d1", "d2", "d3", "d4", "d5", "a"),
	} {
		dst := filepath.Join(e.root, rel)
		move(t, aDir, dst)
		if loc := Locate(m, HealOptions{Roots: []string{e.root}, NoDefaultRoots: true}); loc.Path != "" {
			t.Fatalf("found %s inside skipped/deep dir %s", loc.Path, rel)
		}
		aDir = dst
	}

	shallow := filepath.Join(e.root, "d1", "d2", "a")
	move(t, aDir, shallow)
	if loc := Locate(m, HealOptions{Roots: []string{e.root}, NoDefaultRoots: true}); loc.Path != shallow {
		t.Fatalf("Locate = %+v, want %s", loc, shallow)
	}
	if loc := Locate(m, HealOptions{Roots: []string{e.root}, MaxDepth: 1, NoDefaultRoots: true}); loc.Path != "" {
		t.Fatalf("MaxDepth 1 should not reach %s", shallow)
	}
}

func TestSearchRootsDefaultsAndDedup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvSearchRoots, "")
	gl := filepath.Join(home, "GoLandProjects")
	if err := os.MkdirAll(gl, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	roots := searchRoots(Member{}, HealOptions{Roots: []string{gl, gl, file, filepath.Join(home, "nope"), ""}})
	if len(roots) != 1 || roots[0] != gl {
		t.Fatalf("roots = %v, want only %s", roots, gl)
	}
}

func TestLocateResolvedMemberShortCircuits(t *testing.T) {
	e := newHealEnv(t)
	dir, id := e.project("a")
	if loc := Locate(Member{ProjectID: id, PathHint: dir}, HealOptions{NoDefaultRoots: true}); loc.Path != dir {
		t.Fatalf("Locate = %+v", loc)
	}
}

func TestWriteProjectID(t *testing.T) {
	dir := t.TempDir()
	const id = "dpid_0123456789abcdef0123456789abcdef"
	if err := writeProjectID(dir, id); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectID(dir, id); err != nil {
		t.Fatalf("rewriting the same id must be a no-op: %v", err)
	}
	if err := writeProjectID(dir, "dpid_ffffffffffffffffffffffffffffffff"); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("overwrite of a different id: got %v", err)
	}
	blocked := t.TempDir()
	if err := os.WriteFile(filepath.Join(blocked, ".dendrite"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectID(blocked, id); err == nil {
		t.Fatal("expected error when .dendrite is a file")
	}
}

func TestRebindPoolFiles(t *testing.T) {
	dir := t.TempDir()
	if err := RebindPoolFiles([]string{filepath.Join(dir, "missing.json")}, map[string]string{"/a": "/b"}); err != nil {
		t.Fatalf("missing pool file must be skipped: %v", err)
	}
	if err := RebindPoolFiles([]string{"ignored"}, nil); err != nil {
		t.Fatal(err)
	}

	unrelated := filepath.Join(dir, "unrelated.json")
	if err := os.WriteFile(unrelated, []byte(`{"version":"1.0.0","projects":[{"name":"x","path":"/x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(unrelated)
	if err := RebindPoolFiles([]string{unrelated}, map[string]string{"/a": "/b"}); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(unrelated); string(after) != string(before) {
		t.Fatal("pool without matching entries must not be rewritten")
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RebindPoolFiles([]string{bad}, map[string]string{"/a": "/b"}); err == nil {
		t.Fatal("expected error for malformed pool file")
	}

	roDir := filepath.Join(dir, "ro")
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ro := filepath.Join(roDir, "pool.json")
	if err := os.WriteFile(ro, []byte(`{"version":"1.0.0","projects":[{"name":"a","path":"/a"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
	if err := RebindPoolFiles([]string{ro}, map[string]string{"/a": "/b"}); err == nil {
		t.Fatal("expected save error in read-only directory")
	}
}
