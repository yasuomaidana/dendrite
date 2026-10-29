package ticketlock

import (
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

func TestAllowedByState(t *testing.T) {
	cases := []struct {
		st       tracker.Status
		mode     Mode
		wantMove bool
		wantHint string // substring; empty means allowed
	}{
		{tracker.StatusInProgress, ModeCommit, false, ""},
		{tracker.StatusInProgress, ModeStart, false, ""},
		{tracker.StatusTodo, ModeCommit, true, ""},
		{tracker.StatusBacklog, ModeStart, true, ""},
		{tracker.StatusBlocked, ModeCommit, false, "resume it first with `dendrite chronos ticket set DEN-1"},
		{tracker.StatusBlocked, ModeStart, true, ""},
		{tracker.StatusInReview, ModeCommit, false, "awaiting verification"},
		{tracker.StatusDone, ModeStart, false, "it is finished"},
		{tracker.StatusCancelled, ModeCommit, false, "it is finished"},
	}
	for _, c := range cases {
		move, hint := Allowed(c.st, c.mode, "DEN-1")
		if move != c.wantMove {
			t.Errorf("%s mode %d: move = %v, want %v", c.st, c.mode, move, c.wantMove)
		}
		if (c.wantHint == "") != (hint == "") || !strings.Contains(hint, c.wantHint) {
			t.Errorf("%s mode %d: hint = %q, want it to contain %q", c.st, c.mode, hint, c.wantHint)
		}
	}
}
