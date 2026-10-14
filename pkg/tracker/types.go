package tracker

import (
	"context"
	"fmt"
	"time"
)

// Status represents the operational lifecycle state of a ticket.
type Status string

const (
	StatusBacklog    Status = "BACKLOG"
	StatusTodo       Status = "TODO"
	StatusInProgress Status = "IN_PROGRESS"
	StatusInReview   Status = "IN_REVIEW"
	StatusDone       Status = "DONE"
)

// TicketID is a domain newtype for ticket identifiers (e.g., "DEN-001", "PROJ-142").
type TicketID string

// Ticket encapsulates all metadata for an engineering work item.
type Ticket struct {
	ID                 TicketID  `json:"id"`
	Title              string    `json:"title"`
	Description        string    `json:"description,omitempty"`
	Status             Status    `json:"status"`
	Milestone          string    `json:"milestone,omitempty"`
	TargetPackage      string    `json:"target_package,omitempty"`
	AcceptanceCriteria []string  `json:"acceptance_criteria,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// UnifiedIssue represents a normalized work item across any issue tracker provider (Jira, Plane, Linear, GitHub).
type UnifiedIssue struct {
	ID                  TicketID  `json:"id"`
	Title               string    `json:"title"`
	DescriptionMarkdown string    `json:"description_markdown"`
	State               Status    `json:"state"`
	LinkedSpecs         []string  `json:"linked_specs,omitempty"`
	Labels              []string  `json:"labels,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// IssueTracker defines the abstract interface for managing work tickets in markdown/local stores.
type IssueTracker interface {
	GetTicket(ctx context.Context, id TicketID) (*Ticket, error)
	ListTickets(ctx context.Context) ([]Ticket, error)
	CreateTicket(ctx context.Context, t Ticket) error
	UpdateStatus(ctx context.Context, id TicketID, status Status) error
	Sync(ctx context.Context) error
}

// UnifiedIssueTracker defines the provider strategy pattern interface specified in FEAT-008.
type UnifiedIssueTracker interface {
	GetIssue(ctx context.Context, id TicketID) (*UnifiedIssue, error)
	UpdateStatus(ctx context.Context, id TicketID, state Status) error
	AddComment(ctx context.Context, id TicketID, comment string) error
	LinkPullRequest(ctx context.Context, id TicketID, prURL string) error
	ValidateCredentials(ctx context.Context) error
}

// ErrTicketNotFound is returned when a requested ticket does not exist.
type ErrTicketNotFound struct {
	ID TicketID
}

func (e *ErrTicketNotFound) Error() string {
	return fmt.Sprintf("ticket not found: %s", e.ID)
}
