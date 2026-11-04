package synapsebranch

import (
	"testing"

	"Dendrite/pkg/branchpolicy"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/testkit"
)

func TestBranchOverrideReadsTheMemberOfTheFirstSynapseThatDeclaresOne(t *testing.T) {
	st := synapse.NewStore(t.TempDir())
	plain := fixture()
	plain.ID = "a-plain"
	forked := fixture()
	forked.ID = "b-forked"
	forked.Members[0].Branch, forked.Members[0].PushRemote, forked.Members[0].TargetRemote = "dev", "fork", "upstream"
	archived := fixture()
	archived.ID = "c-archived"
	archived.State = synapse.StateArchived
	archived.Members[0].Branch = "ignored"
	for _, s := range []*synapse.Synapse{plain, forked, archived} {
		testkit.Check(t, st.Save(s))
	}
	id := plain.Members[0].ProjectID

	testkit.Equal(t, "override", BranchOverride(st, id), branchpolicy.Override{Branch: "dev", PushRemote: "fork", TargetRemote: "upstream"})
	testkit.Equal(t, "other member", BranchOverride(st, plain.Members[1].ProjectID), branchpolicy.Override{})
	testkit.Equal(t, "unknown project", BranchOverride(st, "dpid_none"), branchpolicy.Override{})
}

func fixture() *synapse.Synapse {
	return &synapse.Synapse{
		ID: "web-api", Name: "Web", State: synapse.StateActive,
		Members: []synapse.Member{
			{ProjectID: "dpid_0123456789abcdef0123456789abcdef", Alias: "api", Role: synapse.RoleProducer, PathHint: "/repos/api"},
			{ProjectID: "dpid_abcdef0123456789abcdef0123456789", Alias: "web", Role: synapse.RoleConsumer, PathHint: "/repos/web"},
		},
	}
}

func TestBranchOverrideIsEmptyWhenTheStoreCannotBeRead(t *testing.T) {
	file := t.TempDir() + "/not-a-dir"
	testkit.WriteFile(t, file, "x", 0o644)
	testkit.Equal(t, "override", BranchOverride(synapse.NewStore(file), "dpid_x"), branchpolicy.Override{})
}
