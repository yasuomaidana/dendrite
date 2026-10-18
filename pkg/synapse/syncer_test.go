package synapse

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/projectid"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// syncWorld is a workspace of member checkouts, each cloned from its own bare origin.
type syncWorld struct {
	t    *testing.T
	root string
}

func newSyncWorld(t *testing.T) *syncWorld {
	t.Helper()
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	return &syncWorld{t: t, root: t.TempDir()}
}

// repo creates <root>/<name> cloned from <root>/origins/<name>.git on develop, seeded
// with files, and mints its project id. It returns the checkout dir, the origin path and the id.
func (w *syncWorld) repo(name string, files map[string]string) (string, string, string) {
	w.t.Helper()
	origin := filepath.Join(w.root, "origins", name+".git")
	seed := filepath.Join(w.root, "seeds", name)
	dir := filepath.Join(w.root, name)
	gitIn(w.t, w.root, "init", "-q", "--bare", "-b", "develop", origin)
	gitIn(w.t, w.root, "clone", "-q", origin, seed)
	gitIn(w.t, seed, "checkout", "-q", "-b", "develop")
	for f, c := range files {
		w.write(seed, f, c)
	}
	gitIn(w.t, seed, "add", "-A")
	gitIn(w.t, seed, "commit", "-q", "-m", "seed")
	gitIn(w.t, seed, "push", "-q", "-u", "origin", "develop")
	gitIn(w.t, w.root, "clone", "-q", "-b", "develop", origin, dir)
	id, err := projectid.Ensure(dir)
	if err != nil {
		w.t.Fatal(err)
	}
	return dir, origin, id
}

func (w *syncWorld) write(dir, name, content string) {
	w.t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// publish pushes a new commit to name's origin through its seed clone.
func (w *syncWorld) publish(name, file string) string {
	w.t.Helper()
	seed := filepath.Join(w.root, "seeds", name)
	w.write(seed, file, file)
	gitIn(w.t, seed, "add", file)
	gitIn(w.t, seed, "commit", "-q", "-m", "add "+file)
	gitIn(w.t, seed, "push", "-q", "origin", "develop")
	return gitIn(w.t, seed, "rev-parse", "HEAD")
}

func TestSyncSynapse_GoWorkLocalThenRemote(t *testing.T) {
	w := newSyncWorld(t)
	hub, hubOrigin, hubID := w.repo("hub", map[string]string{"go.mod": "module example.com/hub\n\ngo 1.25\n"})
	control, _, controlID := w.repo("control", map[string]string{"go.mod": "module example.com/control\n\ngo 1.22\n"})
	s := &Synapse{ID: "stack", State: StateActive, Sync: Sync{Strategy: "go.work"}, Members: []Member{
		{ProjectID: controlID, Alias: "control", Role: RoleConsumer, PathHint: control},
		{ProjectID: hubID, Alias: "hub", Role: RoleProducer, PathHint: hub, Ref: "develop"},
	}}
	ctx := context.Background()
	fixed := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return fixed }

	head := w.publish("hub", "new.go")

	// Status only: hub is behind, so the synapse drifts; producers are listed first.
	rep := SyncSynapse(ctx, s, SyncOptions{Now: now})
	if rep.Members[0].Alias != "hub" || rep.State != StateDrifted || !rep.Changed {
		t.Fatalf("status report = %+v", rep)
	}
	if d := rep.Members[0].Drift; len(d) != 1 || !strings.Contains(d[0], "1 commit(s) behind origin/develop") {
		t.Fatalf("hub drift = %v", d)
	}

	// Pull + link: hub fast-forwards, the synapse is active again, go.work links hub.
	rep = SyncSynapse(ctx, s, SyncOptions{Pull: true, Link: true, Now: now})
	if rep.State != StateActive || len(rep.Members[0].Drift) != 0 || !rep.Members[0].Repo.Updated {
		t.Fatalf("pull report = %+v", rep)
	}
	hubMember, _ := s.Member("hub")
	if hubMember.LockedSHA != head || !hubMember.LockedAt.Equal(fixed) {
		t.Fatalf("hub lock = %s @ %v, want %s", hubMember.LockedSHA, hubMember.LockedAt, head)
	}
	if len(rep.Links) != 1 || rep.Links[0].Action != LinkWritten || rep.Links[0].Uses[0] != "hub" {
		t.Fatalf("links = %+v", rep.Links)
	}
	work, _ := os.ReadFile(filepath.Join(control, "go.work"))
	for _, want := range []string{goWorkHeader, "go 1.25", "\t.\n", "\t../hub\n"} {
		if !strings.Contains(string(work), want) {
			t.Fatalf("go.work missing %q:\n%s", want, work)
		}
	}
	// Idempotent.
	if rep = SyncSynapse(ctx, s, SyncOptions{Link: true, NoFetch: true}); rep.Links[0].Action != LinkUnchanged || rep.Changed {
		t.Fatalf("second link = %+v", rep)
	}

	// Switch hub to remote: its lock follows the remote ref and go.work is removed.
	hubMember.Source, hubMember.Remote = SourceRemote, hubOrigin
	newHead := w.publish("hub", "later.go")
	rep = SyncSynapse(ctx, s, SyncOptions{Link: true})
	if hubMember.LockedSHA != newHead || rep.Members[0].Source != SourceRemote {
		t.Fatalf("remote lock = %s, want %s (%+v)", hubMember.LockedSHA, newHead, rep.Members[0])
	}
	if rep.Links[0].Action != LinkRemoved {
		t.Fatalf("links after remote switch = %+v", rep.Links)
	}
	if _, err := os.Stat(filepath.Join(control, "go.work")); !os.IsNotExist(err) {
		t.Fatal("generated go.work should be removed")
	}
	if rep = SyncSynapse(ctx, s, SyncOptions{Link: true, NoFetch: true}); rep.Links[0].Action != LinkUnchanged {
		t.Fatalf("no go.work and nothing to link = %+v", rep.Links)
	}

	// Offline: the remote member keeps its lock.
	if rep = SyncSynapse(ctx, s, SyncOptions{NoFetch: true}); rep.Members[0].LockedSHA != newHead {
		t.Fatalf("offline remote lock = %+v", rep.Members[0])
	}
	// Unreachable remote: drift via error.
	hubMember.Remote = filepath.Join(w.root, "gone.git")
	if rep = SyncSynapse(ctx, s, SyncOptions{}); rep.State != StateDrifted || rep.Members[0].Error == "" {
		t.Fatalf("unreachable remote = %+v", rep)
	}
}

func TestSyncSynapse_HandWrittenGoWorkIsLeftAlone(t *testing.T) {
	w := newSyncWorld(t)
	hub, _, hubID := w.repo("hub", map[string]string{"go.mod": "module h\n\ngo 1.25\n"})
	control, _, controlID := w.repo("control", map[string]string{"go.mod": "module c\n\ngo 1.25\n"})
	w.write(control, "go.work", "go 1.25\n\nuse .\n")
	s := &Synapse{ID: "s", State: StateActive, Sync: Sync{Strategy: "go.work"}, Members: []Member{
		{ProjectID: controlID, Alias: "control", Role: RoleConsumer, PathHint: control},
		{ProjectID: hubID, Alias: "hub", Role: RoleProducer, PathHint: hub},
	}}
	rep := SyncSynapse(context.Background(), s, SyncOptions{Link: true, NoFetch: true})
	if len(rep.Links) != 1 || rep.Links[0].Action != LinkSkipped || !strings.Contains(rep.Links[0].Reason, "hand-written") {
		t.Fatalf("links = %+v", rep.Links)
	}
	if data, _ := os.ReadFile(filepath.Join(control, "go.work")); string(data) != "go 1.25\n\nuse .\n" {
		t.Fatalf("hand-written go.work modified: %q", data)
	}
}

func TestSyncSynapse_MissingAndMismatchedMembers(t *testing.T) {
	w := newSyncWorld(t)
	hub, _, hubID := w.repo("hub", map[string]string{"README": "x"})
	control, _, controlID := w.repo("control", map[string]string{"go.mod": "module c\n"})
	gitIn(t, control, "checkout", "-q", "-b", "local-work")
	s := &Synapse{ID: "s", State: StateActive, Sync: Sync{Strategy: "go.work"}, Members: []Member{
		{ProjectID: controlID, Alias: "control", Role: RolePeer, PathHint: control, Ref: "develop"},
		{ProjectID: hubID, Alias: "hub", Role: RolePeer, PathHint: hub, Ref: "release"},
	}}
	ctx := context.Background()
	rep := SyncSynapse(ctx, s, SyncOptions{NoFetch: true})
	byAlias := map[string]MemberSync{}
	for _, m := range rep.Members {
		byAlias[m.Alias] = m
	}
	if d := byAlias["control"].Drift; len(d) != 1 || !strings.Contains(d[0], `checked out on "local-work" but tracks "develop"`) {
		t.Fatalf("control drift = %v", d)
	}
	if d := byAlias["hub"].Drift; len(d) != 1 || !strings.Contains(d[0], `tracked ref "release" not found`) {
		t.Fatalf("hub drift = %v", d)
	}
	if rep.State != StateDrifted {
		t.Fatalf("state = %s", rep.State)
	}

	// A member whose checkout disappeared breaks the synapse; linking reports it.
	if err := os.RemoveAll(hub); err != nil {
		t.Fatal(err)
	}
	rep = SyncSynapse(ctx, s, SyncOptions{NoFetch: true, Link: true})
	if rep.State != StateBroken {
		t.Fatalf("state = %s, want broken", rep.State)
	}
	var hubSync MemberSync
	for _, m := range rep.Members {
		if m.Alias == "hub" {
			hubSync = m
		}
	}
	if !hubSync.Missing || !strings.Contains(hubSync.Error, "synapse heal") {
		t.Fatalf("hub = %+v", hubSync)
	}
	var skipped bool
	for _, l := range rep.Links {
		if l.Alias == "hub" && l.Action == LinkSkipped {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("links = %+v", rep.Links)
	}

	// A local member whose directory is not a git checkout reports an error, not missing.
	plain := filepath.Join(w.root, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	plainID, _ := projectid.Ensure(plain)
	s2 := &Synapse{ID: "d", State: StateDraft, Members: []Member{
		{ProjectID: plainID, Alias: "plain", Role: RolePeer, PathHint: plain},
		{ProjectID: controlID, Alias: "control", Role: RolePeer, PathHint: control},
	}}
	rep = SyncSynapse(ctx, s2, SyncOptions{NoFetch: true})
	if rep.State != StateDraft || rep.Members[0].Error == "" || rep.Members[0].Missing {
		t.Fatalf("draft/plain report = %+v", rep)
	}
}

func TestSyncSynapse_MergeRequestedBranch(t *testing.T) {
	w := newSyncWorld(t)
	hub, _, hubID := w.repo("hub", map[string]string{"README": "x"})
	control, _, controlID := w.repo("control", map[string]string{"README": "y"})
	seed := filepath.Join(w.root, "seeds", "hub")
	gitIn(t, seed, "checkout", "-q", "-b", "feat/a")
	w.write(seed, "a.txt", "a")
	gitIn(t, seed, "add", "a.txt")
	gitIn(t, seed, "commit", "-q", "-m", "a")
	gitIn(t, seed, "push", "-q", "origin", "feat/a")

	s := &Synapse{ID: "s", State: StateActive, Members: []Member{
		{ProjectID: controlID, Alias: "control", Role: RoleConsumer, PathHint: control},
		{ProjectID: hubID, Alias: "hub", Role: RoleProducer, PathHint: hub},
	}}
	rep := SyncSynapse(context.Background(), s, SyncOptions{})
	if p := rep.Members[0].Repo.Pending; len(p) != 1 || p[0].Ref != "origin/feat/a" {
		t.Fatalf("pending = %+v", p)
	}
	rep = SyncSynapse(context.Background(), s, SyncOptions{Merge: map[string]string{"hub": "feat/a"}})
	if rep.Members[0].Repo.Merged != "origin/feat/a" || len(rep.Members[0].Repo.Pending) != 0 {
		t.Fatalf("merge = %+v", rep.Members[0].Repo)
	}
}

func TestLinkCargoPatchBlock(t *testing.T) {
	w := newSyncWorld(t)
	core, _, coreID := w.repo("core", map[string]string{"Cargo.toml": "[package]\nname = \"core\"\n"})
	dag, _, dagID := w.repo("embedag", map[string]string{"Cargo.toml": "[package]\nname = \"embedag\"\n"})
	noMod, _, noModID := w.repo("nomod", map[string]string{"Cargo.toml": "[package]\nname = \"nomod\"\n"})
	cfg := filepath.Join(core, ".cargo", "config.toml")
	w.write(core, ".cargo/config.toml", "[build]\njobs = 4")

	s := &Synapse{ID: "rs", Sync: Sync{Strategy: "cargo-patch"}, Members: []Member{
		{ProjectID: coreID, Alias: "core", Role: RoleConsumer, PathHint: core},
		{ProjectID: dagID, Alias: "embedag", Role: RoleProducer, PathHint: dag, Remote: "https://github.com/o/embedag", Module: "embedag"},
		{ProjectID: noModID, Alias: "nomod", Role: RoleProducer, PathHint: noMod},
	}}
	res := Link(s)
	if len(res) != 1 || res[0].Action != LinkWritten || !strings.Contains(res[0].Reason, "nomod") {
		t.Fatalf("link = %+v", res)
	}
	data, _ := os.ReadFile(cfg)
	want := "[build]\njobs = 4\n\n" + blockStart("rs") + "\n" +
		"# Generated by 'dendrite synapse sync'; edits inside this block are overwritten.\n" +
		"[patch.\"https://github.com/o/embedag\"]\nembedag = { path = " + quoteString(dag) + " }\n" +
		blockEnd("rs") + "\n"
	if string(data) != want {
		t.Fatalf("config.toml =\n%s\nwant\n%s", data, want)
	}
	if res = Link(s); res[0].Action != LinkUnchanged {
		t.Fatalf("relink = %+v", res)
	}

	// Remote embedag: the block is dropped and the user's settings survive.
	s.Members[1].Source = SourceRemote
	if res = Link(s); res[0].Action != LinkWritten {
		t.Fatalf("unlink = %+v", res)
	}
	if data, _ := os.ReadFile(cfg); string(data) != "[build]\njobs = 4\n\n" {
		t.Fatalf("after unlink = %q", data)
	}

	// With nothing of the user's left, the file is removed, and then left absent.
	if err := os.WriteFile(cfg, []byte(want[len("[build]\njobs = 4\n\n"):]), 0o644); err != nil {
		t.Fatal(err)
	}
	if res = Link(s); res[0].Action != LinkRemoved {
		t.Fatalf("remove = %+v", res)
	}
	if res = Link(s); res[0].Action != LinkUnchanged {
		t.Fatalf("absent = %+v", res)
	}
}

func TestLinkStrategiesWithoutFiles(t *testing.T) {
	if res := Link(&Synapse{Sync: Sync{Strategy: "pnpm-workspace"}}); len(res) != 1 || res[0].Action != LinkSkipped {
		t.Fatalf("pnpm = %+v", res)
	}
	for _, strat := range []string{"", "none"} {
		if res := Link(&Synapse{Sync: Sync{Strategy: strat}}); res != nil {
			t.Fatalf("%q = %+v", strat, res)
		}
	}
}

func TestLinkerHelpers(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"1.25", "1.9", 1}, {"1.22", "1.22.0", 0}, {"", "1.20", -1}, {"1.21.3", "1.21.10", -1}}
	for _, c := range cases {
		if got := compareGoVersions(c.a, c.b); got != c.want {
			t.Errorf("compareGoVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if got := replaceBlock("", "x", ""); got != "" {
		t.Fatalf("empty replace = %q", got)
	}
	if got := replaceBlock("a = 1\n", "x", ""); got != "a = 1\n" {
		t.Fatalf("no block to remove = %q", got)
	}
	if got := goDirective(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Fatalf("goDirective missing = %q", got)
	}
	if got := relOrAbs("/a/b", "/a/c"); got != "../c" {
		t.Fatalf("relOrAbs = %q", got)
	}
}

func TestWriteIfChangedReportsErrors(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// The parent "directory" is a regular file.
	res := writeIfChanged(LinkResult{File: filepath.Join(blocker, "sub", "go.work")}, nil, []byte("x"))
	if res.Action != LinkSkipped || res.Reason == "" {
		t.Fatalf("res = %+v", res)
	}
}
