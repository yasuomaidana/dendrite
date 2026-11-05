package projectid

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestStateRoot_WorktreeSharesTheMainCheckoutState(t *testing.T) {
	main := t.TempDir()
	gitIn(t, main, "init", "-q")
	gitIn(t, main, "commit", "-q", "--allow-empty", "-m", "root")
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, main, "worktree", "add", "-q", "-b", "agent/x", wt)

	mainReal, _ := filepath.EvalSymlinks(main)
	if got, _ := filepath.EvalSymlinks(StateRoot(wt)); got != mainReal {
		t.Fatalf("StateRoot(worktree) = %q, want %q", got, mainReal)
	}
	if got := StateRoot(main); got != main {
		t.Fatalf("StateRoot(main) = %q, want itself", got)
	}

	id, err := Ensure(main)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(wt)
	if err != nil || got != id {
		t.Fatalf("worktree Read = %q, %v; want the main checkout's %q", got, err, id)
	}
	if _, err := os.Stat(filepath.Join(wt, ".dendrite")); !os.IsNotExist(err) {
		t.Fatalf("worktree must not get its own .dendrite: %v", err)
	}
}

func TestStateRoot_PlainDirectoryIsItself(t *testing.T) {
	dir := t.TempDir()
	if got := StateRoot(dir); got != dir {
		t.Fatalf("StateRoot = %q, want %q", got, dir)
	}
}

func initRepo(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "root")
	if remote != "" {
		gitIn(t, dir, "remote", "add", "origin", remote)
	}
	return dir
}

func TestRegister_FreshCloneRecoversItsIDFromTheIndex(t *testing.T) {
	ix, err := LoadIndex(filepath.Join(t.TempDir(), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	orig := initRepo(t, "https://github.com/acme/app.git")
	first, err := ix.Register(orig)
	if err != nil {
		t.Fatal(err)
	}
	if first.RootCommit == "" {
		t.Fatal("Register must record the root commit")
	}
	ix.SetPlane(first.ID, &PlaneBinding{Identifier: "APP", ProjectID: "uuid-1"})

	// A clone has the same root commit and remote but no .dendrite/ (it is not committed).
	clone := t.TempDir()
	gitIn(t, clone, "clone", "-q", orig, ".")
	gitIn(t, clone, "remote", "set-url", "origin", "https://github.com/acme/app.git")
	got, err := ix.Register(clone)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != first.ID {
		t.Fatalf("clone got %s, want the indexed %s", got.ID, first.ID)
	}
	if got.Plane == nil || got.Plane.Identifier != "APP" {
		t.Fatalf("the Plane binding must survive re-registration, got %+v", got.Plane)
	}
	if id, _ := Read(clone); id != first.ID {
		t.Fatalf("the recovered ID must be written to the clone, read %q", id)
	}
}

func TestRegister_ForkWithAnotherRemoteGetsANewID(t *testing.T) {
	ix, _ := LoadIndex(filepath.Join(t.TempDir(), "projects.json"))
	orig := initRepo(t, "https://github.com/acme/app.git")
	first, _ := ix.Register(orig)

	fork := t.TempDir()
	gitIn(t, fork, "clone", "-q", orig, ".")
	gitIn(t, fork, "remote", "set-url", "origin", "https://github.com/someone/app.git")
	got, err := ix.Register(fork)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == first.ID {
		t.Fatal("a fork with another remote must not take the original's ID")
	}
}

func TestRegister_WorktreeRegistersTheMainCheckout(t *testing.T) {
	ix, _ := LoadIndex(filepath.Join(t.TempDir(), "projects.json"))
	main := initRepo(t, "")
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, main, "worktree", "add", "-q", "-b", "agent/x", wt)

	e, err := ix.Register(wt)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(main)
	if got, _ := filepath.EvalSymlinks(e.Path); got != want {
		t.Fatalf("index path = %q, want the main checkout %q", got, want)
	}
}
