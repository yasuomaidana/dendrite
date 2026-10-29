// Package ticketlock is the write lock of the ticket lifecycle: a session token, a claim that ties a
// working copy to one Plane ticket, and the fail-closed guard that refuses edits without them.
package ticketlock

import (
	"fmt"

	"Dendrite/pkg/tracker"
)

// Mode says what the lifecycle check protects.
type Mode int

const (
	// ModeCommit protects a commit: the ticket must be In Progress. A ticket in To Do or Backlog is
	// started on the spot; Blocked, To Verify, Done and Cancelled tickets are refused.
	ModeCommit Mode = iota
	// ModeStart protects the start of work: like ModeCommit, and a Blocked ticket is resumed.
	ModeStart
)

// Allowed applies the lifecycle table to a ticket in state st. move is true when the work may go ahead
// after moving the ticket to In Progress; a non-empty hint means the state refuses the work.
func Allowed(st tracker.Status, mode Mode, ticket string) (move bool, hint string) {
	switch st {
	case tracker.StatusInProgress:
		return false, ""
	case tracker.StatusTodo, tracker.StatusBacklog:
		return true, ""
	case tracker.StatusBlocked:
		if mode == ModeCommit {
			return false, fmt.Sprintf("resume it first with `dendrite chronos ticket set %s \"in progress\"`", ticket)
		}
		return true, ""
	case tracker.StatusInReview:
		return false, fmt.Sprintf("it is awaiting verification; reopen it with `dendrite chronos ticket set %s \"in progress\"` for review fixes, or create a follow-up ticket", ticket)
	default: // Done, Cancelled
		return false, "it is finished; create a follow-up ticket with `dendrite chronos create`, or reopen it deliberately with `dendrite chronos ticket set`"
	}
}
