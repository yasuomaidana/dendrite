package clitest

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func JsonOutput(t *testing.T, on bool) {
	t.Helper()
	old := cli.Globals.JSON
	cli.Globals.JSON = on
	t.Cleanup(func() { cli.Globals.JSON = old })
}

// plainOutput makes the command print its human report whatever an earlier test left in globals.
func PlainOutput(t *testing.T) {
	t.Helper()
	prev := cli.Globals.JSON
	cli.Globals.JSON = false
	t.Cleanup(func() { cli.Globals.JSON = prev })
}

func RunGitT(t *testing.T, dir string, args ...string) string {
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

// runWith executes args against a bare root holding only cmd, so a command can be tested before it is wired into newRootCmd.
func RunWith(ctx context.Context, out io.Writer, cmd *cobra.Command, args ...string) error {
	root := cli.NewBareRootCmd()
	root.AddCommand(cmd)
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}
