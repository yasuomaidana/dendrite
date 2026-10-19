package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupCommitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	runGitT(t, repo, "init", "-q", "-b", "feat/x")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitT(t, repo, "add", ".")
	runGitT(t, repo, "commit", "-q", "-m", "init")
	t.Chdir(repo)
	return repo
}

func TestQuantal(t *testing.T) {
	repo := setupCommitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := cliOutput(t, "quantal", "--ticket", "DEN-1", "--no-smart", "--no-local", "-m", "feat: add b", "--all", "--no-push", "--dry-run")
	if code != ExitOK || !strings.Contains(out, "Dry run commit preview generated") {
		t.Fatalf("dry run: exit %d\n%s", code, out)
	}
	out, code = cliOutput(t, "commit", "-t", "DEN-1", "--no-smart", "--no-local", "-m", "feat: add b", "--files", "b.txt", "--no-push")
	if code != ExitOK || !strings.Contains(out, "Commit applied successfully") {
		t.Fatalf("commit: exit %d\n%s", code, out)
	}
	if log := runGitT(t, repo, "log", "-1", "--format=%s"); !strings.Contains(log, "feat: add b") {
		t.Fatalf("commit not created: %q", log)
	}

	for _, args := range [][]string{
		{"quantal", "--smart", "--no-smart"},
		{"quantal", "--files", "a", "--all"},
		{"quantal", "extra"},
	} {
		if _, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
	t.Setenv("DENDRITE_COMMIT_PUSH", "false")
	if out, code := cliOutput(t, "quantal", "--no-smart", "--no-local", "-m", "nothing", "--no-push"); code != ExitError {
		t.Errorf("committing a clean tree should fail: exit %d\n%s", code, out)
	}
}
