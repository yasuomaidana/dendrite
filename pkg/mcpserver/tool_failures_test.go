package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// toolFailure is one tool call that must come back as an error naming what went wrong.
type toolFailure struct {
	name string
	tool string
	args string
	env  map[string]string
	want string
}

func runToolFailures(t *testing.T, srv *Server, cases []toolFailure) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			text, isErr := callTool(t, srv, c.tool, c.args)
			if !isErr || !strings.Contains(text, c.want) {
				t.Fatalf("%s %s = %q (error %v), want an error containing %q", c.tool, c.args, text, isErr, c.want)
			}
		})
	}
}

func TestGraphAndDocusaurusToolsReportFailures(t *testing.T) {
	noEnvPlane(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	dir := testGraphProject(t)
	missing := filepath.Join(t.TempDir(), "missing")
	auditFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(auditFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	runToolFailures(t, srv, []toolFailure{
		{"communities", "dendrite_graph_communities", `{"project_path":"` + dir + `"}`, map[string]string{"STANDIN_FAIL": "graph.communities"}, "graph communities failed"},
		{"god nodes", "dendrite_graph_god_nodes", `{"project_path":"` + dir + `"}`, map[string]string{"STANDIN_FAIL": "graph.god_nodes"}, "graph god nodes failed"},
		{"path", "dendrite_graph_path", `{"project_path":"` + dir + `","from":"a","to":"b"}`, map[string]string{"STANDIN_FAIL": "graph.path"}, "graph path failed"},
		{"explain", "dendrite_graph_explain", `{"project_path":"` + dir + `","target":"a"}`, map[string]string{"STANDIN_FAIL": "graph.explain"}, "graph explain failed"},
		{"a site with an invalid port", "dendrite_docusaurus_generate", `{"port":70000}`, nil, "invalid site configuration"},
		{"a site from missing docs", "dendrite_docusaurus_generate", `{"docs_dir":"` + missing + `","target_dir":"` + filepath.Join(t.TempDir(), "site") + `"}`, nil, "docusaurus generation failed"},
		{"a bundle from a missing site", "dendrite_docusaurus_share", `{"site_dir":"` + missing + `"}`, nil, "failed creating sharing bundle"},
		{"an audit of a project that is a file", "dendrite_docusaurus_audit", `{"project_root":"` + auditFile + `","docs_dir":"` + auditFile + `"}`, nil, "documentation audit failed"},
	})

	h, _ := srv.Handler("dendrite_graph_path")
	if _, err := h(context.Background(), map[string]interface{}{"project_path": dir, "from": "a", "to": "b", "kinds": []string{"calls"}, "max_hops": 3}); err != nil {
		t.Fatalf("typed kinds and an integer hop limit: %v", err)
	}
}
