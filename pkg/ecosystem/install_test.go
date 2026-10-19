package ecosystem

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// fixture is an ecosystem of bare origins, each with main and develop (and rag a feature
// branch), plus a definition pointing at them.
type fixture struct {
	t       *testing.T
	origins string
	def     *synapse.Synapse
	store   *synapse.Store
}

var fixtureIDs = map[string]string{
	"control": "dpid_00000000000000000000000000000001",
	"rag":     "dpid_00000000000000000000000000000002",
	"embed":   "dpid_00000000000000000000000000000003",
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
		"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
	} {
		t.Setenv(k, v)
	}
	f := &fixture{t: t, origins: t.TempDir(), store: synapse.NewStore(t.TempDir())}
	f.def = &synapse.Synapse{ID: "eco", Name: "Eco", State: synapse.StateActive, Sync: synapse.Sync{Strategy: "none"}}
	for _, alias := range []string{"control", "rag", "embed"} {
		origin := filepath.Join(f.origins, alias+".git")
		gitT(t, f.origins, "init", "-q", "--bare", "-b", "main", origin)
		w := filepath.Join(t.TempDir(), alias)
		gitT(t, f.origins, "clone", "-q", origin, w)
		gitT(t, w, "checkout", "-q", "-b", "main")
		// main carries no identity file, like a main cut before it was committed.
		f.commit(w, "README", alias+" main")
		gitT(t, w, "push", "-q", "-u", "origin", "main")
		gitT(t, w, "checkout", "-q", "-b", "develop")
		f.commit(w, ".dendrite/project.id", fixtureIDs[alias])
		gitT(t, w, "push", "-q", "-u", "origin", "develop")
		if alias == "rag" {
			gitT(t, w, "checkout", "-q", "-b", "feature")
			f.commit(w, "feature.txt", "wip")
			gitT(t, w, "push", "-q", "-u", "origin", "feature")
		}
		role := synapse.RoleProducer
		if alias == "control" {
			role = synapse.RoleConsumer
		}
		f.def.Members = append(f.def.Members, synapse.Member{
			ProjectID: fixtureIDs[alias], Alias: alias, Role: role, PathHint: "repo-" + alias, Remote: origin,
		})
	}
	return f
}

func (f *fixture) commit(dir, name, content string) {
	f.t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content+"\n"), 0o644); err != nil {
		f.t.Fatal(err)
	}
	gitT(f.t, dir, "add", name)
	gitT(f.t, dir, "commit", "-q", "-m", "add "+name)
}

func (f *fixture) install(root string, ch synapse.Channel, refs map[string]string) (*Result, error) {
	f.t.Helper()
	return Install(context.Background(), Options{Root: root, Channel: ch, Refs: refs, Definition: f.def, Store: f.store})
}

func branchOf(t *testing.T, dir string) string { return gitT(t, dir, "branch", "--show-current") }

func TestInstall_StableClonesMainEverywhere(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	res, err := f.install(root, synapse.ChannelStable, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, m := range res.Members {
		if !m.Cloned || m.Ref != "main" {
			t.Fatalf("%s: expected a main clone, got %+v", m.Alias, m)
		}
		if b := branchOf(t, m.Dir); b != "main" {
			t.Fatalf("%s on %q, want main", m.Alias, b)
		}
		// main lacks the identity file, so install wrote it.
		if id, err := projectid.Read(m.Dir); err != nil || id != fixtureIDs[m.Alias] {
			t.Fatalf("%s identity %q, %v", m.Alias, id, err)
		}
	}
	saved, err := f.store.Load("eco")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Channel != synapse.ChannelStable {
		t.Fatalf("stored channel %q", saved.Channel)
	}
	for _, m := range saved.Members {
		if !filepath.IsAbs(m.PathHint) || m.LockedSHA == "" {
			t.Fatalf("stored member %s should have an absolute path and lock: %+v", m.Alias, m)
		}
	}
}

func TestInstall_DevSwitchesExistingCheckoutsAndOverrides(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	if _, err := f.install(root, synapse.ChannelStable, nil); err != nil {
		t.Fatalf("stable install: %v", err)
	}

	res, err := f.install(root, synapse.ChannelDev, map[string]string{"rag": "feature"})
	if err != nil {
		t.Fatalf("dev install: %v", err)
	}
	want := map[string]string{"control": "develop", "rag": "feature", "embed": "develop"}
	for _, m := range res.Members {
		if m.Cloned {
			t.Fatalf("%s should be reused, not cloned again", m.Alias)
		}
		if b := branchOf(t, m.Dir); b != want[m.Alias] {
			t.Fatalf("%s on %q, want %q", m.Alias, b, want[m.Alias])
		}
	}
	saved, _ := f.store.Load("eco")
	if rag, _ := saved.Member("rag"); rag.Ref != "feature" || saved.Channel != synapse.ChannelDev {
		t.Fatalf("override or channel not stored: channel=%q rag=%+v", saved.Channel, rag)
	}

	// A later install keeps the stored override; an empty --ref clears it.
	if _, err := f.install(root, synapse.ChannelDev, nil); err != nil {
		t.Fatal(err)
	}
	if b := branchOf(t, filepath.Join(root, "repo-rag")); b != "feature" {
		t.Fatalf("stored override lost, rag on %q", b)
	}
	if _, err := f.install(root, synapse.ChannelDev, map[string]string{"rag": ""}); err != nil {
		t.Fatal(err)
	}
	if b := branchOf(t, filepath.Join(root, "repo-rag")); b != "develop" {
		t.Fatalf("cleared override should return rag to develop, on %q", b)
	}
}

func TestInstall_DirtyMemberStopsInstall(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	if _, err := f.install(root, synapse.ChannelStable, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "repo-embed", "README"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := f.install(root, synapse.ChannelDev, nil)
	if err == nil || !strings.Contains(err.Error(), "embed") {
		t.Fatalf("expected the dirty member to fail the install, got %v", err)
	}
	if b := branchOf(t, filepath.Join(root, "repo-embed")); b != "main" {
		t.Fatalf("dirty member must stay put, on %q", b)
	}
	if b := branchOf(t, filepath.Join(root, "repo-rag")); b != "develop" {
		t.Fatalf("clean members still move, rag on %q", b)
	}
}

func TestInstall_KeepsStoredLocationsAndExplicitDirs(t *testing.T) {
	f := newFixture(t)
	elsewhere := filepath.Join(t.TempDir(), "my-embed")
	gitT(t, f.origins, "clone", "-q", "-b", "develop", filepath.Join(f.origins, "embed.git"), elsewhere)
	control := filepath.Join(t.TempDir(), "Dendrite")
	gitT(t, f.origins, "clone", "-q", "-b", "develop", filepath.Join(f.origins, "control.git"), control)

	prev := *f.def
	prev.Members = append([]synapse.Member(nil), f.def.Members...)
	prev.Members[2].PathHint = elsewhere
	prev.Members = append(prev.Members, synapse.Member{ProjectID: "dpid_00000000000000000000000000000009", Alias: "extra", Role: synapse.RolePeer, PathHint: elsewhere + "-x"})
	prev.Parity.Commands = map[string]string{"extra": "make test"}
	if err := f.store.Save(&prev); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	res, err := Install(context.Background(), Options{
		Root: root, Channel: synapse.ChannelDev, Definition: f.def, Store: f.store,
		Dirs: map[string]string{"control": control},
	})
	// The extra member has no checkout and no remote: install refuses to guess.
	if err == nil || !strings.Contains(err.Error(), "extra") {
		t.Fatalf("expected the unresolvable extra member to fail, got %v (res %+v)", err, res)
	}

	prev.Members = prev.Members[:3]
	prev.Parity.Commands = nil
	if err := f.store.Save(&prev); err != nil {
		t.Fatal(err)
	}
	res, err = Install(context.Background(), Options{
		Root: root, Channel: synapse.ChannelDev, Definition: f.def, Store: f.store,
		Dirs: map[string]string{"control": control},
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	dirs := map[string]string{}
	for _, m := range res.Members {
		dirs[m.Alias] = m.Dir
	}
	if dirs["embed"] != elsewhere || dirs["control"] != control || dirs["rag"] != filepath.Join(root, "repo-rag") {
		t.Fatalf("unexpected member locations: %v", dirs)
	}
}

func TestInstall_RejectsBadInput(t *testing.T) {
	f := newFixture(t)
	if _, err := f.install(t.TempDir(), "nightly", nil); err == nil {
		t.Fatal("expected an invalid channel error")
	}
	if _, err := f.install(t.TempDir(), synapse.ChannelDev, map[string]string{"nope": "x"}); err == nil {
		t.Fatal("expected an unknown member error")
	}
	if _, err := f.install(t.TempDir(), synapse.ChannelDev, map[string]string{"rag": "no-such-branch"}); err == nil {
		t.Fatal("expected a clone error for a missing branch")
	}

	// A directory holding another project is refused.
	root := t.TempDir()
	other := filepath.Join(root, "repo-control")
	if err := os.MkdirAll(filepath.Join(other, ".dendrite"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectid.Path(other), []byte("dpid_0000000000000000000000000000000f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.install(root, synapse.ChannelDev, nil); err == nil || !strings.Contains(err.Error(), "holds project") {
		t.Fatalf("expected a foreign project error, got %v", err)
	}
}

func TestDefinition_EmbeddedEcosystemIsValid(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("embedded definition invalid: %v", err)
	}
	if _, ok := def.Member(ControlAlias); !ok {
		t.Fatal("embedded definition has no control member")
	}
	for _, m := range def.Members {
		if m.Remote == "" || filepath.IsAbs(m.PathHint) || m.Ref != "" {
			t.Fatalf("member %s must carry a remote, a relative directory and no pinned ref: %+v", m.Alias, m)
		}
	}
}
