package devtool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", name)
	gitIn(t, dir, "commit", "-q", "-m", "add "+name)
}

func TestPushCurrentBranch_AndInspectRepo(t *testing.T) {
	t.Setenv("DENDRITE_COMMIT_PUSH", "")
	ctx := context.Background()
	repo, bare := setupCommitRepoWithRemote(t, "feature")

	// Unborn branch: nothing ahead.
	st, err := InspectRepo(ctx, repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Ahead != 0 || st.HasUpstream {
		t.Fatalf("unborn repo state: %+v", st)
	}

	commitFile(t, repo, "a.txt")
	commitFile(t, repo, "b.txt")
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err = InspectRepo(ctx, repo, true)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "feature" || st.Ahead != 2 || st.HasUpstream || len(st.Dirty) != 1 || st.Dirty[0] != "dirty.txt" {
		t.Fatalf("pre-push state: %+v", st)
	}

	out := PushCurrentBranch(ctx, repo)
	if !out.Pushed || out.Err != nil || out.Remote != "origin" || out.Branch != "feature" {
		t.Fatalf("first push: %+v", out)
	}
	gitIn(t, bare, "rev-parse", "--verify", "feature")

	st, _ = InspectRepo(ctx, repo, false)
	if !st.HasUpstream || st.Ahead != 0 || st.Behind != 0 {
		t.Fatalf("post-push state: %+v", st)
	}

	// Existing upstream: plain push.
	commitFile(t, repo, "c.txt")
	if st, _ = InspectRepo(ctx, repo, false); st.Ahead != 1 {
		t.Fatalf("expected 1 ahead, got %+v", st)
	}
	if out = PushCurrentBranch(ctx, repo); !out.Pushed {
		t.Fatalf("second push: %+v", out)
	}

	// Remote gets ahead: InspectRepo with fetch sees Behind.
	other := filepath.Join(t.TempDir(), "other")
	gitIn(t, filepath.Dir(other), "clone", "-q", "-b", "feature", bare, other)
	gitIn(t, other, "config", "user.email", "o@x")
	gitIn(t, other, "config", "user.name", "o")
	commitFile(t, other, "remote.txt")
	gitIn(t, other, "push", "-q")
	if st, _ = InspectRepo(ctx, repo, true); st.Behind != 1 {
		t.Fatalf("expected 1 behind after fetch, got %+v", st)
	}
}

func TestPushCurrentBranch_Guards(t *testing.T) {
	ctx := context.Background()

	t.Run("env disabled", func(t *testing.T) {
		t.Setenv("DENDRITE_COMMIT_PUSH", "false")
		if out := PushCurrentBranch(ctx, t.TempDir()); !out.Skipped || out.Reason != "DENDRITE_COMMIT_PUSH=false" {
			t.Fatalf("%+v", out)
		}
	})

	t.Setenv("DENDRITE_COMMIT_PUSH", "")

	t.Run("no remote", func(t *testing.T) {
		dir := t.TempDir()
		gitIn(t, dir, "init", "-q", "-b", "feature")
		if out := PushCurrentBranch(ctx, dir); !out.Skipped || out.Reason != "no remote configured" {
			t.Fatalf("%+v", out)
		}
	})

	t.Run("main refused", func(t *testing.T) {
		repo, _ := setupCommitRepoWithRemote(t, "main")
		commitFile(t, repo, "a.txt")
		if out := PushCurrentBranch(ctx, repo); out.Pushed || out.Err == nil {
			t.Fatalf("%+v", out)
		}
	})

	t.Run("detached HEAD skipped", func(t *testing.T) {
		repo, _ := setupCommitRepoWithRemote(t, "feature")
		commitFile(t, repo, "a.txt")
		commitFile(t, repo, "b.txt")
		gitIn(t, repo, "checkout", "-q", "--detach", "HEAD~1")
		if out := PushCurrentBranch(ctx, repo); out.Pushed || !out.Skipped || out.Err != nil {
			t.Fatalf("%+v", out)
		}
	})

	t.Run("inspect non-repo", func(t *testing.T) {
		if _, err := InspectRepo(ctx, t.TempDir(), false); err == nil {
			t.Fatal("expected error outside a git repo")
		}
	})
}
