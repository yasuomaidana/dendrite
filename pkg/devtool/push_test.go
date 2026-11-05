package devtool

import (
	"Dendrite/pkg/projectid"
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

	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		validate func(t *testing.T, out PushOutcome)
	}{
		{
			name: "env disabled",
			setup: func(t *testing.T) string {
				t.Setenv("DENDRITE_COMMIT_PUSH", "false")
				return t.TempDir()
			},
			validate: func(t *testing.T, out PushOutcome) {
				if !out.Skipped || out.Reason != "DENDRITE_COMMIT_PUSH=false" {
					t.Fatalf("%+v", out)
				}
			},
		},
		{
			name: "no remote",
			setup: func(t *testing.T) string {
				t.Setenv("DENDRITE_COMMIT_PUSH", "")
				dir := t.TempDir()
				gitIn(t, dir, "init", "-q", "-b", "feature")
				return dir
			},
			validate: func(t *testing.T, out PushOutcome) {
				if !out.Skipped || out.Reason != "no remote configured" {
					t.Fatalf("%+v", out)
				}
			},
		},
		{
			name: "main refused",
			setup: func(t *testing.T) string {
				t.Setenv("DENDRITE_COMMIT_PUSH", "")
				repo, _ := setupCommitRepoWithRemote(t, "main")
				commitFile(t, repo, "a.txt")
				return repo
			},
			validate: func(t *testing.T, out PushOutcome) {
				if out.Pushed || out.Err == nil {
					t.Fatalf("%+v", out)
				}
			},
		},
		{
			name: "detached HEAD skipped",
			setup: func(t *testing.T) string {
				t.Setenv("DENDRITE_COMMIT_PUSH", "")
				repo, _ := setupCommitRepoWithRemote(t, "feature")
				commitFile(t, repo, "a.txt")
				commitFile(t, repo, "b.txt")
				gitIn(t, repo, "checkout", "-q", "--detach", "HEAD~1")
				return repo
			},
			validate: func(t *testing.T, out PushOutcome) {
				if out.Pushed || !out.Skipped || out.Err != nil {
					t.Fatalf("%+v", out)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup(t)
			out := PushCurrentBranch(ctx, dir)
			tc.validate(t, out)
		})
	}

	t.Run("inspect non-repo", func(t *testing.T) {
		if _, err := InspectRepo(ctx, t.TempDir(), false); err == nil {
			t.Fatal("expected error outside a git repo")
		}
	})
}

// withFork gives repo a second remote named fork.
func withFork(t *testing.T, repo string) {
	t.Helper()
	fork := t.TempDir()
	gitIn(t, fork, "init", "-q", "--bare")
	gitIn(t, repo, "remote", "add", "fork", fork)
}

// pushPreferring pushes a fresh commit of a repository with origin and fork remotes, preferring the given remote.
func pushPreferring(t *testing.T, prefer string) PushOutcome {
	t.Helper()
	t.Setenv("DENDRITE_COMMIT_PUSH", "")
	repo, _ := setupCommitRepoWithRemote(t, "feature")
	withFork(t, repo)
	commitFile(t, repo, "a.txt")
	return pushBranch(newGitRunner(context.Background(), repo), prefer)
}

func TestPushGoesToThePreferredRemoteOrFallsBackToOrigin(t *testing.T) {
	for prefer, want := range map[string]string{"fork": "fork", "nowhere": "origin", "": "origin"} {
		if out := pushPreferring(t, prefer); !out.Pushed || out.Remote != want {
			t.Errorf("prefer %q: %+v, want a push to %s", prefer, out, want)
		}
	}
}

func TestPushRemoteForReadsTheSynapseMember(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	repo, _ := setupCommitRepoWithRemote(t, "feature")
	if got := pushRemoteFor(repo); got != "" {
		t.Fatalf("no synapse means the default choice, got %q", got)
	}
	id, err := projectid.Ensure(repo)
	if err != nil {
		t.Fatal(err)
	}
	manifest := "id = \"s\"\nstate = \"active\"\n\n[[members]]\nproject_id = \"" + id + "\"\nalias = \"a\"\npush_remote = \"fork\"\n"
	dir := filepath.Join(os.Getenv("DENDRITE_HOME"), "synapses")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "s.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pushRemoteFor(repo); got != "fork" {
		t.Fatalf("pushRemoteFor = %q, want fork", got)
	}
	t.Chdir(repo)
	if got := pushRemoteFor(""); got != "fork" {
		t.Fatalf("an empty dir means the working directory: %q", got)
	}
}

// A branch that tracks origin/main must publish to its own name, not to main, whatever
// push.default says.
func TestPushNeverFollowsAnUpstreamOntoMain(t *testing.T) {
	t.Setenv("DENDRITE_COMMIT_PUSH", "")
	for _, pushDefault := range []string{"upstream", "simple", "current"} {
		t.Run(pushDefault, func(t *testing.T) {
			repo, bare := setupCommitRepoWithRemote(t, "main")
			commitFile(t, repo, "base.txt")
			gitIn(t, repo, "push", "-q", "origin", "main")
			gitIn(t, repo, "switch", "-q", "-c", "agent/den-1", "--track", "origin/main")
			gitIn(t, repo, "config", "push.default", pushDefault)
			commitFile(t, repo, "work.txt")

			out := PushCurrentBranch(context.Background(), repo)
			if !out.Pushed || out.Err != nil {
				t.Fatalf("%+v", out)
			}
			refs := func(ref string) string {
				cmd := exec.Command("git", "rev-parse", "--verify", "-q", ref)
				cmd.Dir = bare
				b, _ := cmd.Output()
				return string(b)
			}
			head := exec.Command("git", "rev-parse", "HEAD")
			head.Dir = repo
			sha, _ := head.Output()
			if refs("refs/heads/main") == string(sha) {
				t.Fatal("main on the remote moved: the guard was bypassed")
			}
			if refs("refs/heads/agent/den-1") != string(sha) {
				t.Fatal("the branch was not published under its own name")
			}
		})
	}
}
