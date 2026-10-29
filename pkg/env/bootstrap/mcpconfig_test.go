package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"Dendrite/pkg/synapse"
)

// inSynapses makes the repository belong to the given synapses for the length of the test.
func inSynapses(t *testing.T, ids ...string) {
	t.Helper()
	prev := peerGroupsOf
	peerGroupsOf = func(string) []synapse.PeerGroup {
		var groups []synapse.PeerGroup
		for _, id := range ids {
			groups = append(groups, synapse.PeerGroup{Synapse: id, Self: "me"})
		}
		return groups
	}
	t.Cleanup(func() { peerGroupsOf = prev })
}

func readServers(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s is not JSON: %v\n%s", path, err, data)
	}
	return doc.Servers
}

func TestMCPServersForARepositoryInNoSynapse(t *testing.T) {
	inSynapses(t)
	got := mcpServersFor(t.TempDir())
	if len(got) != 1 || got[0].name != "dendrite" || got[0].entry.Command != "dendrite" ||
		!reflect.DeepEqual(got[0].entry.Args, []string{"inter", "serve", "--profile", "core"}) {
		t.Errorf("servers = %+v", got)
	}
}

func TestMCPServersAddOneScopedServerPerSynapse(t *testing.T) {
	inSynapses(t, "dendrite-ecosystem", "kakebo")
	got := mcpServersFor(t.TempDir())
	if len(got) != 3 || got[1].name != "dendrite-synapse-dendrite-ecosystem" || got[2].name != "dendrite-synapse-kakebo" {
		t.Fatalf("servers = %+v", got)
	}
	if want := []string{"inter", "serve", "--synapse", "kakebo"}; !reflect.DeepEqual(got[2].entry.Args, want) {
		t.Errorf("args = %v, want %v", got[2].entry.Args, want)
	}
}

func TestMCPConfigsAreTheSameForClaudeAGYAndJetBrains(t *testing.T) {
	inSynapses(t, "dendrite-ecosystem")
	root := t.TempDir()
	created, updated, err := writeMCPConfigs(root, false)
	if err != nil || len(created) != 3 || len(updated) != 0 {
		t.Fatalf("created=%v updated=%v err=%v", created, updated, err)
	}
	claude := readServers(t, filepath.Join(root, ".mcp.json"))
	agy := readServers(t, filepath.Join(root, ".agents", "mcp.json"))
	jetbrains := readServers(t, filepath.Join(root, ".ai", "mcp", "mcp.json"))
	if len(claude) != 2 || claude["dendrite"] == nil || claude["dendrite-synapse-dendrite-ecosystem"] == nil {
		t.Errorf("Claude's servers = %v", claude)
	}
	if !reflect.DeepEqual(claude, agy) || !reflect.DeepEqual(claude, jetbrains) {
		t.Errorf("the three agent families must see the same servers:\nclaude %v\nagy %v\njetbrains %v", claude, agy, jetbrains)
	}

	created, updated, err = writeMCPConfigs(root, false)
	if err != nil || len(created) != 0 || len(updated) != 0 {
		t.Errorf("a second run changes nothing: created=%v updated=%v err=%v", created, updated, err)
	}
}

func TestMCPConfigsKeepOtherServersAndKeys(t *testing.T) {
	inSynapses(t)
	root := t.TempDir()
	path := filepath.Join(root, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"extra": {"keep": true}, "mcpServers": {"mine": {"command": "mine", "args": ["x"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	created, updated, err := writeMCPConfigs(root, false)
	if err != nil || len(created) != 2 || len(updated) != 1 || updated[0] != ".mcp.json" {
		t.Fatalf("created=%v updated=%v err=%v", created, updated, err)
	}
	servers := readServers(t, path)
	if servers["mine"] == nil || servers["dendrite"] == nil {
		t.Errorf("servers = %v: the user's own server must stay", servers)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"extra"`) {
		t.Errorf("a top-level key was lost:\n%s", data)
	}
}

func TestAMalformedMCPConfigIsLeftAloneAndTheOthersAreStillWritten(t *testing.T) {
	inSynapses(t)
	root := t.TempDir()
	bad := filepath.Join(root, ".agents", "mcp.json")
	_ = os.MkdirAll(filepath.Dir(bad), 0o755)
	_ = os.WriteFile(bad, []byte("{not json"), 0o644)

	created, _, err := writeMCPConfigs(root, false)
	if err == nil || !strings.Contains(err.Error(), ".agents/mcp.json") {
		t.Errorf("err = %v", err)
	}
	if data, _ := os.ReadFile(bad); string(data) != "{not json" {
		t.Errorf("the malformed file was overwritten: %s", data)
	}
	if len(created) != 2 {
		t.Errorf("created = %v: the other two files should still be written", created)
	}
}

func TestMCPConfigsInADryRunWriteNothing(t *testing.T) {
	inSynapses(t)
	root := t.TempDir()
	created, updated, err := writeMCPConfigs(root, true)
	if err != nil || len(created)+len(updated) != 0 {
		t.Errorf("created=%v updated=%v err=%v", created, updated, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mcp.json")); err == nil {
		t.Error("a dry run wrote a file")
	}
}

func TestPrepareInstallsTheLockHooksAndMCPConfigs(t *testing.T) {
	inSynapses(t, "dendrite-ecosystem")
	root := gitRepoAt(t)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPreparer().Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".githooks/pre-commit", ".mcp.json", ".agents/mcp.json", ".ai/mcp/mcp.json", ".claude/settings.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("prepare did not write %s: %v", rel, err)
		}
	}
	if got := gitRun(t, root, "config", "--get", "core.hooksPath"); got != gitHooksDir {
		t.Errorf("core.hooksPath = %q", got)
	}
	if s := readServers(t, filepath.Join(root, ".mcp.json")); s["dendrite-synapse-dendrite-ecosystem"] == nil {
		t.Errorf("servers = %v", s)
	}
}

func TestPrepareDoesNotTouchHooksOrMCPInADryRun(t *testing.T) {
	inSynapses(t)
	root := gitRepoAt(t)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPreparer().Prepare(context.Background(), PrepareOptions{ProjectPath: root, SyncSkills: true, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".githooks", ".mcp.json", ".agents/mcp.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			t.Errorf("a dry run wrote %s", rel)
		}
	}
}
