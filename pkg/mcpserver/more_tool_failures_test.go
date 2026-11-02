package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/query/reuse"
)

// inEmptyDir runs the test in a fresh directory that is not a git repository and whose .dendrite
// is a file, so every tool that reads the working tree or its state fails.
func inEmptyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, ".dendrite"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWorkingTreeToolsReportANonRepository(t *testing.T) {
	inEmptyDir(t)
	noEnvPlane(t)
	srv := toolsWith(t, nil, "")
	runToolFailures(t, srv, []toolFailure{
		{"audit changes", "dendrite_audit_changes", `{"branch":"x","dry_run":true,"max_tokens":100,"model":"m"}`, nil, "audit changes failed"},
	})
	if text, isErr := callTool(t, srv, "dendrite_vcs_guard", `{}`); isErr || !strings.Contains(text, "ALERT") {
		t.Errorf("a guard outside a repository alerts: %q (error %v)", text, isErr)
	}
}

// ownRefs points the reference tools at a refs file in the test's temporary directory.
func ownRefs(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "refs.json")
	old := defaultMCPRefMgr
	defaultMCPRefMgr = reuse.NewRefManager(path)
	t.Cleanup(func() { defaultMCPRefMgr = old })
	return path
}

func TestQualityAndQueryToolsReportFailures(t *testing.T) {
	inEmptyDir(t)
	refs := ownRefs(t)
	srv := toolsWith(t, nil, "")
	missing := filepath.Join(t.TempDir(), "missing")
	runToolFailures(t, srv, []toolFailure{
		{"quality audit without a package", "dendrite_quality_audit", `{}`, nil, "missing pkg"},
		{"slice", "dendrite_query_slice", `{"sources_path":"` + missing + `","entrypoint_symbol":"Main"}`, nil, "Slicing failed"},
		{"swap without sources", "dendrite_query_swap", `{"sources_path":"","from_subsystem":"opengl","to_subsystem":"wgpu"}`, nil, "Subsystem swap failed"},
		{"transpile", "dendrite_query_transpile", `{"sources_artifact":""}`, nil, "Transpilation failed"},
		{"fuse with fewer than two sources", "dendrite_query_fuse", `{"sources":["a"],"fused_member_name":"m","target_directory":"t"}`, nil, "at least two"},
		{"fuse without a target", "dendrite_query_fuse", `{"sources":["a","b"],"fused_member_name":"m","target_directory":""}`, nil, "Architecture fusion failed"},
		{"reuse audit without a capability", "dendrite_query_reuse_audit", `{"capability":""}`, nil, "reuse audit failed"},
		{"ref add twice", "dendrite_ref_add", `{"id":"ref-oxipix","repository":"https://example.com/r.git"}`, nil, "Failed to register ref"},
	})
	if err := os.WriteFile(refs, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	runToolFailures(t, srv, []toolFailure{{"ref list of a corrupt file", "dendrite_ref_list", `{}`, nil, "Failed to list refs"}})
}

func TestPoolAndSchedulerToolsReportFailures(t *testing.T) {
	inEmptyDir(t)
	srv := toolsWith(t, nil, "")
	missing := filepath.Join(t.TempDir(), "missing")
	runToolFailures(t, srv, []toolFailure{
		{"resolve a missing directory", "dendrite_pool_resolve", `{"directories":["` + missing + `",5]}`, nil, "scanning"},
		{"add a missing directory", "dendrite_pool_add", `{"directories":["` + missing + `"]}`, nil, "Failed adding projects"},
		{"switch to an unknown mode", "dendrite_pool_switch", `{"target_project_dir":"` + missing + `","dependency_name":"d","mode":"bogus"}`, nil, "mode switch failed"},
		{"split a missing project", "dendrite_pool_split", `{"source_project_dir":"` + missing + `","module_subpath":"x","new_member_name":"n","target_directory":"` + filepath.Join(t.TempDir(), "t") + `"}`, nil, "split failed"},
		{"a policy that is not valid", "dendrite_analyze_policy", `{"reserve_floor":7}`, nil, "updating scheduler policy"},
		{"trigger without a chunk", "dendrite_analyze_trigger", `{}`, nil, "missing chunk_id"},
	})
}

func TestDistAndPerfToolsReportFailures(t *testing.T) {
	inEmptyDir(t)
	noEnvPlane(t)
	srv := toolsWith(t, nil, "")
	runToolFailures(t, srv, []toolFailure{
		{"coordinate without a project", "dendrite_dist_interop", `{"action":"coordinate"}`, nil, "interop coordination failed"},
		{"spawn without a name", "dendrite_dist_spawn", `{"project_name":"","create_plane_project":false,"init_git":false}`, nil, "spawning project failed"},
	})
	if text, isErr := callTool(t, srv, "dendrite_dist_sources", `{"language":"go","priority":1}`); isErr {
		t.Errorf("sources with filters: %s", text)
	}
}
