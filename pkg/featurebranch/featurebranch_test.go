package featurebranch

import (
	"context"
	"errors"
	"testing"

	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/testkit"
)

// parents is a Source over a fixed child -> parent map.
type parents map[string]Ticket

func (p parents) Parent(_ context.Context, ticket string) (Ticket, bool, error) {
	t, ok := p[ticket]
	return t, ok, nil
}

// titled is parents plus the title of each ticket itself.
type titled struct {
	parents
	titles map[string]string
}

func (s titled) Title(_ context.Context, ticket string) (string, error) {
	title, ok := s.titles[ticket]
	if !ok {
		return "", errors.New("no such ticket")
	}
	return title, nil
}

type failing struct{}

func (failing) Parent(context.Context, string) (Ticket, bool, error) {
	return Ticket{}, false, errors.New("plane is down")
}

// project is a repository with a develop branch, a bare origin that has it, and one commit.
func project(t *testing.T) (dir, origin string) {
	t.Helper()
	origin = t.TempDir()
	testkit.Git(t, origin, "init", "-q", "--bare", "-b", "develop")
	dir = t.TempDir()
	testkit.GitInit(t, dir, "develop")
	testkit.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	testkit.Git(t, dir, "remote", "add", "origin", origin)
	testkit.Git(t, dir, "push", "-q", "origin", "develop")
	return dir, origin
}

func TestSlugKeepsATitleReadableAndShort(t *testing.T) {
	testkit.Table(t, map[string]string{
		"Configurable integration branches": "configurable-integration-branches",
		"  Fix: the  [guard]!  ":            "fix-the-guard",
		"":                                  "",
		"***":                               "",
		"a very long title that goes on and on and on and on": "a-very-long-title-that-goes-on-and-on-an",
	}, Slug)
}

func TestNameAddsTheParentIDAndSlug(t *testing.T) {
	testkit.Equal(t, "with title", Name("feature/", Ticket{ID: "DEN-840", Title: "Roadmap S2"}), "feature/DEN-840-roadmap-s2")
	testkit.Equal(t, "no title", Name("feat/", Ticket{ID: "DEN-9", Title: "!!"}), "feat/DEN-9")
}

func TestResolveWithoutAParentTargetsTheIntegrationBranch(t *testing.T) {
	dir, _ := project(t)
	p, err := Resolve(context.Background(), dir, "DEN-841", parents{})
	testkit.Check(t, err)
	testkit.Equal(t, "target", p.Target, "develop")
	testkit.Equal(t, "feature", p.Feature, false)
	testkit.Equal(t, "remote", p.PushRemote, "origin")
	testkit.Equal(t, "branch without a title", p.Branch, "DEN-841/main")
	testkit.Equal(t, "its own trunk", p.Trunk, p.Branch)
	testkit.Equal(t, "copy", p.CopyName(), "DEN-841")
}

func TestResolveNamesALoneTicketsTrunkAfterItsTitle(t *testing.T) {
	dir, _ := project(t)
	src := titled{parents{}, map[string]string{"DEN-890": "Fix login crash"}}
	p, err := Resolve(context.Background(), dir, "DEN-890", src)
	testkit.Check(t, err)
	testkit.Equal(t, "branch", p.Branch, "DEN-890-fix-login-crash/main")
	testkit.Equal(t, "target", p.Target, "develop")
	testkit.Equal(t, "copy", p.CopyName(), "DEN-890-fix-login-crash")

	testkit.Git(t, dir, "branch", "DEN-890-old-title/main")
	p, err = Resolve(context.Background(), dir, "DEN-890", src)
	testkit.Check(t, err)
	testkit.Equal(t, "an existing trunk keeps its name", p.Branch, "DEN-890-old-title/main")

	if _, err := Resolve(context.Background(), dir, "DEN-1", src); err == nil {
		t.Fatal("a title that cannot be read must not be taken for an empty one")
	}
}

func TestResolvePutsASubTicketInTheFolderOfItsTopMostParent(t *testing.T) {
	dir, _ := project(t)
	src := titled{parents{"DEN-881": {"DEN-880", "Branch naming"}}, map[string]string{"DEN-881": "Slug names"}}
	p, err := Resolve(context.Background(), dir, "DEN-881", src)
	testkit.Check(t, err)
	testkit.Equal(t, "branch", p.Branch, "DEN-880-branch-naming/DEN-881-slug-names")
	testkit.Equal(t, "target", p.Target, "DEN-880-branch-naming/main")
	testkit.Equal(t, "trunk", p.Trunk, p.Target)
	testkit.Equal(t, "copy", p.CopyName(), "DEN-881-slug-names")

	testkit.Git(t, dir, "branch", "DEN-880-renamed/main")
	p, err = Resolve(context.Background(), dir, "DEN-881", src)
	testkit.Check(t, err)
	testkit.Equal(t, "existing folder", p.Branch, "DEN-880-renamed/DEN-881-slug-names")
	testkit.Equal(t, "existing trunk", p.Target, "DEN-880-renamed/main")
}

func TestResolveKeepsALegacyFeatureBranchAsTheTrunk(t *testing.T) {
	dir, _ := project(t)
	testkit.Git(t, dir, "branch", "feature/DEN-880-old")
	src := titled{parents{"DEN-881": {"DEN-880", "Branch naming"}}, map[string]string{"DEN-881": "Slug names"}}
	p, err := Resolve(context.Background(), dir, "DEN-881", src)
	testkit.Check(t, err)
	testkit.Equal(t, "target", p.Target, "feature/DEN-880-old")
	testkit.Equal(t, "branch beside it, not below it", p.Branch, "DEN-880-branch-naming/DEN-881-slug-names")
}

func TestResolveNamesTheFeatureBranchAfterTheTopMostParent(t *testing.T) {
	dir, _ := project(t)
	src := parents{"DEN-3": {"DEN-2", "Middle"}, "DEN-2": {"DEN-1", "Roadmap S2: gate wrapper"}}
	p, err := Resolve(context.Background(), dir, "DEN-3", src)
	testkit.Check(t, err)
	testkit.Equal(t, "target", p.Target, "DEN-1-roadmap-s2-gate-wrapper/main")
	testkit.Equal(t, "branch", p.Branch, "DEN-1-roadmap-s2-gate-wrapper/DEN-3")
	testkit.Equal(t, "feature", p.Feature, true)
	testkit.Equal(t, "parent", p.Parent.ID, "DEN-1")
}

func TestResolveKeepsTheNameOfAnExistingFeatureBranch(t *testing.T) {
	dir, _ := project(t)
	testkit.Git(t, dir, "branch", "feature/DEN-1-old-title")
	p, err := Resolve(context.Background(), dir, "DEN-2", parents{"DEN-2": {"DEN-1", "Renamed in Plane"}})
	testkit.Check(t, err)
	testkit.Equal(t, "target", p.Target, "feature/DEN-1-old-title")

	testkit.Git(t, dir, "branch", "feature/DEN-12-other")
	q, err := Resolve(context.Background(), dir, "DEN-5", parents{"DEN-5": {"DEN-1", "Fresh"}})
	testkit.Check(t, err)
	testkit.Equal(t, "DEN-12 is not DEN-1", q.Target, "feature/DEN-1-old-title")
}

func TestResolveStopsAtACycleOfParents(t *testing.T) {
	dir, _ := project(t)
	p, err := Resolve(context.Background(), dir, "DEN-1", parents{"DEN-1": {"DEN-2", "A"}, "DEN-2": {"DEN-1", "B"}})
	testkit.Check(t, err)
	testkit.Equal(t, "feature", p.Feature, true)
}

func TestResolveReportsALookupFailure(t *testing.T) {
	dir, _ := project(t)
	if _, err := Resolve(context.Background(), dir, "DEN-1", failing{}); err == nil {
		t.Fatal("a Plane failure must not be taken for 'no parent'")
	}
}

func TestEnsureCreatesAndPushesTheFeatureBranchOnce(t *testing.T) {
	dir, origin := project(t)
	p, _ := Resolve(context.Background(), dir, "DEN-2", parents{"DEN-2": {"DEN-1", "Feature"}})
	pushErr, err := Ensure(dir, p)
	testkit.Check(t, err)
	testkit.Check(t, pushErr)
	testkit.Equal(t, "local", testkit.Git(t, dir, "rev-parse", "--verify", "refs/heads/"+p.Target) != "", true)
	testkit.Equal(t, "remote", testkit.Git(t, origin, "rev-parse", "--verify", "refs/heads/"+p.Target) != "", true)

	pushErr, err = Ensure(dir, p)
	testkit.Check(t, err)
	testkit.Check(t, pushErr)
}

func TestEnsureTracksABranchThatOnlyTheRemoteHas(t *testing.T) {
	dir, origin := project(t)
	other := t.TempDir()
	testkit.Git(t, other, "clone", "-q", origin, ".")
	testkit.Git(t, other, "branch", "feature/DEN-1-x")
	testkit.Git(t, other, "push", "-q", "origin", "feature/DEN-1-x")
	testkit.Git(t, dir, "fetch", "-q", "origin")

	p, _ := Resolve(context.Background(), dir, "DEN-2", parents{"DEN-2": {"DEN-1", "Anything"}})
	testkit.Equal(t, "reuses the remote name", p.Target, "feature/DEN-1-x")
	pushErr, err := Ensure(dir, p)
	testkit.Check(t, err)
	testkit.Check(t, pushErr)
	testkit.Equal(t, "local copy", testkit.Git(t, dir, "rev-parse", "--verify", "refs/heads/feature/DEN-1-x") != "", true)
}

func TestEnsureReportsAFailedPushButKeepsTheBranch(t *testing.T) {
	dir, _ := project(t)
	testkit.Git(t, dir, "remote", "set-url", "origin", "/nonexistent/remote.git")
	p, _ := Resolve(context.Background(), dir, "DEN-2", parents{"DEN-2": {"DEN-1", "F"}})
	pushErr, err := Ensure(dir, p)
	testkit.Check(t, err)
	if pushErr == nil {
		t.Fatal("expected the push error")
	}
	testkit.Equal(t, "branch kept", testkit.Git(t, dir, "rev-parse", "--verify", "refs/heads/"+p.Target) != "", true)
}

func TestEnsureNeedsNothingWithoutAFeatureAndFailsWithoutACommit(t *testing.T) {
	dir, _ := project(t)
	pushErr, err := Ensure(dir, Plan{Target: "develop"})
	testkit.Check(t, err)
	testkit.Check(t, pushErr)

	empty := t.TempDir()
	testkit.GitInit(t, empty, "develop")
	if _, err := Ensure(empty, Plan{Feature: true, Target: "feature/DEN-1", Integration: "develop", Remote: "origin", PushRemote: "origin"}); err == nil {
		t.Fatal("a repository with no integration commit cannot start a feature branch")
	}
}

func TestRecordMakesBranchpolicyTargetTheFeatureBranch(t *testing.T) {
	dir, _ := project(t)
	testkit.Git(t, dir, "checkout", "-q", "-b", "agent/den-2")
	testkit.Equal(t, "before", branchpolicy.For(dir).Branch, "develop")

	testkit.Check(t, Record(dir, "agent/den-2", "feature/DEN-1-f", "develop"))
	got := branchpolicy.For(dir)
	testkit.Equal(t, "target", got.Branch, "feature/DEN-1-f")
	testkit.Equal(t, "integration kept", got.Integration, "develop")
	testkit.Equal(t, "source", got.Source, branchpolicy.SourceTicket)
	testkit.Equal(t, "for a named branch", branchpolicy.ForBranch(dir, "agent/den-2").Branch, "feature/DEN-1-f")
	testkit.Equal(t, "other branch", branchpolicy.ForBranch(dir, "agent/den-9").Branch, "develop")
	testkit.Equal(t, "bases", got.Bases()[1], "feature/DEN-1-f")
	if !got.IsProtected("feature/DEN-1-f") || !got.IsProtected("develop") {
		t.Error("both the feature branch and the integration branch are protected")
	}

	testkit.Check(t, Record(dir, "agent/den-2", "develop", "develop"))
	testkit.Equal(t, "cleared", branchpolicy.For(dir).Branch, "develop")
	testkit.Check(t, Record(dir, "agent/den-2", "", "develop"))
}

func TestResolveOutsideAGitRepositoryStillNamesTheBranch(t *testing.T) {
	p, err := Resolve(context.Background(), t.TempDir(), "DEN-2", parents{"DEN-2": {"DEN-1", "No repo"}})
	testkit.Check(t, err)
	testkit.Equal(t, "target", p.Target, "DEN-1-no-repo/main")
}
