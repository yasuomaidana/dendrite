package soma

import (
	"strings"
	"testing"

	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/pullrequest/prtest"
	"Dendrite/pkg/testkit"
)

// retiring is a repository on main with a bare origin, a develop one commit ahead of main, and an agent branch
// made before feature folders; it is the working directory, and gh is the local one.
func retiring(t *testing.T) (repo string, gh *prtest.GitHub) {
	t.Helper()
	clitest.JsonOutput(t, false)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	gh = prtest.New(t)
	origin := t.TempDir()
	testkit.Git(t, origin, "init", "-q", "--bare", "-b", "main")
	repo = t.TempDir()
	testkit.GitInit(t, repo, "main")
	testkit.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	testkit.Git(t, repo, "remote", "add", "origin", origin)
	testkit.Git(t, repo, "push", "-q", "origin", "main")
	testkit.Git(t, repo, "checkout", "-q", "-b", "develop")
	testkit.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "only on develop")
	testkit.Git(t, repo, "push", "-q", "origin", "develop")
	testkit.Git(t, repo, "branch", "agent/den-1")
	testkit.Git(t, repo, "branch", "DEN-2-new/main")
	testkit.Git(t, repo, "checkout", "-q", "main")
	t.Chdir(repo)
	return repo, gh
}

func TestBranchRetireProposesWhatIsLeftAndListsWhoStillTargetsIt(t *testing.T) {
	_, gh := retiring(t)
	out, err := runBranch(t, "retire", "develop")
	testkit.Check(t, err)
	testkit.Contains(t, out, "develop has 1 commit(s) main does not: pull request https://github.example/o/r/pull/1", "- agent/den-1 still merges into develop")
	testkit.NotContains(t, out, "DEN-2-new/main still", "Nothing uses develop")
	testkit.Contains(t, gh.Calls(), "pr create --base main --head develop --title Retire develop: merge it into main")

	gh.Merge("develop", "main", "abc")
	out, err = runBranch(t, "retire", "develop")
	testkit.Check(t, err)
	testkit.Contains(t, out, "develop was merged", "fetch to see it")
	testkit.Equal(t, "nothing proposed twice", strings.Count(gh.Calls(), "pr create"), 1)
}

func TestBranchRetireSaysWhenNothingIsLeft(t *testing.T) {
	repo, gh := retiring(t)
	testkit.Git(t, repo, "merge", "-q", "--ff-only", "develop")
	testkit.Git(t, repo, "push", "-q", "origin", "main")
	testkit.Git(t, repo, "branch", "-D", "agent/den-1")
	out, err := runBranch(t, "retire", "develop")
	testkit.Check(t, err)
	testkit.Contains(t, out, "main holds every commit of develop", "Nothing uses develop any more", "dendrite work dispose")
	testkit.Equal(t, "no pull request needed", gh.Calls(), "")

	testkit.Git(t, repo, "branch", "-D", "develop")
	clitest.JsonOutput(t, true)
	out, err = runBranch(t, "retire", "develop")
	testkit.Check(t, err)
	testkit.Contains(t, out, `"retired": true`, `"dependants": []`)
}

func TestBranchRetireRefusesTheIntegrationBranchAMissingBranchAndReportsGh(t *testing.T) {
	_, gh := retiring(t)
	if _, err := runBranch(t, "retire", "main"); err == nil || !strings.Contains(err.Error(), "is the integration branch") {
		t.Fatalf("retiring the integration branch: %v", err)
	}
	if _, err := runBranch(t, "retire", "nope"); err == nil || !strings.Contains(err.Error(), "there is no branch nope") {
		t.Fatalf("a branch that does not exist: %v", err)
	}
	testkit.Git(t, ".", "branch", "-D", "develop")
	out, err := runBranch(t, "retire", "develop")
	testkit.Check(t, err)
	testkit.Contains(t, out, "develop has 1 commit(s) main does not") // read from origin/develop
	testkit.Git(t, ".", "branch", "develop", "origin/develop")
	gh.Break()
	if _, err := runBranch(t, "retire", "develop"); err == nil || !strings.Contains(err.Error(), "opening the pull request of develop into main") {
		t.Fatalf("a failing gh: %v", err)
	}
}
