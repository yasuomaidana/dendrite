package soma

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/workcopy"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	clitest.SetupWorkRepo(t)
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

// workOut runs `dendrite work <args>` and returns what it printed; a failure stops the test.
func workOut(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	testkit.Check(t, runWorkCommand(context.Background(), args, &buf))
	return buf.String()
}

// workFails runs `dendrite work <args>` and stops the test unless it is refused.
func workFails(t *testing.T, why string, args ...string) {
	t.Helper()
	if err := runWorkCommand(context.Background(), args, io.Discard); err == nil {
		t.Fatal(why)
	}
}

func TestWorkCommand_Lifecycle(t *testing.T) {
	repo, root := clitest.SetupWorkRepo(t)
	testkit.Equal(t, "empty json list", strings.TrimSpace(workOut(t, "list", "--json")), "[]")

	copyPath := filepath.Join(root, "Proj-DEN-7")
	testkit.Contains(t, workOut(t, "new", "DEN-7", "--base", "main"), copyPath)
	workFails(t, "expected duplicate new to fail", "new", "DEN-7")
	testkit.Contains(t, workOut(t, "list"), "DEN-7/main", "no commits")

	// Commit in the copy and merge it: gc should remove it.
	testkit.WriteFile(t, filepath.Join(copyPath, "f.txt"), "x\n", 0o644)
	clitest.RunGitT(t, copyPath, "add", "f.txt")
	clitest.RunGitT(t, copyPath, "commit", "-q", "-m", "f")
	clitest.RunGitT(t, repo, "merge", "-q", "--no-edit", "DEN-7/main")

	testkit.Contains(t, workOut(t, "gc", "--dry-run"), "would remove")
	testkit.Contains(t, workOut(t, "gc"), "[✓] removed "+copyPath+" (merged into main)")
	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Fatalf("copy still exists: %v", err)
	}

	// done: explicit removal keeps an unmerged branch.
	workOut(t, "new", "keep", "--branch", "feat/keep")
	keepPath := filepath.Join(root, "Proj-keep")
	testkit.WriteFile(t, filepath.Join(keepPath, "k.txt"), "k\n", 0o644)
	workFails(t, "expected refusal for dirty copy", "done", "keep")
	testkit.Contains(t, workOut(t, "done", "--force", "keep"), "removed "+keepPath)
}

func TestWorkCommand_Adopt(t *testing.T) {
	repo, root := clitest.SetupWorkRepo(t)
	ctx := context.Background()
	var buf bytes.Buffer

	if err := runWorkCommand(ctx, []string{"adopt"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No worktrees outside") {
		t.Fatalf("adopt with nothing: %s", buf.String())
	}

	sibling := filepath.Join(filepath.Dir(repo), "Proj-side")
	clitest.RunGitT(t, repo, "worktree", "add", "-q", "-b", "side", sibling)
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
	prev := cli.NewWorkManager
	cli.NewWorkManager = func() *workcopy.Manager { return workcopy.New(notRepo) }
	defer func() { cli.NewWorkManager = prev }()
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
	AutoCleanWorkCopies(ctx, &buf)
	if !strings.Contains(buf.String(), "Working-copy cleanup skipped") {
		t.Fatalf("expected warning, got %q", buf.String())
	}
}

func TestAutoCleanWorkCopiesPrintsOnlyRemovals(t *testing.T) {
	repo, root := clitest.SetupWorkRepo(t)
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
	clitest.RunGitT(t, aPath, "add", "a.txt")
	clitest.RunGitT(t, aPath, "commit", "-q", "-m", "a")
	clitest.RunGitT(t, repo, "merge", "-q", "--no-edit", "a/main")

	buf.Reset()
	AutoCleanWorkCopies(ctx, &buf)
	out := buf.String()
	if !strings.Contains(out, "removed "+aPath) || strings.Contains(out, "Proj-b") {
		t.Fatalf("auto clean output: %q", out)
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

// runSessionCommand runs `dendrite session <args...>`.
func runWorkCommand(ctx context.Context, args []string, out io.Writer) error {
	return runCmd(ctx, out, append([]string{"work"}, args...)...)
}
