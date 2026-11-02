package mcpserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
)

func TestSynapseToolsReportASynapseThatIsGone(t *testing.T) {
	st, _ := synapseFixture(t)
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(st.Dir()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"synapse_sync_status", "synapse_search"} {
		h, _ := s.Handler(name)
		res, err := h(context.Background(), map[string]any{"query": "x"})
		if err != nil || !res.IsError || !strings.Contains(res.Content[0].Text, "loading synapse") {
			t.Errorf("%s: %+v, %v", name, res, err)
		}
	}
}

func TestSynapseSearchAnswersWithHits(t *testing.T) {
	st, dirs := synapseFixture(t)
	for _, dir := range dirs {
		if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\n// LocateBinary finds it.\nfunc LocateBinary() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := NewSynapseServer(SynapseScope{Store: st, ID: "web-api"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := s.Handler("synapse_search")
	res, err := h(context.Background(), map[string]any{"query": "LocateBinary", "top_k": float64(2)})
	if err != nil || res.IsError || !strings.Contains(res.Content[0].Text, "hits") {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func repoWithIdentity(t *testing.T, withID bool) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if withID {
		if _, err := projectid.Ensure(dir); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSynapsePeersToolHandlesEveryRepositoryShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	srv := NewServer(ServerConfig{})
	registerSynapsePeersTool(srv)

	if text, isErr := callTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+t.TempDir()+`"}`); !isErr || !strings.Contains(text, "not inside a git repository") {
		t.Errorf("outside a repository: %q (error %v)", text, isErr)
	}
	if text, isErr := callTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+repoWithIdentity(t, false)+`"}`); isErr || strings.TrimSpace(text) != "[]" {
		t.Errorf("a repository without an identity is in no synapse: %q (error %v)", text, isErr)
	}
	identified := repoWithIdentity(t, true)
	if text, isErr := callTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+identified+`"}`); isErr || strings.TrimSpace(text) != "[]" {
		t.Errorf("an identified repository with no synapse: %q (error %v)", text, isErr)
	}

	if err := os.WriteFile(filepath.Join(home, "synapses"), []byte("x"), 0o644); err == nil {
		// DefaultStore keeps its manifests under home/synapses; a file there cannot be listed.
		if text, isErr := callTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+identified+`"}`); !isErr || !strings.Contains(text, "listing peers") {
			t.Errorf("an unreadable store: %q (error %v)", text, isErr)
		}
	}

	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if text, isErr := callTool(t, srv, "dendrite_synapse_peers", `{"dir":"`+identified+`"}`); !isErr || !strings.Contains(text, "opening the synapse store") {
		t.Errorf("no home directory: %q (error %v)", text, isErr)
	}
}
