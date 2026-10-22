package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/workcopy"
)

// TestMain keeps every test in this package away from the real dendrite-work/
// directory: session commands run working-copy cleanup as a side effect.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dendrite-work-test-")
	if err == nil {
		_ = os.Setenv(workcopy.EnvWorkDir, dir)
	}
	// Commands must not pick up the developer's real keys from .env files.
	_ = os.Setenv(dotenv.EnvDisable, "off")
	code := m.Run()
	if err == nil {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

func runGitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// setupWorkRepo creates a repo with one commit and points the work CLI at it.
func setupWorkRepo(t *testing.T) (repo, root string) {
	t.Helper()
	base := workcopy.Canon(t.TempDir())
	repo = filepath.Join(base, "Proj")
	root = filepath.Join(base, "work")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(workcopy.EnvWorkDir, root)
	t.Setenv("DENDRITE_STATE_DIR", filepath.Join(base, "state"))
	runGitT(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitT(t, repo, "add", ".")
	runGitT(t, repo, "commit", "-q", "-m", "init")

	prev := newWorkManager
	newWorkManager = func() *workcopy.Manager { return workcopy.New(repo) }
	t.Cleanup(func() { newWorkManager = prev })
	return repo, root
}

func TestWorkCommand_UsageAndErrors(t *testing.T) {
	ctx := context.Background()
	for _, args := range [][]string{nil, {"help"}} {
		var buf bytes.Buffer
		if err := runWorkCommand(ctx, args, &buf); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "dendrite work [command]") {
			t.Errorf("usage missing for %v: %s", args, buf.String())
		}
	}
	setupWorkRepo(t)
	var buf bytes.Buffer
	cases := [][]string{
		{"bogus"},
		{"new"},
		{"new", "a", "b"},
		{"new", "--nope"},
		{"done"},
		{"done", "--nope"},
		{"done", "missing"},
		{"list", "--nope"},
		{"gc", "--nope"},
		{"adopt", "--nope"},
	}
	for _, args := range cases {
		if err := runWorkCommand(ctx, args, &buf); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}

func TestWorkCommand_Lifecycle(t *testing.T) {
	repo, root := setupWorkRepo(t)
	ctx := context.Background()
	var buf bytes.Buffer

	if err := runWorkCommand(ctx, []string{"list", "--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Fatalf("empty json list = %q", buf.String())
	}

	buf.Reset()
	if err := runWorkCommand(ctx, []string{"new", "DEN-7", "--base", "main"}, &buf); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(root, "Proj-DEN-7")
	if !strings.Contains(buf.String(), copyPath) {
		t.Fatalf("new output: %s", buf.String())
	}
	if err := runWorkCommand(ctx, []string{"new", "DEN-7"}, &buf); err == nil {
		t.Fatal("expected duplicate new to fail")
	}

	buf.Reset()
	if err := runWorkCommand(ctx, []string{"list"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "agent/den-7") || !strings.Contains(buf.String(), "no commits") {
		t.Fatalf("list output: %s", buf.String())
	}

	// Commit in the copy and merge it: gc should remove it.
	if err := os.WriteFile(filepath.Join(copyPath, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitT(t, copyPath, "add", "f.txt")
	runGitT(t, copyPath, "commit", "-q", "-m", "f")
	runGitT(t, repo, "merge", "-q", "--no-edit", "agent/den-7")

	buf.Reset()
	if err := runWorkCommand(ctx, []string{"gc", "--dry-run"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "would remove") {
		t.Fatalf("dry run output: %s", buf.String())
	}

	buf.Reset()
	if err := runWorkCommand(ctx, []string{"gc"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[✓] removed "+copyPath+" (merged into main)") {
		t.Fatalf("gc output: %s", buf.String())
	}
	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Fatalf("copy still exists: %v", err)
	}

	// done: explicit removal keeps an unmerged branch.
	buf.Reset()
	if err := runWorkCommand(ctx, []string{"new", "keep", "--branch", "feat/keep"}, &buf); err != nil {
		t.Fatal(err)
	}
	keepPath := filepath.Join(root, "Proj-keep")
	if err := os.WriteFile(filepath.Join(keepPath, "k.txt"), []byte("k\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runWorkCommand(ctx, []string{"done", "keep"}, &buf); err == nil {
		t.Fatal("expected refusal for dirty copy")
	}
	buf.Reset()
	if err := runWorkCommand(ctx, []string{"done", "--force", "keep"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "removed "+keepPath) {
		t.Fatalf("done output: %s", buf.String())
	}
}

func TestWorkCommand_Adopt(t *testing.T) {
	repo, root := setupWorkRepo(t)
	ctx := context.Background()
	var buf bytes.Buffer

	if err := runWorkCommand(ctx, []string{"adopt"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No worktrees outside") {
		t.Fatalf("adopt with nothing: %s", buf.String())
	}

	sibling := filepath.Join(filepath.Dir(repo), "Proj-side")
	runGitT(t, repo, "worktree", "add", "-q", "-b", "side", sibling)
	buf.Reset()
	if err := runWorkCommand(ctx, []string{"adopt", "--dry-run"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "would move") {
		t.Fatalf("adopt dry run: %s", buf.String())
	}
	buf.Reset()
	if err := runWorkCommand(ctx, []string{"adopt"}, &buf); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Proj-side")); err != nil {
		t.Fatalf("sibling not moved: %v\n%s", err, buf.String())
	}
}

func TestWorkCommand_NotARepo(t *testing.T) {
	notRepo := t.TempDir()
	prev := newWorkManager
	newWorkManager = func() *workcopy.Manager { return workcopy.New(notRepo) }
	defer func() { newWorkManager = prev }()
	t.Setenv(workcopy.EnvWorkDir, "")

	ctx := context.Background()
	var buf bytes.Buffer
	for _, args := range [][]string{{"new", "x"}, {"list"}, {"list", "--json"}, {"gc"}, {"adopt"}} {
		if err := runWorkCommand(ctx, args, &buf); err == nil {
			t.Errorf("expected error outside a repo for %v", args)
		}
	}

	// Session hooks never fail: they only warn.
	buf.Reset()
	autoCleanWorkCopies(ctx, &buf)
	if !strings.Contains(buf.String(), "Working-copy cleanup skipped") {
		t.Fatalf("expected warning, got %q", buf.String())
	}
}

func TestAutoCleanWorkCopiesPrintsOnlyRemovals(t *testing.T) {
	repo, root := setupWorkRepo(t)
	ctx := context.Background()
	var buf bytes.Buffer
	if err := runWorkCommand(ctx, []string{"new", "a"}, &buf); err != nil {
		t.Fatal(err)
	}
	if err := runWorkCommand(ctx, []string{"new", "b"}, &buf); err != nil {
		t.Fatal(err)
	}
	aPath := filepath.Join(root, "Proj-a")
	if err := os.WriteFile(filepath.Join(aPath, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitT(t, aPath, "add", "a.txt")
	runGitT(t, aPath, "commit", "-q", "-m", "a")
	runGitT(t, repo, "merge", "-q", "--no-edit", "agent/a")

	buf.Reset()
	autoCleanWorkCopies(ctx, &buf)
	out := buf.String()
	if !strings.Contains(out, "removed "+aPath) || strings.Contains(out, "Proj-b") {
		t.Fatalf("auto clean output: %q", out)
	}
}

func TestSessionResumeRunsWorkCleanup(t *testing.T) {
	repo, root := setupWorkRepo(t)
	ctx := context.Background()
	var buf bytes.Buffer
	if err := runWorkCommand(ctx, []string{"new", "s"}, &buf); err != nil {
		t.Fatal(err)
	}
	sPath := filepath.Join(root, "Proj-s")
	if err := os.WriteFile(filepath.Join(sPath, "s.txt"), []byte("s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitT(t, sPath, "add", "s.txt")
	runGitT(t, sPath, "commit", "-q", "-m", "s")
	runGitT(t, repo, "merge", "-q", "--no-edit", "agent/s")

	buf.Reset()
	if err := runSessionCommand(ctx, []string{"checkpoint", "--worktree", repo}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "removed "+sPath) {
		t.Fatalf("checkpoint did not clean merged copy: %s", buf.String())
	}

	// JSON output must stay valid JSON even when cleanup runs.
	buf.Reset()
	if err := runSessionCommand(ctx, []string{"resume", "--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("resume --json is not valid JSON: %v\n%s", err, buf.String())
	}
}

func TestCopyStatus(t *testing.T) {
	if got := copyStatus(workcopy.Copy{}); got != "active" {
		t.Errorf("got %q", got)
	}
	got := copyStatus(workcopy.Copy{Story: true, Dirty: true, Ahead: 2, Untouched: true, MergedInto: "main", UpstreamGone: true})
	want := "story, dirty, 2 unpushed, no commits, merged into main, upstream gone"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
