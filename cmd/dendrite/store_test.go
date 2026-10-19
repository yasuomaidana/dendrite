package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/store/segmented"
)

// useTempStore points engram and cognition at a segmented store under t.TempDir().
func useTempStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	prev := openSegmentedStore
	openSegmentedStore = func() (*segmented.SegmentedStore, error) { return segmented.NewSegmentedStore(dir) }
	t.Cleanup(func() { openSegmentedStore = prev })
}

func TestEngram_Lifecycle(t *testing.T) {
	useTempStore(t)
	file := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(file, []byte(`{"nodes":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"engram", "commit", "--project", "demo", "--chunk", "c1", "--file", file}, ExitOK, "Committed chunk 'c1' for project 'demo'"},
		{[]string{"store", "commit", "--project", "demo", "-c", "c2", "-f", file, "--final"}, ExitOK, "Committed chunk 'c2'"},
		{[]string{"engram", "status"}, ExitOK, "Persistent Segmented Store Status"},
		{[]string{"engram", "status", "--project", "demo", "--json"}, ExitOK, "{"},
		{[]string{"engram", "chunks"}, ExitOK, "Registered Chunk Checkpoints (2)"},
		{[]string{"engram", "checkpoints", "--project", "demo", "--json"}, ExitOK, `"c1"`},
		{[]string{"engram", "query", "--project", "demo"}, ExitOK, "graph.json"},
		{[]string{"engram", "query", "--project", "demo", "--json"}, ExitOK, "["},
		{[]string{"engram", "inspect", "demo"}, ExitOK, "Store Analysis Details: demo"},
		{[]string{"engram", "inspect", "demo", "--json"}, ExitOK, "{"},
		{[]string{"engram", "rm", "demo"}, ExitUsage, "needs --force"},
		{[]string{"engram", "rm", "demo", "--force"}, ExitOK, "Safely removed store 'demo'"},
		{[]string{"engram", "purge", "--project", "demo"}, ExitOK, "purged all segmented analysis data for project: demo"},
		{[]string{"engram", "query"}, ExitUsage, ""},
		{[]string{"engram", "commit", "--project", "demo"}, ExitUsage, ""},
		{[]string{"engram", "commit", "--project", "demo", "--chunk", "c3", "--file", filepath.Join(t.TempDir(), "missing")}, ExitError, "read input file"},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
	if out, code := cliOutput(t, "-y", "engram", "rm", "gone"); code == ExitUsage {
		t.Errorf("--yes should confirm removal, got usage error\n%s", out)
	}
}

func TestEngram_StoreUnavailable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	prev := openSegmentedStore
	openSegmentedStore = func() (*segmented.SegmentedStore, error) { return segmented.NewSegmentedStore(filepath.Join(blocker, "store")) }
	t.Cleanup(func() { openSegmentedStore = prev })
	for _, args := range [][]string{{"engram", "status"}, {"cognition", "status"}} {
		if out, code := cliOutput(t, args...); code != ExitError || !strings.Contains(out, "open segmented store") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
}

func TestShortHash(t *testing.T) {
	if shortHash("abc") != "abc" || shortHash("0123456789abcdef") != "0123456789ab" {
		t.Fatal("shortHash must not slice past short hashes")
	}
}
