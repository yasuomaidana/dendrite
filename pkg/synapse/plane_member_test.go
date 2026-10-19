package synapse

import (
	"strings"
	"testing"
)

func TestCodec_PlaneProjectRoundTrip(t *testing.T) {
	in := &Synapse{
		ID: "mesh", Name: "mesh", State: StateActive,
		Members: []Member{
			{ProjectID: "dpid_a", Alias: "den", Role: RolePeer, PathHint: "/a", PlaneProject: "DEN"},
			{ProjectID: "dpid_b", Alias: "git", Role: RolePeer, PathHint: "/b"},
		},
	}
	data, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "plane_project") != 1 {
		t.Fatalf("plane_project must be emitted only when set:\n%s", data)
	}
	out, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if out.Members[0].PlaneProject != "DEN" || out.Members[1].PlaneProject != "" {
		t.Errorf("round trip lost plane_project: %+v", out.Members)
	}
}
