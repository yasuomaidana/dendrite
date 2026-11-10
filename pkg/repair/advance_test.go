package repair

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/pullrequest/prtest"
	"Dendrite/pkg/testkit"
)

// chain is a world with the repairs A -> r1 -> r2 started (branches R1-3-fix-login-crash/main and
// R2-7-fix-login-crash/main) and a local gh.
type chain struct {
	*world
	gh       *prtest.GitHub
	one, two Link
}

func newChain(t *testing.T) *chain {
	t.Helper()
	c := &chain{world: newWorld(t), gh: prtest.New(t)}
	c.one = c.start(t, StartOptions{Consumer: c.consumer, Dependency: "r1", Ticket: "DEN-890", DepTicket: "R1-3", Upstream: c.r1}).Link
	c.two = c.start(t, StartOptions{Consumer: c.one.Copy, Dependency: "r2", Ticket: "DEN-890", DepTicket: "R2-7", Upstream: c.r2}).Link
	return c
}

// merge commits a fix on the link's branch, merges it into the upstream's main and tells gh; it returns the
// merge commit.
func (c *chain) merge(t *testing.T, k Link) string {
	t.Helper()
	testkit.WriteFile(t, filepath.Join(k.Copy, "fix.txt"), "fixed\n", 0o644)
	testkit.Git(t, k.Copy, "add", "-A")
	testkit.Git(t, k.Copy, "commit", "-q", "-m", "fix")
	testkit.Git(t, k.Copy, "push", "-q", "origin", k.Branch)
	testkit.Git(t, k.Copy, "push", "-q", "origin", k.Branch+":main")
	sha := testkit.Git(t, k.Copy, "rev-parse", "HEAD")
	c.gh.Merge(k.Branch, "main", sha)
	return sha
}

func (c *chain) advance(t *testing.T) map[string]Step {
	t.Helper()
	steps, err := Advance(context.Background(), c.ledger)
	testkit.Check(t, err)
	out := map[string]Step{}
	for _, s := range steps {
		out[s.ID] = s
	}
	return out
}

func TestTreeAndLeafFirstFollowTheChain(t *testing.T) {
	c := newChain(t)
	other := Link{ID: "DEN-9/x", Parent: "gone"} // its parent was dropped: it starts a chain of its own
	rows := Tree([]Link{c.one, c.two, other})
	testkit.Equal(t, "rows", len(rows), 3)
	testkit.Equal(t, "root", rows[0].Link.ID+" "+rows[1].Link.ID, "DEN-890/r1 DEN-890/r2")
	testkit.Equal(t, "depths", rows[0].Depth*10+rows[1].Depth, 1)
	testkit.Equal(t, "orphan is a root", rows[2].Depth, 0)
	order := LeafFirst([]Link{c.one, c.two, other})
	testkit.Equal(t, "leaf first", order[0].ID, "DEN-890/r2")
	testkit.Equal(t, "nothing", len(LeafFirst(nil)), 0)
}

func TestProposeOpensPullRequestsLeafFirstAndLinksThem(t *testing.T) {
	c := newChain(t)
	opened, err := Propose(context.Background(), c.ledger)
	testkit.Check(t, err)
	testkit.Equal(t, "both", len(opened), 2)
	testkit.Equal(t, "leaf first", opened[0].ID, "DEN-890/r2")
	testkit.Contains(t, c.gh.Calls(), "--base main --head R2-7-fix-login-crash/main --title [R2-7] Repair for DEN-890: r2", "Waits for the repair of `r2`: https://github.example/o/r/pull/1")
	stored, _ := c.ledger.Get("DEN-890/r1")
	testkit.Equal(t, "url stored", stored.PullRequest, "https://github.example/o/r/pull/2")

	again, err := Propose(context.Background(), c.ledger)
	testkit.Check(t, err)
	testkit.Equal(t, "nothing to open twice", len(again), 0)
}

func TestProposeStopsAtAPullRequestItCannotOpen(t *testing.T) {
	c := newChain(t)
	c.gh.Break()
	if _, err := Propose(context.Background(), c.ledger); err == nil || !strings.Contains(err.Error(), "repair DEN-890/r2") {
		t.Fatalf("a failing gh: %v", err)
	}
	testkit.WriteFile(t, c.ledger.Path, "not json", 0o644)
	if _, err := Propose(context.Background(), c.ledger); err == nil {
		t.Fatal("an unreadable ledger")
	}
	if _, err := Advance(context.Background(), c.ledger); err == nil {
		t.Fatal("an unreadable ledger")
	}
}

func TestAdvanceWalksAChainThroughMergeAndRelease(t *testing.T) {
	c := newChain(t)
	ctx := context.Background()

	steps := c.advance(t)
	testkit.Contains(t, steps["DEN-890/r2"].Note, "no pull request yet")
	testkit.Contains(t, steps["DEN-890/r1"].Note, "waits for DEN-890/r2 to be merged")
	_, err := Propose(ctx, c.ledger)
	testkit.Check(t, err)
	testkit.Contains(t, c.advance(t)["DEN-890/r2"].Note, "is not merged yet: https://github.example/o/r/pull/1")

	// r2 is merged: r1's copy is pinned to r2 at the merge commit; r1 itself has to wait for its own merge.
	sha2 := c.merge(t, c.two)
	steps = c.advance(t)
	testkit.Equal(t, "r2 merged", steps["DEN-890/r2"].To, StageMerged)
	testkit.Equal(t, "it moved", steps["DEN-890/r2"].Moved(), true)
	testkit.Contains(t, manifest(t, c.one.Copy), `r2 = { git = "`+c.r2+`", rev = "`+sha2+`" }`)
	testkit.Equal(t, "r1 still repairing", steps["DEN-890/r1"].To, StageRepairing)

	// r1 is merged too: the consumer is pinned to r1; no release yet, so nothing else moves.
	testkit.Git(t, c.one.Copy, "add", "-A")
	sha1 := c.merge(t, c.one)
	steps = c.advance(t)
	testkit.Equal(t, "r1 merged", steps["DEN-890/r1"].To, StageMerged)
	testkit.Contains(t, manifest(t, c.consumer), `rev = "`+sha1+`"`)
	testkit.Contains(t, steps["DEN-890/r2"].Note, "no release holds")

	// r2 releases v0.3.1 and r1 v1.0.1 (plus a later one): each consumer goes back to a published version.
	testkit.Git(t, c.two.Copy, "tag", "v0.3.1")
	testkit.Git(t, c.two.Copy, "push", "-q", "origin", "v0.3.1")
	testkit.Git(t, c.one.Copy, "tag", "v1.0.1")
	testkit.Git(t, c.one.Copy, "tag", "v1.1.0")
	testkit.Git(t, c.one.Copy, "push", "-q", "origin", "--tags")
	testkit.Check(t, os.RemoveAll(c.one.Copy)) // r1's copy was disposed of: the release is looked up in the upstream
	steps = c.advance(t)
	testkit.Contains(t, steps["DEN-890/r2"].Note, "the consumer "+c.one.Copy+" is gone")
	testkit.Equal(t, "r1 released", steps["DEN-890/r1"].To, StageReleased)
	testkit.Contains(t, manifest(t, c.consumer), `r1 = "1.0.1"`)
	stored, _ := c.ledger.Get("DEN-890/r1")
	testkit.Equal(t, "version", stored.Version, "v1.0.1")
	testkit.Equal(t, "merge commit", stored.MergeSHA, sha1)

	testkit.Equal(t, "a released link is left alone", len(c.advance(t)), 1)
}

func TestAReleaseRestoresADependencyThatWasNotPublishedExactly(t *testing.T) {
	c := &chain{world: newWorld(t), gh: prtest.New(t)}
	was := `r1 = { git = "` + c.r1 + `", branch = "main" }`
	testkit.WriteFile(t, filepath.Join(c.consumer, "Cargo.toml"), "[dependencies]\n"+was+"\n", 0o644)
	link := c.start(t, StartOptions{Consumer: c.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: c.r1}).Link
	c.merge(t, link)
	testkit.Equal(t, "merged", c.advance(t)[link.ID].To, StageMerged)
	testkit.Git(t, link.Copy, "tag", "v2.0.0")
	testkit.Git(t, link.Copy, "push", "-q", "origin", "v2.0.0")
	testkit.Check(t, os.Chmod(filepath.Join(c.consumer, "Cargo.toml"), 0o400))
	testkit.Contains(t, c.advance(t)[link.ID].Note, "restoring r1 in Cargo.toml")
	testkit.Check(t, os.Chmod(filepath.Join(c.consumer, "Cargo.toml"), 0o644))
	testkit.Equal(t, "released", c.advance(t)[link.ID].To, StageReleased)
	testkit.Contains(t, manifest(t, c.consumer), was)
	testkit.Equal(t, "go keeps the v", versionFor("go.mod", "v2.0.0")+" "+versionFor("Cargo.toml", "v2.0.0"), "v2.0.0 2.0.0")
}

func TestAdvanceReportsWhatStopsALink(t *testing.T) {
	c := &chain{world: newWorld(t), gh: prtest.New(t)}
	link := c.start(t, StartOptions{Consumer: c.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: c.r1}).Link

	c.gh.Set(link.Branch, `{"url":"u","number":1,"state":"MERGED","baseRefName":"main","mergeCommit":null}`)
	testkit.Contains(t, c.advance(t)[link.ID].Note, "reports no merge commit")

	sha := c.merge(t, link)
	testkit.Check(t, os.Chmod(filepath.Join(c.consumer, "Cargo.toml"), 0o400))
	testkit.Contains(t, c.advance(t)[link.ID].Note, "pinning r1 to "+sha)
	testkit.Check(t, os.Chmod(filepath.Join(c.consumer, "Cargo.toml"), 0o644))
	testkit.Equal(t, "merged", c.advance(t)[link.ID].To, StageMerged)

	// The release cannot be looked up, then cannot be written into the manifest.
	testkit.Git(t, link.Copy, "remote", "set-url", "origin", filepath.Join(c.root, "gone.git"))
	testkit.Contains(t, c.advance(t)[link.ID].Note, "looking for a release of r1")
	testkit.Git(t, link.Copy, "remote", "set-url", "origin", c.r1)
	testkit.Git(t, link.Copy, "tag", "v1.0.1")
	testkit.Git(t, link.Copy, "push", "-q", "origin", "v1.0.1")
	testkit.Check(t, os.Chmod(filepath.Join(c.consumer, "Cargo.toml"), 0o400))
	testkit.Contains(t, c.advance(t)[link.ID].Note, "moving r1 to v1.0.1")
	testkit.Check(t, os.Chmod(filepath.Join(c.consumer, "Cargo.toml"), 0o644))

	c.gh.Break()
	fresh := c.start(t, StartOptions{Consumer: c.consumer, Dependency: "r1", Ticket: "DEN-891", Upstream: c.r1}).Link
	testkit.Contains(t, c.advance(t)[fresh.ID].Note, "not logged in")
}

func TestAReleaseThatCannotBeLookedUpLeavesTheLinkMerged(t *testing.T) {
	c := &chain{world: newWorld(t), gh: prtest.New(t)}
	link := c.start(t, StartOptions{Consumer: c.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: c.r1}).Link
	c.merge(t, link)
	testkit.Equal(t, "merged", c.advance(t)[link.ID].To, StageMerged)
	merged, _ := c.ledger.Get(link.ID)

	bogus := merged
	bogus.MergeSHA = "not-a-commit"
	testkit.Check(t, c.ledger.Put(bogus))
	testkit.Contains(t, c.advance(t)[link.ID].Note, "looking for a release of r1")

	gone := merged
	gone.Copy, gone.Upstream = filepath.Join(c.root, "removed"), filepath.Join(c.root, "gone.git")
	testkit.Check(t, c.ledger.Put(gone))
	step := c.advance(t)[link.ID]
	testkit.Contains(t, step.Note, "looking for a release of r1")
	testkit.Equal(t, "still merged", step.To, StageMerged)
}

func TestProposeForAForkTargetsTheUpstreamsDefaultBranch(t *testing.T) {
	c := &chain{world: newWorld(t), gh: prtest.New(t)}
	fork := filepath.Join(t.TempDir(), "fork.git")
	testkit.Git(t, filepath.Dir(fork), "clone", "-q", "--bare", c.r1, fork)
	c.start(t, StartOptions{Consumer: c.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: c.r1, Fork: fork})
	opened, err := Propose(context.Background(), c.ledger)
	testkit.Check(t, err)
	testkit.Equal(t, "one", len(opened), 1)
	testkit.Contains(t, c.gh.Calls(), "pr create --base main --head DEN-890-fix-login-crash/main --title [DEN-890] Repair for DEN-890: r1")
}

func TestAdvanceQuietlyPrintsOnlyWhatMovedAndWarnsInsteadOfFailing(t *testing.T) {
	c := &chain{world: newWorld(t), gh: prtest.New(t)}
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	c.ledger = &Ledger{Path: filepath.Join(home, "repairs.json")}
	link := c.start(t, StartOptions{Consumer: c.consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: c.r1}).Link
	var out strings.Builder

	AdvanceQuietly(context.Background(), &out)
	testkit.Equal(t, "a link that waits prints nothing", out.String(), "")
	testkit.Contains(t, Step{ID: "a/b", From: StageRepairing, To: StageRepairing, Note: "waits"}.Line(), "[=] a/b: repairing; waits")

	c.merge(t, link)
	AdvanceQuietly(context.Background(), &out)
	testkit.Contains(t, out.String(), "[✓] DEN-890/r1: repairing -> merged; merged as ")

	out.Reset()
	testkit.WriteFile(t, c.ledger.Path, "not json", 0o644)
	AdvanceQuietly(context.Background(), &out)
	testkit.Contains(t, out.String(), "[!] Repairs not advanced:")
}
