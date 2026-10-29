package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

// synapseRepos makes two repositories with project IDs and one synapse "mesh" holding both.
func synapseRepos(t *testing.T) (self, peer string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	self, peer = filepath.Join(t.TempDir(), "docula"), filepath.Join(t.TempDir(), "oxipix")
	var ids []string
	for _, d := range []string{self, peer} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		id, err := projectid.Ensure(d)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s := &synapse.Synapse{ID: "mesh", Name: "mesh", State: synapse.StateActive, Members: []synapse.Member{
		{ProjectID: ids[0], Alias: "docula", Role: synapse.RoleConsumer, PathHint: self, PlaneProject: "DOC"},
		{ProjectID: ids[1], Alias: "oxipix", Role: synapse.RoleProducer, PathHint: peer, PlaneProject: "OXI"},
	}}
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
	return self, peer
}

const wantSynapsesSection = "\n## Synapses\n" +
	"This project belongs to the synapses below. Its peers are other repositories it depends on or is used by; ask a peer for a feature instead of editing its code.\n\n" +
	"### mesh (you are `docula`)\n" +
	"- **oxipix** (producer) - Plane project OXI\n\n" +
	"To ask a peer for a feature, file a ticket in its Plane project:\n" +
	"```bash\n" +
	"dendrite chronos request <peer-alias> --name \"...\" --desc \"goal and acceptance criteria\" [--synapse <id>] [--from <TICKET>]\n" +
	"```\n" +
	"A peer that is in several synapses with this project needs `--synapse`, which records where the request came from; the peer works through them with `dendrite chronos next --synapse <id>`.\n"

func TestRenderSynapsesSection_ListsPeersWithoutPaths(t *testing.T) {
	self, peer := synapseRepos(t)
	got := renderSynapsesSection(self)
	if got != wantSynapsesSection {
		t.Fatalf("section:\n%s\nwant:\n%s", got, wantSynapsesSection)
	}
	if strings.Contains(got, peer) || strings.Contains(got, self) {
		t.Error("AGENTS.md is committed: the section must not contain filesystem paths")
	}
}

func TestRenderSynapsesSection_OmittedWithoutSynapsesOrIdentity(t *testing.T) {
	synapseRepos(t)
	lone := t.TempDir()
	if got := renderSynapsesSection(lone); got != "" {
		t.Errorf("a repository without a project ID has no section, got %q", got)
	}
	if _, err := projectid.Ensure(lone); err != nil {
		t.Fatal(err)
	}
	if got := renderSynapsesSection(lone); got != "" {
		t.Errorf("a repository in no synapse has no section, got %q", got)
	}
}

func TestPrepare_WritesTheSynapsesSectionIntoAgentsMD(t *testing.T) {
	self, _ := synapseRepos(t)
	if _, err := NewPreparer().Prepare(context.Background(), PrepareOptions{ProjectPath: self, DefaultTool: "copilot", EnforceCICD: true, SyncSkills: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(self, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), wantSynapsesSection) || !strings.HasSuffix(string(data), "\n") || strings.HasSuffix(string(data), "\n\n") {
		t.Errorf("AGENTS.md must end with the section and a single newline:\n...%s", string(data)[max(0, len(data)-400):])
	}
}
