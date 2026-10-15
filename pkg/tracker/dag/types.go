package dag

import (
	"time"

	"Dendrite/pkg/tracker"
)

// Priority levels matching Plane v2 schema.
const (
	PriorityUrgent = "urgent"
	PriorityHigh   = "high"
	PriorityMedium = "medium"
	PriorityLow    = "low"
	PriorityNone   = "none"
)

// Node represents a work item (ticket or story) in the dependency DAG.
type Node struct {
	ID             string         `json:"id"`
	SequenceID     int            `json:"sequence_id"`
	Title          string         `json:"title"`
	Description    string         `json:"description,omitempty"`
	Milestone      string         `json:"milestone"`
	Story          string         `json:"story,omitempty"`
	Status         tracker.Status `json:"status"`
	Priority       string         `json:"priority"`
	EstimatePoints int            `json:"estimate_points"`
	Complexity     float64        `json:"complexity"`
	StartDate      string         `json:"start_date,omitempty"`
	TargetDate     string         `json:"target_date,omitempty"`
	ParentID       string         `json:"parent_id,omitempty"`
	BlockedBy      []string       `json:"blocked_by"`
	Blocks         []string       `json:"blocks"`
	UnblockWeight  float64        `json:"unblock_weight"`
	PriorityScore  float64        `json:"priority_score"`
	TotalScore     float64        `json:"total_score"`
}

// MilestoneSchedule represents a calculated timeline for a milestone in Gantt view.
type MilestoneSchedule struct {
	Name            string   `json:"name"`
	StartDate       string   `json:"start_date"`
	TargetDate      string   `json:"target_date"`
	TicketCount     int      `json:"ticket_count"`
	CompletedCount  int      `json:"completed_count"`
	Tickets         []string `json:"tickets"`
}

// SelectionResult encapsulates the output of a smart ticket selection query.
type SelectionResult struct {
	SelectedTicket    *Node    `json:"selected_ticket"`
	IsActive          bool     `json:"is_active"`
	IsBlocked         bool     `json:"is_blocked"`
	BlockingTickets   []string `json:"blocking_tickets,omitempty"`
	UnblockedDownstream []string `json:"unblocked_downstream,omitempty"`
	TotalScore        float64  `json:"total_score"`
	Reason            string   `json:"reason"`
	Milestone         string   `json:"milestone"`
	GanttStartDate    string   `json:"gantt_start_date"`
	GanttTargetDate   string   `json:"gantt_target_date"`
}

// GanttPlan represents the full calculated development plan for Gantt diagram rendering.
type GanttPlan struct {
	AnchorDate time.Time                    `json:"anchor_date"`
	Milestones map[string]*MilestoneSchedule `json:"milestones"`
	Tickets    map[string]*Node             `json:"tickets"`
	Order      []string                     `json:"order"`
}
