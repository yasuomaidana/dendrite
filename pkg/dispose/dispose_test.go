package dispose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/pullrequest/prtest"
	"Dendrite/pkg/repair"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/workcopy"
)

// project is a repository on main with a bare origin that has main, and a local gh.
func project(t *testing.T) (dir string, gh *prtest.GitHub) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	gh = prtest.New(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	testkit.Git(t, filepath.Dir(origin), "init", "-q", "--bare", "-b", "main", origin)
	dir = workcopy.Canon(t.TempDir())
	testkit.GitInit(t, dir, "main")
	testkit.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	testkit.Git(t, dir, "remote", "add", "origin", origin)
	testkit.Git(t, dir, "push", "-q", "-u", "origin", "main")
	return dir, gh
}

// branch makes a branch from base with one commit of its own, pushes it and returns to main.
func branch(t *testing.T, dir, name, base string) {
	t.Helper()
	testkit.Git(t, dir, "checkout", "-q", "-b", name, base)
	testkit.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "work on "+name)
	testkit.Git(t, dir, "push", "-q", "origin", name)
	testkit.Git(t, dir, "checkout", "-q", "main")
}

// land merges ref into main with a merge commit, here and on origin, as a merged pull request does.
func land(t *testing.T, dir, ref string) {
	t.Helper()
	testkit.Git(t, dir, "merge", "-q", "--no-ff", "-m", "merge "+ref, ref)
	testkit.Git(t, dir, "push", "-q", "origin", "main")
}

func named(t *testing.T, items []Item, name string) Item {
	t.Helper()
	for _, it := range items {
		if it.Name == name {
			return it
		}
	}
	t.Fatalf("no item %q in %+v", name, items)
	return Item{}
}

func branches(t *testing.T, dir string) string {
	t.Helper()
	return testkit.Git(t, dir, "branch", "-a", "--format=%(refname:short)")
}

func TestAFeatureWhoseTrunkIsMergedGoesWithEveryBranchOfItsFolder(t *testing.T) {
	dir, _ := project(t)
	ctx := context.Background()
	branch(t, dir, "DEN-1-done/main", "main")
	branch(t, dir, "DEN-1-done/DEN-2-part", "main")
	testkit.Git(t, dir, "branch", "-f", "DEN-1-done/main", "DEN-1-done/DEN-2-part") // the trunk took the part in
	branch(t, dir, "DEN-3-open/main", "main")
	land(t, dir, "DEN-1-done/main")

	testkit.Git(t, dir, "branch", "DEN-4-new/main", "main") // a trunk nothing was committed on yet
	items := Features(ctx, Options{Dir: dir, Offline: true})
	testkit.Equal(t, "only the merged feature", len(items), 1)
	it := items[0]
	testkit.Equal(t, "name", it.Name, "DEN-1-done")
	testkit.Equal(t, "why", it.Why, "merged into main")
	testkit.Equal(t, "disposable", it.Disposable(), true)
	testkit.Check(t, it.Delete(ctx))
	left := branches(t, dir)
	testkit.NotContains(t, left, "DEN-1-done")
	testkit.Contains(t, left, "DEN-3-open/main", "origin/DEN-3-open/main")
}

func TestAFeatureStaysWhileABranchHasUnmergedWorkOrIsCheckedOut(t *testing.T) {
	dir, _ := project(t)
	ctx := context.Background()
	branch(t, dir, "DEN-1-done/main", "main")
	land(t, dir, "DEN-1-done/main")
	branch(t, dir, "DEN-1-done/DEN-2-late", "main")

	it := Features(ctx, Options{Dir: dir, Offline: true})[0]
	testkit.Contains(t, it.Kept, "DEN-1-done/DEN-2-late holds work its trunk does not have")
	if err := it.Delete(ctx); err == nil || !strings.Contains(err.Error(), "is kept") {
		t.Fatalf("a kept item must not be deleted: %v", err)
	}
	testkit.Git(t, dir, "branch", "-D", "DEN-1-done/DEN-2-late")

	copy := filepath.Join(t.TempDir(), "copy")
	testkit.Git(t, dir, "worktree", "add", "-q", copy, "DEN-1-done/main")
	it = Features(ctx, Options{Dir: dir, Offline: true})[0]
	testkit.Contains(t, it.Kept, "DEN-1-done/main is checked out in ")
}

func TestASquashMergedFeatureIsFoundThroughItsPullRequestWhenOnline(t *testing.T) {
	dir, gh := project(t)
	ctx := context.Background()
	branch(t, dir, "DEN-1-squashed/main", "main")
	gh.Merge("DEN-1-squashed/main", "main", "abc")
	branch(t, dir, "DEN-2-elsewhere/main", "main")
	gh.Merge("DEN-2-elsewhere/main", "release", "abc")

	testkit.Equal(t, "offline sees nothing", len(Features(ctx, Options{Dir: dir, Offline: true})), 0)
	items := Features(ctx, Options{Dir: dir})
	testkit.Equal(t, "one", len(items), 1)
	testkit.Equal(t, "why", items[0].Why, "its pull request into main was merged")
}

func TestARetiredDevelopGoesOnceNothingUsesIt(t *testing.T) {
	dir, _ := project(t)
	ctx := context.Background()
	o := Options{Dir: dir, Offline: true}
	testkit.Equal(t, "no develop", len(Retired(ctx, o)), 0)
	branch(t, dir, "develop", "main")
	testkit.Equal(t, "develop is ahead of main", len(Retired(ctx, o)), 0)
	land(t, dir, "develop")

	testkit.Git(t, dir, "branch", "agent/den-9", "main")
	testkit.Contains(t, Retired(ctx, o)[0].Kept, "agent/den-9 still merges into it")
	testkit.Git(t, dir, "branch", "-D", "agent/den-9")

	o.Tracked = func(b string) string { return "synapse eco still tracks " + b }
	testkit.Equal(t, "tracked", Retired(ctx, o)[0].Kept, "synapse eco still tracks develop")
	o.Tracked = func(string) string { return "" }

	testkit.Git(t, dir, "checkout", "-q", "develop")
	testkit.Contains(t, Retired(ctx, o)[0].Kept, "it is checked out in ")
	testkit.Git(t, dir, "checkout", "-q", "main")

	it := Retired(ctx, o)[0]
	testkit.Equal(t, "disposable", it.Disposable(), true)
	testkit.Check(t, it.Delete(ctx))
	testkit.NotContains(t, branches(t, dir), "develop")

	// A project that integrates on develop has nothing to retire.
	only := workcopy.Canon(t.TempDir())
	testkit.GitInit(t, only, "develop")
	testkit.Git(t, only, "commit", "-q", "--allow-empty", "-m", "base")
	testkit.Equal(t, "develop is the integration branch", len(Retired(ctx, Options{Dir: only, Offline: true})), 0)
}

func TestDeletingABranchReportsWhatGitRefuses(t *testing.T) {
	dir, _ := project(t)
	ctx := context.Background()
	branch(t, dir, "DEN-1-done/main", "main")
	land(t, dir, "DEN-1-done/main")
	it := Features(ctx, Options{Dir: dir, Offline: true})[0]

	testkit.Git(t, dir, "remote", "set-url", "origin", filepath.Join(dir, "gone.git"))
	if err := it.Delete(ctx); err == nil {
		t.Fatal("a remote that cannot be reached")
	}
	testkit.Git(t, dir, "update-ref", "-d", "refs/remotes/origin/DEN-1-done/main")
	testkit.Git(t, dir, "checkout", "-q", "DEN-1-done/main")
	if err := it.Delete(ctx); err == nil {
		t.Fatal("a branch that is checked out cannot be deleted")
	}
}

// merged is a ledger with one repair of r1 for a consumer, already merged; its copy is a clone of upstream.
func merged(t *testing.T) (ledger *repair.Ledger, link repair.Link, consumer string) {
	t.Helper()
	consumer, _ = project(t)
	t.Setenv(workcopy.EnvWorkDir, t.TempDir())
	upstream := filepath.Join(t.TempDir(), "r1.git")
	testkit.Git(t, consumer, "clone", "-q", "--bare", ".", upstream)
	testkit.WriteFile(t, filepath.Join(consumer, "Cargo.toml"), "[dependencies]\nr1 = \"1.0\"\n", 0o644)
	testkit.Git(t, consumer, "checkout", "-q", "-b", "DEN-890-fix/main")
	ledger = &repair.Ledger{Path: filepath.Join(t.TempDir(), "repairs.json")}
	res, err := repair.Start(context.Background(), ledger, repair.StartOptions{Consumer: consumer, Dependency: "r1", Ticket: "DEN-890", Upstream: upstream})
	testkit.Check(t, err)
	link = res.Link
	link.Stage = repair.StageMerged
	testkit.Check(t, ledger.Put(link))
	return ledger, link, consumer
}

func TestAMergedRepairLeavesItsCopyAndItsRemoteBranchToDispose(t *testing.T) {
	ledger, link, _ := merged(t)
	ctx := context.Background()

	items, err := Repairs(ctx, ledger, Options{})
	testkit.Check(t, err)
	testkit.Equal(t, "copy and branch", len(items), 2)
	copy := named(t, items, link.Copy)
	testkit.Equal(t, "kind", copy.Kind, KindRepairCopy)
	testkit.Equal(t, "why", copy.Why, "repair DEN-890/r1 is merged")
	remote := named(t, items, link.Branch+" on "+link.Remote)
	testkit.Equal(t, "kind", remote.Kind, KindRepairBranch)

	groups := Grouped(items)
	testkit.Equal(t, "two groups", len(groups), 2)
	testkit.Equal(t, "copies first", groups[0].Kind, KindRepairCopy)

	testkit.Check(t, remote.Delete(ctx))
	testkit.Equal(t, "gone from the remote", testkit.Git(t, link.Remote, "branch", "--list", link.Branch), "")
	testkit.Check(t, copy.Delete(ctx))
	if _, err := os.Stat(link.Copy); !os.IsNotExist(err) {
		t.Fatalf("the copy is still there: %v", err)
	}

	// Nothing is left: a merged link stays in the ledger, a released one is forgotten.
	items, err = Repairs(ctx, ledger, Options{})
	testkit.Check(t, err)
	testkit.Equal(t, "nothing left", len(items), 0)
	_, err = ledger.Get(link.ID)
	testkit.Check(t, err)
	link.Stage = repair.StageReleased
	testkit.Check(t, ledger.Put(link))
	_, err = Repairs(ctx, ledger, Options{})
	testkit.Check(t, err)
	if _, err := ledger.Get(link.ID); err == nil {
		t.Fatal("a released repair that left nothing behind is forgotten")
	}
}

func TestARepairCopyWithUnsavedWorkIsKeptAndARepairInProgressIsNotListed(t *testing.T) {
	ledger, link, _ := merged(t)
	ctx := context.Background()

	testkit.WriteFile(t, filepath.Join(link.Copy, "wip.txt"), "x", 0o644)
	items, _ := Repairs(ctx, ledger, Options{Offline: true})
	testkit.Equal(t, "dirty", named(t, items, link.Copy).Kept, "uncommitted changes")
	kept := Grouped(items)[0]
	testkit.Equal(t, "the dirty copy is in the kept half of its group", len(kept.Kept)*10+len(kept.Disposable), 10)
	testkit.Equal(t, "offline knows the pushed branch", len(items), 2)

	testkit.Git(t, link.Copy, "add", "-A")
	testkit.Git(t, link.Copy, "commit", "-q", "-m", "local only")
	items, _ = Repairs(ctx, ledger, Options{Offline: true})
	testkit.Equal(t, "unpushed", named(t, items, link.Copy).Kept, "1 commit(s) no remote has")

	testkit.Check(t, os.RemoveAll(filepath.Join(link.Copy, ".git")))
	items, _ = Repairs(ctx, ledger, Options{Offline: true})
	testkit.Equal(t, "unreadable", named(t, items, link.Copy).Kept, "its state cannot be read")
	testkit.Equal(t, "offline without a readable copy offers no remote branch", len(items), 1)

	link.Stage = repair.StageRepairing
	testkit.Check(t, ledger.Put(link))
	items, _ = Repairs(ctx, ledger, Options{})
	testkit.Equal(t, "in progress", len(items), 0)
}

func TestARepairMadeInAWorktreeIsRemovedThroughItsRepository(t *testing.T) {
	consumer, _ := project(t)
	t.Setenv(workcopy.EnvWorkDir, t.TempDir())
	ctx := context.Background()
	upstream := filepath.Join(t.TempDir(), "r1.git")
	testkit.Git(t, consumer, "clone", "-q", "--bare", ".", upstream)
	checkout := workcopy.Canon(filepath.Join(t.TempDir(), "r1"))
	testkit.Git(t, consumer, "clone", "-q", upstream, checkout)
	testkit.WriteFile(t, filepath.Join(consumer, "Cargo.toml"), "[dependencies]\nr1 = \"1.0\"\n", 0o644)
	ledger := &repair.Ledger{Path: filepath.Join(t.TempDir(), "repairs.json")}
	res, err := repair.Start(ctx, ledger, repair.StartOptions{Consumer: consumer, Dependency: "r1", Ticket: "DEN-890", DepTicket: "R1-3", Checkout: checkout})
	testkit.Check(t, err)
	link := res.Link
	link.Stage = repair.StageReleased
	testkit.Check(t, ledger.Put(link))

	items, err := Find(ctx, ledger, Options{Dir: consumer})
	testkit.Check(t, err)
	testkit.Check(t, named(t, items, link.Copy).Delete(ctx))
	testkit.NotContains(t, testkit.Git(t, checkout, "worktree", "list"), filepath.Base(link.Copy))

	// With the copy gone, the remote branch is deleted from the consumer; with both gone there is nowhere to push from.
	items, _ = Repairs(ctx, ledger, Options{})
	testkit.Equal(t, "the remote branch is still offered", len(items), 1)
	testkit.Check(t, os.Rename(consumer, consumer+"-moved"))
	items, _ = Repairs(ctx, ledger, Options{})
	testkit.Equal(t, "nowhere to push from", len(items), 0)
}

func TestFindReportsAnUnreadableLedger(t *testing.T) {
	dir, _ := project(t)
	ledger := &repair.Ledger{Path: filepath.Join(t.TempDir(), "repairs.json")}
	testkit.WriteFile(t, ledger.Path, "not json", 0o644)
	if _, err := Find(context.Background(), ledger, Options{Dir: dir}); err == nil {
		t.Fatal("an unreadable ledger")
	}
}
