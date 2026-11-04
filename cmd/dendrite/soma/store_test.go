package soma

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/store/segmented"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngram_Lifecycle(t *testing.T) {
	clitest.UseTempStore(t)
	file := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(file, []byte(`{"nodes":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"engram", "commit", "--project", "demo", "--chunk", "c1", "--file", file}, cli.ExitOK, "Committed chunk 'c1' for project 'demo'"},
		{[]string{"store", "commit", "--project", "demo", "-c", "c2", "-f", file, "--final"}, cli.ExitOK, "Committed chunk 'c2'"},
		{[]string{"engram", "status"}, cli.ExitOK, "Persistent Segmented Store Status"},
		{[]string{"engram", "status", "--project", "demo", "--json"}, cli.ExitOK, "{"},
		{[]string{"engram", "chunks"}, cli.ExitOK, "Registered Chunk Checkpoints (2)"},
		{[]string{"engram", "checkpoints", "--project", "demo", "--json"}, cli.ExitOK, `"c1"`},
		{[]string{"engram", "query", "--project", "demo"}, cli.ExitOK, "graph.json"},
		{[]string{"engram", "query", "--project", "demo", "--json"}, cli.ExitOK, "["},
		{[]string{"engram", "inspect", "demo"}, cli.ExitOK, "Store Analysis Details: demo"},
		{[]string{"engram", "inspect", "demo", "--json"}, cli.ExitOK, "{"},
		{[]string{"engram", "rm", "demo"}, cli.ExitUsage, "needs --force"},
		{[]string{"engram", "rm", "demo", "--force"}, cli.ExitOK, "Safely removed store 'demo'"},
		{[]string{"engram", "purge", "--project", "demo"}, cli.ExitOK, "purged all segmented analysis data for project: demo"},
		{[]string{"engram", "query"}, cli.ExitUsage, ""},
		{[]string{"engram", "commit", "--project", "demo"}, cli.ExitUsage, ""},
		{[]string{"engram", "commit", "--project", "demo", "--chunk", "c3", "--file", filepath.Join(t.TempDir(), "missing")}, cli.ExitError, "read input file"},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
	if out, code := cliOutput(t, "-y", "engram", "rm", "gone"); code == cli.ExitUsage {
		t.Errorf("--yes should confirm removal, got usage error\n%s", out)
	}
}

func TestEngram_StoreUnavailable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	prev := cli.OpenSegmentedStore
	cli.OpenSegmentedStore = func() (*segmented.SegmentedStore, error) {
		return segmented.NewSegmentedStore(filepath.Join(blocker, "store"))
	}
	t.Cleanup(func() { cli.OpenSegmentedStore = prev })
	clitest.ExpectError(t, cliOutput, [][]string{{"engram", "status"}, {"cognition", "status"}}, "open segmented store")
}

func TestShortHash(t *testing.T) {
	if shortHash("abc") != "abc" || shortHash("0123456789abcdef") != "0123456789ab" {
		t.Fatal("shortHash must not slice past short hashes")
	}
}
