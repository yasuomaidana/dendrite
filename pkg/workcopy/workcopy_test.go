package workcopy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func reportFor(t *testing.T, reports []Report, path string) Report {
	t.Helper()
	for _, r := range reports {
		if r.Path == Canon(path) {
			return r
		}
	}
	t.Fatalf("no report for %s in %+v", path, reports)
	return Report{}
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
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "Proj-DEN-1-fix"); path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	if _, err := m.Create(ctx, "DEN-1 fix", "", ""); err == nil {
		t.Fatal("expected duplicate create to fail")
	}

	copies, err := m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(copies) != 1 || copies[0].Branch != "agent/den-1-fix" || !copies[0].Untouched || copies[0].Dirty {
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
	git(t, repo, "merge", "-q", "--no-edit", "agent/merged")

	untouched, _ := m.Create(ctx, "untouched", "", "")

	dirty, _ := m.Create(ctx, "dirty", "", "")
	commitIn(t, dirty, "dirty.txt")
	git(t, repo, "merge", "-q", "--no-edit", "agent/dirty")
	writeFile(t, filepath.Join(dirty, "wip.txt"), "wip\n")

	unmerged, _ := m.Create(ctx, "unmerged", "", "")
	commitIn(t, unmerged, "unmerged.txt")

	busy, _ := m.Create(ctx, "busy", "", "")
	commitIn(t, busy, "busy.txt")
	git(t, repo, "merge", "-q", "--no-edit", "agent/busy")

	stateOnly, _ := m.Create(ctx, "state", "", "")
	commitIn(t, stateOnly, "state.txt")
	git(t, repo, "merge", "-q", "--no-edit", "agent/state")
	if err := os.MkdirAll(filepath.Join(stateOnly, ".dendrite", "jobs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stateOnly, ".dendrite", "jobs", "x.json"), "{}")

	inUse := func(p string) bool { return p == Canon(busy) }

	dry, err := m.GC(ctx, GCOptions{DryRun: true, InUse: inUse})
	if err != nil {
		t.Fatal(err)
	}
	if r := reportFor(t, dry, merged); r.Action != "would remove" {
		t.Fatalf("dry run: %+v", r)
	}
	if !dirExists(merged) {
		t.Fatal("dry run must not delete")
	}

	reports, err := m.GC(ctx, GCOptions{InUse: inUse})
	if err != nil {
		t.Fatal(err)
	}
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
	if out := git(t, repo, "branch", "--list", "agent/merged"); out != "" {
		t.Errorf("merged branch not deleted: %q", out)
	}
	if out := git(t, repo, "branch", "--list", "agent/unmerged"); out == "" {
		t.Error("unmerged branch must be kept")
	}
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
	git(t, repo, "merge", "-q", "--no-edit", "agent/here")

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
	git(t, gone, "push", "-q", "-u", "origin", "agent/gone")
	git(t, repo, "push", "-q", "origin", "--delete", "agent/gone")
	git(t, repo, "fetch", "-q", "--prune", "origin")

	ahead, _ := m.Create(ctx, "ahead", "", "")
	commitIn(t, ahead, "a1.txt")
	git(t, ahead, "push", "-q", "-u", "origin", "agent/ahead")
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
	if !strings.Contains(r.Reason, "branch agent/work kept") || dirExists(work) {
		t.Fatalf("unexpected: %+v", r)
	}
	if out := git(t, repo, "branch", "--list", "agent/work"); out == "" {
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
