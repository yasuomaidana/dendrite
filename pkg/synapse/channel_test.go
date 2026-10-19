package synapse

import (
	"strings"
	"testing"
)

func TestChannel_DefaultRefAndParse(t *testing.T) {
	cases := map[string]Channel{"stable": ChannelStable, "main": ChannelStable, "dev": ChannelDev, "develop": ChannelDev}
	for in, want := range cases {
		got, err := ParseChannel(in)
		if err != nil || got != want {
			t.Fatalf("ParseChannel(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseChannel("nightly"); err == nil {
		t.Fatal("expected an error for an unknown channel")
	}
	if ChannelStable.DefaultRef() != "main" || ChannelDev.DefaultRef() != "develop" || Channel("").DefaultRef() != "" {
		t.Fatal("unexpected channel default refs")
	}
}

func TestMember_TrackedRef(t *testing.T) {
	m := Member{Alias: "a"}
	if got := m.TrackedRef(ChannelDev); got != "develop" {
		t.Fatalf("dev channel: got %q", got)
	}
	if got := m.TrackedRef(ChannelStable); got != "main" {
		t.Fatalf("stable channel: got %q", got)
	}
	if got := m.TrackedRef(""); got != "" {
		t.Fatalf("no channel follows the checkout, got %q", got)
	}
	m.Ref = "feature/x"
	if got := m.TrackedRef(ChannelDev); got != "feature/x" {
		t.Fatalf("an override wins over the channel, got %q", got)
	}
}

func TestChannel_CodecRoundTripAndValidate(t *testing.T) {
	s := &Synapse{
		ID: "eco", Name: "Eco", State: StateActive, Channel: ChannelDev,
		Members: []Member{
			{ProjectID: "dpid_a", Alias: "a", Role: RoleProducer, PathHint: "/a"},
			{ProjectID: "dpid_b", Alias: "b", Role: RoleConsumer, PathHint: "/b", Ref: "main"},
		},
	}
	data, err := Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `channel = "dev"`) {
		t.Fatalf("channel not encoded:\n%s", data)
	}
	back, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.Channel != ChannelDev || back.Members[1].Ref != "main" {
		t.Fatalf("round trip lost channel or override: %+v", back)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	back.Channel = "nightly"
	if err := back.Validate(); err == nil || !strings.Contains(err.Error(), "channel") {
		t.Fatalf("expected an invalid channel error, got %v", err)
	}
	if _, err := Unmarshal([]byte("id = \"x\"\nchannel = \"dev\"\nchannel = \"stable\"\n")); err == nil {
		t.Fatal("expected a duplicate channel key error")
	}
}
