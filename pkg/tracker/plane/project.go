package plane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to create plane project: status %d", resp.StatusCode)
	}

	var created PlaneProject
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}
	return &created, nil
}

// ListProjects queries all projects in the workspace.
func (c *Client) ListProjects(ctx context.Context) ([]PlaneProject, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/", c.workspaceSlug)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to list plane projects: status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var paginated struct {
		Results []PlaneProject `json:"results"`
	}
	if err := json.Unmarshal(bodyBytes, &paginated); err == nil && len(paginated.Results) > 0 {
		return paginated.Results, nil
	}

	var directList []PlaneProject
	if err := json.Unmarshal(bodyBytes, &directList); err == nil {
		return directList, nil
	}

	return paginated.Results, nil
}
