package canvas

import (
	"context"
	"time"
)

// ActionID identifies a standard Canvas writing or UI creative action.
type ActionID string

const (
	// 15 Standard Interactive Writing Action IDs
	ActionWriteEmail        ActionID = "WRITE_EMAIL"
	ActionWriteStory        ActionID = "WRITE_STORY"
	ActionWriteReview       ActionID = "WRITE_REVIEW"
	ActionWriteSpeech       ActionID = "WRITE_SPEECH"
	ActionWriteEssay        ActionID = "WRITE_ESSAY"
	ActionWriteOutline      ActionID = "WRITE_OUTLINE"
	ActionWriteFeedback     ActionID = "WRITE_FEEDBACK"
	ActionWriteReport       ActionID = "WRITE_REPORT"
	ActionWriteLessonPlan   ActionID = "WRITE_LESSON_PLAN"
	ActionWriteCoverLetter  ActionID = "WRITE_COVER_LETTER"
	ActionWriteResume       ActionID = "WRITE_RESUME"
	ActionWriteScript       ActionID = "WRITE_SCRIPT"
	ActionWritePoem         ActionID = "WRITE_POEM"
	ActionWritePlan         ActionID = "WRITE_PLAN"
	ActionWritePresentation ActionID = "WRITE_PRESENTATION"

	// 4 UI Creative Action IDs bridging Penpot and Project State
	ActionDesignUI     ActionID = "DESIGN_UI_COMPONENT"
	ActionSyncPenpot   ActionID = "SYNC_PENPOT_FRAME"
	ActionRelateTicket ActionID = "RELATE_TICKET_FEATURE"
	ActionUpdateState  ActionID = "UPDATE_PROJECT_STATE"
)

// WritingMode dictates the operational transformation applied by an action handler.
type WritingMode string

const (
	ModeDraft    WritingMode = "DRAFT"
	ModeRefine   WritingMode = "REFINE"
	ModeExpand   WritingMode = "EXPAND"
	ModeCondense WritingMode = "CONDENSE"
	ModeTone     WritingMode = "CHANGE_TONE"
	ModeCritique WritingMode = "CRITIQUE"
	ModeFormat   WritingMode = "FORMAT"
)

// ActionRequest carries the input parameters for executing a canvas action.
type ActionRequest struct {
	Action         ActionID               `json:"action"`
	Mode           WritingMode            `json:"mode,omitempty"`
	Title          string                 `json:"title,omitempty"`
	Prompt         string                 `json:"prompt"`
	CurrentContent string                 `json:"current_content,omitempty"`
	Parameters     map[string]interface{} `json:"parameters,omitempty"`
	Context        map[string]interface{} `json:"context,omitempty"`
}

// ActionResponse holds the generated content, structured output, and metadata.
type ActionResponse struct {
	Action         ActionID               `json:"action"`
	Mode           WritingMode            `json:"mode,omitempty"`
	Status         string                 `json:"status"` // "success" or "error"
	Title          string                 `json:"title,omitempty"`
	Content        string                 `json:"content"`
	StructuredData map[string]interface{} `json:"structured_data,omitempty"`
	Diff           string                 `json:"diff,omitempty"`
	Notes          []string               `json:"notes,omitempty"`
	UIBridgeResult *UIBridgeResult        `json:"ui_bridge_result,omitempty"`
}

// UIBridgeResult reports execution telemetry and URLs for Penpot UI operations.
type UIBridgeResult struct {
	TicketID       string            `json:"ticket_id,omitempty"`
	FeatureName    string            `json:"feature_name,omitempty"`
	PenpotFrameURL string            `json:"penpot_frame_url,omitempty"`
	ComponentPath  string            `json:"component_path,omitempty"`
	Tokens         map[string]string `json:"tokens,omitempty"`
	ProjectState   string            `json:"project_state,omitempty"`
	PlaneUpdated   bool              `json:"plane_updated"`
	Details        string            `json:"details,omitempty"`
}

// TicketFeatureRelation maps a Plane ticket to a project feature, component, and Penpot frame.
type TicketFeatureRelation struct {
	TicketID       string    `json:"ticket_id"`
	FeatureName    string    `json:"feature_name"`
	Description    string    `json:"description,omitempty"`
	ComponentPath  string    `json:"component_path,omitempty"`
	PenpotProject  string    `json:"penpot_project,omitempty"`
	PenpotFrameID  string    `json:"penpot_frame_id,omitempty"`
	PenpotFrameURL string    `json:"penpot_frame_url,omitempty"`
	CurrentState   string    `json:"current_state,omitempty"`
	LastSyncedAt   time.Time `json:"last_synced_at,omitempty"`
}

// ProjectStateUpdateRequest configures a project state mutation.
type ProjectStateUpdateRequest struct {
	TicketID       string `json:"ticket_id"`
	TargetState    string `json:"target_state"` // e.g. "In Progress", "To Verify", "Done"
	Comment        string `json:"comment,omitempty"`
	PenpotFrameURL string `json:"penpot_frame_url,omitempty"`
	PushToPlane    bool   `json:"push_to_plane"`
}

// ProjectStateUpdateResponse reports the transition outcome.
type ProjectStateUpdateResponse struct {
	TicketID      string    `json:"ticket_id"`
	PreviousState string    `json:"previous_state"`
	NewState      string    `json:"new_state"`
	PlaneSynced   bool      `json:"plane_synced"`
	Message       string    `json:"message"`
	Timestamp     time.Time `json:"timestamp"`
}

// ActionHandler executes a discrete Canvas creative action.
type ActionHandler interface {
	ActionID() ActionID
	Execute(ctx context.Context, req ActionRequest) (*ActionResponse, error)
}
