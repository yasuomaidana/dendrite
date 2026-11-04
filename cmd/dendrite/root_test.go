package main

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/cmd/dendrite/session"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runCLI executes the real command tree with args, writing command output to out.
func runCLI(ctx context.Context, out io.Writer, args ...string) error {
	root := newRootCmd()
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

func TestRootCmd_HelpShowsLoreGroups(t *testing.T) {
	var buf bytes.Buffer
	if err := runCLI(context.Background(), &buf); err != nil {
		t.Fatalf("root help: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Soma & Axon Hillock:", "Synaptic Cleft & Gating:", "synapse", "session", "bouton", "chronos", "soma", "--json", "--state-dir"} {
		if !strings.Contains(out, want) {
			t.Errorf("root help missing %q:\n%s", want, out)
		}
	}
}

func TestRootCmd_Version(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		var buf bytes.Buffer
		if err := runCLI(context.Background(), &buf, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.TrimSpace(buf.String()) != "Dendrite CLI v"+cli.Version {
			t.Errorf("%v printed %q", args, buf.String())
		}
	}
}

func TestRootCmd_AliasesAndCaseInsensitive(t *testing.T) {
	root := newRootCmd()
	cases := map[string]string{
		"env": "soma", "plane": "chronos", "agent": "bouton", "agt": "bouton", "query": "arbor",
		"ENV": "soma", "Synapse": "synapse", "handoff": "session", "mcp": "inter", "commit": "quantal",
	}
	for in, want := range cases {
		cmd, _, err := root.Find([]string{in})
		if err != nil || cmd.Name() != want {
			t.Errorf("Find(%q) = %v, %v; want %s", in, cmd.Name(), err, want)
		}
	}
}

func TestRootCmd_EveryCommandHasGroupAndShort(t *testing.T) {
	for _, c := range newRootCmd().Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		if c.GroupID == "" {
			t.Errorf("command %q has no help group", c.Name())
		}
		if c.Short == "" {
			t.Errorf("command %q has no Short description", c.Name())
		}
	}
}

func TestExitCodes(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"root help", nil, cli.ExitOK},
		{"unknown root command", []string{"bogus"}, cli.ExitUsage},
		{"unknown subcommand", []string{"synapse", "bogus"}, cli.ExitUsage},
		{"group without args prints help", []string{"bouton", "jobs"}, cli.ExitOK},
		{"unknown flag", []string{"session", "resume", "--nope"}, cli.ExitUsage},
		{"wrong arg count", []string{"synapse", "inspect"}, cli.ExitUsage},
		{"missing required flag", []string{"synapse", "create", "x"}, cli.ExitUsage},
		{"leaf help", []string{"session", "checkpoint", "--help"}, cli.ExitOK},
		{"legacy help", []string{"chronos", "--help"}, cli.ExitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := runCLI(ctx, &buf, tt.args...)
			if got := cli.ExitCodeFor(err); got != tt.want {
				t.Fatalf("exit code for %v = %d (err %v); want %d", tt.args, got, err, tt.want)
			}
		})
	}
	if got := cli.ExitCodeFor(errors.New("boom")); got != cli.ExitError {
		t.Fatalf("runtime error exit code = %d; want %d", got, cli.ExitError)
	}
}

func TestRunWith_SingleCommand(t *testing.T) {
	var buf bytes.Buffer
	if err := clitest.RunWith(context.Background(), &buf, session.NewSessionCmd(), "handoff", "--help"); err != nil {
		t.Fatalf("runWith: %v", err)
	}
	if !strings.Contains(buf.String(), "checkpoint") {
		t.Fatalf("expected session help, got:\n%s", buf.String())
	}
}

func TestRunGroup_Suggestions(t *testing.T) {
	var buf bytes.Buffer
	err := runCLI(context.Background(), &buf, "session", "resme")
	if err == nil || !strings.Contains(err.Error(), "resume") {
		t.Fatalf("expected suggestion for 'resme', got %v", err)
	}
}

func TestGlobalFlags_ApplyToEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", "")
	var buf bytes.Buffer
	// resume fails without a checkpoint, but the persistent flag must already be applied.
	_ = runCLI(context.Background(), &buf, "--state-dir", dir, "session", "resume")
	if got := os.Getenv("DENDRITE_STATE_DIR"); got != dir {
		t.Fatalf("DENDRITE_STATE_DIR = %q; want %q", got, dir)
	}
}

// TestEveryLeafHasHelp runs `<path> --help` for every runnable command in the tree.
func TestEveryLeafHasHelp(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			path := strings.Fields(sub.CommandPath())[1:]
			var buf bytes.Buffer
			if err := runCLI(context.Background(), &buf, append(path, "--help")...); err != nil {
				t.Errorf("%v --help: %v", path, err)
			} else if !strings.Contains(buf.String(), "Usage:") && !sub.DisableFlagParsing {
				t.Errorf("%v --help printed no usage: %q", path, buf.String())
			}
			walk(sub)
		}
	}
	walk(newRootCmd())
}

func TestMain(m *testing.M) { clitest.Main(m) }
