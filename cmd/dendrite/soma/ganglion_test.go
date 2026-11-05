package soma

import (
	"Dendrite/cmd/dendrite/analysis"
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/workspace"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGanglion_Lifecycle(t *testing.T) {
	clitest.UseTempRegistry(t)
	ctx := context.Background()
	code := t.TempDir()

	steps := []struct {
		args []string
		want string
	}{
		{[]string{"ganglion", "add", "alpha", code}, "Project 'alpha' registered"},
		{[]string{"project", "switch", "alpha"}, "Switched active context to 'alpha'"},
		{[]string{"ganglion", "list"}, "[*] alpha"},
		{[]string{"ganglion", "list", "--json"}, `"name": "alpha"`},
		{[]string{"ganglion", "rm", "alpha", "--purge"}, "Project 'alpha' removed (Purge Data: true)"},
		{[]string{"ganglion", "list", "--json"}, "[]"},
	}
	for _, s := range steps {
		var buf bytes.Buffer
		if err := runCmd(ctx, &buf, s.args...); err != nil {
			t.Fatalf("%v: %v\n%s", s.args, err, buf.String())
		}
		if !strings.Contains(buf.String(), s.want) {
			t.Errorf("%v: want %q in %q", s.args, s.want, buf.String())
		}
	}
}

func TestGanglion_Errors(t *testing.T) {
	clitest.UseTempRegistry(t)
	ctx := context.Background()
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"ganglion", "add", "only-name"}, cli.ExitUsage},
		{[]string{"ganglion", "switch", "missing"}, cli.ExitError},
		{[]string{"ganglion", "remove", "missing"}, cli.ExitError},
		{[]string{"ganglion", "list", "extra"}, cli.ExitUsage},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		err := runCmd(ctx, &buf, c.args...)
		if got := cli.ExitCodeFor(err); got != c.code {
			t.Errorf("%v: exit %d, want %d (err %v)", c.args, got, c.code, err)
		}
	}
}

func TestGanglion_RegistryUnavailable(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "config.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := cli.NewProjectRegistry
	cli.NewProjectRegistry = func() (*workspace.Registry, error) { return workspace.NewRegistry(bad, dir) }
	t.Cleanup(func() { cli.NewProjectRegistry = prev })

	var buf bytes.Buffer
	err := runCmd(context.Background(), &buf, "ganglion", "list")
	if err == nil || !strings.Contains(err.Error(), "open workspace registry") {
		t.Fatalf("want registry error, got %v", err)
	}
}

// Runners over the commands this package builds (the full tree lives in package main).
var runCmd = clitest.Runner(NewGanglionCmd, NewPlexusCmd, NewEngramCmd, NewWorkCmd, analysis.NewCognitionCmd)
var cliOutput = clitest.Output(NewGanglionCmd, NewPlexusCmd, NewEngramCmd, NewWorkCmd, analysis.NewCognitionCmd)

func TestMain(m *testing.M) { clitest.Main(m) }
