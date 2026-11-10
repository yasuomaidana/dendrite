package workcopy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/testkit"
)

// newRepo creates a real git repository at <tmp>/Proj with one commit on main,
// and points the working-copy root at <tmp>/work.
func newRepo(t *testing.T) (repo, root string) {
	t.Helper()
	base := Canon(t.TempDir())
	repo = filepath.Join(base, "Proj")
	root = filepath.Join(base, "work")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvWorkDir, root)
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	git(t, repo, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(repo, "README.md"), "hello\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	return repo, root
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitIn(t *testing.T, dir, file string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, file), file+"\n")
	git(t, dir, "add", file)
	git(t, dir, "commit", "-q", "-m", "add "+file)
}

// newManager returns a manager whose cwd is the main checkout.
func newManager(repo string) *Manager {
	m := New(repo)
	m.getwd = func() (string, error) { return repo, nil }
	return m
}

// byPath is the item of items whose path (as pathOf reads it) is the canonical form of path.
func byPath[T any](t *testing.T, items []T, path string, pathOf func(T) string) T {
	t.Helper()
	for _, it := range items {
		if pathOf(it) == Canon(path) {
			return it
		}
	}
	t.Fatalf("nothing at %s in %+v", path, items)
	var zero T
	return zero
}

func reportFor(t *testing.T, reports []Report, path string) Report {
	t.Helper()
	return byPath(t, reports, path, func(r Report) string { return r.Path })
}

func TestRootDefaultsToSiblingDir(t *testing.T) {
	repo, _ := newRepo(t)
	t.Setenv(EnvWorkDir, "")
	ctx := context.Background()
	m := newManager(repo)
	root, err := m.Root(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(filepath.Dir(repo), DirName); root != want {
		t.Fatalf("root = %s, want %s", root, want)
	}
	stories, _ := m.StoriesDir(ctx)
	if want := filepath.Join(root, "Proj-stories"); stories != want {
		t.Fatalf("stories = %s, want %s", stories, want)
	}
}

func TestCreateListAndRootFromInsideCopy(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)

	path, err := m.Create(ctx, "DEN-1 fix", "", "")
	testkit.Check(t, err)
	testkit.Equal(t, "path", path, filepath.Join(root, "Proj-DEN-1-fix"))
	if _, err := m.Create(ctx, "DEN-1 fix", "", ""); err == nil {
		t.Fatal("expected duplicate create to fail")
	}

	copies, err := m.List(ctx)
	testkit.Check(t, err)
	if len(copies) != 1 || copies[0].Branch != "DEN-1-fix/main" || !copies[0].Untouched || copies[0].Dirty {
		t.Fatalf("unexpected copies: %+v", copies)
	}

	// A manager rooted inside the copy resolves the same main checkout and root.
	inner := New(path)
	top, err := inner.MainTop(ctx)
	if err != nil || top != repo {
		t.Fatalf("MainTop from copy = %s (%v), want %s", top, err, repo)
	}
}

func TestGCRemovesOnlyMergedCleanCopies(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)

	merged, _ := m.Create(ctx, "merged", "", "")
	commitIn(t, merged, "merged.txt")
	git(t, repo, "merge", "-q", "--no-edit", "merged/main")

	untouched, _ := m.Create(ctx, "untouched", "", "")

	dirty, _ := m.Create(ctx, "dirty", "", "")
	commitIn(t, dirty, "dirty.txt")
	git(t, repo, "merge", "-q", "--no-edit", "dirty/main")
	writeFile(t, filepath.Join(dirty, "wip.txt"), "wip\n")

	unmerged, _ := m.Create(ctx, "unmerged", "", "")
	commitIn(t, unmerged, "unmerged.txt")

	busy, _ := m.Create(ctx, "busy", "", "")
	commitIn(t, busy, "busy.txt")
	git(t, repo, "merge", "-q", "--no-edit", "busy/main")

	stateOnly, _ := m.Create(ctx, "state", "", "")
	commitIn(t, stateOnly, "state.txt")
	git(t, repo, "merge", "-q", "--no-edit", "state/main")
	testkit.Check(t, os.MkdirAll(filepath.Join(stateOnly, ".dendrite", "jobs"), 0o755))
	writeFile(t, filepath.Join(stateOnly, ".dendrite", "jobs", "x.json"), "{}")

	inUse := func(p string) bool { return p == Canon(busy) }

	dry, err := m.GC(ctx, GCOptions{DryRun: true, InUse: inUse})
	testkit.Check(t, err)
	testkit.Equal(t, "dry run", reportFor(t, dry, merged).Action, "would remove")
	testkit.Equal(t, "dry run deletes nothing", dirExists(merged), true)

	reports, err := m.GC(ctx, GCOptions{InUse: inUse})
	testkit.Check(t, err)
	want := map[string]string{
		merged:    "removed",
		stateOnly: "removed",
		untouched: "kept",
		dirty:     "kept",
		unmerged:  "kept",
		busy:      "kept",
	}
	for p, action := range want {
		if r := reportFor(t, reports, p); r.Action != action {
			t.Errorf("%s: got %+v, want %s", filepath.Base(p), r, action)
		}
		if exists := dirExists(p); exists != (action == "kept") {
			t.Errorf("%s: exists=%v after gc", filepath.Base(p), exists)
		}
	}
	testkit.Equal(t, "merged branch deleted", git(t, repo, "branch", "--list", "merged/main"), "")
	testkit.Equal(t, "unmerged branch kept", git(t, repo, "branch", "--list", "unmerged/main") != "", true)
	if _, err := os.Stat(filepath.Join(root, metaDirName, "Proj-merged.json")); !os.IsNotExist(err) {
		t.Errorf("metadata of removed copy should be gone: %v", err)
	}
}

func TestGCKeepsCurrentDirAndStoryVariants(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)

	here, _ := m.Create(ctx, "here", "", "")
	commitIn(t, here, "here.txt")
	git(t, repo, "merge", "-q", "--no-edit", "here/main")

	stories, _ := m.StoriesDir(ctx)
	variant := filepath.Join(stories, "S1", "v1")
	git(t, repo, "worktree", "add", "-q", "-b", "story/S1/v1", variant)

	m.getwd = func() (string, error) { return filepath.Join(here, "sub"), nil }
	reports, err := m.GC(ctx, GCOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r := reportFor(t, reports, here); r.Action != "kept" || r.Reason != "current working directory" {
		t.Errorf("cwd copy: %+v", r)
	}
	if r := reportFor(t, reports, variant); r.Action != "kept" || !strings.Contains(r.Reason, "story") {
		t.Errorf("story variant: %+v", r)
	}
}

func TestGCUpstreamGoneAndUnpushed(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	remote := filepath.Join(filepath.Dir(repo), "remote.git")
	git(t, repo, "init", "-q", "--bare", remote)
	git(t, repo, "remote", "add", "origin", remote)
	m := newManager(repo)

	gone, _ := m.Create(ctx, "gone", "", "")
	commitIn(t, gone, "gone.txt")
	git(t, gone, "push", "-q", "-u", "origin", "gone/main")
	git(t, repo, "push", "-q", "origin", "--delete", "gone/main")
	git(t, repo, "fetch", "-q", "--prune", "origin")

	ahead, _ := m.Create(ctx, "ahead", "", "")
	commitIn(t, ahead, "a1.txt")
	git(t, ahead, "push", "-q", "-u", "origin", "ahead/main")
	commitIn(t, ahead, "a2.txt")

	reports, err := m.GC(ctx, GCOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r := reportFor(t, reports, gone); r.Action != "removed" || r.Reason != "upstream branch deleted" {
		t.Errorf("gone: %+v", r)
	}
	if r := reportFor(t, reports, ahead); r.Action != "kept" || !strings.Contains(r.Reason, "unpushed") {
		t.Errorf("ahead: %+v", r)
	}
}

func TestRemove(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)

	work, _ := m.Create(ctx, "work", "", "")
	commitIn(t, work, "w.txt")
	writeFile(t, filepath.Join(work, "README.md"), "changed\n")

	if _, err := m.Remove(ctx, "work", RemoveOptions{}); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("expected dirty refusal, got %v", err)
	}
	git(t, work, "checkout", "--", "README.md")

	r, err := m.Remove(ctx, "work", RemoveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Reason, "branch work/main kept") || dirExists(work) {
		t.Fatalf("unexpected: %+v", r)
	}
	if out := git(t, repo, "branch", "--list", "work/main"); out == "" {
		t.Error("unmerged branch must survive a non-forced remove")
	}

	forced, _ := m.Create(ctx, "forced", "", "")
	writeFile(t, filepath.Join(forced, "README.md"), "changed\n")
	if _, err := m.Remove(ctx, forced, RemoveOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if dirExists(forced) {
		t.Fatal("forced remove left the directory")
	}
	if _, err := m.Remove(ctx, "missing", RemoveOptions{}); err == nil {
		t.Fatal("expected error for unknown copy")
	}
}

func TestAdoptMovesOutsideWorktrees(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)

	sibling := filepath.Join(filepath.Dir(repo), "Proj-feature")
	git(t, repo, "worktree", "add", "-q", "-b", "feat/x", sibling)
	writeFile(t, filepath.Join(sibling, "wip.txt"), "uncommitted\n")
	other := filepath.Join(filepath.Dir(repo), "scratch")
	git(t, repo, "worktree", "add", "-q", "-b", "feat/y", other)

	dry, err := m.Adopt(ctx, GCOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := reportFor(t, dry, sibling); r.Action != "would move" {
		t.Fatalf("dry: %+v", r)
	}

	reports, err := m.Adopt(ctx, GCOptions{InUse: func(p string) bool { return p == Canon(other) }})
	if err != nil {
		t.Fatal(err)
	}
	if r := reportFor(t, reports, sibling); r.Action != "moved" {
		t.Fatalf("sibling: %+v", r)
	}
	if r := reportFor(t, reports, other); r.Action != "kept" {
		t.Fatalf("in-use: %+v", r)
	}
	moved := filepath.Join(root, "Proj-feature")
	if _, err := os.Stat(filepath.Join(moved, "wip.txt")); err != nil {
		t.Fatalf("uncommitted file lost in move: %v", err)
	}
	copies, _ := m.List(ctx)
	if len(copies) != 1 || copies[0].Path != moved || !copies[0].Dirty {
		t.Fatalf("adopted copy not listed: %+v", copies)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{"DEN-12": "DEN-12", " a/b c ": "a-b-c", "..": "copy", "": "copy"}
	for in, want := range cases {
		if got := SanitizeName(in); got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGCCountsAMergeIntoTheRecordedTargetAndOnlyThat(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)
	git(t, repo, "branch", "feature/DEN-1-x")

	child, _ := m.Create(ctx, "child", "", "feature/DEN-1-x")
	git(t, repo, "config", "branch.child/main.dendrite-target", "feature/DEN-1-x")
	commitIn(t, child, "child.txt")
	other, _ := m.Create(ctx, "other", "", "")
	commitIn(t, other, "other.txt")

	// Merged into main only: for the child that is not where its work goes (main holds nothing of it), and for
	// the unrecorded copy it is.
	git(t, repo, "merge", "-q", "--no-edit", "other/main")
	cs, err := m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c := copyFor(t, cs, other); c.MergedInto != "main" {
		t.Errorf("an unrecorded copy merged into main: %+v", c)
	}
	if c := copyFor(t, cs, child); c.MergedInto != "" {
		t.Errorf("the child is not merged anywhere yet: %+v", c)
	}

	// Merging the child into its feature branch, not into main, is what finishes it.
	git(t, repo, "checkout", "-q", "feature/DEN-1-x")
	git(t, repo, "merge", "-q", "--no-edit", "child/main")
	git(t, repo, "checkout", "-q", "main")
	cs, err = m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c := copyFor(t, cs, child); c.MergedInto != "feature/DEN-1-x" {
		t.Errorf("merged into the recorded feature branch: %+v", c)
	}
}

func copyFor(t *testing.T, cs []Copy, path string) Copy {
	t.Helper()
	return byPath(t, cs, path, func(c Copy) string { return c.Path })
}

func TestCreateChecksOutABranchThatAlreadyExists(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)
	git(t, repo, "branch", "DEN-880-naming/main")

	path, err := m.Create(ctx, "DEN-880-naming", "DEN-880-naming/main", "no-such-ref")
	if err != nil {
		t.Fatalf("an existing trunk needs no base: %v", err)
	}
	if got := git(t, path, "symbolic-ref", "--short", "HEAD"); got != "DEN-880-naming/main" {
		t.Fatalf("branch = %s", got)
	}
	if c, err := m.Resolve(ctx, path); err != nil || !c.Untouched {
		t.Fatalf("a copy of an existing branch starts untouched: %+v, %v", c, err)
	}
}

func TestResolveFindsATicketCopyByItsTicketAlone(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)
	slugged, _ := m.Create(ctx, "DEN-88-slug-names", "", "")
	if _, err := m.Create(ctx, "DEN-880-other", "", ""); err != nil {
		t.Fatal(err)
	}

	c, err := m.Resolve(ctx, "DEN-88")
	if err != nil || c.Path != Canon(slugged) {
		t.Fatalf("DEN-88 = %+v, %v; DEN-880 is another ticket", c, err)
	}
	if _, err := m.Create(ctx, "DEN-88-second", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Resolve(ctx, "DEN-88"); err == nil || !strings.Contains(err.Error(), "matches 2") {
		t.Fatalf("two copies of one ticket are ambiguous, got %v", err)
	}
	if c, err := m.Resolve(ctx, "DEN-88-second"); err != nil || c.Branch != "DEN-88-second/main" {
		t.Fatalf("the full name still wins: %+v, %v", c, err)
	}
}
