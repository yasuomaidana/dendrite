package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/workspace"
)

func TestMesh_ReadCommands(t *testing.T) {
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"mesh", "sources"}, ExitOK, "Distributed Codebase Sources"},
		{[]string{"dist", "list-sources", "--json"}, ExitOK, `"libraries"`},
		{[]string{"mesh", "decompose", "emu", "spec.md"}, ExitOK, "Forced Modular Decomposition Plan for 'emu'"},
		{[]string{"mesh", "decompose", "emu", "--json"}, ExitOK, "{"},
		{[]string{"mesh", "resolve", "no-such-capability-xyz"}, ExitError, ""},
		{[]string{"mesh", "resolve"}, ExitUsage, ""},
		{[]string{"mesh", "interop", "list"}, ExitOK, "Active Interop Coordinations (0)"},
		{[]string{"mesh", "interop", "complete", "missing"}, ExitError, ""},
		{[]string{"mesh", "interop", "reconcile", "missing"}, ExitError, ""},
		{[]string{"mesh", "interop", "coordinate", "a", "b"}, ExitUsage, ""},
		{[]string{"mesh", "spawn", "only-name"}, ExitUsage, ""},
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
	if out, code := cliOutput(t, "mesh", "resolve", capability); code != ExitOK || !strings.Contains(out, "Highest Priority Match") {
		t.Fatalf("resolve %q: exit %d\n%s", capability, code, out)
	}
	if out, code := cliOutput(t, "mesh", "resolve", capability, "--json"); code != ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("resolve --json: exit %d\n%s", code, out)
	}
}

func TestMesh_InteropInOneProcess(t *testing.T) {
	cmd := newMeshCmd()
	var buf bytes.Buffer
	if err := runWith(context.Background(), &buf, cmd, "mesh", "interop", "coordinate", "app", "lib", "streaming"); err != nil {
		t.Fatalf("coordinate: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "Interop Coordination Initialized") {
		t.Fatalf("coordinate output: %s", buf.String())
	}
}

func TestMesh_Spawn(t *testing.T) {
	useTempRegistry(t)
	prev := newPlaneClient
	newPlaneClient = func() (*plane.Client, error) { return nil, errors.New("no Plane") }
	t.Cleanup(func() { newPlaneClient = prev })

	dir := filepath.Join(t.TempDir(), "svc")
	out, code := cliOutput(t, "mesh", "spawn", "svc", dir, "go", "--satellite", "--identifier", "svc", "--desc", "demo")
	if code != ExitOK || !strings.Contains(out, "Spawned Project 'svc'") {
		t.Fatalf("spawn: exit %d\n%s", code, out)
	}
	if _, err := workspace.NewRegistry(filepath.Join(t.TempDir(), "c.json"), t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
