package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/workspace"
)

// useTempRegistry points the ganglion commands at a registry under t.TempDir().
func useTempRegistry(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := newProjectRegistry
	newProjectRegistry = func() (*workspace.Registry, error) {
		return workspace.NewRegistry(filepath.Join(dir, "config.json"), filepath.Join(dir, "projects"))
	}
	t.Cleanup(func() { newProjectRegistry = prev })
	return dir
}

func TestGanglion_Lifecycle(t *testing.T) {
	useTempRegistry(t)
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
		if err := runCLI(ctx, &buf, s.args...); err != nil {
			t.Fatalf("%v: %v\n%s", s.args, err, buf.String())
		}
		if !strings.Contains(buf.String(), s.want) {
			t.Errorf("%v: want %q in %q", s.args, s.want, buf.String())
		}
	}
}

func TestGanglion_Errors(t *testing.T) {
	useTempRegistry(t)
	ctx := context.Background()
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"ganglion", "add", "only-name"}, ExitUsage},
		{[]string{"ganglion", "switch", "missing"}, ExitError},
		{[]string{"ganglion", "remove", "missing"}, ExitError},
		{[]string{"ganglion", "list", "extra"}, ExitUsage},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		err := runCLI(ctx, &buf, c.args...)
		if got := exitCodeFor(err); got != c.code {
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
	prev := newProjectRegistry
	newProjectRegistry = func() (*workspace.Registry, error) { return workspace.NewRegistry(bad, dir) }
	t.Cleanup(func() { newProjectRegistry = prev })

	var buf bytes.Buffer
	err := runCLI(context.Background(), &buf, "ganglion", "list")
	if err == nil || !strings.Contains(err.Error(), "open workspace registry") {
		t.Fatalf("want registry error, got %v", err)
	}
}
