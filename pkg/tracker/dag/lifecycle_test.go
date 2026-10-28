package dag

import (
	"testing"

	"Dendrite/pkg/tracker"
)

func lifecycleGraph(t *testing.T, statuses map[string]tracker.Status, edges [][2]string) *Graph {
	t.Helper()
	g := NewGraph()
	for id, st := range statuses {
		g.AddNode(&Node{ID: id, Title: id, Status: st, Priority: "high"})
	}
	for _, e := range edges {
		if err := g.AddEdge(e[0], e[1]); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

func TestSmartSelectSkipsBlockedAndCancelled(t *testing.T) {
	g := lifecycleGraph(t, map[string]tracker.Status{
		"DEN-001": tracker.StatusBlocked,
		"DEN-002": tracker.StatusCancelled,
		"DEN-003": tracker.StatusTodo,
	}, nil)
	res, err := g.SmartSelect("")
	if err != nil {
		t.Fatal(err)
	}
	if res.SelectedTicket == nil || res.SelectedTicket.ID != "DEN-003" {
		t.Errorf("selected %+v, want DEN-003", res.SelectedTicket)
	}
}

func TestCancelledBlockerDoesNotHoldBackDependents(t *testing.T) {
	g := lifecycleGraph(t, map[string]tracker.Status{
		"DEN-001": tracker.StatusCancelled,
		"DEN-002": tracker.StatusTodo,
	}, [][2]string{{"DEN-001", "DEN-002"}})
	res, err := g.SmartSelect("")
	if err != nil {
		t.Fatal(err)
	}
	if res.SelectedTicket == nil || res.SelectedTicket.ID != "DEN-002" || res.IsBlocked {
		t.Errorf("got %+v, want unblocked DEN-002", res)
	}
}

func TestBlockedBlockerStillHoldsBackDependents(t *testing.T) {
	g := lifecycleGraph(t, map[string]tracker.Status{
		"DEN-001": tracker.StatusBlocked,
		"DEN-002": tracker.StatusTodo,
	}, [][2]string{{"DEN-001", "DEN-002"}})
	res, err := g.SmartSelect("")
	if err != nil {
		t.Fatal(err)
	}
	if res.SelectedTicket == nil || res.SelectedTicket.ID != "DEN-002" || !res.IsBlocked {
		t.Errorf("got %+v, want DEN-002 reported blocked by the Blocked DEN-001", res)
	}
}
