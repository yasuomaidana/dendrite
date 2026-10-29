package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

func peersRepo(t *testing.T, root, name string) (dir, id string) {
	t.Helper()
	dir = filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := projectid.Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, id
}

func TestSynapsePeersTool_ListsPeersAndIsEmptyOutsideSynapses(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	root := t.TempDir()
	docula, doculaID := peersRepo(t, root, "docula")
	oxipix, oxipixID := peersRepo(t, root, "oxipix")
	lone, _ := peersRepo(t, root, "lone")

	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&synapse.Synapse{ID: "mesh", Name: "mesh", State: synapse.StateActive, Members: []synapse.Member{
		{ProjectID: doculaID, Alias: "docula", Role: synapse.RoleConsumer, PathHint: docula, PlaneProject: "DOC"},
		{ProjectID: oxipixID, Alias: "oxipix", Role: synapse.RoleProducer, PathHint: oxipix, PlaneProject: "OXI"},
	}}); err != nil {
		t.Fatal(err)
	}

	s := newDefaultServerForProfileTest()
	call := func(dir string) string {
		resp := rpc(t, s, "tools/call", CallToolParams{Name: "dendrite_synapse_peers", Arguments: map[string]interface{}{"dir": dir}})
		return mustJSON(t, resp.Result)
	}

	got := call(filepath.Join(docula)) // from the repository root; a subdirectory resolves the same way
	for _, want := range []string{`mesh`, `oxipix`, `producer`, `OXI`, `OK`} {
		if !strings.Contains(got, want) {
			t.Errorf("peers of docula lack %q: %s", want, got)
		}
	}
	if strings.Contains(got, `\"alias\":\"docula\"`) {
		t.Errorf("a project is not its own peer: %s", got)
	}
	if got := call(lone); !strings.Contains(got, `[]`) || strings.Contains(got, "isError\":true") {
		t.Errorf("a repository in no synapse gets an empty list: %s", got)
	}
	if got := call(t.TempDir()); !strings.Contains(got, `"isError":true`) {
		t.Errorf("a directory outside any repository is an error: %s", got)
	}

	// The tool is read-only and cheap: the core profile keeps it.
	core := ApplyProfile(s, ProfileCore)
	if _, ok := core.Handler("dendrite_synapse_peers"); !ok {
		t.Error("the core profile must expose dendrite_synapse_peers")
	}
}
