package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGoModule(t *testing.T, root, name, requires string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mod := "module example.com/" + name + "\n\ngo 1.22\n"
	if requires != "" {
		mod += "\nrequire example.com/" + requires + " v0.0.0\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPlexus_Lifecycle(t *testing.T) {
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	root := t.TempDir()
	core := writeGoModule(t, root, "core", "")
	api := writeGoModule(t, root, "api", "core")

	if out, code := cliOutput(t, "plexus", "resolve"); code != ExitError {
		t.Fatalf("resolve on an empty pool: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "plexus", "list"); code != ExitOK || !strings.Contains(out, "No projects registered") {
		t.Fatalf("empty list: exit %d\n%s", code, out)
	}
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"plexus", "add", core, api}, "registered 2 project(s)"},
		{[]string{"pool", "ls"}, "Registered Pool Members (2)"},
		{[]string{"plexus", "list", "--json"}, `"go.mod"`},
		{[]string{"plexus", "resolve"}, "Topological Build Order"},
		{[]string{"plexus", "resolve", "--json"}, "{"},
		{[]string{"plexus", "rm", "api"}, "Removed 'api' from pool."},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != ExitOK || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d\n%s", s.args, code, out)
		}
	}
	if _, code := cliOutput(t, "plexus", "add", filepath.Join(root, "missing")); code != ExitError {
		t.Errorf("adding a missing directory should fail: exit %d", code)
	}
}

func TestPlexus_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"plexus", "add"},
		{"plexus", "switch", "core"},
		{"plexus", "split", "--module", "pkg/x"},
		{"plexus", "remove"},
	} {
		if out, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d\n%s", args, code, ExitUsage, out)
		}
	}
}

func TestPlexus_SwitchAndSplitFailures(t *testing.T) {
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	dir := t.TempDir()
	if _, code := cliOutput(t, "plexus", "switch", "core", "--in", dir, "--mode", "bogus"); code != ExitError {
		t.Errorf("switch with a bad mode: exit %d", code)
	}
	if _, code := cliOutput(t, "plexus", "split", "--from", dir, "--module", "nope", "--name", "x", "--target-dir", filepath.Join(dir, "out")); code != ExitError {
		t.Errorf("split of a missing module: exit %d", code)
	}
}
