package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/store/status"
)

// ownStore points the store tools at a store inside the test's temporary home and returns it.
func ownStore(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	st, err := segmented.NewSegmentedStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldStore, oldInspector := defaultMCPStore, defaultMCPStoreInspector
	defaultMCPStore, defaultMCPStoreInspector = st, status.NewStoreInspector(st)
	t.Cleanup(func() { defaultMCPStore, defaultMCPStoreInspector = oldStore, oldInspector })
	return dir
}

func TestStoreToolsWithoutAStoreRegisterNothing(t *testing.T) {
	old := defaultMCPStore
	defaultMCPStore = nil
	t.Cleanup(func() { defaultMCPStore = old })
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	for _, tool := range srv.ListTools() {
		if strings.HasPrefix(tool.Name, "dendrite_store_") {
			t.Errorf("%s must not be registered without a store", tool.Name)
		}
	}
}

func TestStoreToolsCountChunksAndSegments(t *testing.T) {
	ownStore(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	for _, args := range []string{
		`{"project":"p","chunk_id":"c1","identifier":"a.go","data":"package a","is_final":true}`,
		`{"project":"p","chunk_id":"c2","identifier":"b.go","data":"package b","type":"function"}`,
	} {
		if text, isErr := callTool(t, srv, "dendrite_store_commit_chunk", args); isErr {
			t.Fatalf("commit: %s", text)
		}
	}
	text, isErr := callTool(t, srv, "dendrite_store_status", `{"project":"p"}`)
	if isErr || !strings.Contains(text, `"stored_chunks": 1`) || !strings.Contains(text, `"partial_chunks": 1`) || !strings.Contains(text, `"total_segments": 2`) {
		t.Fatalf("status: %s", text)
	}
	if text, isErr = callTool(t, srv, "dendrite_store_query_partial", `{"project":"p"}`); isErr {
		t.Fatalf("query: %s", text)
	}
}

func TestStoreToolsReportAStoreThatCannotBeRead(t *testing.T) {
	dir := ownStore(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})
	// A database path that is a directory cannot be opened.
	db := filepath.Join(dir, "meta.db")
	if err := os.RemoveAll(db); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(db, 0o755); err != nil {
		t.Fatal(err)
	}
	runToolFailures(t, srv, []toolFailure{
		{"status", "dendrite_store_status", `{"project":"p"}`, nil, "querying segments"},
		{"commit", "dendrite_store_commit_chunk", `{"project":"p","chunk_id":"c","identifier":"i","data":"d"}`, nil, "Failed to commit chunk"},
		{"query", "dendrite_store_query_partial", `{"project":"p"}`, nil, "querying partial segments"},
		{"inspect", "dendrite_store_inspect", `{"store_id":"p"}`, nil, "Inspect failed"},
		{"remove", "dendrite_store_rm", `{"store_id":"p","force":true}`, nil, "Removal failed"},
	})
}
