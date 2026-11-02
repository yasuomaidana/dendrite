package plane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// CreatePlaneIssueWithDetails creates a new issue with full Gantt dates, priority, and parent link.
func (c *Client) CreatePlaneIssueWithDetails(ctx context.Context, req CreateIssueRequest) (*PlaneIssue, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("ticket name cannot be empty")
	}
	if IsEmptyContext(req.DescriptionHTML) {
		return nil, ErrEmptyTicketContext
	}
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/", c.workspaceSlug, c.projectID)
	body := map[string]interface{}{
		"name":             req.Name,
		"description_html": req.DescriptionHTML,
	}
	if req.StateID != "" {
		body["state"] = req.StateID
	}
	if req.EstimatePoint != nil {
		ep := *req.EstimatePoint
		if ep > 12 {
			ep = 12
		} else if ep < 0 {
			ep = 0
		}
		body["estimate_point"] = ep
	}
	if req.Priority != "" {
		body["priority"] = req.Priority
	}
	if req.StartDate != nil {
		body["start_date"] = *req.StartDate
	}
	if req.TargetDate != nil {
		body["target_date"] = *req.TargetDate
	}
	if req.Parent != nil {
		body["parent"] = *req.Parent
	}
	if req.ExternalID != "" {
		body["external_id"] = req.ExternalID
	}
	if req.ExternalSource != "" {
		body["external_source"] = req.ExternalSource
	}
	if len(req.Labels) > 0 {
		body["labels"] = req.Labels
	}

	var created PlaneIssue
	if err := c.call(ctx, http.MethodPost, endpoint, body, &created, callSpec{failure: "failed to create issue", withBody: true}); err != nil {
		return nil, err
	}

	// Link to module if requested
	if req.ModuleID != "" && created.ID != "" {
		_ = c.AddIssuesToModule(ctx, req.ModuleID, []string{created.ID})
	}

	// Link blocker relations if requested
	if len(req.BlockedBy) > 0 && created.ID != "" {
		_ = c.AddIssueRelation(ctx, created.ID, "blocked_by", req.BlockedBy)
	}

	return &created, nil
}

// UpdatePlaneIssue patches an existing issue in Plane (backward compatible).
func (c *Client) UpdatePlaneIssue(ctx context.Context, id, name, descHTML, stateID string, estimate *int) (*PlaneIssue, error) {
	return c.UpdatePlaneIssueWithDetails(ctx, id, UpdateIssueRequest{
		Name:            name,
		DescriptionHTML: descHTML,
		StateID:         stateID,
		EstimatePoint:   estimate,
	})
}

// UpdatePlaneIssueWithDetails patches an existing issue in Plane with Gantt dates, priority, and parent link.
func (c *Client) UpdatePlaneIssueWithDetails(ctx context.Context, id string, req UpdateIssueRequest) (*PlaneIssue, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/%s/", c.workspaceSlug, c.projectID, id)
	body := make(map[string]interface{})
	if req.Name != "" {
		body["name"] = req.Name
	}
	if req.DescriptionHTML != "" {
		body["description_html"] = req.DescriptionHTML
	}
	if req.StateID != "" {
		body["state"] = req.StateID
	}
	if req.EstimatePoint != nil {
		ep := *req.EstimatePoint
		if ep > 12 {
			ep = 12
		} else if ep < 0 {
			ep = 0
		}
		body["estimate_point"] = ep
	}
	if req.Priority != "" {
		body["priority"] = req.Priority
	}
	if req.StartDate != nil {
		body["start_date"] = *req.StartDate
	}
	if req.TargetDate != nil {
		body["target_date"] = *req.TargetDate
	}
	if req.Parent != nil {
		body["parent"] = *req.Parent
	}

	var updated PlaneIssue
	if err := c.call(ctx, http.MethodPatch, endpoint, body, &updated, callSpec{failure: "failed to patch issue", notFound: ErrIssueNotFound, withBody: true}); err != nil {
		return nil, err
	}
	return &updated, nil
}

// ListModules queries all modules (milestones) for the project.
func (c *Client) ListModules(ctx context.Context) ([]PlaneModule, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/modules/", c.workspaceSlug, c.projectID)
	return fetchResourceList[PlaneModule](ctx, c, endpoint, "failed to list modules")
}

// CreateModule registers a new module (milestone) in Plane (backward compatible).
func (c *Client) CreateModule(ctx context.Context, name, status, desc string) (*PlaneModule, error) {
	return c.CreateModuleWithDates(ctx, name, status, desc, nil, nil)
}

// CreateModuleWithDates registers a new module (milestone) in Plane with Gantt dates.
func (c *Client) CreateModuleWithDates(ctx context.Context, name, status, desc string, startDate, targetDate *string) (*PlaneModule, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/modules/", c.workspaceSlug, c.projectID)
	if status == "" {
		status = "in-progress"
	}
	body := map[string]interface{}{
		"name":        name,
		"status":      status,
		"description": desc,
	}
	if startDate != nil {
		body["start_date"] = *startDate
	}
	if targetDate != nil {
		body["target_date"] = *targetDate
	}

	var created PlaneModule
	if err := c.call(ctx, http.MethodPost, endpoint, body, &created, callSpec{failure: "failed to create module"}); err != nil {
		return nil, err
	}
	return &created, nil
}

// UpdateModule patches an existing module (milestone) in Plane with Gantt dates and status.
func (c *Client) UpdateModule(ctx context.Context, moduleID string, req UpdateModuleRequest) (*PlaneModule, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/modules/%s/", c.workspaceSlug, c.projectID, moduleID)
	body := make(map[string]interface{})
	if req.Name != "" {
		body["name"] = req.Name
	}
	if req.Status != "" {
		body["status"] = req.Status
	}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if req.StartDate != nil {
		body["start_date"] = *req.StartDate
	}
	if req.TargetDate != nil {
		body["target_date"] = *req.TargetDate
	}

	var updated PlaneModule
	if err := c.call(ctx, http.MethodPatch, endpoint, body, &updated, callSpec{failure: "failed to patch module", notFound: ErrModuleNotFound}); err != nil {
		return nil, err
	}
	return &updated, nil
}

// AddIssuesToModule binds a list of issue IDs to a given module.
func (c *Client) AddIssuesToModule(ctx context.Context, moduleID string, issueIDs []string) error {
	if len(issueIDs) == 0 {
		return nil
	}
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/modules/%s/issues/", c.workspaceSlug, c.projectID, moduleID)
	body := map[string]interface{}{
		"issues": issueIDs,
	}

	return c.call(ctx, http.MethodPost, endpoint, body, nil, callSpec{failure: "failed to add issues to module", notFound: ErrModuleNotFound})
}

// ListModuleIssues queries all issues attached to a specific module.
func (c *Client) ListModuleIssues(ctx context.Context, moduleID string) ([]PlaneIssue, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/modules/%s/module-issues/", c.workspaceSlug, c.projectID, moduleID)
	data, err := c.callRaw(ctx, http.MethodGet, endpoint, nil, callSpec{failure: "failed to list module issues"})
	if err != nil {
		return nil, err
	}
	return decodeList[PlaneIssue](data), nil
}

// AddIssueRelation establishes relations (blocking, blocked_by, relates_to, duplicate) between issues.
func (c *Client) AddIssueRelation(ctx context.Context, issueID string, relationType string, relatedIssueIDs []string) error {
	if len(relatedIssueIDs) == 0 {
		return nil
	}
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/issues/%s/issue-relation/", c.workspaceSlug, c.projectID, issueID)
	body := map[string]interface{}{
		"relation_type": relationType,
		"issues":        relatedIssueIDs,
	}

	if err := c.call(ctx, http.MethodPost, endpoint, body, nil, callSpec{failure: "failed to add issue relation", withBody: true}); err != nil {
		return err
	}

	notifyRelationsChanged(c, append([]string{issueID}, relatedIssueIDs...))
	return nil
}
