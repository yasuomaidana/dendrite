package planesvc

import (
	"context"

	"Dendrite/pkg/tracker/plane"
)

// issueReply is the shape every issue.* method answers with.
type issueReply struct {
	Issue   plane.PlaneIssue `json:"issue"`
	Created bool             `json:"created"`
}

// GetIssue reads one issue; ref is DEN-n, a Plane number or the issue UUID.
func (s *Service) GetIssue(ctx context.Context, c *plane.Client, ref string) (*plane.PlaneIssue, error) {
	params := target(c)
	params["ref"] = ref
	var out issueReply
	if err := s.call(ctx, false, "issue.get", params, &out); err != nil {
		return nil, err
	}
	return &out.Issue, nil
}

// CreateIssue creates an issue. With req.ExternalID set it is idempotent: an issue that already
// carries the id comes back with created == false and nothing is posted. The module and the
// blocked_by links are made here, only for a new issue, until the sidecar owns them (DEN-355/356).
func (s *Service) CreateIssue(ctx context.Context, c *plane.Client, req plane.CreateIssueRequest) (*plane.PlaneIssue, bool, error) {
	params := target(c)
	params["name"] = req.Name
	params["description_html"] = req.DescriptionHTML
	setText(params, "state", req.StateID)
	setText(params, "priority", req.Priority)
	setText(params, "external_id", req.ExternalID)
	setText(params, "external_source", req.ExternalSource)
	setPtr(params, "estimate_point", req.EstimatePoint)
	setPtr(params, "start_date", req.StartDate)
	setPtr(params, "target_date", req.TargetDate)
	setPtr(params, "parent", req.Parent)
	if len(req.Labels) > 0 {
		params["labels"] = req.Labels
	}
	var out issueReply
	if err := s.call(ctx, false, "issue.create", params, &out); err != nil {
		return nil, false, err
	}
	if out.Created && out.Issue.ID != "" {
		if req.ModuleID != "" {
			_ = c.AddIssuesToModule(ctx, req.ModuleID, []string{out.Issue.ID})
		}
		if len(req.BlockedBy) > 0 {
			_ = c.AddIssueRelation(ctx, out.Issue.ID, "blocked_by", req.BlockedBy)
		}
	}
	return &out.Issue, out.Created, nil
}

// UpdateIssue changes the fields of req that are set; ref is DEN-n, a Plane number or a UUID.
func (s *Service) UpdateIssue(ctx context.Context, c *plane.Client, ref string, req plane.UpdateIssueRequest) (*plane.PlaneIssue, error) {
	params := target(c)
	params["ref"] = ref
	setText(params, "name", req.Name)
	setText(params, "description_html", req.DescriptionHTML)
	setText(params, "state", req.StateID)
	setText(params, "priority", req.Priority)
	setPtr(params, "estimate_point", req.EstimatePoint)
	setPtr(params, "start_date", req.StartDate)
	setPtr(params, "target_date", req.TargetDate)
	setPtr(params, "parent", req.Parent)
	// A non-nil empty list is sent: it clears the field.
	if req.Assignees != nil {
		params["assignees"] = req.Assignees
	}
	if req.Labels != nil {
		params["labels"] = req.Labels
	}
	var out issueReply
	if err := s.call(ctx, false, "issue.update", params, &out); err != nil {
		return nil, err
	}
	return &out.Issue, nil
}

// Comment adds a comment to the issue.
func (s *Service) Comment(ctx context.Context, c *plane.Client, ref, comment string) error {
	params := target(c)
	params["ref"] = ref
	params["comment"] = comment
	return s.call(ctx, false, "issue.comment", params, nil)
}

func setText(params map[string]any, key, v string) {
	if v != "" {
		params[key] = v
	}
}

func setPtr[T any](params map[string]any, key string, v *T) {
	if v != nil {
		params[key] = *v
	}
}
