package vcs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withMain publishes a main branch (one commit behind develop's future) on the fixture origin.
func (f *syncFixture) withMain() {
	f.t.Helper()
	gitT(f.t, f.writer, "checkout", "-q", "-b", "main")
	f.commit(f.writer, "m.txt", "main")
	gitT(f.t, f.writer, "push", "-q", "-u", "origin", "main")
	gitT(f.t, f.writer, "checkout", "-q", "develop")
}

func TestRepoSync_EnforceSwitchesCleanCheckout(t *testing.T) {
	f := newSyncFixture(t)
	f.withMain()
	gitT(t, f.subject, "checkout", "-q", "-b", "feature")
	rs := NewRepoSync()

	st, err := rs.Sync(context.Background(), f.subject, RepoSyncOptions{Ref: "main", Enforce: true})
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	if st.Branch != "main" || !st.OnTarget || st.Switched != "feature" {
		t.Fatalf("expected a switch from feature to main, got branch=%q on=%v switched=%q", st.Branch, st.OnTarget, st.Switched)
	}
	if got := gitT(t, f.subject, "rev-parse", "--abbrev-ref", "@{u}"); got != "origin/main" {
		t.Fatalf("new main should track origin/main, tracks %q", got)
	}

	// Back on develop with a stale local main: the existing branch is reused and fast-forwarded.
	gitT(t, f.subject, "checkout", "-q", "develop")
	gitT(t, f.writer, "checkout", "-q", "main")
	f.commit(f.writer, "m2.txt", "main two")
	gitT(t, f.writer, "push", "-q", "origin", "main")
	st, err = rs.Sync(context.Background(), f.subject, RepoSyncOptions{Ref: "main", Enforce: true})
	if err != nil {
		t.Fatalf("enforce onto existing branch: %v", err)
	}
	if st.Branch != "main" || st.Behind != 0 || !st.Updated {
		t.Fatalf("expected main fast-forwarded, got branch=%q behind=%d updated=%v", st.Branch, st.Behind, st.Updated)
	}
}

func TestRepoSync_EnforceRefusesDirtyCheckout(t *testing.T) {
	f := newSyncFixture(t)
	f.withMain()
	if err := os.WriteFile(filepath.Join(f.subject, "a.txt"), []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewRepoSync().Sync(context.Background(), f.subject, RepoSyncOptions{Ref: "main", Enforce: true})
	if !errors.Is(err, ErrOffTrackedBranch) {
		t.Fatalf("expected ErrOffTrackedBranch, got %v", err)
	}
	if b := gitT(t, f.subject, "branch", "--show-current"); b != "develop" {
		t.Fatalf("a dirty checkout must not be switched, now on %q", b)
	}
}

func TestRepoSync_EnforceLeavesWorkCopiesAndNonBranches(t *testing.T) {
	f := newSyncFixture(t)
	f.withMain()
	ctx := context.Background()
	rs := NewRepoSync()

	wc := filepath.Join(t.TempDir(), WorkCopyDir, "Repo-DEN-1")
	gitT(t, f.subject, "worktree", "add", "-q", "-b", "agent/den-1", wc)
	st, err := rs.Sync(ctx, wc, RepoSyncOptions{Ref: "main", Enforce: true})
	if err != nil {
		t.Fatalf("work copy: %v", err)
	}
	if st.Branch != "agent/den-1" || !strings.Contains(strings.Join(st.Notes, "\n"), "stays on") {
		t.Fatalf("work copy must stay on its branch, got %q notes=%v", st.Branch, st.Notes)
	}

	gitT(t, f.writer, "tag", "v1")
	gitT(t, f.writer, "push", "-q", "origin", "v1")
	st, err = rs.Sync(ctx, f.subject, RepoSyncOptions{Ref: "v1", Enforce: true})
	if err != nil || st.Branch != "develop" || st.Switched != "" {
		t.Fatalf("a tag ref is not enforced: branch=%q switched=%q err=%v", st.Branch, st.Switched, err)
	}

	st, err = rs.Sync(ctx, f.subject, RepoSyncOptions{Enforce: true})
	if err != nil || st.Switched != "" {
		t.Fatalf("no ref means nothing to enforce: switched=%q err=%v", st.Switched, err)
	}
}

func TestRepoSync_EnforceFailsWhenBranchIsBusy(t *testing.T) {
	f := newSyncFixture(t)
	f.withMain()
	// main is checked out in another worktree of the same repository, so switch fails.
	gitT(t, f.subject, "fetch", "-q", "origin")
	gitT(t, f.subject, "worktree", "add", "-q", filepath.Join(t.TempDir(), "other"), "-b", "main", "origin/main")
	gitT(t, f.subject, "checkout", "-q", "-b", "feature")
	_, err := NewRepoSync().Sync(context.Background(), f.subject, RepoSyncOptions{Ref: "main", Enforce: true})
	if err == nil || !strings.Contains(err.Error(), "switching") {
		t.Fatalf("expected a switch failure, got %v", err)
	}
}

func TestRepoSync_Clone(t *testing.T) {
	f := newSyncFixture(t)
	f.withMain()
	ctx := context.Background()
	rs := NewRepoSync()
	root := t.TempDir()

	onMain := filepath.Join(root, "main")
	if err := rs.Clone(ctx, f.origin, "main", onMain); err != nil {
		t.Fatalf("clone main: %v", err)
	}
	if b := gitT(t, onMain, "branch", "--show-current"); b != "main" {
		t.Fatalf("expected main, got %q", b)
	}

	sha := gitT(t, f.writer, "rev-parse", "develop")
	atSHA := filepath.Join(root, "sha")
	if err := rs.Clone(ctx, f.origin, sha, atSHA); err != nil {
		t.Fatalf("clone sha: %v", err)
	}
	if got := gitT(t, atSHA, "rev-parse", "HEAD"); got != sha {
		t.Fatalf("expected HEAD %s, got %s", sha, got)
	}

	if err := rs.Clone(ctx, f.origin, "no-such-branch", filepath.Join(root, "x")); err == nil {
		t.Fatal("expected a clone error for a missing branch")
	}
	if err := rs.Clone(ctx, f.origin, strings.Repeat("a", 40), filepath.Join(root, "y")); err == nil {
		t.Fatal("expected a checkout error for an unknown commit")
	}
}

func TestRepoSync_Exclude(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	rs := NewRepoSync()
	wc := filepath.Join(t.TempDir(), "wt")
	gitT(t, f.subject, "worktree", "add", "-q", "-b", "side", wc)

	for i := 0; i < 2; i++ { // idempotent, and a worktree shares the common exclude file
		if err := rs.Exclude(ctx, wc, "/.dendrite/project.id"); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(f.subject, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "/.dendrite/project.id"); n != 1 {
		t.Fatalf("pattern listed %d times:\n%s", n, data)
	}
	if err := os.MkdirAll(filepath.Join(f.subject, ".dendrite"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.subject, ".dendrite", "project.id"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := gitT(t, f.subject, "status", "--porcelain"); got != "" {
		t.Fatalf("excluded file still shows: %q", got)
	}
	if err := rs.Exclude(ctx, t.TempDir(), "x"); err == nil {
		t.Fatal("expected an error outside a repository")
	}
}
