package workcopy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failingGit runs real git but fails any invocation whose arguments contain match.
func failingGit(match string) GitRunner {
	return func(ctx context.Context, dir string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), match) {
			return "", errors.New("injected failure: " + match)
		}
		return RunGit(ctx, dir, args...)
	}
}

func TestNewDefaultsToCurrentDir(t *testing.T) {
	if m := New(""); m.repoDir != "." {
		t.Fatalf("repoDir = %q", m.repoDir)
	}
}

func TestOutsideRepoEverythingFails(t *testing.T) {
	ctx := context.Background()
	m := New(t.TempDir())
	if _, err := m.MainTop(ctx); err == nil {
		t.Error("MainTop")
	}
	if _, err := m.Root(ctx); err == nil {
		t.Error("Root")
	}
	if _, err := m.StoriesDir(ctx); err == nil {
		t.Error("StoriesDir")
	}
	if _, err := m.Create(ctx, "x", "", ""); err == nil {
		t.Error("Create")
	}
	if _, err := m.List(ctx); err == nil {
		t.Error("List")
	}
	if _, err := m.Remove(ctx, "x", RemoveOptions{}); err == nil {
		t.Error("Remove")
	}
	if _, err := m.GC(ctx, GCOptions{}); err == nil {
		t.Error("GC")
	}
	if _, err := m.Adopt(ctx, GCOptions{}); err == nil {
		t.Error("Adopt")
	}
}

func TestStoriesDirFor(t *testing.T) {
	repo, root := newRepo(t)
	if got := StoriesDirFor(repo, "fallback"); got != filepath.Join(root, "Proj-stories") {
		t.Errorf("with explicit root: %s", got)
	}
	if got := StoriesDirFor(t.TempDir(), "fallback"); got != "fallback" {
		t.Errorf("outside repo: %s", got)
	}
	t.Setenv(EnvWorkDir, "")
	if got := StoriesDirFor(repo, "fallback"); got != "fallback" {
		t.Errorf("test binary without explicit root must fall back: %s", got)
	}
	prev := isTestBinary
	isTestBinary = func() bool { return false }
	defer func() { isTestBinary = prev }()
	if got := StoriesDirFor(repo, "fallback"); got != filepath.Join(filepath.Dir(repo), DirName, "Proj-stories") {
		t.Errorf("default root: %s", got)
	}
}

func TestCreateErrors(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)

	if _, err := m.Create(ctx, "a", "", "no-such-ref"); err == nil || !strings.Contains(err.Error(), "resolve base") {
		t.Errorf("bad base: %v", err)
	}
	if _, err := m.Create(ctx, "a", "main", ""); err == nil {
		t.Error("existing branch must fail")
	}

	// Metadata cannot be written when .meta is a file.
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, metaDirName), "x")
	path, err := m.Create(ctx, "b", "", "")
	if err == nil || !strings.Contains(err.Error(), "metadata") || !dirExists(path) {
		t.Errorf("meta failure: %s %v", path, err)
	}

	// The root cannot be created under a file.
	blocker := filepath.Join(filepath.Dir(repo), "blocker")
	writeFile(t, blocker, "x")
	t.Setenv(EnvWorkDir, filepath.Join(blocker, "work"))
	if _, err := m.Create(ctx, "c", "", ""); err == nil {
		t.Error("root under a file must fail")
	}
}

func TestGitFailuresAreReported(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)
	merged, _ := m.Create(ctx, "merged", "", "")
	commitIn(t, merged, "m.txt")
	git(t, repo, "merge", "-q", "--no-edit", "agent/merged")

	m.git = failingGit("worktree list")
	if _, err := m.List(ctx); err == nil {
		t.Error("List must surface worktree list failure")
	}
	if _, err := m.Adopt(ctx, GCOptions{}); err == nil {
		t.Error("Adopt must surface worktree list failure")
	}

	m.git = failingGit("status")
	copies, err := m.List(ctx)
	if err != nil || len(copies) != 1 || !copies[0].Dirty {
		t.Errorf("unreadable status must count as dirty: %+v %v", copies, err)
	}

	m.git = failingGit("worktree remove")
	reports, err := m.GC(ctx, GCOptions{})
	if err != nil || reports[0].Action != "kept" || !strings.Contains(reports[0].Reason, "injected") {
		t.Errorf("gc removal failure: %+v %v", reports, err)
	}
	if _, err := m.Remove(ctx, "merged", RemoveOptions{}); err == nil {
		t.Error("Remove must surface removal failure")
	}

	m.git = failingGit("branch -D")
	if _, err := m.Remove(ctx, "merged", RemoveOptions{}); err == nil || !strings.Contains(err.Error(), "branch not deleted") {
		t.Errorf("branch delete failure: %v", err)
	}
	if dirExists(merged) || dirExists(root) {
		t.Error("copy and empty root should be gone even when the branch survives")
	}
}

func TestRemoveRefusesUnpushedAndHandlesDetached(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	remote := filepath.Join(filepath.Dir(repo), "remote.git")
	git(t, repo, "init", "-q", "--bare", remote)
	git(t, repo, "remote", "add", "origin", remote)
	m := newManager(repo)

	ahead, _ := m.Create(ctx, "ahead", "", "")
	commitIn(t, ahead, "a.txt")
	git(t, ahead, "push", "-q", "-u", "origin", "agent/ahead")
	commitIn(t, ahead, "b.txt")
	if _, err := m.Remove(ctx, ahead, RemoveOptions{}); err == nil || !strings.Contains(err.Error(), "unpushed") {
		t.Errorf("expected unpushed refusal: %v", err)
	}

	detached := filepath.Join(root, "Proj-detached")
	git(t, repo, "worktree", "add", "-q", "--detach", detached)
	c, err := m.Resolve(ctx, detached)
	if err != nil || c.Branch != "" || c.MergedInto != "main" {
		t.Fatalf("detached copy: %+v %v", c, err)
	}
	r, err := m.Remove(ctx, "detached", RemoveOptions{})
	if err != nil || r.Reason != "removed on request" {
		t.Errorf("detached remove: %+v %v", r, err)
	}
}

func TestGCWithoutWorkingDir(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)
	path, _ := m.Create(ctx, "x", "", "")
	m.getwd = func() (string, error) { return "", errors.New("no cwd") }
	reports, err := m.GC(ctx, GCOptions{})
	if err != nil || reportFor(t, reports, path).Reason != "no commits yet" {
		t.Errorf("gc without cwd: %+v %v", reports, err)
	}
}

func TestAdoptEdgeCases(t *testing.T) {
	repo, root := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)

	here := filepath.Join(filepath.Dir(repo), "here")
	git(t, repo, "worktree", "add", "-q", "-b", "here", here)
	clash := filepath.Join(filepath.Dir(repo), "Proj-clash")
	git(t, repo, "worktree", "add", "-q", "-b", "clash", clash)
	if err := os.MkdirAll(filepath.Join(root, "Proj-clash"), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(filepath.Dir(repo), "Proj-broken")
	git(t, repo, "worktree", "add", "-q", "-b", "broken", broken)

	m.getwd = func() (string, error) { return here, nil }
	m.git = failingGit("worktree move")
	reports, err := m.Adopt(ctx, GCOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r := reportFor(t, reports, here); r.Reason != "current working directory" {
		t.Errorf("cwd: %+v", r)
	}
	if r := reportFor(t, reports, clash); !strings.Contains(r.Reason, "already exists") {
		t.Errorf("clash: %+v", r)
	}
	if r := reportFor(t, reports, broken); r.Action != "kept" || !strings.Contains(r.Reason, "injected") {
		t.Errorf("move failure: %+v", r)
	}

	// The root cannot be created under a file.
	blocker := filepath.Join(filepath.Dir(repo), "blocker")
	writeFile(t, blocker, "x")
	t.Setenv(EnvWorkDir, filepath.Join(blocker, "work"))
	m.git = RunGit
	reports, err = m.Adopt(ctx, GCOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r := reportFor(t, reports, broken); r.Action != "kept" {
		t.Errorf("mkdir failure: %+v", r)
	}
}

func TestCorruptMetadataIsIgnored(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	m := newManager(repo)
	path, _ := m.Create(ctx, "x", "", "")
	writeFile(t, metaPath(path), "{not json")
	c, err := m.Resolve(ctx, "x")
	if err != nil || c.Untouched {
		t.Errorf("corrupt meta: %+v %v", c, err)
	}
}

func TestPathHelpers(t *testing.T) {
	if isWithin("/abs", "relative") {
		t.Error("mixed absolute/relative paths are never nested")
	}
	prev := evalSymlinks
	evalSymlinks = func(string) (string, error) { return "", errors.New("unresolvable") }
	defer func() { evalSymlinks = prev }()
	if got := Canon("/a/b"); got != "/a/b" {
		t.Errorf("Canon fallback = %s", got)
	}
}
