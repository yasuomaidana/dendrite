package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGoSources(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "package demo\n\nfunc Entry() int { return helper() }\n\nfunc helper() int { return 1 }\n\nfunc unused() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestArbor_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"arbor", "slice", "keep", "parsing"},
		{"arbor", "slice", "--entrypoint", "Entry", "--direction", "sideways"},
		{"arbor", "fuse", "--sources", "only-one", "--name", "x"},
		{"arbor", "fuse", "--name", "x"},
		{"arbor", "reuse"},
		{"arbor", "keep only parsing"},
		{"query", "swap", "extra"},
	} {
		if out, code := cliOutput(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d\n%s", args, code, ExitUsage, out)
		}
	}
}

// The slicer's graph loader is still a stub (pkg/query/slice buildOrLoadGraph returns an
// empty graph), so from the CLI every entrypoint is reported missing.
func TestArbor_SliceErrors(t *testing.T) {
	dir := writeGoSources(t)
	if out, code := cliOutput(t, "arbor", "slice", "--sources", dir, "--entrypoint", "Entry", "keep"); code != ExitError || !strings.Contains(out, "not found in knowledge graph") {
		t.Fatalf("slice: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "arbor", "slice", "--sources", filepath.Join(dir, "missing"), "--entrypoint", "Entry"); code != ExitError || !strings.Contains(out, "sources path does not exist") {
		t.Fatalf("missing sources: exit %d\n%s", code, out)
	}
}

func TestArbor_SwapTranspileReuse(t *testing.T) {
	dir := writeGoSources(t)
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"arbor", "swap", "--sources", dir}, "Transition:       opengl -> wgpu"},
		{[]string{"arbor", "swap", "--sources", dir, "--from", "vulkan", "--to", "threejs", "--lang", "ts", "--json"}, "{"},
		{[]string{"arbor", "transpile", "--sources", dir, "--no-std", "fixed", "point"}, "no_std Compatible:  true"},
		{[]string{"arbor", "transpile", "--sources", dir, "--json"}, "{"},
		{[]string{"arbor", "reuse", "--capability", "totp", "--lang", "go"}, "3-Tier Library Reuse Evaluation"},
		{[]string{"arbor", "reuse", "--capability", "totp", "--json"}, "{"},
	}
	// reuse records ADRs in docs/adr and differential tests in tests/ of the working directory.
	t.Chdir(t.TempDir())
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != ExitOK || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d\n%s", s.args, code, out)
		}
	}
}

func TestArbor_Fuse(t *testing.T) {
	a, b := writeGoSources(t), writeGoSources(t)
	target := filepath.Join(t.TempDir(), "fused")
	out, code := cliOutput(t, "arbor", "fuse", "--sources", a+","+b, "--name", "fused", "--target-dir", target, "--no-register-pool")
	if code != ExitOK || !strings.Contains(out, "Unified Member:      fused") {
		t.Fatalf("fuse: exit %d\n%s", code, out)
	}
}
