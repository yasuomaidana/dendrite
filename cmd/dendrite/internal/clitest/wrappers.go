package clitest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/spf13/cobra"

	"Dendrite/cmd/dendrite/internal/cli"
)

// RootWith is a bare root command holding only cmds.
func RootWith(cmds ...*cobra.Command) *cobra.Command {
	root := cli.NewBareRootCmd()
	root.AddCommand(cmds...)
	return root
}

// Runner runs a command line against a root holding only the commands newCmds build: the way the tests of one
// command package execute it, since the full tree lives in package main.
func Runner(newCmds ...func() *cobra.Command) func(ctx context.Context, out io.Writer, args ...string) error {
	return func(ctx context.Context, out io.Writer, args ...string) error {
		cmds := make([]*cobra.Command, len(newCmds))
		for i, newCmd := range newCmds {
			cmds[i] = newCmd()
		}
		root := RootWith(cmds...)
		root.SetOut(out)
		root.SetErr(out)
		root.SetArgs(args)
		return root.ExecuteContext(ctx)
	}
}

// Output runs a command line the way a user would and returns what they would see (the output, then any error
// the way main prints it) and the exit code.
func Output(newCmds ...func() *cobra.Command) func(t *testing.T, args ...string) (string, int) {
	run := Runner(newCmds...)
	return func(t *testing.T, args ...string) (string, int) {
		t.Helper()
		var buf bytes.Buffer
		err := run(context.Background(), &buf, args...)
		if err != nil {
			fmt.Fprintf(&buf, "[-] %v\n", err)
		}
		return buf.String(), cli.ExitCodeFor(err)
	}
}

// UsageErrors runs each command line through output and requires the usage exit code.
func UsageErrors(output func(t *testing.T, args ...string) (string, int)) func(t *testing.T, cases [][]string) {
	return func(t *testing.T, cases [][]string) {
		t.Helper()
		for _, args := range cases {
			if _, code := output(t, args...); code != cli.ExitUsage {
				t.Errorf("%v: exit %d, want %d", args, code, cli.ExitUsage)
			}
		}
	}
}
