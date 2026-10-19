package synapsesync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// failingPlane returns a client whose server lists DEN but fails every path containing failOn.
func failingPlane(t *testing.T, failOn string) *plane.Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/workspaces/ws/projects/":
			_, _ = w.Write([]byte(`{"results":[{"id":"uuid-den","identifier":"DEN"}]}`))
		case strings.Contains(r.URL.Path, failOn):
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"x"}`))
		}
	}))
	t.Cleanup(ts.Close)
	c, err := plane.NewClient(plane.Config{BaseURL: ts.URL, WorkspaceSlug: "ws", ProjectID: "uuid-den", DirectToken: "tok"}, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func withDevStatus(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "DEV_STATUS.md"), []byte(devStatus), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRun_StoreUnavailable(t *testing.T) {
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := Run(context.Background(), Options{StartDir: t.TempDir()}); err == nil {
		t.Error("Run: expected error without a home directory")
	}
	if _, _, err := Discover(Options{StartDir: t.TempDir()}); err == nil {
		t.Error("Discover: expected error without a home directory")
	}
}

func TestRun_CorruptManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	st, _ := synapse.DefaultStore()
	if err := os.MkdirAll(st.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Dir(), "bad.toml"), []byte("not = [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, _ := newRepo(t, "r")
	if _, err := Run(context.Background(), Options{All: true}); err == nil {
		t.Error("--all: expected parse error")
	}
	if _, err := Run(context.Background(), Options{StartDir: repo}); err == nil {
		t.Error("start dir: expected parse error")
	}
}

func TestRun_SharedMemberDedupAndExplicitStore(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	a, _ := newRepo(t, "a")
	b, _ := newRepo(t, "b")
	c, _ := newRepo(t, "c")
	st := synapse.NewStore(filepath.Join(t.TempDir(), "syn"))
	for _, s := range []*synapse.Synapse{
		{ID: "one", Name: "one", State: synapse.StateActive, Members: []synapse.Member{member(t, a, "a", ""), member(t, b, "b", "")}},
		{ID: "two", Name: "two", State: synapse.StateActive, Members: []synapse.Member{member(t, a, "a", ""), member(t, c, "c", "")}},
	} {
		if err := st.Save(s); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := Run(context.Background(), Options{StartDir: a, Store: st, GitOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Members) != 3 || rep.Members[0].Alias != "a" || len(rep.Synapses) != 2 || rep.Failed() {
		t.Fatalf("expected a first, 3 unique members, 2 synapses: %+v", rep)
	}

	syns, targets, err := Discover(Options{All: true, Store: st})
	if err != nil || len(syns) != 2 || len(targets) != 3 {
		t.Fatalf("Discover --all: %d synapses, %d targets, err=%v", len(syns), len(targets), err)
	}
}

func TestRun_CancelledContext(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, Options{StartDir: t.TempDir()}); err == nil {
		t.Error("expected context error")
	}
}

func TestRun_SaveStateFailure(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	a, _ := newRepo(t, "a")
	b, _ := newRepo(t, "b")
	dir := filepath.Join(t.TempDir(), "syn")
	st := synapse.NewStore(dir)
	if err := st.Save(&synapse.Synapse{ID: "m", Name: "m", State: synapse.StateActive, Members: []synapse.Member{
		member(t, a, "a", ""), member(t, b, "b", ""),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(b); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := Run(context.Background(), Options{SynapseID: "m", Store: st, GitOnly: true}); err == nil {
		t.Error("expected save failure for read-only store")
	}
}

func TestRun_MemberFailures(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_COMMIT_PUSH", "")
	ctx := context.Background()

	t.Run("not a git repo", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := projectid.Ensure(dir); err != nil {
			t.Fatal(err)
		}
		rep, _ := Run(ctx, Options{StartDir: dir, GitOnly: true})
		if len(rep.Members[0].Errors) != 1 {
			t.Errorf("expected git error: %+v", rep.Members[0])
		}
	})

	t.Run("push refused on main", func(t *testing.T) {
		repo, _ := newRepo(t, "m")
		git(t, repo, "checkout", "-q", "-b", "main")
		git(t, repo, "branch", "-q", "--set-upstream-to=origin/feature")
		writeAndCommit(t, repo, "x.txt")
		rep, _ := Run(ctx, Options{StartDir: repo, GitOnly: true})
		m := rep.Members[0]
		if m.Push == nil || m.Push.Pushed || len(m.Errors) != 1 {
			t.Errorf("expected refused push: %+v", m)
		}
	})

	t.Run("state reconcile failure", func(t *testing.T) {
		repo, _ := newRepo(t, "p")
		rep, _ := Run(ctx, Options{StartDir: repo, PlaneOnly: true, Plane: failingPlane(t, "/states/"), DefaultPlaneProject: "DEN"})
		if len(rep.Members[0].Errors) != 1 {
			t.Errorf("expected reconcile error: %+v", rep.Members[0])
		}
	})

	t.Run("status push failure", func(t *testing.T) {
		repo, _ := newRepo(t, "q")
		withDevStatus(t, repo)
		rep, _ := Run(ctx, Options{StartDir: repo, PlaneOnly: true, Plane: failingPlane(t, "/modules/"), DefaultPlaneProject: "DEN"})
		m := rep.Members[0]
		if len(m.Errors) != 1 || m.Plane.StatusPushed {
			t.Errorf("expected status push error: %+v", m)
		}
	})

	t.Run("bookkeeping failures", func(t *testing.T) {
		repo := t.TempDir() // no project.id: bookkeeping keyed by path
		git(t, repo, "init", "-q", "-b", "feature")
		withDevStatus(t, repo)
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		rep, _ := Run(ctx, Options{StartDir: repo, PlaneOnly: true, ForcePlane: true, Plane: failingPlane(t, "/never/"), DefaultPlaneProject: "DEN", StateDir: blocker})
		m := rep.Members[0]
		if !m.Plane.StatusPushed || len(m.Errors) != 1 {
			t.Errorf("expected push success but bookkeeping error: %+v", m)
		}

		// Unreadable bookkeeping entry (a directory where the JSON should be) is treated as stale.
		stateDir := t.TempDir()
		mr := &MemberReport{Dir: filepath.Clean(repo), PlaneProject: "DEN"}
		if err := os.MkdirAll(filepath.Join(stateDir, stateKey(mr)+".json"), 0o755); err != nil {
			t.Fatal(err)
		}
		rep, _ = Run(ctx, Options{StartDir: repo, PlaneOnly: true, Plane: failingPlane(t, "/never/"), DefaultPlaneProject: "DEN", StateDir: stateDir})
		if !rep.Members[0].Plane.StatusPushed {
			t.Errorf("unreadable bookkeeping must not block the push: %+v", rep.Members[0])
		}

		// No home directory: bookkeeping cannot be located, push still happens.
		t.Setenv("DENDRITE_HOME", "")
		t.Setenv("HOME", "")
		rep, _ = Run(ctx, Options{StartDir: repo, PlaneOnly: true, Store: synapse.NewStore(t.TempDir()), Plane: failingPlane(t, "/never/"), DefaultPlaneProject: "DEN"})
		if !rep.Members[0].Plane.StatusPushed || len(rep.Members[0].Errors) != 1 {
			t.Errorf("expected push with bookkeeping error: %+v", rep.Members[0])
		}
	})
}

func TestIntervalFromEnv(t *testing.T) {
	for raw, want := range map[string]time.Duration{"": DefaultInterval, "0": 0, "90s": 90 * time.Second, "junk": DefaultInterval, "-1m": DefaultInterval} {
		t.Setenv("DENDRITE_SYNC_INTERVAL", raw)
		if got := IntervalFromEnv(); got != want {
			t.Errorf("DENDRITE_SYNC_INTERVAL=%q: got %s want %s", raw, got, want)
		}
	}
}

func TestWatch(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if err := Watch(context.Background(), Options{}, 0, nil); err == nil {
		t.Fatal("expected error for non-positive interval")
	}
	ctx, cancel := context.WithCancel(context.Background())
	passes := 0
	err := Watch(ctx, Options{All: true}, time.Millisecond, func(r *Report, err error) {
		if err != nil {
			t.Errorf("pass error: %v", err)
		}
		passes++
		if passes == 3 {
			cancel()
		}
	})
	if err != nil || passes != 3 {
		t.Fatalf("Watch: err=%v passes=%d", err, passes)
	}
}
