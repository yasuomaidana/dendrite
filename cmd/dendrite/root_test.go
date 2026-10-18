package main

import (
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

// runWith executes args against a bare root holding only cmd, so a command can be tested before it is wired into newRootCmd.
func runWith(ctx context.Context, out io.Writer, cmd *cobra.Command, args ...string) error {
	root := newBareRootCmd()
	root.AddCommand(cmd)
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

// runSynapseCommand runs `dendrite synapse <args...>`.
func runSynapseCommand(ctx context.Context, args []string, out io.Writer) error {
	return runCLI(ctx, out, append([]string{"synapse"}, args...)...)
}

// runSessionCommand runs `dendrite session <args...>`.
func runSessionCommand(ctx context.Context, args []string, out io.Writer) error {
	return runCLI(ctx, out, append([]string{"session"}, args...)...)
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
		if strings.TrimSpace(buf.String()) != "Dendrite CLI v"+version {
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
		{"root help", nil, ExitOK},
		{"unknown root command", []string{"bogus"}, ExitUsage},
		{"unknown subcommand", []string{"synapse", "bogus"}, ExitUsage},
		{"group without args prints help", []string{"bouton", "jobs"}, ExitOK},
		{"unknown flag", []string{"session", "resume", "--nope"}, ExitUsage},
		{"wrong arg count", []string{"synapse", "inspect"}, ExitUsage},
		{"missing required flag", []string{"synapse", "create", "x"}, ExitUsage},
		{"leaf help", []string{"session", "checkpoint", "--help"}, ExitOK},
		{"legacy help", []string{"chronos", "--help"}, ExitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := runCLI(ctx, &buf, tt.args...)
			if got := exitCodeFor(err); got != tt.want {
				t.Fatalf("exit code for %v = %d (err %v); want %d", tt.args, got, err, tt.want)
			}
		})
	}
	if got := exitCodeFor(errors.New("boom")); got != ExitError {
		t.Fatalf("runtime error exit code = %d; want %d", got, ExitError)
	}
}

func TestRunWith_SingleCommand(t *testing.T) {
	var buf bytes.Buffer
	if err := runWith(context.Background(), &buf, newSessionCmd(), "handoff", "--help"); err != nil {
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

func TestExtractGlobalFlags(t *testing.T) {
	globals = globalOptions{}
	out := extractGlobalFlags([]string{"next", "--state-dir", "/s", "--plane-url=http://p", "--json", "-y", "--ticket", "DEN-1"})
	if strings.Join(out, " ") != "next --json -y --ticket DEN-1" {
		t.Fatalf("remaining args = %v", out)
	}
	if globals.StateDir != "/s" || globals.PlaneURL != "http://p" || !globals.JSON || !globals.Yes {
		t.Fatalf("globals not recorded: %+v", globals)
	}
}

func TestGatewayURL(t *testing.T) {
	globals = globalOptions{}
	t.Setenv("LITELLM_PROXY_URL", "")
	if gatewayV1URL() != "http://localhost:4000/v1" {
		t.Fatalf("default gateway = %s", gatewayV1URL())
	}
	t.Setenv("LITELLM_PROXY_URL", "http://gw:4000/v1/")
	if gatewayBaseURL() != "http://gw:4000" {
		t.Fatalf("env gateway base = %s", gatewayBaseURL())
	}
	globals.GatewayURL = "http://flag:1"
	if gatewayV1URL() != "http://flag:1/v1" {
		t.Fatalf("flag gateway = %s", gatewayV1URL())
	}
}

func TestSharedFlagHelpers(t *testing.T) {
	var dry bool
	var ticket, model, project, file string
	c := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	addDryRunFlag(c, &dry)
	addTicketFlag(c, &ticket, "")
	addModelFlag(c, &model, "m")
	addProjectFlag(c, &project, "", "project")
	addFileFlag(c, &file, "", "file")
	if err := c.ParseFlags([]string{"--dry-run", "-t", "DEN-1", "-f", "a.md", "--project", "p"}); err != nil {
		t.Fatal(err)
	}
	if !dry || ticket != "DEN-1" || model != "m" || project != "p" || file != "a.md" {
		t.Fatalf("parsed %v %q %q %q %q", dry, ticket, model, project, file)
	}
	for _, v := range []cobra.PositionalArgs{exactArgs(1), minArgs(2), maxArgs(0), rangeArgs(2, 3)} {
		if err := v(c, []string{"a"}); err != nil && !isUsageError(err) {
			t.Fatalf("validator error is not a usage error: %v", err)
		}
	}
}

func TestInvokeCmd(t *testing.T) {
	if err := invokeCmd(context.Background(), newSessionCmd(), "--help"); err != nil {
		t.Fatalf("invokeCmd help: %v", err)
	}
	if err := invokeCmd(context.Background(), newSessionCmd(), "bogus"); !isUsageError(err) {
		t.Fatalf("invokeCmd bogus = %v; want usage error", err)
	}
}
