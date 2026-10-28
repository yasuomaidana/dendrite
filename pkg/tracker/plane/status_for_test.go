package plane

import (
	"testing"

	"Dendrite/pkg/tracker"
)

func TestStatusForCoversCanonicalBoardAndLegacyNames(t *testing.T) {
	cases := []struct {
		group, name string
		want        tracker.Status
	}{
		{"backlog", "Backlog", tracker.StatusBacklog},
		{"unstarted", "To Do", tracker.StatusTodo},
		{"unstarted", "Todo", tracker.StatusTodo},
		{"started", "In Progress", tracker.StatusInProgress},
		{"started", "To Verify", tracker.StatusInReview},
		{"started", "In Review", tracker.StatusInReview},
		{"started", "Blocked", tracker.StatusBlocked},
		{"completed", "Done", tracker.StatusDone},
		{"cancelled", "Cancelled", tracker.StatusCancelled},
		{"cancelled", "Canceled", tracker.StatusCancelled},
		{"", "something odd", tracker.StatusBacklog},
	}
	for _, c := range cases {
		if got := StatusFor(c.group, c.name); got != c.want {
			t.Errorf("StatusFor(%q, %q) = %s, want %s", c.group, c.name, got, c.want)
		}
	}
}

func TestEveryCanonicalStateMapsToADistinctStatus(t *testing.T) {
	seen := map[tracker.Status]string{}
	for _, s := range CanonicalStates {
		st := StatusFor(s.Group, s.Name)
		if prev, dup := seen[st]; dup {
			t.Errorf("%s and %s both map to %s", prev, s.Name, st)
		}
		seen[st] = s.Name
	}
}
