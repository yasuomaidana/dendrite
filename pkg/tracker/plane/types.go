package plane

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Standard typed errors for Plane v2 provider operations.
var (
	ErrEmptyBaseURL       = errors.New("plane base URL cannot be empty")
	ErrEmptyWorkspace     = errors.New("plane workspace slug cannot be empty")
	ErrEmptyProjectID     = errors.New("plane project ID cannot be empty")
	ErrMissingAPIToken    = errors.New("plane API token environment variable is empty or not set")
	ErrUnauthorized       = errors.New("plane request unauthorized: invalid API key")
	ErrIssueNotFound      = errors.New("plane issue not found")
	ErrStateNotFound      = errors.New("no matching plane state found for requested status")
	ErrValidationTimeout  = errors.New("plane credential validation timed out")
	ErrPageNotFound       = errors.New("plane page not found")
	ErrModuleNotFound     = errors.New("plane module not found")
	ErrProjectNotFound    = errors.New("plane project not found")
	ErrEmptyTicketContext = errors.New("ticket context/description cannot be empty")
)

var contextHtmlTagRegex = regexp.MustCompile("<[^>]*>")

// IsEmptyContext checks whether a description or context string is empty,
// accounting for HTML markup tags like <div><p></p></div> and non-breaking spaces.
func IsEmptyContext(htmlOrText string) bool {
	cleaned := contextHtmlTagRegex.ReplaceAllString(htmlOrText, " ")
	cleaned = strings.ReplaceAll(cleaned, "&nbsp;", " ")
	return strings.TrimSpace(cleaned) == ""
}

// Config encapsulates settings for the Plane v2 REST client.
type Config struct {
	BaseURL       string `yaml:"base_url" json:"base_url"`
	WorkspaceSlug string `yaml:"workspace_slug" json:"workspace_slug"`
	ProjectID     string `yaml:"project_id" json:"project_id"`
	APITokenEnv   string `yaml:"api_token_env" json:"api_token_env"`
	DirectToken   string `yaml:"direct_token,omitempty" json:"direct_token,omitempty"`
}

// PlaneProject represents a project in Plane v2.
type PlaneProject struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Identifier  string    `json:"identifier"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PlaneStateDetail holds state metadata embedded inside an issue response.
type PlaneStateDetail struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	Slug  string `json:"slug"`
}

// PlaneState represents a workflow state in Plane.
type PlaneState struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Group    string  `json:"group"`
	Color    string  `json:"color"`
	Slug     string  `json:"slug"`
	Sequence float64 `json:"sequence"`
}

// PlaneIssue encapsulates issue details returned by Plane v2 REST API.
type PlaneIssue struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	DescriptionHTML     string           `json:"description_html"`
	DescriptionStripped string           `json:"description_stripped"`
	SequenceID          int              `json:"sequence_id"`
	State               string           `json:"state"`
	StateDetail         PlaneStateDetail `json:"state_detail"`
	EstimatePoint       *int             `json:"estimate_point,omitempty"`
	Priority            string           `json:"priority,omitempty"`
	StartDate           *string          `json:"start_date,omitempty"`
	TargetDate          *string          `json:"target_date,omitempty"`
	Parent              *string          `json:"parent,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
	CompletedAt         *time.Time       `json:"completed_at,omitempty"`
	ModuleIDs           []string         `json:"module_ids,omitempty"`
}

// PlaneModule represents a milestone / module in Plane.
type PlaneModule struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Status          string    `json:"status"` // "backlog", "planned", "in-progress", "paused", "completed", "cancelled"
	StartDate       *string   `json:"start_date,omitempty"`
	TargetDate      *string   `json:"target_date,omitempty"`
	TotalIssues     int       `json:"total_issues,omitempty"`
	CompletedIssues int       `json:"completed_issues,omitempty"`
	SortOrder       float64   `json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// IssueRelation represents an issue relationship (blocking, blocked_by, duplicate, relates_to).
type IssueRelation struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	SequenceID   int    `json:"sequence_id"`
	RelationType string `json:"relation_type"`
	Name         string `json:"name"`
}

// IssueRelationsResponse holds categorized relations for an issue.
type IssueRelationsResponse struct {
	Blocking  []IssueRelation `json:"blocking"`
	BlockedBy []IssueRelation `json:"blocked_by"`
	Duplicate []IssueRelation `json:"duplicate"`
	RelatesTo []IssueRelation `json:"relates_to"`
}

// CreateIssueRequest defines inputs for creating an issue with full metadata.
type CreateIssueRequest struct {
	Name            string   `json:"name"`
	DescriptionHTML string   `json:"description_html,omitempty"`
	StateID         string   `json:"state,omitempty"`
	EstimatePoint   *int     `json:"estimate_point,omitempty"`
	Priority        string   `json:"priority,omitempty"`
	StartDate       *string  `json:"start_date,omitempty"`
	TargetDate      *string  `json:"target_date,omitempty"`
	Parent          *string  `json:"parent,omitempty"`
	ModuleID        string   `json:"module_id,omitempty"`
	BlockedBy       []string `json:"blocked_by,omitempty"`
}

// UpdateIssueRequest defines inputs for modifying an issue in Plane.
type UpdateIssueRequest struct {
	Name            string  `json:"name,omitempty"`
	DescriptionHTML string  `json:"description_html,omitempty"`
	StateID         string  `json:"state,omitempty"`
	EstimatePoint   *int    `json:"estimate_point,omitempty"`
	Priority        string  `json:"priority,omitempty"`
	StartDate       *string `json:"start_date,omitempty"`
	TargetDate      *string `json:"target_date,omitempty"`
	Parent          *string `json:"parent,omitempty"`
}

// CreateModuleRequest defines inputs for creating a module/milestone with date boundaries.
type CreateModuleRequest struct {
	Name        string  `json:"name"`
	Status      string  `json:"status,omitempty"`
	Description string  `json:"description,omitempty"`
	StartDate   *string `json:"start_date,omitempty"`
	TargetDate  *string `json:"target_date,omitempty"`
}

// UpdateModuleRequest defines inputs for patching a module/milestone.
type UpdateModuleRequest struct {
	Name        string  `json:"name,omitempty"`
	Status      string  `json:"status,omitempty"`
	Description string  `json:"description,omitempty"`
	StartDate   *string `json:"start_date,omitempty"`
	TargetDate  *string `json:"target_date,omitempty"`
}

// PlanePage represents a document / page in Plane.
type PlanePage struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	DescriptionHTML     string    `json:"description_html"`
	DescriptionStripped string    `json:"description_stripped"`
	IsLocked            bool      `json:"is_locked"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// SyncDirection specifies documentation synchronization direction.
type SyncDirection string

const (
	DirectionToPlane       SyncDirection = "to-plane"
	DirectionFromPlane     SyncDirection = "from-plane"
	DirectionBidirectional SyncDirection = "bidirectional"
)

// DocSyncReport summarizes documentation synchronization outcomes.
type DocSyncReport struct {
	Direction    SyncDirection `json:"direction"`
	FilesPushed  int           `json:"files_pushed"`
	FilesPulled  int           `json:"files_pulled"`
	PagesCreated int           `json:"pages_created"`
	PagesUpdated int           `json:"pages_updated"`
	TotalSynced  int           `json:"total_synced"`
	Duration     time.Duration `json:"duration"`
	Errors       []string      `json:"errors,omitempty"`
}

// StatusReplicationReport summarizes DEV_STATUS synchronization with Plane.
type StatusReplicationReport struct {
	TotalTickets      int           `json:"total_tickets"`
	CompletedTickets  int           `json:"completed_tickets"`
	InProgressTickets int           `json:"in_progress_tickets"`
	TodoTickets       int           `json:"todo_tickets"`
	BacklogTickets    int           `json:"backlog_tickets"`
	ToVerifyTickets   int           `json:"to_verify_tickets"`
	DoneTickets       int           `json:"done_tickets"`
	OverallProgress   float64       `json:"overall_progress_percent"`
	ActiveMilestone   string        `json:"active_milestone"`
	CurrentFocus      string        `json:"current_focus"`
	ModulesCount      int           `json:"modules_count"`
	PagesCount        int           `json:"pages_count"`
	RelationsCount    int           `json:"relations_count,omitempty"`
	Duration          time.Duration `json:"duration"`
	OtherProjects     []string      `json:"other_projects,omitempty"`
}
