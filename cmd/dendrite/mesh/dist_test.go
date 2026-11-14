package mesh

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/cmd/dendrite/internal/planecli"
	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/workspace"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestMesh_ReadCommands(t *testing.T) {
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"mesh", "sources"}, cli.ExitOK, "Distributed Codebase Sources"},
		{[]string{"dist", "list-sources", "--json"}, cli.ExitOK, `"libraries"`},
		{[]string{"mesh", "decompose", "emu", "spec.md"}, cli.ExitOK, "Forced Modular Decomposition Plan for 'emu'"},
		{[]string{"mesh", "decompose", "emu", "--json"}, cli.ExitOK, "{"},
		{[]string{"mesh", "resolve", "no-such-capability-xyz"}, cli.ExitError, ""},
		{[]string{"mesh", "resolve"}, cli.ExitUsage, ""},
		{[]string{"mesh", "interop", "list"}, cli.ExitOK, "Active Interop Coordinations (0)"},
		{[]string{"mesh", "interop", "complete", "missing"}, cli.ExitError, ""},
		{[]string{"mesh", "interop", "reconcile", "missing"}, cli.ExitError, ""},
		{[]string{"mesh", "interop", "coordinate", "a", "b"}, cli.ExitUsage, ""},
		{[]string{"mesh", "spawn", "only-name"}, cli.ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}

	// Pick a capability the default registry knows about and resolve it.
	out, _ := cliOutput(t, "mesh", "sources")
	i := strings.Index(out, "Capabilities: ")
	if i < 0 {
		t.Fatal("no capabilities listed")
	}
	capability := strings.TrimSpace(strings.Split(strings.SplitN(out[i+len("Capabilities: "):], "\n", 2)[0], ",")[0])
	if out, code := cliOutput(t, "mesh", "resolve", capability); code != cli.ExitOK || !strings.Contains(out, "Highest Priority Match") {
		t.Fatalf("resolve %q: exit %d\n%s", capability, code, out)
	}
	if out, code := cliOutput(t, "mesh", "resolve", capability, "--json"); code != cli.ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("resolve --json: exit %d\n%s", code, out)
	}
}

func TestMesh_InteropInOneProcess(t *testing.T) {
	cmd := NewMeshCmd()
	var buf bytes.Buffer
	if err := clitest.RunWith(context.Background(), &buf, cmd, "mesh", "interop", "coordinate", "app", "lib", "streaming"); err != nil {
		t.Fatalf("coordinate: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "Interop Coordination Initialized") {
		t.Fatalf("coordinate output: %s", buf.String())
	}
}

func TestMesh_Spawn(t *testing.T) {
	clitest.UseTempRegistry(t)
	prev := planecli.NewPlaneClient
	planecli.NewPlaneClient = func() (*plane.Client, error) { return nil, errors.New("no Plane") }
	t.Cleanup(func() { planecli.NewPlaneClient = prev })

	dir := filepath.Join(t.TempDir(), "svc")
	out, code := cliOutput(t, "mesh", "spawn", "svc", dir, "go", "--satellite", "--identifier", "svc", "--desc", "demo")
	if code != cli.ExitOK || !strings.Contains(out, "Spawned Project 'svc'") {
		t.Fatalf("spawn: exit %d\n%s", code, out)
	}
	if _, err := workspace.NewRegistry(filepath.Join(t.TempDir(), "c.json"), t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
