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
		"main beats a develop beside it": {func(t *testing.T, d string) { testkit.Git(t, d, "branch", "develop") }, Override{}, "main", SourceMain},
		"a named develop is still used": {func(t *testing.T, d string) {
			withDirective(t, d, "develop")
			testkit.Git(t, d, "branch", "develop")
		}, Override{}, "develop", SourceDirective},
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
	testkit.Equal(t, "branch", got.Branch, "main")
	testkit.Equal(t, "source", got.Source, SourceDefault)
	only := repo(t, "develop")
	got = Resolve(only, Override{})
	testkit.Equal(t, "a develop with no trunk beside it", got.Branch, "develop")
	testkit.Equal(t, "its source", got.Source, SourceDevelop)
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

func TestAnInstalledTargetOfTheCurrentBranchReplacesTheIntegrationBranch(t *testing.T) {
	dir := repo(t, "main")
	testkit.Git(t, dir, "branch", "develop")
	testkit.Git(t, dir, "checkout", "-q", "-b", "agent/x")
	testkit.Git(t, dir, "config", TargetKey("agent/x"), "feature/DEN-1-f")

	got := For(dir)
	testkit.Equal(t, "merge target", got.Branch, "feature/DEN-1-f")
	testkit.Equal(t, "integration", got.Integration, "main")
	testkit.Equal(t, "source", got.Source, SourceTicket)
	testkit.Equal(t, "protected first", got.Protected()[0], "feature/DEN-1-f")
	testkit.Equal(t, "protected has integration", got.Protected()[1], "main")
	testkit.Equal(t, "no duplicates", len(got.Protected()), 4)
	testkit.Equal(t, "named branch", ForBranch(dir, "agent/x").Branch, "feature/DEN-1-f")
}

func TestBranchesMadeBeforeFeatureFoldersKeepDevelopWhileItExists(t *testing.T) {
	dir := repo(t, "main")
	testkit.Equal(t, "no develop to keep", ForBranch(dir, "agent/y").Branch, "main")
	testkit.Git(t, dir, "branch", "develop")
	for branch, want := range map[string]string{
		"agent/y": "develop", "feature/DEN-1-f": "develop", "DEN-2-fix/main": "main", "DEN-1-f/DEN-3-part": "main",
	} {
		testkit.Equal(t, branch, ForBranch(dir, branch).Branch, want)
	}
	legacy := ForBranch(dir, "agent/y")
	testkit.Equal(t, "source", legacy.Source, SourceLegacy)
	testkit.Equal(t, "integration is still main", legacy.Integration, "main")

	withDirective(t, dir, "develop")
	testkit.Equal(t, "a named develop needs no legacy rule", ForBranch(dir, "agent/y").Source, SourceDirective)
}

func TestARecordedTargetThatIsNotABranchNameOrChangesNothingIsIgnored(t *testing.T) {
	dir := repo(t, "main")
	testkit.Git(t, dir, "branch", "develop")
	testkit.Git(t, dir, "checkout", "-q", "-b", "agent/x")
	testkit.Git(t, dir, "config", TargetKey("agent/x"), "not a branch")
	testkit.Equal(t, "invalid", For(dir).Branch, "develop")
	testkit.Git(t, dir, "config", TargetKey("agent/x"), "develop")
	testkit.Equal(t, "recorded develop", For(dir).Source, SourceTicket)
	testkit.Git(t, dir, "config", TargetKey("agent/x"), "main")
	testkit.Equal(t, "same as integration", For(dir).Source, SourceMain)
}

func TestForOnADetachedHeadUsesTheIntegrationBranch(t *testing.T) {
	dir := repo(t, "main")
	testkit.Git(t, dir, "branch", "develop")
	testkit.Git(t, dir, "checkout", "-q", "--detach")
	testkit.Equal(t, "detached", For(dir).Branch, "main")
}

func TestOnlyATrunkIsReachedByPullRequest(t *testing.T) {
	testkit.Table(t, map[string]bool{"main": true, "master": true, "develop": false, "DEN-1-f/main": false, "release/2": false},
		func(branch string) bool { return Target{Branch: branch}.ByPullRequest() })
}
