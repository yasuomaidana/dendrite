package bootstrap

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepoAt(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	return root
}

func gitRun(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestMergeManagedBlock(t *testing.T) {
	fresh, changed := mergeManagedBlock("")
	if !changed || !strings.HasPrefix(fresh, "#!/bin/sh\n") || !strings.Contains(fresh, "dendrite hook pre-commit || exit 1") {
		t.Errorf("fresh script:\n%s", fresh)
	}
	if again, changed := mergeManagedBlock(fresh); changed || again != fresh {
		t.Errorf("a script that holds the block is left as it is (changed=%v)", changed)
	}

	user := "#!/bin/sh\necho lint\nexit_code=0"
	appended, changed := mergeManagedBlock(user)
	if !changed || !strings.HasPrefix(appended, user+"\n\n"+hookBlockBegin) {
		t.Errorf("the user's commands stay first and the block follows:\n%s", appended)
	}

	stale := "#!/bin/sh\necho before\n" + hookBlockBegin + "\nold content\n" + hookBlockEnd + "\necho after\n"
	replaced, changed := mergeManagedBlock(stale)
	if !changed || strings.Contains(replaced, "old content") || !strings.Contains(replaced, "echo before\n"+hookBlockBegin) || !strings.HasSuffix(replaced, hookBlockEnd+"\necho after\n") {
		t.Errorf("a stale block is replaced in place:\n%s", replaced)
	}

	broken := "#!/bin/sh\n" + hookBlockBegin + "\nno end marker\n"
	if got, changed := mergeManagedBlock(broken); !changed || strings.Count(got, hookBlockBegin) != 2 {
		t.Errorf("a block with no end is not trusted; a new one is appended:\n%s", got)
	}
}

func TestInstallGitHookInAFreshRepository(t *testing.T) {
	root := gitRepoAt(t)
	created, updated, note, err := installGitHook(root, false)
	if err != nil || note != "" || len(updated) != 0 || len(created) != 1 || created[0] != preCommitHookPath {
		t.Fatalf("created=%v updated=%v note=%q err=%v", created, updated, note, err)
	}
	info, err := os.Stat(filepath.Join(root, preCommitHookPath))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("the hook must be executable: %v, %v", info, err)
	}
	if got := gitRun(t, root, "config", "--get", "core.hooksPath"); got != gitHooksDir {
		t.Errorf("core.hooksPath = %q", got)
	}
	created, updated, note, err = installGitHook(root, false)
	if err != nil || note != "" || len(created) != 0 || len(updated) != 0 {
		t.Errorf("a second run changes nothing: created=%v updated=%v note=%q err=%v", created, updated, note, err)
	}
}

func TestInstallGitHookKeepsTheUsersHookAndTheirHooksPath(t *testing.T) {
	root := gitRepoAt(t)
	path := filepath.Join(root, preCommitHookPath)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte("#!/bin/sh\necho my own check\n"), 0o755)
	created, updated, _, err := installGitHook(root, false)
	if err != nil || len(created) != 0 || len(updated) != 1 {
		t.Fatalf("created=%v updated=%v err=%v", created, updated, err)
	}
	if data, _ := os.ReadFile(path); !strings.HasPrefix(string(data), "#!/bin/sh\necho my own check\n") {
		t.Errorf("the user's own hook was disturbed:\n%s", data)
	}

	other := gitRepoAt(t)
	gitRun(t, other, "config", "core.hooksPath", "tools/hooks")
	_, _, note, err := installGitHook(other, false)
	if err != nil || !strings.Contains(note, `core.hooksPath is "tools/hooks"`) || !strings.Contains(note, "dendrite hook pre-commit") {
		t.Errorf("note=%q err=%v", note, err)
	}
	if got := gitRun(t, other, "config", "--get", "core.hooksPath"); got != "tools/hooks" {
		t.Errorf("a hooks path of the repository's own must stay: %q", got)
	}

	preset := gitRepoAt(t)
	gitRun(t, preset, "config", "core.hooksPath", gitHooksDir)
	if _, _, note, err := installGitHook(preset, false); err != nil || note != "" {
		t.Errorf("note=%q err=%v", note, err)
	}
}

func TestInstallGitHookSkipsNonRepositoriesAndDryRuns(t *testing.T) {
	plain := t.TempDir()
	if c, u, n, err := installGitHook(plain, false); err != nil || len(c)+len(u) != 0 || n != "" {
		t.Errorf("not a git repository: %v %v %q %v", c, u, n, err)
	}
	if _, err := os.Stat(filepath.Join(plain, gitHooksDir)); err == nil {
		t.Error("nothing may be written outside a git repository")
	}

	root := gitRepoAt(t)
	created, _, _, err := installGitHook(root, true)
	if err != nil || len(created) != 1 {
		t.Errorf("a dry run reports what it would create: %v %v", created, err)
	}
	if _, err := os.Stat(filepath.Join(root, preCommitHookPath)); err == nil {
		t.Error("a dry run wrote the hook")
	}
	if out, _ := exec.Command("git", "-C", root, "config", "--get", "core.hooksPath").Output(); strings.TrimSpace(string(out)) != "" {
		t.Error("a dry run changed core.hooksPath")
	}
}

func TestInstallGitHookReportsWhatItCannotWrite(t *testing.T) {
	root := gitRepoAt(t)
	if err := os.WriteFile(filepath.Join(root, gitHooksDir), []byte("a file where the directory belongs"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := installGitHook(root, false); err == nil {
		t.Error("a hooks directory that cannot be created is an error")
	}

	// git replaces .git/config through a lock file, so it is the directory that must not be writable.
	locked := gitRepoAt(t)
	gitDir := filepath.Join(locked, ".git")
	if err := os.Chmod(gitDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitDir, 0o755) })
	probe := filepath.Join(gitDir, "probe")
	canWrite := os.WriteFile(probe, nil, 0o644) == nil // root ignores permissions
	_ = os.Remove(probe)
	_, _, _, err := installGitHook(locked, false)
	failed := err != nil && strings.Contains(err.Error(), "core.hooksPath")
	if canWrite == failed {
		t.Errorf("writable = %v but err = %v", canWrite, err)
	}
}

// path with a stand-in dendrite first, followed by the system's.
func withDendrite(t *testing.T, body string) []string {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "dendrite"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := dir + string(os.PathListSeparator) + "/usr/bin:/bin"
	return append(os.Environ(), "PATH="+path)
}

func TestTheInstalledHookRefusesAndAllowsLikeTheLock(t *testing.T) {
	root := gitRepoAt(t)
	if _, _, _, err := installGitHook(root, false); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, preCommitHookPath)
	run := func(body string) (int, string) {
		cmd := exec.Command("/bin/sh", script)
		cmd.Env = withDendrite(t, body)
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), string(out)
		}
		return 0, string(out)
	}
	if code, _ := run("exit 0"); code != 0 {
		t.Errorf("lock allows: exit %d", code)
	}
	if code, out := run(`echo "blocked: no Dendrite session/ticket claim" >&2; exit 2`); code != 1 || !strings.Contains(out, "no Dendrite session/ticket claim") {
		t.Errorf("lock refuses: exit %d, %q", code, out)
	}
	if code, out := run(""); code != 1 || !strings.Contains(out, "dendrite is not on PATH") {
		t.Errorf("no dendrite: exit %d, %q (the hook fails closed)", code, out)
	}
}

func TestGitHonoursTheInstalledHook(t *testing.T) {
	root := gitRepoAt(t)
	if _, _, _, err := installGitHook(root, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "a.txt")

	commit := func(body string, extra ...string) error {
		args := append([]string{"-C", root, "commit", "-q", "-m", "work"}, extra...)
		cmd := exec.Command("git", args...)
		cmd.Env = append(withDendrite(t, body), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("git commit: %s", out)
		}
		return err
	}
	if err := commit("exit 2"); err == nil {
		t.Fatal("git committed although the lock refused")
	}
	if err := commit("exit 2", "--no-verify"); err != nil {
		t.Errorf("--no-verify is the human's escape hatch: %v", err)
	}
	gitRun(t, root, "reset", "-q", "--soft", "HEAD~0")
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "b.txt")
	if err := commit("exit 0"); err != nil {
		t.Errorf("git refused although the lock allowed: %v", err)
	}
}
