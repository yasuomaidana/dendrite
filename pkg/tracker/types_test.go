package tracker

import (
	"testing"
)

func TestStatusDisplayNameOfEveryStatus(t *testing.T) {
	cases := []struct {
		status   Status
		expected string
	}{
		{StatusDone, "Done"},
		{StatusInReview, "To Verify"},
		{StatusInProgress, "In Progress"},
		{StatusTodo, "To Do"},
		{StatusBlocked, "Blocked"},
		{StatusCancelled, "Cancelled"},
		{StatusBacklog, "Backlog"},
		{Status("UNKNOWN"), "Backlog"},
	}

	for _, tc := range cases {
		if got := StatusDisplayName(tc.status); got != tc.expected {
			t.Errorf("StatusDisplayName(%s) = %s, want %s", tc.status, got, tc.expected)
		}
	}
}

func TestErrTicketNotFound(t *testing.T) {
	err := &ErrTicketNotFound{ID: "DEN-999"}
	expected := "ticket not found: DEN-999"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
