package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ragDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package demo\n\n// LocateBinary finds the sidecar executable.\nfunc LocateBinary(name string) string { return name }\n"
	if err := os.WriteFile(filepath.Join(dir, "pkg", "demo", "demo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRAGAskArgumentsAndFailures(t *testing.T) {
	noEnvPlane(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	dir := ragDir(t)

	cases := []struct {
		name string
		args string
		want string
		fail bool
	}{
		{"a paid model, an explicit top_k and a share", `{"question":"LocateBinary","project_path":"` + dir + `","paid_model":"some-model","top_k":3,"docs_share":0.5}`, "local answer", false},
		{"a share without top_k asks for the context pack", `{"question":"LocateBinary","project_path":"` + dir + `","docs_share":0.5}`, "local answer", false},
		{"a share out of range", `{"question":"q","project_path":"` + dir + `","docs_share":2}`, "docs_share must be from 0 to 1", true},
		{"the tickets corpus needs Plane", `{"question":"q","project_path":"` + dir + `","corpus":"tickets"}`, "needs Plane", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, isErr := callTool(t, srv, "dendrite_rag_ask", c.args)
			if isErr != c.fail || !strings.Contains(text, c.want) {
				t.Fatalf("text = %q (error %v), want %q (error %v)", text, isErr, c.want, c.fail)
			}
		})
	}

	t.Run("integer top_k from a Go caller", func(t *testing.T) {
		h, _ := srv.Handler("dendrite_rag_ask")
		if _, err := h(context.Background(), map[string]interface{}{"question": "LocateBinary", "project_path": dir, "top_k": 2}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("the sidecar fails", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", "ask")
		if text, isErr := callTool(t, srv, "dendrite_rag_ask", `{"question":"q","project_path":"`+dir+`"}`); !isErr || !strings.Contains(text, "rag ask failed") {
			t.Fatalf("text = %q (error %v)", text, isErr)
		}
	})
}

func TestRAGRetrieveArgumentsAndFailures(t *testing.T) {
	noEnvPlane(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	dir := ragDir(t)

	if text, isErr := callTool(t, srv, "dendrite_rag_retrieve", `{"query":"LocateBinary","project_path":"`+dir+`","detail":"map","token_budget":900}`); isErr {
		t.Fatalf("detail with a budget: %s", text)
	}
	h, _ := srv.Handler("dendrite_rag_retrieve")
	if _, err := h(context.Background(), map[string]interface{}{"query": "LocateBinary", "project_path": dir, "top_k": 2}); err != nil {
		t.Fatalf("integer top_k: %v", err)
	}
	if text, isErr := callTool(t, srv, "dendrite_rag_retrieve", `{"query":"q","project_path":"`+dir+`","corpus":"tickets"}`); !isErr || !strings.Contains(text, "needs Plane") {
		t.Fatalf("tickets corpus: %q (error %v)", text, isErr)
	}
	t.Run("the sidecar fails", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", "search")
		if text, isErr := callTool(t, srv, "dendrite_rag_retrieve", `{"query":"q","project_path":"`+dir+`"}`); !isErr || !strings.Contains(text, "rag retrieval failed") {
			t.Fatalf("text = %q (error %v)", text, isErr)
		}
	})
}

func TestRAGShowArgumentsAndFailures(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	dir := ragDir(t)
	if text, isErr := callTool(t, srv, "dendrite_rag_show", `{"ids":["",3]}`); !isErr || !strings.Contains(text, "ids is required") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
	t.Run("the sidecar fails", func(t *testing.T) {
		t.Setenv("STANDIN_FAIL", "chunks.get")
		if text, isErr := callTool(t, srv, "dendrite_rag_show", `{"ids":["pkg/demo/demo.go#1"],"project_path":"`+dir+`"}`); !isErr || !strings.Contains(text, "rag show failed") {
			t.Fatalf("text = %q (error %v)", text, isErr)
		}
	})
}
