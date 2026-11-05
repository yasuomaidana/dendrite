package plane

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// CreateProject creates a new project under the current workspace in Plane.
func (c *Client) CreateProject(ctx context.Context, name, identifier, description string) (*PlaneProject, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/", c.workspaceSlug)
	body := map[string]interface{}{
		"name":        name,
		"identifier":  strings.ToUpper(strings.TrimSpace(identifier)),
		"description": description,
	}
	var created PlaneProject
	if err := c.call(ctx, http.MethodPost, endpoint, body, &created, callSpec{failure: "failed to create plane project"}); err != nil {
		return nil, err
	}
	return &created, nil
}

// ListProjects queries all projects in the workspace.
func (c *Client) ListProjects(ctx context.Context) ([]PlaneProject, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/", c.workspaceSlug)
	return fetchResourceList[PlaneProject](ctx, c, endpoint, "failed to list plane projects")
}

// EnsureLifecycleStates converges a project's workflow states onto CanonicalStates
// (see ReconcileStates) and returns the names of any states it created.
func (c *Client) EnsureLifecycleStates(ctx context.Context, projectID string) ([]string, error) {
	report, err := c.ReconcileStates(ctx, projectID, false)
	if err != nil {
		return nil, err
	}
	return report.Created(), nil
}
