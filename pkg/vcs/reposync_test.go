package vcs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// syncFixture is a bare "origin", a writer clone that publishes commits, and the
// subject clone that RepoSync inspects and updates.
type syncFixture struct {
	t       *testing.T
	origin  string
	writer  string
	subject string
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	// RepoSync's merge and rebase run through DefaultGitRunner, which inherits the environment.
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	root := t.TempDir()
	f := &syncFixture{t: t, origin: filepath.Join(root, "origin.git"), writer: filepath.Join(root, "writer"), subject: filepath.Join(root, "subject")}
	gitT(t, root, "init", "-q", "--bare", "-b", "develop", f.origin)
	gitT(t, root, "clone", "-q", f.origin, f.writer)
	gitT(t, f.writer, "checkout", "-q", "-b", "develop")
	f.commit(f.writer, "a.txt", "one")
	gitT(t, f.writer, "push", "-q", "-u", "origin", "develop")
	gitT(t, root, "clone", "-q", "-b", "develop", f.origin, f.subject)
	return f
}

// commit writes name=content in dir and commits it.
func (f *syncFixture) commit(dir, name, content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0o644); err != nil {
		f.t.Fatal(err)
	}
	gitT(f.t, dir, "add", name)
	gitT(f.t, dir, "commit", "-q", "-m", "update "+name)
}

// publish commits on the writer's develop and pushes it.
func (f *syncFixture) publish(name, content string) {
	f.t.Helper()
	f.commit(f.writer, name, content)
	gitT(f.t, f.writer, "push", "-q", "origin", "develop")
}

func TestRepoSync_FastForwardsTrackedBranch(t *testing.T) {
	f := newSyncFixture(t)
	f.publish("b.txt", "two")
	rs := NewRepoSync()

	st, err := rs.Inspect(context.Background(), f.subject, RepoSyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "develop" || st.Target != "origin/develop" || !st.OnTarget || st.Behind != 1 || st.Ahead != 0 {
		t.Fatalf("inspect = %+v", st)
	}

	st, err = rs.Sync(context.Background(), f.subject, RepoSyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Updated || st.Behind != 0 || st.Head != gitT(t, f.writer, "rev-parse", "HEAD") {
		t.Fatalf("sync = %+v", st)
	}
	if _, err := os.Stat(filepath.Join(f.subject, "b.txt")); err != nil {
		t.Fatal("fast-forward did not bring b.txt")
	}

	// Up to date: nothing to do.
	st, err = rs.Sync(context.Background(), f.subject, RepoSyncOptions{})
	if err != nil || st.Updated {
		t.Fatalf("second sync = %+v, %v", st, err)
	}
}

func TestRepoSync_DirtyTreeRefusedUnlessAutostash(t *testing.T) {
	f := newSyncFixture(t)
	f.publish("b.txt", "two")
	if err := os.WriteFile(filepath.Join(f.subject, "a.txt"), []byte("local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Untracked files never count as dirty.
	if err := os.WriteFile(filepath.Join(f.subject, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs := NewRepoSync()

	st, err := rs.Sync(context.Background(), f.subject, RepoSyncOptions{})
	if !errors.Is(err, ErrDirtyWorktree) || st.DirtyFiles != 1 || st.Updated {
		t.Fatalf("sync = %+v, %v; want ErrDirtyWorktree with 1 dirty file", st, err)
	}

	st, err = rs.Sync(context.Background(), f.subject, RepoSyncOptions{Autostash: true})
	if err != nil || !st.Updated {
		t.Fatalf("autostash sync = %+v, %v", st, err)
	}
	if data, _ := os.ReadFile(filepath.Join(f.subject, "a.txt")); string(data) != "local edit\n" {
		t.Fatalf("local edit lost after autostash: %q", data)
	}
}

func TestRepoSync_DetachedHead(t *testing.T) {
	f := newSyncFixture(t)
	gitT(t, f.subject, "checkout", "-q", "--detach")
	f.publish("b.txt", "two")
	rs := NewRepoSync()

	st, err := rs.Sync(context.Background(), f.subject, RepoSyncOptions{})
	if err != nil || !st.Detached || st.Target != "" || st.Updated {
		t.Fatalf("detached sync without ref = %+v, %v", st, err)
	}
	// Pending is computed against HEAD when there is no target.
	if len(st.Pending) != 1 || st.Pending[0].Ref != "origin/develop" || st.Pending[0].Commits != 1 {
		t.Fatalf("pending = %+v", st.Pending)
	}
	if _, err := rs.Sync(context.Background(), f.subject, RepoSyncOptions{Merge: "develop"}); !errors.Is(err, ErrDetachedHead) {
		t.Fatalf("merge on detached HEAD err = %v", err)
	}
	if got := displayBranch(st); got != "detached HEAD" {
		t.Fatalf("displayBranch = %q", got)
	}
}

func TestRepoSync_PendingBranchesAndMerge(t *testing.T) {
	f := newSyncFixture(t)
	gitT(t, f.writer, "checkout", "-q", "-b", "feat/x")
	f.commit(f.writer, "x.txt", "feature")
	gitT(t, f.writer, "push", "-q", "origin", "feat/x")
	rs := NewRepoSync()

	st, err := rs.Inspect(context.Background(), f.subject, RepoSyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Pending) != 1 || st.Pending[0] != (PendingBranch{Ref: "origin/feat/x", Commits: 1}) {
		t.Fatalf("pending = %+v", st.Pending)
	}

	st, err = rs.Sync(context.Background(), f.subject, RepoSyncOptions{Merge: "feat/x"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Merged != "origin/feat/x" || !st.Updated || len(st.Pending) != 0 || st.Ahead != 1 {
		t.Fatalf("merge sync = %+v", st)
	}
}

func TestRepoSync_MergeConflictIsAborted(t *testing.T) {
	f := newSyncFixture(t)
	gitT(t, f.writer, "checkout", "-q", "-b", "feat/conflict")
	f.commit(f.writer, "a.txt", "theirs")
	gitT(t, f.writer, "push", "-q", "origin", "feat/conflict")
	f.commit(f.subject, "a.txt", "ours")
	rs := NewRepoSync()

	_, err := rs.Sync(context.Background(), f.subject, RepoSyncOptions{Merge: "origin/feat/conflict"})
	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("expected aborted merge error, got %v", err)
	}
	if out := gitT(t, f.subject, "status", "--porcelain"); out != "" {
		t.Fatalf("merge not aborted cleanly:\n%s", out)
	}
}

func TestRepoSync_RefOtherThanCheckedOutBranch(t *testing.T) {
	f := newSyncFixture(t)
	gitT(t, f.subject, "checkout", "-q", "-b", "local-work")
	f.publish("b.txt", "two")
	rs := NewRepoSync()

	st, err := rs.Sync(context.Background(), f.subject, RepoSyncOptions{Ref: "develop"})
	if err != nil {
		t.Fatal(err)
	}
	if st.OnTarget || st.Updated || st.Behind != 1 || st.Target != "origin/develop" || len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "not switching") {
		t.Fatalf("sync = %+v", st)
	}
	if b := gitT(t, f.subject, "branch", "--show-current"); b != "local-work" {
		t.Fatalf("branch switched to %q", b)
	}

	// No upstream and no origin/local-work: nothing to compare against.
	st, err = rs.Inspect(context.Background(), f.subject, RepoSyncOptions{NoFetch: true})
	if err != nil || st.Target != "" {
		t.Fatalf("inspect without upstream = %+v, %v", st, err)
	}
	// Unknown ref: no target.
	st, err = rs.Inspect(context.Background(), f.subject, RepoSyncOptions{Ref: "nope", NoFetch: true})
	if err != nil || st.Target != "" {
		t.Fatalf("inspect unknown ref = %+v, %v", st, err)
	}
}

func TestRepoSync_FallsBackToRemoteBranchWithoutUpstream(t *testing.T) {
	f := newSyncFixture(t)
	gitT(t, f.subject, "branch", "--unset-upstream")
	f.publish("b.txt", "two")
	st, err := NewRepoSync().Sync(context.Background(), f.subject, RepoSyncOptions{})
	if err != nil || st.Target != "origin/develop" || !st.Updated {
		t.Fatalf("sync = %+v, %v", st, err)
	}
}

func TestRepoSync_TagRefAndRebase(t *testing.T) {
	f := newSyncFixture(t)
	gitT(t, f.writer, "tag", "-a", "v1.0.0", "-m", "v1")
	gitT(t, f.writer, "push", "-q", "origin", "v1.0.0")
	f.publish("b.txt", "two")
	rs := NewRepoSync()

	st, err := rs.Inspect(context.Background(), f.subject, RepoSyncOptions{Ref: "v1.0.0"})
	if err != nil || st.Target != "v1.0.0" || st.OnTarget {
		t.Fatalf("tag inspect = %+v, %v", st, err)
	}

	f.commit(f.subject, "mine.txt", "local")
	st, err = rs.Sync(context.Background(), f.subject, RepoSyncOptions{Rebase: true})
	if err != nil || !st.Updated || st.Ahead != 1 || st.Behind != 0 {
		t.Fatalf("rebase sync = %+v, %v", st, err)
	}

	// A diverged branch cannot fast-forward.
	f.publish("c.txt", "three")
	f.commit(f.subject, "mine2.txt", "local")
	if _, err := rs.Sync(context.Background(), f.subject, RepoSyncOptions{}); err == nil || !strings.Contains(err.Error(), "updating develop") {
		t.Fatalf("diverged ff-only err = %v", err)
	}
	// A conflicting rebase is aborted.
	f.publish("mine2.txt", "theirs")
	if _, err := rs.Sync(context.Background(), f.subject, RepoSyncOptions{Rebase: true}); err == nil {
		t.Fatal("expected conflicting rebase to fail")
	}
	if _, err := os.Stat(filepath.Join(f.subject, ".git", "rebase-merge")); !os.IsNotExist(err) {
		t.Fatal("rebase left in progress")
	}
}

func TestRepoSync_FetchFailureAndNonRepo(t *testing.T) {
	f := newSyncFixture(t)
	gitT(t, f.subject, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	st, err := NewRepoSync().Inspect(context.Background(), f.subject, RepoSyncOptions{})
	if err != nil || len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "fetch failed") {
		t.Fatalf("inspect = %+v, %v", st, err)
	}
	if _, err := (&RepoSync{}).Inspect(context.Background(), t.TempDir(), RepoSyncOptions{}); err == nil {
		t.Fatal("expected error for non-repo dir")
	}
	if _, err := (&RepoSync{}).Sync(context.Background(), t.TempDir(), RepoSyncOptions{}); err == nil {
		t.Fatal("expected sync error for non-repo dir")
	}
}

func TestRepoSync_ResolveRemoteRef(t *testing.T) {
	f := newSyncFixture(t)
	gitT(t, f.writer, "tag", "-a", "v2", "-m", "v2")
	gitT(t, f.writer, "push", "-q", "origin", "v2")
	head := gitT(t, f.writer, "rev-parse", "HEAD")
	rs := NewRepoSync()
	ctx := context.Background()

	for _, ref := range []string{"develop", "v2", ""} {
		got, err := rs.ResolveRemoteRef(ctx, f.origin, ref)
		if err != nil || got != head {
			t.Errorf("ResolveRemoteRef(%q) = %q, %v; want %s", ref, got, err, head)
		}
	}
	if got, err := rs.ResolveRemoteRef(ctx, "unused", head); err != nil || got != head {
		t.Fatalf("full sha passthrough = %q, %v", got, err)
	}
	if _, err := rs.ResolveRemoteRef(ctx, f.origin, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing ref err = %v", err)
	}
	if _, err := rs.ResolveRemoteRef(ctx, filepath.Join(t.TempDir(), "gone.git"), "develop"); err == nil {
		t.Fatal("expected error for unreachable remote")
	}
}

func TestRepoSyncHelpers(t *testing.T) {
	if isFullSHA("abc") || isFullSHA(strings.Repeat("g", 40)) || !isFullSHA(strings.Repeat("a", 40)) {
		t.Fatal("isFullSHA")
	}
	if a, b := parseCounts("3\t4"); a != 3 || b != 4 {
		t.Fatal("parseCounts")
	}
	if a, b := parseCounts(""); a != 0 || b != 0 {
		t.Fatal("parseCounts empty")
	}
	if firstLine("a\nb") != "a" || firstLine("a") != "a" {
		t.Fatal("firstLine")
	}
	if (RepoSyncOptions{Remote: "up"}).remote() != "up" {
		t.Fatal("remote override")
	}
}
