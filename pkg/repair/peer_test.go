package repair

import (
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/workcopy"
)

// peers is a synapse store where the project at consumer is linked with a member "embedag" (module
// github.com/x/embedag) whose checkout exists, and a member "gone" that has only a remote.
func peers(t *testing.T, state synapse.State) (st *synapse.Store, consumer, checkout string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	consumer, checkout = workcopy.Canon(t.TempDir()), workcopy.Canon(t.TempDir())
	me, err := projectid.Ensure(consumer)
	testkit.Check(t, err)
	dep, err := projectid.Ensure(checkout)
	testkit.Check(t, err)
	st = synapse.NewStore(filepath.Join(t.TempDir(), "synapses"))
	testkit.Check(t, st.Save(&synapse.Synapse{
		ID: "eco", Name: "eco", State: state, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Members: []synapse.Member{
			{ProjectID: me, Alias: "control", Role: synapse.RoleConsumer, PathHint: consumer},
			{ProjectID: dep, Alias: "embedag", Role: synapse.RoleProducer, PathHint: checkout, Module: "github.com/x/embedag-core", Remote: "https://x/embedag.git", PlaneProject: "EMBEDAG"},
			{ProjectID: "dpid_gone", Alias: "gone", Role: synapse.RoleProducer, Source: synapse.SourceRemote, Remote: "https://x/gone.git"},
		},
	}))
	return st, consumer, checkout
}

func TestFindPeerMatchesAnAliasAModuleOrItsLastSegment(t *testing.T) {
	st, consumer, checkout := peers(t, synapse.StateActive)
	for _, dep := range []string{"embedag", "github.com/x/embedag-core", "embedag-core"} {
		p, ok := FindPeer(st, consumer, dep)
		testkit.Equal(t, dep+" found", ok, true)
		testkit.Equal(t, dep+" checkout", p.Checkout, checkout)
		testkit.Equal(t, dep+" plane", p.PlaneProject, "EMBEDAG")
	}
	p, ok := FindPeer(st, consumer, "gone")
	testkit.Equal(t, "remote-only peer", ok, true)
	testkit.Equal(t, "no checkout", p.Checkout, "")
	testkit.Equal(t, "remote", p.Remote, "https://x/gone.git")

	for _, dep := range []string{"control", "serde"} {
		if _, ok := FindPeer(st, consumer, dep); ok {
			t.Errorf("%s is not a peer", dep)
		}
	}
	if _, ok := FindPeer(st, t.TempDir(), "embedag"); ok {
		t.Error("a directory that is not a project has no peers")
	}
}

func TestFindPeerSkipsArchivedSynapses(t *testing.T) {
	st, consumer, _ := peers(t, synapse.StateArchived)
	if _, ok := FindPeer(st, consumer, "embedag"); ok {
		t.Error("an archived synapse links nothing")
	}
}
