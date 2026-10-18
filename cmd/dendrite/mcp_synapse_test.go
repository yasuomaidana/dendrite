package main

import (
	"reflect"
	"testing"
)

func TestExtractSynapseFlag(t *testing.T) {
	cases := []struct {
		args    []string
		id      string
		rest    []string
		wantErr bool
	}{
		{args: []string{"--synapse", "web-api"}, id: "web-api", rest: []string{}},
		{args: []string{"synapse_info", "--synapse=web-api", "{}"}, id: "web-api", rest: []string{"synapse_info", "{}"}},
		{args: []string{"x"}, id: "", rest: []string{"x"}},
		{args: []string{"--synapse"}, wantErr: true},
	}
	for _, c := range cases {
		id, rest, err := extractSynapseFlag(c.args)
		if (err != nil) != c.wantErr {
			t.Fatalf("extractSynapseFlag(%v) err = %v", c.args, err)
		}
		if c.wantErr {
			continue
		}
		if id != c.id || !reflect.DeepEqual(rest, c.rest) {
			t.Fatalf("extractSynapseFlag(%v) = %q, %v; want %q, %v", c.args, id, rest, c.id, c.rest)
		}
	}
}

func TestBuildSynapseMCPServerUnknown(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if _, err := buildSynapseMCPServer("missing"); err == nil {
		t.Fatal("expected error for an unknown synapse")
	}
}
