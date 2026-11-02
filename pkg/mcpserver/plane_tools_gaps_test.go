package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaneSyncToolsReportEachFailure(t *testing.T) {
	dir := t.TempDir()
	ledger := filepath.Join(dir, "DEV_STATUS.md")
	if err := os.WriteFile(ledger, []byte("### Milestone 1: A\n- [ ] **DEN-1**: one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	healthy := []apiRule{
		on("GET", "/states/", 200, `{"results":[]}`),
		on("GET", "/modules/", 200, `{"results":[]}`),
		on("GET", "/issues/", 200, `{"results":[]}`),
		on("GET", "/projects/", 200, `{"results":[]}`),
		on("GET", "/pages/", 200, `[]`),
	}
	with := func(first apiRule) []apiRule { return append([]apiRule{first}, healthy...) }

	cases := []struct {
		name  string
		rules []apiRule
		tool  string
		args  string
		want  string
	}{
		{"replicate: Plane cannot be read", with(on("GET", "/issues/", 500, "")), "dendrite_plane_replicate", `{"dry_run":true}`, "replication failed"},
		{"replicate: the file cannot be written", healthy, "dendrite_plane_replicate", `{"file":"` + filepath.Join(dir, "no", "such", "dir", "out.md") + `"}`, "failed to write"},
		{"push: the ledger is missing", healthy, "dendrite_plane_push_status", `{"file":"` + filepath.Join(dir, "missing.md") + `"}`, "failed to read"},
		{"push: Plane refuses the states", with(on("GET", "/states/", 500, "")), "dendrite_plane_push_status", `{"file":"` + ledger + `"}`, "push to plane failed"},
		{"docs: Plane cannot list pages", with(on("GET", "/pages/", 500, "")), "dendrite_plane_doc_sync", `{"direction":"pull","dir":"` + dir + `"}`, "documentation sync failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := toolsWith(t, newScriptedPlane(t, c.rules...), "")
			if text, isErr := callTool(t, srv, c.tool, c.args); !isErr || !strings.Contains(text, c.want) {
				t.Fatalf("text = %q (error %v), want %q", text, isErr, c.want)
			}
		})
	}

	t.Run("without a Plane client", func(t *testing.T) {
		noEnvPlane(t)
		srv := toolsWith(t, nil, "")
		for _, tool := range []string{"dendrite_plane_replicate", "dendrite_plane_push_status", "dendrite_plane_doc_sync"} {
			if text, isErr := callTool(t, srv, tool, `{}`); !isErr || !strings.Contains(text, "plane client error") {
				t.Errorf("%s: %q (error %v)", tool, text, isErr)
			}
		}
	})
	t.Run("document directions", func(t *testing.T) {
		srv := toolsWith(t, newScriptedPlane(t, healthy...), "")
		for _, direction := range []string{"to-plane", "push", "from-plane", "pull", "anything else", ""} {
			if text, isErr := callTool(t, srv, "dendrite_plane_doc_sync", `{"direction":"`+direction+`","dir":"`+dir+`"}`); isErr {
				t.Errorf("%q: %s", direction, text)
			}
		}
	})
}

func TestJetBrainsAndCommitToolsReportFailures(t *testing.T) {
	srv := toolsWith(t, nil, "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := callTool(t, srv, "dendrite_jetbrains_setup", `{"project_dir":"`+file+`","project_only":true}`); !isErr || !strings.Contains(text, "jetbrains setup failed") {
		t.Errorf("jetbrains: %q (error %v)", text, isErr)
	}

	t.Chdir(t.TempDir()) // not a git repository
	if text, isErr := callTool(t, srv, "dendrite_commit", `{"message":"m","files":["a.go",7],"stage_all":false,"use_local_model":false,"dry_run":true}`); !isErr || !strings.Contains(text, "commit failed") {
		t.Errorf("commit: %q (error %v)", text, isErr)
	}
}
