package repair

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/pool/switcher"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/workcopy"
)

// world is a consumer A (on DEN-890-fix-login-crash/main) that uses crate r1 from a registry, the upstream
// repository of r1 (which uses r2) and of r2, a ledger, and a working-copy root; all in temporary directories.
type world struct {
	ledger           *Ledger
	consumer, r1, r2 string
	root             string
	started          time.Time
}

// upstream makes a bare repository with one commit holding files, and returns its path (used as its URL).
func upstream(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), name+".git")
	testkit.Git(t, filepath.Dir(bare), "init", "-q", "--bare", "-b", "main", bare)
	seed := t.TempDir()
	testkit.GitInit(t, seed, "main")
	for file, content := range files {
		testkit.WriteFile(t, filepath.Join(seed, file), content, 0o644)
	}
	testkit.Git(t, seed, "add", "-A")
	testkit.Git(t, seed, "commit", "-q", "-m", "init "+name)
	testkit.Git(t, seed, "remote", "add", "origin", bare)
	testkit.Git(t, seed, "push", "-q", "origin", "main")
	return bare
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{root: workcopy.Canon(t.TempDir()), started: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	t.Setenv(workcopy.EnvWorkDir, w.root)
	w.ledger = &Ledger{Path: filepath.Join(t.TempDir(), "home", "repairs.json")}
	w.r2 = upstream(t, "r2", map[string]string{"Cargo.toml": "[package]\nname = \"r2\"\n"})
	w.r1 = upstream(t, "r1", map[string]string{"Cargo.toml": "[package]\nname = \"r1\"\n\n[dependencies]\nr2 = \"0.3\"\n"})
	w.consumer = workcopy.Canon(t.TempDir())
	testkit.GitInit(t, w.consumer, "main")
	testkit.WriteFile(t, filepath.Join(w.consumer, "Cargo.toml"), "[package]\nname = \"a\"\n\n[dependencies]\nr1 = \"1.0\"\n", 0o644)
	testkit.Git(t, w.consumer, "add", "-A")
	testkit.Git(t, w.consumer, "commit", "-q", "-m", "init a")
	testkit.Git(t, w.consumer, "checkout", "-q", "-b", "DEN-890-fix-login-crash/main")
	return w
}

func (w *world) start(t *testing.T, o StartOptions) Started {
	t.Helper()
	o.Now = func() time.Time { w.started = w.started.Add(time.Minute); return w.started }
	res, err := Start(context.Background(), w.ledger, o)
	testkit.Check(t, err)
	return res
}

func manifest(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	testkit.Check(t, err)
	return string(data)
}

func TestBranchNameFollowsTheConsumersBranch(t *testing.T) {
	// consumer branch, consumer ticket, dependency ticket ("-" for none)
	testkit.Table(t, map[string]string{
		"DEN-890-fix-login-crash/main DEN-890 -":    "DEN-890-fix-login-crash/main",
		"DEN-880-naming/DEN-890-part DEN-890 -":     "DEN-890-part/main",
		"R1-3-fix-crash/main DEN-890 R2-7":          "R2-7-fix-crash/main",
		"den-890-Fix-Crash/main DEN-890 EMBEDAG-12": "EMBEDAG-12-fix-crash/main",
		"agent/den-890 DEN-890 -":                   "DEN-890/main",
		"- DEN-890 DCORE-3":                         "DCORE-3/main",
	}, func(in string) string {
		f := strings.Fields(in)
		return BranchName(strings.TrimPrefix(f[0], "-"), f[1], strings.TrimPrefix(f[2], "-"))
	})
}

func TestStartClonesTheDependencySwitchesTheConsumerAndRecordsTheLink(t *testing.T) {
	w := newWorld(t)
	res := w.start(t, StartOptions{Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: w.r1})
	link := res.Link
	testkit.Equal(t, "no warnings", len(res.Warnings), 0)
	testkit.Equal(t, "id", link.ID, "DEN-890/r1")
	testkit.Equal(t, "branch", link.Branch, "DEN-890-fix-login-crash/main")
	testkit.Equal(t, "copy", link.Copy, filepath.Join(w.root, "r1-DEN-890-fix-login-crash"))
	testkit.Equal(t, "stage", link.Stage, StageRepairing)
	testkit.Equal(t, "not a fork", link.Fork, false)
	testkit.Equal(t, "original", link.Original[0], switcher.Entry{Manifest: "Cargo.toml", Mode: switcher.ModePublished, Raw: `r1 = "1.0"`})
	testkit.Equal(t, "checked out", testkit.Git(t, link.Copy, "symbolic-ref", "--short", "HEAD"), link.Branch)
	testkit.Equal(t, "pushed", testkit.Git(t, w.r1, "rev-parse", "--verify", "refs/heads/"+link.Branch) != "", true)
	testkit.Contains(t, manifest(t, w.consumer), `r1 = { git = "`+w.r1+`", branch = "DEN-890-fix-login-crash/main" }`)

	stored, err := w.ledger.Get("DEN-890/r1")
	testkit.Check(t, err)
	testkit.Equal(t, "stored", stored.Copy, link.Copy)
	if _, err := Start(context.Background(), w.ledger, StartOptions{Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: w.r1}); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("a second repair of the same dependency: %v", err)
	}
}

func TestARepairStartedInsideARepairCopyIsItsChild(t *testing.T) {
	w := newWorld(t)
	first := w.start(t, StartOptions{Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: w.r1}).Link
	second := w.start(t, StartOptions{Consumer: first.Copy, Dependency: "r2", Ticket: "DEN-890", Upstream: w.r2}).Link
	testkit.Equal(t, "parent", second.Parent, first.ID)
	testkit.Equal(t, "branch follows the copy's branch", second.Branch, "DEN-890-fix-login-crash/main")
	testkit.Contains(t, manifest(t, first.Copy), `r2 = { git = "`+w.r2+`"`)

	links, err := w.ledger.Links()
	testkit.Check(t, err)
	testkit.Equal(t, "oldest first", links[0].ID, first.ID)
	testkit.Equal(t, "children", Children(links, first.ID)[0].ID, "DEN-890/r2")
	testkit.Equal(t, "leaf", len(Children(links, second.ID)), 0)
}

func TestStartUsesAWorktreeOfALocalCheckoutAndTheDependencysOwnTicket(t *testing.T) {
	w := newWorld(t)
	checkout := workcopy.Canon(filepath.Join(t.TempDir(), "r1"))
	testkit.Git(t, filepath.Dir(checkout), "clone", "-q", w.r1, checkout)
	res := w.start(t, StartOptions{Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890", DepTicket: "EMBEDAG-12", Checkout: checkout})
	testkit.Equal(t, "branch", res.Link.Branch, "EMBEDAG-12-fix-login-crash/main")
	testkit.Equal(t, "a worktree of the checkout", res.Link.Copy, filepath.Join(w.root, "r1-EMBEDAG-12-fix-login-crash"))
	testkit.Equal(t, "upstream is the checkout's origin", res.Link.Upstream, w.r1)
	testkit.Contains(t, testkit.Git(t, checkout, "worktree", "list"), "r1-EMBEDAG-12-fix-login-crash")
}

func TestStartPushesToAForkAndWarnsWhenThePushFails(t *testing.T) {
	w := newWorld(t)
	fork := filepath.Join(t.TempDir(), "fork.git")
	testkit.Git(t, filepath.Dir(fork), "clone", "-q", "--bare", w.r1, fork)
	res := w.start(t, StartOptions{Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: w.r1, Fork: fork})
	testkit.Equal(t, "fork", res.Link.Fork, true)
	testkit.Equal(t, "pushed to the fork", res.Link.Remote, fork)
	testkit.Equal(t, "upstream remote", testkit.Git(t, res.Link.Copy, "remote", "get-url", "upstream"), w.r1)
	testkit.Contains(t, manifest(t, w.consumer), `git = "`+fork+`"`)

	// A second consumer ticket whose push cannot reach the remote: the repair still starts, with a warning.
	testkit.Git(t, w.consumer, "checkout", "-q", "-b", "DEN-891-other/main", "main")
	testkit.Check(t, os.Chmod(filepath.Join(w.r1, "refs", "heads"), 0o500))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(w.r1, "refs", "heads"), 0o755) })
	res = w.start(t, StartOptions{Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-891", Upstream: w.r1})
	testkit.Equal(t, "one warning", len(res.Warnings), 1)
	testkit.Contains(t, res.Warnings[0], "DEN-891-other/main was not pushed")
}

func TestStartRefusesWhatItCannotDo(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	for name, o := range map[string]StartOptions{
		"no ticket":           {Consumer: w.consumer, Dependency: "r1", Upstream: w.r1},
		"no source":           {Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890"},
		"unknown dependency":  {Consumer: w.consumer, Dependency: "nope", Ticket: "DEN-890", Upstream: w.r1},
		"unreachable clone":   {Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: filepath.Join(w.root, "gone.git")},
		"checkout not a repo": {Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890", Checkout: t.TempDir()},
	} {
		if _, err := Start(ctx, w.ledger, o); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	links, err := w.ledger.Links()
	testkit.Check(t, err)
	testkit.Equal(t, "nothing recorded", len(links), 0)
	testkit.Contains(t, manifest(t, w.consumer), `r1 = "1.0"`)
}

func TestLedgerPutReplacesRemoveDropsAndABrokenFileIsReported(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "repairs.json")}
	testkit.Check(t, l.Put(Link{ID: "a", Stage: StageRepairing}))
	testkit.Check(t, l.Put(Link{ID: "b"}))
	testkit.Check(t, l.Put(Link{ID: "a", Stage: StageMerged}))
	links, err := l.Links()
	testkit.Check(t, err)
	testkit.Equal(t, "two links", len(links), 2)
	got, err := l.Get("a")
	testkit.Check(t, err)
	testkit.Equal(t, "replaced", got.Stage, StageMerged)
	if _, err := l.Get("zzz"); !errors.Is(err, ErrNoLink) {
		t.Fatalf("a link that is not there: %v", err)
	}
	testkit.Check(t, l.Remove("a"))
	testkit.Check(t, l.Remove("a"))
	testkit.Check(t, l.Remove("b"))
	links, _ = l.Links()
	testkit.Equal(t, "empty", len(links), 0)

	testkit.WriteFile(t, l.Path, "not json", 0o644)
	if _, err := l.Links(); err == nil {
		t.Fatal("an unreadable ledger must be reported")
	}
	if l.Put(Link{ID: "x"}) == nil || l.Remove("x") == nil {
		t.Fatal("Put and Remove must not overwrite an unreadable ledger")
	}
	if _, err := l.Get("x"); err == nil {
		t.Fatal("Get must report an unreadable ledger")
	}
}

func TestOpenUsesTheDendriteHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	l, err := Open()
	testkit.Check(t, err)
	testkit.Equal(t, "path", l.Path, filepath.Join(home, "repairs.json"))
}

func TestStartPointsAGoModuleAtTheWorkingCopyAndStampsTheLink(t *testing.T) {
	w := newWorld(t)
	testkit.WriteFile(t, filepath.Join(w.consumer, "go.mod"), "module example.com/a\n\ngo 1.24\n\nrequire example.com/r1 v1.0.0\n", 0o644)
	testkit.Check(t, os.Remove(filepath.Join(w.consumer, "Cargo.toml")))
	before := time.Now().Add(-time.Second)
	res, err := Start(context.Background(), w.ledger, StartOptions{Consumer: w.consumer, Dependency: "example.com/r1", Ticket: "DEN-890", Upstream: w.r1})
	testkit.Check(t, err)
	data, _ := os.ReadFile(filepath.Join(w.consumer, "go.mod"))
	testkit.Contains(t, string(data), "replace example.com/r1 => "+res.Link.Copy)
	if _, err := os.Stat(filepath.Join(w.consumer, "go.work")); !os.IsNotExist(err) {
		t.Errorf("no go.work is left behind: %v", err)
	}

	// A consumer that already has a go.work keeps it as it was.
	testkit.Git(t, w.consumer, "checkout", "-q", "-b", "DEN-892-again/main")
	testkit.WriteFile(t, filepath.Join(w.consumer, "go.mod"), "module example.com/a\n\ngo 1.24\n\nrequire example.com/r1 v1.0.0\n", 0o644)
	testkit.WriteFile(t, filepath.Join(w.consumer, "go.work"), "go 1.24\n\nuse .\n", 0o644)
	_, err = Start(context.Background(), w.ledger, StartOptions{Consumer: w.consumer, Dependency: "example.com/r1", Ticket: "DEN-892", Upstream: w.r1})
	testkit.Check(t, err)
	work, _ := os.ReadFile(filepath.Join(w.consumer, "go.work"))
	testkit.Equal(t, "go.work untouched", string(work), "go 1.24\n\nuse .\n")
	testkit.Equal(t, "stamped now", res.Link.StartedAt.After(before), true)
}

func TestStartStopsBeforeChangingAnythingWhenItCannotFinish(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	o := StartOptions{Consumer: w.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: w.r1}

	testkit.WriteFile(t, w.ledger.Path, "not json", 0o644)
	if _, err := Start(ctx, w.ledger, o); err == nil || !strings.Contains(err.Error(), "not readable") {
		t.Fatalf("an unreadable ledger: %v", err)
	}
	testkit.Check(t, os.Remove(w.ledger.Path))

	plain := t.TempDir() // a consumer that is not a git repository has no working-copy root
	testkit.WriteFile(t, filepath.Join(plain, "Cargo.toml"), "[dependencies]\nr1 = \"1.0\"\n", 0o644)
	if _, err := Start(ctx, w.ledger, StartOptions{Consumer: plain, Dependency: "r1", Ticket: "DEN-890", Upstream: w.r1}); err == nil {
		t.Fatal("a consumer outside git")
	}

	testkit.Check(t, os.Chmod(filepath.Join(w.consumer, "Cargo.toml"), 0o400))
	if _, err := Start(ctx, w.ledger, o); err == nil || !strings.Contains(err.Error(), "was not switched") {
		t.Fatalf("a manifest that cannot be written: %v", err)
	}
}

func TestLedgerReportsWhereItCannotReadOrWrite(t *testing.T) {
	dir := t.TempDir()
	if _, err := (&Ledger{Path: dir}).Links(); err == nil {
		t.Error("a ledger path that is a directory")
	}
	testkit.Check(t, (&Ledger{Path: filepath.Join(dir, "new", "repairs.json")}).Remove("x")) // an empty ledger is written as []
	locked := filepath.Join(dir, "locked")
	testkit.Check(t, os.Mkdir(locked, 0o500))
	if (&Ledger{Path: filepath.Join(locked, "home", "repairs.json")}).Put(Link{ID: "a"}) == nil {
		t.Error("a home that cannot be created")
	}
	blocked := filepath.Join(dir, "blocked.json")
	testkit.Check(t, os.Mkdir(blocked+".tmp", 0o755))
	if (&Ledger{Path: blocked}).Put(Link{ID: "a"}) == nil {
		t.Error("a temporary file that cannot be written")
	}
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := Open(); err == nil {
		t.Error("no home directory")
	}
}
