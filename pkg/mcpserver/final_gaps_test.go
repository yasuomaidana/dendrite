package mcpserver

import (
	"Dendrite/pkg/testkit"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/query/slice"
)

func TestCanvasToolsReportBadRequests(t *testing.T) {
	noEnvPlane(t)
	srv := toolsWith(t, nil, "")
	runToolFailures(t, srv, []toolFailure{
		{"an unknown action", "dendrite_canvas_action_dispatch", `{"action":"no-such-action","prompt":"p"}`, nil, "unknown"},
		{"a relation without a ticket", "dendrite_canvas_penpot_bridge", `{"subaction":"relate"}`, nil, ""},
		{"a state update without a ticket", "dendrite_canvas_penpot_bridge", `{"subaction":"update_state"}`, nil, ""},
	})
}

func TestStoreStatusReportsMissingCheckpoints(t *testing.T) {
	dir := ownStore(t)
	srv := toolsWith(t, nil, "")
	if out, err := exec.Command("sqlite3", filepath.Join(dir, "meta.db"), "DROP TABLE checkpoints").CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, out)
	}
	runToolFailures(t, srv, []toolFailure{{"status", "dendrite_store_status", `{"project":"p"}`, nil, "listing checkpoints"}})
}

// fixedSlicer answers every slice request with the same result.
type fixedSlicer struct{ result *slice.SliceResult }

func (f fixedSlicer) Slice(context.Context, slice.SliceRequest) (*slice.SliceResult, error) {
	return f.result, nil
}

func TestQuerySliceAnswersWithTheSlice(t *testing.T) {
	old := defaultMCPQuerySlicer
	defaultMCPQuerySlicer = fixedSlicer{result: &slice.SliceResult{}}
	t.Cleanup(func() { defaultMCPQuerySlicer = old })
	srv := toolsWith(t, nil, "")
	if text, isErr := callTool(t, srv, "dendrite_query_slice", `{"sources_path":".","entrypoint_symbol":"main"}`); isErr || !strings.Contains(text, "{") {
		t.Fatalf("slice: %q (error %v)", text, isErr)
	}
}

func TestPenpotSyncAndModelMigrationDryRuns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv := toolsWith(t, nil, "")

	design := filepath.Join(t.TempDir(), "Button.tsx")
	if err := os.WriteFile(design, []byte("export const Button = () => null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := callTool(t, srv, "dendrite_ui_penpot_sync", `{"file":"`+design+`","dry_run":true,"comment_plane":false}`); isErr {
		t.Errorf("penpot sync: %s", text)
	}

	manifest := filepath.Join(home, ".ollama", "models", "manifests", "registry.ollama.ai", "library", "tiny")
	if err := os.MkdirAll(manifest, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]interface{}{"schemaVersion": 2, "config": map[string]interface{}{"digest": "sha256:aa", "size": 2}, "layers": []interface{}{}})
	if err := os.WriteFile(filepath.Join(manifest, "latest"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := callTool(t, srv, "dendrite_inference_migrate_model", `{"model":"tiny","dry_run":true}`); isErr {
		t.Errorf("model migration: %s", text)
	}
}

func gitIn(t *testing.T, dir string, args ...string) { testkit.Git(t, dir, args...) }

func TestRemoteSyncReportsAConflictingRebase(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	local := filepath.Join(root, "local")
	other := filepath.Join(root, "other")
	gitIn(t, root, "init", "-q", "--bare", "-b", "main", remote)
	gitIn(t, root, "clone", "-q", remote, local)
	if err := os.WriteFile(filepath.Join(local, "f.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, local, "add", ".")
	gitIn(t, local, "commit", "-q", "-m", "base")
	gitIn(t, local, "push", "-q", "origin", "HEAD:main")
	gitIn(t, local, "branch", "-M", "main")
	gitIn(t, local, "branch", "--set-upstream-to=origin/main", "main")

	gitIn(t, root, "clone", "-q", remote, other)
	if err := os.WriteFile(filepath.Join(other, "f.txt"), []byte("remote change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, other, "commit", "-q", "-am", "remote")
	gitIn(t, other, "push", "-q", "origin", "HEAD:main")

	if err := os.WriteFile(filepath.Join(local, "f.txt"), []byte("local change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, local, "commit", "-q", "-am", "local")

	t.Chdir(local)
	srv := toolsWith(t, nil, "")
	runToolFailures(t, srv, []toolFailure{{"remote sync", "dendrite_remote_sync", `{"branch":"main"}`, nil, "remote sync failed"}})
	gitIn(t, local, "rebase", "--abort")
}

func TestQualityAuditReportsAScanThatCannotRun(t *testing.T) {
	t.Chdir(t.TempDir()) // not a git repository
	srv := toolsWith(t, nil, "")
	runToolFailures(t, srv, []toolFailure{{"quality audit", "dendrite_quality_audit", `{"pkg":"pkg/none"}`, nil, "anti-mock scan failed"}})
}
