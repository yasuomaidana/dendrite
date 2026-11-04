package commit

import (
	analysiscmd "Dendrite/cmd/dendrite/analysis"
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	qualitycmd "Dendrite/cmd/dendrite/quality"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuantal(t *testing.T) {
	repo := clitest.SetupCommitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := cliOutput(t, "quantal", "--ticket", "DEN-1", "--no-smart", "--no-local", "-m", "feat: add b", "--all", "--no-push", "--dry-run")
	if code != cli.ExitOK || !strings.Contains(out, "Dry run commit preview generated") {
		t.Fatalf("dry run: exit %d\n%s", code, out)
	}
	out, code = cliOutput(t, "commit", "-t", "DEN-1", "--no-smart", "--no-local", "-m", "feat: add b", "--files", "b.txt", "--no-push")
	if code != cli.ExitOK || !strings.Contains(out, "Commit applied successfully") {
		t.Fatalf("commit: exit %d\n%s", code, out)
	}
	if log := clitest.RunGitT(t, repo, "log", "-1", "--format=%s"); !strings.Contains(log, "feat: add b") {
		t.Fatalf("commit not created: %q", log)
	}

	for _, args := range [][]string{
		{"quantal", "--smart", "--no-smart"},
		{"quantal", "--files", "a", "--all"},
		{"quantal", "extra"},
	} {
		if _, code := cliOutput(t, args...); code != cli.ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, cli.ExitUsage)
		}
	}
	t.Setenv("DENDRITE_COMMIT_PUSH", "false")
	if out, code := cliOutput(t, "quantal", "--no-smart", "--no-local", "-m", "nothing", "--no-push"); code != cli.ExitError {
		t.Errorf("committing a clean tree should fail: exit %d\n%s", code, out)
	}
}

// Runners over the commands this package builds (the full tree lives in package main).
var cliOutput = clitest.Output(NewQuantalCmd, NewMyelinCmd, qualitycmd.NewAllostasisCmd, analysiscmd.NewPruneCmd)

func TestMain(m *testing.M) { clitest.Main(m) }
