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

// EnsureLifecycleStates ensures the required lifecycle states exist for a project.
// In particular, it provisions the "To Verify" state (group "started") if missing.
// It returns a slice containing the names of any states created.
func (c *Client) EnsureLifecycleStates(ctx context.Context, projectID string) ([]string, error) {
	if strings.TrimSpace(projectID) == "" {
		projectID = c.projectID
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, ErrEmptyProjectID
	}

	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/states/", c.workspaceSlug, projectID)
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
		return nil, fmt.Errorf("failed to list plane states: status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var paginated struct {
		Results []PlaneState `json:"results"`
	}
	var existingStates []PlaneState
	if err := json.Unmarshal(bodyBytes, &paginated); err == nil && len(paginated.Results) > 0 {
		existingStates = paginated.Results
	} else {
		var directList []PlaneState
		if err := json.Unmarshal(bodyBytes, &directList); err == nil {
			existingStates = directList
		}
	}

	for _, s := range existingStates {
		if strings.EqualFold(strings.TrimSpace(s.Name), "To Verify") {
			return nil, nil
		}
	}

	postBody := map[string]interface{}{
		"name":  "To Verify",
		"group": "started",
		"color": "#F59E0B",
	}
	postReq, err := c.newRequest(ctx, http.MethodPost, endpoint, postBody)
	if err != nil {
		return nil, err
	}

	postResp, err := c.httpClient.Do(postReq)
	if err != nil {
		return nil, err
	}
	defer postResp.Body.Close()

	if postResp.StatusCode == http.StatusUnauthorized || postResp.StatusCode == http.StatusForbidden {
		return nil, ErrUnauthorized
	}
	if postResp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to create plane state: status %d", postResp.StatusCode)
	}

	return []string{"To Verify"}, nil
}
