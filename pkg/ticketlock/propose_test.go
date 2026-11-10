package ticketlock

import (
	"context"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/pullrequest/prtest"
	"Dendrite/pkg/testkit"
)

// proposing is a finish rig whose working copy is a repository on a ticket trunk, with integration as the only
// other branch and a bare origin that has it.
func proposing(t *testing.T, integration string) (*finishRig, *[]Proposal) {
	t.Helper()
	r := newFinishRig(t, "To Do", twoCriteria)
	origin := t.TempDir()
	testkit.Git(t, origin, "init", "-q", "--bare", "-b", integration)
	testkit.GitInit(t, r.Dir, integration)
	testkit.Git(t, r.Dir, "commit", "-q", "--allow-empty", "-m", "base")
	testkit.Git(t, r.Dir, "remote", "add", "origin", origin)
	testkit.Git(t, r.Dir, "push", "-q", "origin", integration)
	testkit.Git(t, r.Dir, "checkout", "-q", "-b", "DEN-377-work/main")
	r.branch = "DEN-377-work/main"
	var seen []Proposal
	r.checks.Propose = func(_ context.Context, p Proposal) (string, error) {
		seen = append(seen, p)
		return "https://github.example/o/r/pull/9", nil
	}
	return r, &seen
}

func TestFinishProposesABranchThatMergesIntoMainAndWaits(t *testing.T) {
	r, seen := proposing(t, "main")
	res, err := r.finish()
	testkit.Check(t, err)
	testkit.Equal(t, "state", res.State, "To Verify")
	testkit.Equal(t, "proposed once", len(*seen), 1)
	p := (*seen)[0]
	testkit.Equal(t, "base", p.Base, "main")
	testkit.Equal(t, "head", p.Head, "DEN-377-work/main")
	testkit.Equal(t, "remote", p.Remote, "origin")
	testkit.Equal(t, "title", p.Title, "[DEN-377] work")
	testkit.Contains(t, p.Body, "Ticket DEN-377", "[AC-1] it works", "[AC-2] it is tested")
	testkit.Contains(t, strings.Join(res.Notes, "\n"), "pull request of DEN-377-work/main into main: https://github.example/o/r/pull/9")
	testkit.Contains(t, r.lastComment(), "pull/9")
}

func TestFinishProposesNothingForABranchThatMergesElsewhere(t *testing.T) {
	r, seen := proposing(t, "develop")
	res, err := r.finish()
	testkit.Check(t, err)
	testkit.Equal(t, "state", res.State, "Done")
	testkit.Equal(t, "not proposed", len(*seen), 0)
}

func TestAPullRequestThatCannotBeOpenedIsANoteThatSaysHow(t *testing.T) {
	r, _ := proposing(t, "main")
	r.checks.Propose = func(context.Context, Proposal) (string, error) {
		return "", errors.New("gh: not logged in\nrun gh auth login")
	}
	res, err := r.finish()
	testkit.Check(t, err)
	testkit.Equal(t, "still waits for a person", res.State, "To Verify")
	notes := strings.Join(res.Notes, "\n")
	testkit.Contains(t, notes, "could not be opened (gh: not logged in)", "gh pr create --base main --head DEN-377-work/main")
	testkit.NotContains(t, notes, "run gh auth login")
}

func TestTheDefaultProposalPushesTheBranchAndOpensThePullRequest(t *testing.T) {
	gh := prtest.New(t)
	r, _ := proposing(t, "main")
	p := Proposal{Dir: r.Dir, Remote: "origin", Base: "main", Head: "DEN-377-work/main", Title: "[DEN-377] work", Body: "b"}

	url, err := proposeByPullRequest(context.Background(), p)
	testkit.Check(t, err)
	testkit.Equal(t, "url", url, "https://github.example/o/r/pull/1")
	testkit.Equal(t, "pushed", testkit.Git(t, r.Dir, "rev-parse", "origin/DEN-377-work/main"), testkit.Git(t, r.Dir, "rev-parse", "HEAD"))
	testkit.Contains(t, gh.Calls(), "pr create --base main --head DEN-377-work/main")

	p.Remote = "nowhere"
	if _, err := proposeByPullRequest(context.Background(), p); err == nil || !strings.Contains(err.Error(), "git push nowhere") {
		t.Fatalf("a failed push must be reported, got %v", err)
	}
	testkit.Equal(t, "the default checks propose by pull request", DefaultFinishChecks(r.Dir).Propose != nil, true)
}
