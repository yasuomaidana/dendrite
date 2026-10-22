package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"Dendrite/pkg/synapse"
)

func TestSynapseCommand_TreeRegistration(t *testing.T) {
	cmd, _, err := newRootCmd().Find([]string{"syn"})
	if err != nil || cmd.Name() != "synapse" {
		t.Fatalf("alias 'syn' did not resolve to synapse: %v", err)
	}
	if cmd.GroupID != "cleft" {
		t.Errorf("synapse group = %q", cmd.GroupID)
	}
	if cmd.Short != "Typed cross-project links with scoped MCP servers and self-healing paths" {
		t.Errorf("unexpected description: %q", cmd.Short)
	}
}

func TestSynapseCommand_SetSource(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	control, hub := t.TempDir(), t.TempDir()
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := runSynapseCommand(context.Background(), args, &out)
		return out.String(), err
	}
	if _, err := run("create", "stack", "--members", control+"=control:consumer,"+hub+"=mcphub:producer"); err != nil {
		t.Fatal(err)
	}

	// hub has no origin remote, so remote mode needs an explicit URL.
	if _, err := run("set-source", "stack", "mcphub", "remote"); err == nil || !strings.Contains(err.Error(), "remote URL") {
		t.Fatalf("expected missing remote URL error, got %v", err)
	}
	out, err := run("set-source", "stack", "mcphub", "remote", "--remote", "https://example.com/hub.git", "--ref=develop", "--module", "dendrite.dev/mcphub")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "stack/mcphub now consumed from remote {remote develop}") {
		t.Fatalf("output = %q", out)
	}

	st, _ := synapse.DefaultStore()
	s, _ := st.Load("stack")
	m, _ := s.Member("mcphub")
	if m.Source != synapse.SourceRemote || m.Remote != "https://example.com/hub.git" || m.Ref != "develop" || m.Module != "dendrite.dev/mcphub" {
		t.Fatalf("member = %+v", m)
	}

	// A lock recorded by sync is dropped when the ref changes.
	m.LockedSHA = "abc"
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
	if _, err := run("set-source", "stack", "mcphub", "local", "--ref", "main"); err != nil {
		t.Fatal(err)
	}
	s, _ = st.Load("stack")
	m, _ = s.Member("mcphub")
	if m.Source != synapse.SourceLocal || m.Ref != "main" || m.LockedSHA != "" || m.Remote == "" {
		t.Fatalf("after local switch member = %+v", m)
	}

	insp, err := run("inspect", "stack")
	if err != nil || !strings.Contains(insp, "{main}") {
		t.Fatalf("inspect = %q, %v", insp, err)
	}

	if err := os.RemoveAll(hub); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"set-source", "stack", "mcphub", "local"}, "no local checkout"},
		{[]string{"set-source", "stack", "mcphub", "vendored"}, "invalid source"},
		{[]string{"set-source", "stack", "ghost", "local"}, "no member"},
		{[]string{"set-source", "nope", "mcphub", "local"}, "nope"},
		{[]string{"set-source", "stack"}, "accepts 3 arg(s)"},
		{[]string{"set-source", "stack", "mcphub", "local", "--ref"}, "needs an argument"},
		{[]string{"set-source", "stack", "mcphub", "local", "--bogus"}, "unknown flag"},
	} {
		if _, err := run(tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestDescribeSource(t *testing.T) {
	if got := describeSource(synapse.Member{}); got != "" {
		t.Fatalf("plain local member = %q", got)
	}
	got := describeSource(synapse.Member{Source: synapse.SourceRemote, Ref: "develop", LockedSHA: "0123456789abcdef"})
	if got != " {remote develop @0123456}" {
		t.Fatalf("describeSource = %q", got)
	}
}
