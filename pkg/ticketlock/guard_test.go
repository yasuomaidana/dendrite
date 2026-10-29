package ticketlock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// world is a fake machine for the guard: one repository, one clock and a Plane that answers with
// whatever the test says.
type world struct {
	t       *testing.T
	home    string
	top     string
	store   *Store
	now     time.Time
	branch  string
	linked  bool
	state   tracker.Status
	planeEr error
	asked   int
}

func newWorld(t *testing.T) *world {
	t.Helper()
	top := t.TempDir()
	if err := os.MkdirAll(filepath.Join(top, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &world{t: t, home: t.TempDir(), top: top, store: &Store{Dir: t.TempDir()}, now: t0,
		branch: "agent/den-375", linked: true, state: tracker.StatusInProgress}
}

// linkedWorktree turns the repository into a linked worktree: its .git is a file.
func (w *world) linkedWorktree() {
	w.t.Helper()
	_ = os.RemoveAll(filepath.Join(w.top, ".git"))
	if err := os.WriteFile(filepath.Join(w.top, ".git"), []byte("gitdir: elsewhere"), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) deps() Deps {
	return Deps{
		Store: w.store,
		Now:   func() time.Time { return w.now },
		Home:  w.home,
		RepoTop: func(string) (string, error) {
			return w.top, nil
		},
		Branch: func(string) string { return w.branch },
		Linked: func(string) bool { return w.linked },
		PlaneStatus: func(context.Context, string, string) (tracker.Status, error) {
			w.asked++
			return w.state, w.planeEr
		},
	}
}

func (w *world) session(age time.Duration) {
	w.t.Helper()
	if _, err := w.store.MintToken(TokenOptions{SessionID: "s-1", Agent: "claude"}, w.now.Add(-age)); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) claim(ticket string, state tracker.Status, checkedAgo time.Duration) {
	w.t.Helper()
	c := Claim{Ticket: ticket, SessionID: "s-1", Branch: w.branch, ClaimedAt: w.now, PlaneState: state, CheckedAt: w.now.Add(-checkedAgo)}
	if err := w.store.WriteClaim(c); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) target() string { return filepath.Join(w.top, "pkg", "new", "file.go") }

func TestRequireDenyMessageIsTheInstruction(t *testing.T) {
	w := newWorld(t)
	err := Require(context.Background(), w.deps(), w.target())
	const want = "blocked: no Dendrite session/ticket claim — call dendrite_ticket_claim and dendrite_query_context first (or run: dendrite session resume && dendrite work new <ticket>)"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v\nwant %s", err, want)
	}
	var lock *ErrLocked
	if !errors.As(err, &lock) {
		t.Errorf("%T is not an *ErrLocked", err)
	}
}

func TestRequireAllowAndDenyPaths(t *testing.T) {
	cases := []struct {
		name  string
		setup func(w *world)
		deny  string // substring of the refusal; empty means the write is allowed
		asked int    // how many times Plane was asked
	}{
		{"no token", func(w *world) {}, "no Dendrite session", 0},
		{"expired token", func(w *world) { w.session(TokenTTL + time.Minute) }, "no Dendrite session", 0},
		{"token but no claim", func(w *world) { w.session(time.Hour) }, "no Dendrite session/ticket claim", 0},
		{"corrupt claim", func(w *world) {
			w.session(time.Hour)
			p := w.store.claimPath(w.branch)
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			_ = os.WriteFile(p, []byte("{"), 0o644)
		}, "unreadable", 0},
		{"claim for another ticket", func(w *world) {
			w.session(time.Hour)
			w.claim("DEN-999", tracker.StatusInProgress, time.Minute)
		}, "belongs to den-375 but the claim is for DEN-999", 0},
		{"fresh cache is trusted", func(w *world) {
			w.session(time.Hour)
			w.claim("DEN-375", tracker.StatusInProgress, time.Minute)
			w.planeEr = errors.New("must not be asked")
		}, "", 0},
		{"ticket id case does not matter", func(w *world) {
			w.session(time.Hour)
			w.claim("den-375", tracker.StatusInProgress, time.Minute)
		}, "", 0},
		{"stale cache, Plane says In Progress", func(w *world) {
			w.session(time.Hour)
			w.claim("DEN-375", tracker.StatusInProgress, CacheTTL+time.Minute)
		}, "", 1},
		{"stale cache, Plane says Blocked", func(w *world) {
			w.session(time.Hour)
			w.claim("DEN-375", tracker.StatusInProgress, CacheTTL+time.Minute)
			w.state = tracker.StatusBlocked
		}, "DEN-375 is Blocked, not In Progress", 1},
		{"fresh cache of a ticket that is not In Progress asks Plane", func(w *world) {
			w.session(time.Hour)
			w.claim("DEN-375", tracker.StatusTodo, time.Minute)
			w.state = tracker.StatusDone
		}, "is Done", 1},
		{"Plane down, cache two hours old", func(w *world) {
			w.session(time.Hour)
			w.claim("DEN-375", tracker.StatusInProgress, 2*time.Hour)
			w.planeEr = errors.New("connection refused")
		}, "", 1},
		{"Plane down, cache older than a day", func(w *world) {
			w.session(time.Hour)
			w.claim("DEN-375", tracker.StatusInProgress, OfflineTTL+time.Minute)
			w.planeEr = errors.New("connection refused")
		}, "Plane did not answer", 1},
		{"Plane down, cache says not In Progress", func(w *world) {
			w.session(time.Hour)
			w.claim("DEN-375", tracker.StatusBlocked, time.Minute)
			w.planeEr = errors.New("connection refused")
		}, "Plane did not answer", 1},
		{"develop in the main checkout", func(w *world) {
			w.branch = "develop"
			w.session(time.Hour)
			w.claim("DEN-375", tracker.StatusInProgress, time.Minute)
		}, "main checkout", 0},
		{"master in the main checkout", func(w *world) { w.branch = "master" }, "main checkout", 0},
		{"main in a linked worktree falls through to the claim", func(w *world) {
			w.branch = "main"
			w.linkedWorktree()
		}, "no Dendrite session", 0},
		{"a feature branch needs only its own claim", func(w *world) {
			w.branch = "feature/x"
			w.session(time.Hour)
			w.claim("DEN-5", tracker.StatusInProgress, time.Minute)
		}, "", 0},
		{"repository that is not a Plane project", func(w *world) { w.linked = false }, "", 0},
		{"inside Dendrite's own state", func(w *world) {}, "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t)
			c.setup(w)
			target := w.target()
			if c.name == "inside Dendrite's own state" {
				target = filepath.Join(w.top, ".dendrite", "session", "x.json")
			}
			err := Require(context.Background(), w.deps(), target)
			switch {
			case c.deny == "" && err != nil:
				t.Errorf("want allowed, got %v", err)
			case c.deny != "" && (err == nil || !strings.Contains(err.Error(), c.deny)):
				t.Errorf("err = %v, want it to contain %q", err, c.deny)
			}
			if w.asked != c.asked {
				t.Errorf("Plane asked %d times, want %d", w.asked, c.asked)
			}
		})
	}
}

func TestRequireRefreshesTheClaimCache(t *testing.T) {
	w := newWorld(t)
	w.session(time.Hour)
	w.claim("DEN-375", tracker.StatusInProgress, CacheTTL+time.Minute)
	if err := Require(context.Background(), w.deps(), w.target()); err != nil {
		t.Fatal(err)
	}
	c, err := w.store.ClaimFor(w.branch)
	if err != nil || !c.CheckedAt.Equal(w.now) {
		t.Fatalf("claim = %+v, err = %v: the check time must be refreshed", c, err)
	}
	// Now Plane is down, but the refreshed cache is still good.
	w.planeEr = errors.New("down")
	w.now = w.now.Add(time.Minute)
	if err := Require(context.Background(), w.deps(), w.target()); err != nil {
		t.Errorf("the refreshed cache should cover a Plane outage: %v", err)
	}
}

func TestRequireRefusesWhenTheCheckCannotBeRecorded(t *testing.T) {
	w := newWorld(t)
	w.session(time.Hour)
	w.claim("DEN-375", tracker.StatusInProgress, CacheTTL+time.Minute)
	dir := filepath.Join(w.store.Dir, "claims")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	// Root ignores directory permissions: then the record succeeds and the write is allowed.
	canWrite := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o644) == nil
	err := Require(context.Background(), w.deps(), w.target())
	switch {
	case canWrite && err != nil:
		t.Errorf("the directory is writable, so the check is recorded: %v", err)
	case !canWrite && (err == nil || !strings.Contains(err.Error(), "cannot record")):
		t.Errorf("err = %v, want a refusal that the check cannot be recorded", err)
	}
}

func TestRequireAllowsClaudePlansAndPathsOutsideGit(t *testing.T) {
	w := newWorld(t)
	d := w.deps()
	d.RepoTop = func(string) (string, error) {
		t.Fatal("a plan file must not even look for a repository")
		return "", nil
	}
	if err := Require(context.Background(), d, filepath.Join(w.home, ".claude", "plans", "p.md")); err != nil {
		t.Errorf("plan file: %v", err)
	}
	d.RepoTop = func(string) (string, error) { return "", errors.New("not a git repository") }
	if err := Require(context.Background(), d, filepath.Join(t.TempDir(), "x")); err != nil {
		t.Errorf("outside git: %v", err)
	}
	d.RepoTop = func(string) (string, error) { return "", nil }
	if err := Require(context.Background(), d, filepath.Join(t.TempDir(), "x")); err != nil {
		t.Errorf("empty top level: %v", err)
	}
	d.Home = ""
	d.RepoTop = func(string) (string, error) { return "", errors.New("no git") }
	if err := Require(context.Background(), d, "/anywhere"); err != nil {
		t.Errorf("no home: %v", err)
	}
}

func TestNearestDirAndWithin(t *testing.T) {
	dir := t.TempDir()
	if got := nearestDir(dir); got != dir {
		t.Errorf("an existing directory is its own nearest: %q", got)
	}
	if got := nearestDir(filepath.Join(dir, "a", "b", "c.go")); got != dir {
		t.Errorf("a new file is placed in the nearest existing directory: %q", got)
	}
	file := filepath.Join(dir, "f")
	_ = os.WriteFile(file, nil, 0o644)
	if got := nearestDir(file); got != dir {
		t.Errorf("a file is placed in its directory: %q", got)
	}
	if got := nearestDir("/definitely/not/here"); got != "/" {
		t.Errorf("walks up to the root: %q", got)
	}
	for path, want := range map[string]bool{"/a/b": true, "/a": true, "/ab": false, "/c": false, "/a/../c": false} {
		if got := within(path, "/a"); got != want {
			t.Errorf("within(%q, /a) = %v", path, got)
		}
	}
}

func TestIsMainCheckout(t *testing.T) {
	w := newWorld(t)
	if !isMainCheckout(w.top) {
		t.Error("a directory .git is a main checkout")
	}
	w.linkedWorktree()
	if isMainCheckout(w.top) {
		t.Error("a .git file is a linked worktree")
	}
	if isMainCheckout(t.TempDir()) {
		t.Error("no .git at all")
	}
}

func TestDefaultDepsReadGitAndThePlaneLink(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"checkout", "-q", "-b", "agent/den-375"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	want := func(context.Context, string, string) (tracker.Status, error) { return tracker.StatusInProgress, nil }
	d := DefaultDeps(want)

	top, err := d.RepoTop(repo)
	if err != nil {
		t.Fatal(err)
	}
	if real, _ := filepath.EvalSymlinks(repo); top != real {
		t.Errorf("RepoTop = %q, want %q", top, real)
	}
	if b := d.Branch(top); b != "agent/den-375" {
		t.Errorf("Branch = %q", b)
	}
	if d.Branch(t.TempDir()) != "" {
		t.Error("a directory outside git has no branch")
	}
	if d.Linked(top) {
		t.Error("a repository with no .dendrite/plane.json is not a Plane project")
	}
	if err := plane.WriteLink(top, plane.ProjectLink{Identifier: "T", ProjectID: "p-1"}); err != nil {
		t.Fatal(err)
	}
	if !d.Linked(top) {
		t.Error("a repository with .dendrite/plane.json is a Plane project")
	}
	if st, _ := d.PlaneStatus(context.Background(), top, "T-1"); st != tracker.StatusInProgress || d.Store == nil || d.Now == nil {
		t.Error("DefaultDeps lost its parts")
	}
}
