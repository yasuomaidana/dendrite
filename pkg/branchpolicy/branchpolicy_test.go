package branchpolicy

import (
	"Dendrite/pkg/projectid"
	"path/filepath"
	"testing"

	"Dendrite/pkg/testkit"
)

// repo makes a git repository whose first branch is first and which has one commit.
func repo(t *testing.T, first string) string {
	t.Helper()
	dir := t.TempDir()
	testkit.GitInit(t, dir, first)
	testkit.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	return dir
}

// writeGit writes a directives file holding only a [git] table.
func writeGit(t *testing.T, dir, body string) {
	t.Helper()
	testkit.WriteFile(t, filepath.Join(dir, filepath.FromSlash(DirectivePath)), "schema = 1\n[git]\n"+body, 0o644)
}

func withDirective(t *testing.T, dir, branch string) {
	t.Helper()
	writeGit(t, dir, "integration_branch = \""+branch+"\"\n")
}

func TestResolveChoosesTheStrongestSource(t *testing.T) {
	for name, tc := range map[string]struct {
		setup      func(t *testing.T, dir string)
		ov         Override
		wantBranch string
		wantSource Source
	}{
		"member beats directive": {func(t *testing.T, d string) { withDirective(t, d, "trunk") }, Override{Branch: "release/2"}, "release/2", SourceMember},
		"directive beats develop": {func(t *testing.T, d string) {
			withDirective(t, d, "trunk")
			testkit.Git(t, d, "branch", "develop")
		}, Override{}, "trunk", SourceDirective},
		"develop keeps legacy projects working": {func(t *testing.T, d string) { testkit.Git(t, d, "branch", "develop") }, Override{}, "develop", SourceDevelop},
		"remote default": {func(t *testing.T, d string) {
			testkit.Git(t, d, "update-ref", "refs/remotes/origin/trunk", "HEAD")
			testkit.Git(t, d, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
		}, Override{}, "trunk", SourceRemoteHEAD},
		"main":                           {func(t *testing.T, d string) {}, Override{}, "main", SourceMain},
		"invalid member name is ignored": {func(t *testing.T, d string) {}, Override{Branch: "a b"}, "main", SourceMain},
	} {
		dir := repo(t, "main")
		tc.setup(t, dir)
		got := Resolve(dir, tc.ov)
		if got.Branch != tc.wantBranch || got.Source != tc.wantSource {
			t.Errorf("%s: got %s from %q, want %s from %q", name, got.Branch, got.Source, tc.wantBranch, tc.wantSource)
		}
	}
}

func TestResolveFallsBackWhenNothingExists(t *testing.T) {
	master := repo(t, "master")
	testkit.Equal(t, "master", Resolve(master, Override{}).Source, SourceMaster)
	other := repo(t, "scratch")
	got := Resolve(other, Override{})
	testkit.Equal(t, "branch", got.Branch, "develop")
	testkit.Equal(t, "source", got.Source, SourceDefault)
}

func TestResolveSurvivesABrokenDirectivesFile(t *testing.T) {
	dir := repo(t, "main")
	path := filepath.Join(dir, filepath.FromSlash(DirectivePath))
	testkit.WriteFile(t, path, "this is = not [valid", 0o644)
	got := Resolve(dir, Override{})
	testkit.Equal(t, "branch", got.Branch, "main")
	testkit.Equal(t, "prefix", got.FeaturePrefix, "feature/")
}

func TestResolveRemotes(t *testing.T) {
	dir := repo(t, "main")
	for name, tc := range map[string]struct {
		ov           Override
		push, target string
	}{
		"origin by default":        {Override{}, "origin", "origin"},
		"fork push targets itself": {Override{PushRemote: "fork"}, "fork", "fork"},
		"fork with upstream":       {Override{PushRemote: "fork", TargetRemote: "upstream"}, "fork", "upstream"},
	} {
		got := Resolve(dir, tc.ov)
		if got.PushRemote != tc.push || got.Remote != tc.target {
			t.Errorf("%s: push %s target %s, want %s %s", name, got.PushRemote, got.Remote, tc.push, tc.target)
		}
	}
	testkit.Equal(t, "Ref", Resolve(dir, Override{TargetRemote: "upstream", Branch: "dev"}).Ref(), "upstream/dev")
}

func TestResolveUsesTheDirectiveFeaturePrefix(t *testing.T) {
	dir := repo(t, "main")
	writeGit(t, dir, "feature_prefix = \"feat/\"\n")
	testkit.Equal(t, "prefix", Resolve(dir, Override{}).FeaturePrefix, "feat/")
}

func TestTargetBranchLists(t *testing.T) {
	tg := Target{Branch: "trunk", Remote: "up"}
	testkit.Equal(t, "protected", len(tg.Protected()), 4)
	testkit.Equal(t, "first", tg.Protected()[0], "trunk")
	testkit.Equal(t, "no duplicate develop", len(Target{Branch: "develop"}.Protected()), 3)
	if !tg.IsProtected("main") || tg.IsProtected("agent/DEN-1") {
		t.Error("IsProtected wrong")
	}
	got := tg.MergeTargets()
	testkit.Equal(t, "merge targets", len(got), 8)
	testkit.Equal(t, "remote form", got[1], "up/trunk")
}

func TestExplicitOnlyForChosenBranches(t *testing.T) {
	testkit.Equal(t, "member", Target{Source: SourceMember}.Explicit(), true)
	testkit.Equal(t, "directive", Target{Source: SourceDirective}.Explicit(), true)
	testkit.Equal(t, "detected", Target{Source: SourceMain}.Explicit(), false)
}

func TestValidBranch(t *testing.T) {
	for name, ok := range map[string]bool{
		"develop": true, "release/1.2": true, "agent/DEN-1": true,
		"": false, "my branch": false, "a..b": false, "-x": false, "a/": false, "/a": false, "a.lock": false, "a.": false, "a//b": false, "a~b": false, "a@{1}": false,
	} {
		testkit.Equal(t, name, ValidBranch(name), ok)
	}
}

// synapseWorld is a Dendrite home with the given manifests and a git project that has an id.
func synapseWorld(t *testing.T, manifests map[string]string) (project, id string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	for name, body := range manifests {
		testkit.WriteFile(t, filepath.Join(home, synapsesDir, name), body, 0o644)
	}
	project = repo(t, "main")
	id, err := projectid.Ensure(project)
	testkit.Check(t, err)
	return project, id
}

func manifestWith(state, id, fields string) string {
	return "id = \"s\"\nstate = \"" + state + "\"\n\n[[members]]\nproject_id = \"" + id + "\"\nalias = \"a\"\n" + fields
}

func TestForReadsTheSynapseMemberOfTheProject(t *testing.T) {
	project, id := synapseWorld(t, nil)
	testkit.Equal(t, "no synapse", For(project).Branch, "main")

	home, _ := projectid.HomeDir()
	for name, body := range map[string]string{
		"a-archived.toml": manifestWith("archived", id, "branch = \"ignored\"\n"),
		"b-plain.toml":    manifestWith("active", id, ""),
		"c-forked.toml":   manifestWith("active", id, "branch = \"dev\"\npush_remote = \"fork\"\ntarget_remote = \"upstream\"\n"),
		"d-later.toml":    manifestWith("active", id, "branch = \"later\"\n"),
		"e-broken.toml":   "this is = not [valid",
		"f-other.toml":    manifestWith("active", "dpid_other", "branch = \"other\"\n"),
	} {
		testkit.WriteFile(t, filepath.Join(home, synapsesDir, name), body, 0o644)
	}
	got := For(project)
	testkit.Equal(t, "branch", got.Branch, "dev")
	testkit.Equal(t, "push", got.PushRemote, "fork")
	testkit.Equal(t, "target", got.Remote, "upstream")
	testkit.Equal(t, "source", got.Source, SourceMember)
}

func TestMemberOverrideIsEmptyWithoutAProjectOrAHome(t *testing.T) {
	plain := repo(t, "main")
	t.Setenv("DENDRITE_HOME", t.TempDir())
	testkit.Equal(t, "no project id", MemberOverride(plain), Override{})

	project, _ := synapseWorld(t, nil)
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	testkit.Equal(t, "no home", MemberOverride(project), Override{})

	testkit.Equal(t, "unreadable file", memberIn(filepath.Join(t.TempDir(), "missing.toml"), "dpid_x"), Override{})
}

func TestBasesListTheRemoteTrackingBranchFirst(t *testing.T) {
	got := Target{Branch: "release", Remote: "fork"}.Bases()
	testkit.Equal(t, "remote first", got[0], "fork/release")
	testkit.Equal(t, "local second", got[1], "release")
}

func TestMemberInIgnoresManifestsWithoutTheProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.toml")
	testkit.WriteFile(t, path, manifestWith("active", "dpid_other", "branch = \"x\"\n"), 0o644)
	testkit.Equal(t, "other project", memberIn(path, "dpid_me"), Override{})
}
