package pullrequest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/pullrequest"
	"Dendrite/pkg/pullrequest/prtest"
	"Dendrite/pkg/testkit"
)

func TestOpenCreatesAPullRequestOnceAndViewReadsIt(t *testing.T) {
	gh := prtest.New(t)
	ctx, dir := context.Background(), t.TempDir()
	req := pullrequest.Request{Dir: dir, Base: "main", Head: "DEN-1-fix/main", Title: "[DEN-1] Fix", Body: "why"}

	if _, err := pullrequest.View(ctx, dir, req.Head); !errors.Is(err, pullrequest.ErrNone) {
		t.Fatalf("a branch with no pull request: %v", err)
	}
	pr, err := pullrequest.Open(ctx, req)
	testkit.Check(t, err)
	testkit.Equal(t, "url is the last line gh printed", pr.URL, "https://github.example/o/r/pull/1")
	testkit.Equal(t, "state", pr.State, pullrequest.StateOpen)
	testkit.Contains(t, gh.Calls(), "pr create --base main --head DEN-1-fix/main --title [DEN-1] Fix --body why")

	again, err := pullrequest.Open(ctx, req)
	testkit.Check(t, err)
	testkit.Equal(t, "the open pull request is kept", again.Number, 1)
	testkit.Equal(t, "created once", strings.Count(gh.Calls(), "pr create"), 1)
}

func TestViewReportsAMergeAndItsCommit(t *testing.T) {
	gh := prtest.New(t)
	gh.Merge("develop", "main", "abc123")
	pr, err := pullrequest.View(context.Background(), t.TempDir(), "develop")
	testkit.Check(t, err)
	testkit.Equal(t, "merged", pr.Merged(), true)
	testkit.Equal(t, "merge commit", pr.MergeCommit, "abc123")
	testkit.Equal(t, "base", pr.Base, "main")
}

func TestOpenProposesAgainAfterAClosedOrDifferentlyBasedPullRequest(t *testing.T) {
	gh := prtest.New(t)
	ctx, dir := context.Background(), t.TempDir()
	gh.Set("x/main", `{"url":"u","number":7,"state":"CLOSED","baseRefName":"main","mergeCommit":null}`)
	_, err := pullrequest.Open(ctx, pullrequest.Request{Dir: dir, Base: "main", Head: "x/main", Title: "t", Body: "b"})
	testkit.Check(t, err)
	gh.Set("y/main", `{"url":"u","number":8,"state":"OPEN","baseRefName":"develop","mergeCommit":null}`)
	_, err = pullrequest.Open(ctx, pullrequest.Request{Dir: dir, Base: "main", Head: "y/main", Title: "t", Body: "b"})
	testkit.Check(t, err)
	testkit.Equal(t, "both proposed", strings.Count(gh.Calls(), "pr create"), 2)
}

func TestFailuresOfGhAreReported(t *testing.T) {
	gh := prtest.New(t)
	ctx, dir := context.Background(), t.TempDir()
	gh.Set("bad/main", "not json")
	if _, err := pullrequest.View(ctx, dir, "bad/main"); err == nil || !strings.Contains(err.Error(), "unreadable answer") {
		t.Fatalf("unreadable JSON: %v", err)
	}
	gh.Break()
	if _, err := pullrequest.View(ctx, dir, "x"); err == nil || errors.Is(err, pullrequest.ErrNone) || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("a failing gh is not 'no pull request': %v", err)
	}
	if _, err := pullrequest.Open(ctx, pullrequest.Request{Dir: dir, Base: "main", Head: "x"}); err == nil {
		t.Fatal("Open must report a failing gh")
	}
}

func TestPushAndOpenPushesTheBranchThenProposesIt(t *testing.T) {
	gh := prtest.New(t)
	origin, dir := t.TempDir(), t.TempDir()
	testkit.Git(t, origin, "init", "-q", "--bare", "-b", "main")
	testkit.GitInit(t, dir, "main")
	testkit.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	testkit.Git(t, dir, "remote", "add", "origin", origin)
	testkit.Git(t, dir, "checkout", "-q", "-b", "DEN-1-fix/main")
	req := pullrequest.Request{Dir: dir, Remote: "origin", Base: "main", Head: "DEN-1-fix/main", Title: "[DEN-1] Fix", Body: "b"}

	url, err := pullrequest.PushAndOpen(context.Background(), req)
	testkit.Check(t, err)
	testkit.Equal(t, "url", url, "https://github.example/o/r/pull/1")
	testkit.Equal(t, "pushed", testkit.Git(t, dir, "rev-parse", "origin/DEN-1-fix/main"), testkit.Git(t, dir, "rev-parse", "HEAD"))
	testkit.Contains(t, gh.Calls(), "pr create --base main --head DEN-1-fix/main")

	req.Remote = "nowhere"
	if _, err := pullrequest.PushAndOpen(context.Background(), req); err == nil || !strings.Contains(err.Error(), "git push nowhere") {
		t.Fatalf("a failed push must be reported, got %v", err)
	}
}
