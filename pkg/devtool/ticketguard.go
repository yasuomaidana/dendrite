package devtool

import (
	"context"
	"fmt"
	"os"
	"strings"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// TicketGuardEnv switches the lifecycle guard off when set to "off", "0" or "false".
const TicketGuardEnv = "DENDRITE_TICKET_GUARD"

// GuardMode says what the guard is protecting.
type GuardMode int

const (
	// GuardCommit protects a commit: the ticket must be In Progress. A ticket in To Do or Backlog
	// is started on the spot; Blocked, To Verify, Done and Cancelled tickets are refused.
	GuardCommit GuardMode = iota
	// GuardStart protects the start of work: like GuardCommit, and a Blocked ticket is resumed.
	GuardStart
)

// GuardResult is what the guard found and did.
type GuardResult struct {
	Ticket string         `json:"ticket"`
	Before tracker.Status `json:"before"`
	After  tracker.Status `json:"after"`
	// Moved is true when the guard moved the ticket to In Progress.
	Moved bool `json:"moved,omitempty"`
	// Note is a one-line explanation for the user ("started DEN-1 (was To Do)"); empty when nothing
	// needed saying. Skipped explains why no check ran (Plane not configured or unreachable).
	Note    string `json:"note,omitempty"`
	Skipped string `json:"skipped,omitempty"`
}

// ErrTicketLifecycle is returned when the ticket's state does not allow the work.
type ErrTicketLifecycle struct {
	Ticket string
	State  tracker.Status
	Hint   string
}

func (e *ErrTicketLifecycle) Error() string {
	return fmt.Sprintf("ticket %s is %s: %s (bypass once with --no-ticket-guard or %s=off)", e.Ticket, tracker.StatusDisplayName(e.State), e.Hint, TicketGuardEnv)
}

// GuardEnabled reports whether the lifecycle guard is on.
func GuardEnabled() bool {
	switch strings.ToLower(os.Getenv(TicketGuardEnv)) {
	case "off", "0", "false":
		return false
	}
	return true
}

// newGuardClient builds the Plane client for the repository at repoDir (empty: the current directory):
// the environment's credentials with the project named by the repository's .dendrite/plane.json. A seam
// for tests.
var newGuardClient = func(repoDir string) (*plane.Client, error) {
	cfg, err := plane.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	if repoDir != "" && os.Getenv("PLANE_PROJECT_ID") == "" {
		if link, _, lerr := plane.LinkForDir(repoDir); lerr == nil {
			cfg.ProjectID = link.ProjectID
			if link.Workspace != "" && os.Getenv("PLANE_WORKSPACE_SLUG") == "" {
				cfg.WorkspaceSlug = link.Workspace
			}
		}
	}
	return plane.NewClient(cfg, nil)
}

// EnsureTicketInProgress checks the ticket against the Plane project of the repository at repoDir and,
// where the lifecycle allows it, moves it to In Progress. It never blocks work it cannot judge: when Plane
// is not configured or does not answer, the result says so in Skipped and no error is returned. A missing
// or unknown ticket, or a state that does not allow the work, is an error.
func EnsureTicketInProgress(ctx context.Context, repoDir, ticket string, mode GuardMode, dryRun bool) (GuardResult, error) {
	res := GuardResult{Ticket: ticket}
	client, err := newGuardClient(repoDir)
	if err != nil {
		res.Skipped = "Plane is not configured: " + firstLine(err.Error())
		return res, nil
	}
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		res.Skipped = "Plane did not answer: " + firstLine(err.Error())
		return res, nil
	}
	if ticket == "" {
		return res, fmt.Errorf("no ticket: every change follows a ticket. Pull one with `dendrite chronos next` or create one with `dendrite chronos create`, then pass it with --ticket (bypass once with --no-ticket-guard or %s=off)", TicketGuardEnv)
	}
	issue := plane.ResolveIssue(issues, ticket)
	if issue == nil {
		return res, fmt.Errorf("ticket %s does not exist in Plane. Create it with `dendrite chronos create --name \"...\"` (bypass once with --no-ticket-guard or %s=off)", ticket, TicketGuardEnv)
	}
	states, err := client.ListStates(ctx)
	if err != nil {
		res.Skipped = "Plane did not answer: " + firstLine(err.Error())
		return res, nil
	}
	res.Before = tracker.StatusBacklog
	for _, st := range states {
		if st.ID == issue.State {
			res.Before = plane.StatusFor(st.Group, st.Name)
			break
		}
	}
	res.After = res.Before

	switch res.Before {
	case tracker.StatusInProgress:
		return res, nil
	case tracker.StatusTodo, tracker.StatusBacklog:
	case tracker.StatusBlocked:
		if mode == GuardCommit {
			return res, &ErrTicketLifecycle{ticket, res.Before, fmt.Sprintf("resume it first with `dendrite chronos ticket set %s \"in progress\"`", ticket)}
		}
	case tracker.StatusInReview:
		return res, &ErrTicketLifecycle{ticket, res.Before, fmt.Sprintf("it is awaiting verification; reopen it with `dendrite chronos ticket set %s \"in progress\"` for review fixes, or create a follow-up ticket", ticket)}
	default: // Done, Cancelled
		return res, &ErrTicketLifecycle{ticket, res.Before, "it is finished; create a follow-up ticket with `dendrite chronos create`, or reopen it deliberately with `dendrite chronos ticket set`"}
	}

	if dryRun {
		res.Note = fmt.Sprintf("would start %s (%s)", ticket, tracker.StatusDisplayName(res.Before))
		return res, nil
	}
	if err := client.UpdateStatus(ctx, tracker.TicketID(issue.ID), tracker.StatusInProgress); err != nil {
		return res, fmt.Errorf("start %s: %w", ticket, err)
	}
	res.After, res.Moved = tracker.StatusInProgress, true
	res.Note = fmt.Sprintf("started %s (was %s)", ticket, tracker.StatusDisplayName(res.Before))
	return res, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
