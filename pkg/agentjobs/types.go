package agentjobs

import (
	"encoding/json"
	"time"
)

// Status represents the lifecycle status of an agent job.
type Status string

const (
	StatusQueued          Status = "queued"
	StatusRunning         Status = "running"
	StatusSucceeded       Status = "succeeded"
	StatusFailedRetryable Status = "failed_retryable"
	StatusQuotaExhausted  Status = "quota_exhausted"
	StatusEarlyExit       Status = "early_exit"
	StatusNeedsReview     Status = "needs_review"
	StatusVerified        Status = "verified"
	StatusFailed          Status = "failed"
	StatusParked          Status = "parked"
	StatusEscalated       Status = "escalated"
	StatusCommitted       Status = "committed"
	StatusCancelled       Status = "cancelled"
)

// Class represents the outcome classification of an agent execution.
type Class string

const (
	ClassQuota      Class = "QUOTA"
	ClassTransient  Class = "TRANSIENT"
	ClassPermission Class = "PERMISSION"
	ClassEarlyExit  Class = "EARLY_EXIT"
	ClassDone       Class = "DONE"

	QUOTA      Class = ClassQuota
	TRANSIENT  Class = ClassTransient
	PERMISSION Class = ClassPermission
	EARLY_EXIT Class = ClassEarlyExit
	DONE       Class = ClassDone
)

// Job represents a detached or scheduled agent task managed by the ledger.
type Job struct {
	ID             string         `json:"id"`
	Ticket         string         `json:"ticket"`
	Complexity     int            `json:"complexity"`
	Worktree       string         `json:"worktree"`
	Scope          []string       `json:"scope"`
	PromptFile     string         `json:"prompt_file"`
	Agent          string         `json:"agent"`
	Model          string         `json:"model"`
	Tier           int            `json:"tier"`
	Attempts       map[string]int `json:"attempts"`
	ConversationID string         `json:"conversation_id"`
	Status         Status         `json:"status"`
	ResumeAfter    time.Time      `json:"resume_after,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	PGID           int            `json:"pgid,omitempty"`
	// Baseline maps files that were dirty before the first attempt to their git blob hash,
	// so the scope check does not blame the agent for pre-existing changes.
	Baseline map[string]string `json:"baseline,omitempty"`
}

type jobJSON Job

// MarshalJSON customizes JSON serialization to omit zero-valued ResumeAfter.
func (j Job) MarshalJSON() ([]byte, error) {
	if j.ResumeAfter.IsZero() {
		type jobWithoutResumeAfter struct {
			jobJSON
			ResumeAfter *time.Time `json:"resume_after,omitempty"`
		}
		return json.Marshal(jobWithoutResumeAfter{
			jobJSON:     jobJSON(j),
			ResumeAfter: nil,
		})
	}
	return json.Marshal(jobJSON(j))
}

// Event represents an entry in the job's write-ahead log (events.jsonl).
type Event struct {
	Seq  int64          `json:"seq"`
	Time time.Time      `json:"time"`
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}
