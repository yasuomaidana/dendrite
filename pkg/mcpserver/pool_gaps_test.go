package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/pool/dag"
)

func TestPlaneClientOfBuildsFromTheEnvironment(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "token")
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("PLANE_PROJECT_ID", "proj")
	c, err := planeClientOf(Dependencies{})
	if err != nil || c == nil {
		t.Fatalf("client = %v, err = %v", c, err)
	}
}

func goProject(t *testing.T, module string, requires ...string) string {
	t.Helper()
	dir := t.TempDir()
	mod := "module " + module + "\n\ngo 1.22\n"
	for _, r := range requires {
		mod += "\nrequire " + r + " v0.0.0\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPoolResolveSkipsNonStringsAndReportsACycle(t *testing.T) {
	a := goProject(t, "example.com/a", "example.com/b")
	b := goProject(t, "example.com/b", "example.com/a")
	srv := toolsWith(t, nil, "")
	if text, isErr := callTool(t, srv, "dendrite_pool_resolve", `{"directories":[5,"`+a+`","`+b+`"]}`); isErr && !strings.Contains(text, "resolution failed") {
		t.Errorf("text = %q", text)
	}
}

func TestPoolAddReportsAConfigItCannotSave(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := defaultMCPPoolManager
	defaultMCPPoolManager = dag.NewPoolManager(filepath.Join(blocker, "pool.json"))
	t.Cleanup(func() { defaultMCPPoolManager = old })
	srv := toolsWith(t, nil, "")
	dir := goProject(t, "example.com/c")
	if text, isErr := callTool(t, srv, "dendrite_pool_add", `{"directories":["`+dir+`"]}`); !isErr || !strings.Contains(text, "saving pool config") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestPoolSplitExtractsAModule(t *testing.T) {
	src := goProject(t, "example.com/src")
	mod := filepath.Join(src, "internal", "matcher")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "m.go"), []byte("package matcher\n\nfunc Match(a, b string) bool { return a == b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "m_test.go"), []byte("package matcher\n\nimport \"testing\"\n\nfunc TestMatch(t *testing.T) {\n\tif !Match(\"a\", \"a\") {\n\t\tt.Fatal()\n\t}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := toolsWith(t, nil, "")
	target := filepath.Join(t.TempDir(), "matcher")
	text, isErr := callTool(t, srv, "dendrite_pool_split", `{"source_project_dir":"`+src+`","module_subpath":"internal/matcher","new_member_name":"matcher","target_directory":"`+target+`"}`)
	t.Logf("split: %q (error %v)", text, isErr)
}

func TestQuerySliceOfASmallProject(t *testing.T) {
	src := goProject(t, "example.com/slice")
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc used() int { return 1 }\n\nfunc unused() int { return 2 }\n\nfunc main() { _ = used() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := toolsWith(t, nil, "")
	text, isErr := callTool(t, srv, "dendrite_query_slice", `{"sources_path":"`+src+`","entrypoint_symbol":"main","direction":"forward"}`)
	t.Logf("slice: %q (error %v)", text, isErr)
}
