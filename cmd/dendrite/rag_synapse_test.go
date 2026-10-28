package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

func projectWithID(t *testing.T, name string) (dir, id string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := projectid.Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, id
}

func saveSynapse(t *testing.T, st *synapse.Store, id string, members ...synapse.Member) {
	t.Helper()
	if err := st.Save(&synapse.Synapse{ID: id, Name: id, State: synapse.StateActive, Members: members}); err != nil {
		t.Fatal(err)
	}
}

func TestPickSynapseNamesTheRightOneOrExplainsWhyNot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	st := synapse.NewStore(filepath.Join(home, "synapses"))
	a, aid := projectWithID(t, "a")
	b, bid := projectWithID(t, "b")
	c, cid := projectWithID(t, "c")
	lone, _ := projectWithID(t, "lone")
	m := func(dir, id, alias string) synapse.Member {
		return synapse.Member{ProjectID: id, Alias: alias, Role: synapse.RolePeer, PathHint: dir}
	}
	saveSynapse(t, st, "ab", m(a, aid, "a"), m(b, bid, "b"))
	saveSynapse(t, st, "bc", m(b, bid, "b"), m(c, cid, "c"))

	if s, err := pickSynapse(a, ""); err != nil || s.ID != "ab" {
		t.Errorf("a is only in ab: %v, %v", s, err)
	}
	if s, err := pickSynapse(b, "bc"); err != nil || s.ID != "bc" {
		t.Errorf("an explicit --synapse wins: %v, %v", s, err)
	}
	if _, err := pickSynapse(b, ""); err == nil || !strings.Contains(err.Error(), "several synapses (ab, bc)") {
		t.Errorf("b is in two synapses: %v", err)
	}
	if _, err := pickSynapse(lone, ""); err == nil || !strings.Contains(err.Error(), "in no synapse") {
		t.Errorf("lone: %v", err)
	}
	if _, err := pickSynapse(t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "no project id") {
		t.Errorf("a directory without a project id: %v", err)
	}
}
