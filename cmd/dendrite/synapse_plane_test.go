package main

import (
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/testkit"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSplitPlaneSuffix(t *testing.T) {
	testkit.Table(t, map[string][2]string{
		"../git=git:peer@dgit": {"../git=git:peer", "DGIT"},
		"/tmp/x":               {"/tmp/x", ""},
		"/tmp/me@host/repo":    {"/tmp/me@host/repo", ""},
		"/tmp/x@":              {"/tmp/x@", ""},
	}, func(in string) [2]string { rest, plane := splitPlaneSuffix(in); return [2]string{rest, plane} })
}

func TestSynapseCommand_PlaneBinding(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	dir1, dir2 := t.TempDir(), t.TempDir()
	ctx := context.Background()
	var out bytes.Buffer

	if err := runSynapseCommand(ctx, []string{"create", "mesh", "--members", fmt.Sprintf("%s=den@DEN,%s=git", dir1, dir2)}, &out); err != nil {
		t.Fatal(err)
	}
	if err := runSynapseCommand(ctx, []string{"set-plane", "mesh", "git", "dgit"}, &out); err != nil {
		t.Fatal(err)
	}
	st, _ := synapse.DefaultStore()
	syn, err := st.Load("mesh")
	if err != nil {
		t.Fatal(err)
	}
	if syn.Members[0].PlaneProject != "DEN" || syn.Members[1].PlaneProject != "DGIT" {
		t.Fatalf("unexpected bindings: %+v", syn.Members)
	}

	out.Reset()
	if err := runSynapseCommand(ctx, []string{"inspect", "mesh"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "plane=DGIT") {
		t.Errorf("inspect should show plane binding: %s", out.String())
	}

	if err := runSynapseCommand(ctx, []string{"set-plane", "mesh", "git", "-"}, &out); err != nil {
		t.Fatal(err)
	}
	syn, _ = st.Load("mesh")
	if syn.Members[1].PlaneProject != "" {
		t.Errorf("expected binding cleared, got %q", syn.Members[1].PlaneProject)
	}

	for _, bad := range [][]string{
		{"set-plane", "mesh"},
		{"set-plane", "mesh", "git", "not valid!"},
		{"set-plane", "mesh", "nobody", "DEN"},
		{"set-plane", "missing", "git", "DEN"},
	} {
		if err := runSynapseCommand(ctx, bad, &out); err == nil {
			t.Errorf("expected error for %v", bad)
		}
	}
}
