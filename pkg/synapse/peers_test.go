package synapse

import (
	"testing"

	"Dendrite/pkg/projectid"
)

func planeBinding(ident string) *projectid.PlaneBinding {
	return &projectid.PlaneBinding{Identifier: ident, ProjectID: "uuid-" + ident}
}

func TestPeersOf_GroupsPeersPerSynapseAndFillsPlaneFromTheIndex(t *testing.T) {
	e := newHealEnv(t)
	aDir, aID := e.project("ws/a")
	bDir, bID := e.project("ws/b")
	_, cID := e.project("ws/c")
	cDir := e.root + "/ws/c"
	if !e.index.SetPlane(cID, planeBinding("CEE")) {
		t.Fatal("c must be indexed")
	}

	e.synapse("s1",
		Member{ProjectID: aID, Alias: "a", Role: RoleConsumer, PathHint: aDir},
		Member{ProjectID: bID, Alias: "b", Role: RoleProducer, PathHint: bDir, PlaneProject: "BEE"})
	e.synapse("s2",
		Member{ProjectID: aID, Alias: "a-core", Role: RolePeer, PathHint: aDir},
		Member{ProjectID: cID, Alias: "c", Role: RolePeer, PathHint: cDir})
	arch := e.synapse("old", Member{ProjectID: aID, Alias: "a", Role: RolePeer, PathHint: aDir},
		Member{ProjectID: cID, Alias: "c", Role: RolePeer, PathHint: cDir})
	arch.State = StateArchived
	if err := e.store.Save(arch); err != nil {
		t.Fatal(err)
	}

	groups, err := PeersOf(e.store, e.index, aID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Synapse != "s1" || groups[1].Synapse != "s2" {
		t.Fatalf("groups = %+v (archived synapses must be skipped)", groups)
	}
	if groups[0].Self != "a" || groups[1].Self != "a-core" {
		t.Errorf("self aliases = %q, %q", groups[0].Self, groups[1].Self)
	}
	b := groups[0].Peers[0]
	if b.Alias != "b" || b.Role != RoleProducer || b.PlaneProject != "BEE" || b.Path != bDir || b.Status != StatusOK {
		t.Errorf("peer b = %+v", b)
	}
	if c := groups[1].Peers[0]; c.PlaneProject != "CEE" {
		t.Errorf("the Plane identifier must come from the index when the manifest has none: %+v", c)
	}

	if g, _ := PeersOf(e.store, e.index, "dpid_"+"0123456789abcdef0123456789abcdef"); len(g) != 0 {
		t.Errorf("a project in no synapse has no peers, got %+v", g)
	}
}
