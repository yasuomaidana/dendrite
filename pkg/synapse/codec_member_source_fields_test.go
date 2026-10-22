package synapse

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMemberSourceFieldsRoundTrip(t *testing.T) {
	locked := time.Date(2026, 10, 18, 9, 30, 0, 0, time.UTC)
	in := &Synapse{
		ID: "stack", Name: "stack", State: StateActive,
		Members: []Member{
			{ProjectID: "dpid_a", Alias: "control", Role: RoleConsumer, PathHint: "/w/control"},
			{
				ProjectID: "dpid_b", Alias: "mcphub", Role: RoleProducer, PathHint: "/w/hub",
				Source: SourceRemote, Remote: "https://github.com/o/hub.git", Ref: "develop",
				Module: "dendrite.dev/mcphub", LockedSHA: "abc123", LockedAt: locked,
			},
		},
	}
	data, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "source = ") != 1 {
		t.Fatalf("empty optional member keys must be omitted:\n%s", data)
	}
	out, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, data)
	}
	if !reflect.DeepEqual(in.Members, out.Members) {
		t.Fatalf("members round trip:\n got %+v\nwant %+v", out.Members, in.Members)
	}
	if !out.Members[1].IsRemote() || out.Members[0].IsRemote() {
		t.Fatal("IsRemote mismatch")
	}

	bad := strings.Replace(string(data), `locked_at = "2026-10-18T09:30:00Z"`, `locked_at = "yesterday"`, 1)
	if _, err := Unmarshal([]byte(bad)); err == nil || !strings.Contains(err.Error(), "locked_at") {
		t.Fatalf("expected locked_at parse error, got %v", err)
	}
	bare := strings.Replace(string(data), `locked_at = "2026-10-18T09:30:00Z"`, `locked_at = 2026-10-18T09:30:00Z`, 1)
	if _, err := Unmarshal([]byte(bare)); err != nil {
		t.Fatalf("bare RFC3339 locked_at must parse: %v", err)
	}
}

func TestValidateMemberSource(t *testing.T) {
	base := func(m Member) *Synapse {
		return &Synapse{ID: "s", Members: []Member{
			{ProjectID: "dpid_a", Alias: "a", Role: RolePeer},
			m,
		}}
	}
	cases := []struct {
		m       Member
		wantErr string
	}{
		{Member{ProjectID: "dpid_b", Alias: "b", Role: RolePeer, Source: SourceLocal}, ""},
		{Member{ProjectID: "dpid_b", Alias: "b", Role: RolePeer, Source: SourceRemote, Remote: "u"}, ""},
		{Member{ProjectID: "dpid_b", Alias: "b", Role: RolePeer, Source: SourceRemote}, "requires a remote URL"},
		{Member{ProjectID: "dpid_b", Alias: "b", Role: RolePeer, Source: "vendored"}, "invalid source"},
	}
	for _, c := range cases {
		err := base(c.m).Validate()
		if c.wantErr == "" && err != nil {
			t.Errorf("%+v: unexpected %v", c.m, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%+v: err = %v, want %q", c.m, err, c.wantErr)
		}
	}
}
