package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
)

var specRegex = regexp.MustCompile(`\b(FEAT-\d+|ADR-\d+)\b`)

// Client is a Plane v2 REST API client adhering to tracker.UnifiedIssueTracker.
type Client struct {
	baseURL       string
	workspaceSlug string
	projectID     string
	apiKey        string
	httpClient    *http.Client
}

// NewClient constructs a new Plane v2 issue tracker client.
func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, ErrEmptyBaseURL
	}
	if strings.TrimSpace(cfg.WorkspaceSlug) == "" {
		return nil, ErrEmptyWorkspace
	}
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, ErrEmptyProjectID
	}

	token := cfg.DirectToken
	if token == "" && cfg.APITokenEnv != "" {
		token = strings.TrimSpace(os.Getenv(cfg.APITokenEnv))
	}
	if token == "" {
		return nil, ErrMissingAPIToken
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	return &Client{
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		workspaceSlug: cfg.WorkspaceSlug,
		projectID:     cfg.ProjectID,
		apiKey:        token,
		httpClient:    httpClient,
	}, nil
}

// NewClientFromEnv constructs a Plane client using system environment variables or sensible local defaults.
func NewClientFromEnv() (*Client, error) {
	baseURL := os.Getenv("PLANE_BASE_URL")
	if baseURL == "" {
		port := os.Getenv("PLANE_API_PORT")
		if port == "" {
			port = "8090"
		}
		baseURL = fmt.Sprintf("http://localhost:%s", port)
	}

	workspaceSlug := os.Getenv("PLANE_WORKSPACE_SLUG")
	if workspaceSlug == "" {
		workspaceSlug = "dendrite-ws"
	}

	projectID := os.Getenv("PLANE_PROJECT_ID")
	if projectID == "" {
		projectID = "e6380603-1428-4a66-995e-e06039915d52"
	}

	apiKey := os.Getenv("PLANE_API_KEY")
	if apiKey == "" {
		apiKey = "plane_sec_dendrite_token_12345"
	}

	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		workspaceSlug: workspaceSlug,
		projectID:     projectID,
		apiKey:        apiKey,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// WorkspaceSlug returns the active workspace slug.
func (c *Client) WorkspaceSlug() string {
	return c.workspaceSlug
}

// ProjectID returns the active project ID.
func (c *Client) ProjectID() string {
	return c.projectID
}

func (c *Client) newRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	url := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// GetIssue retrieves an issue from Plane v2 and maps it into a UnifiedIssue.
func (c *Client) GetIssue(ctx context.Context, id tracker.TicketID) (*tracker.UnifiedIssue, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/%s/", c.workspaceSlug, c.projectID, id)
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
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrIssueNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("plane api error: status %d", resp.StatusCode)
	}

	var issueResp struct {
		ID                  string `json:"id"`
		Name                string `json:"name"`
		DescriptionStripped string `json:"description_stripped"`
		StateDetail         struct {
			Name  string `json:"name"`
			Group string `json:"group"`
		} `json:"state_detail"`
		LabelDetails []struct {
			Name string `json:"name"`
		} `json:"label_details"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&issueResp); err != nil {
		return nil, err
	}

	state := mapPlaneState(issueResp.StateDetail.Group, issueResp.StateDetail.Name)
	var labels []string
	for _, l := range issueResp.LabelDetails {
		labels = append(labels, l.Name)
	}

	specs := extractLinkedSpecs(issueResp.Name + " " + issueResp.DescriptionStripped)
	createdAt, _ := time.Parse(time.RFC3339, issueResp.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, issueResp.UpdatedAt)

	return &tracker.UnifiedIssue{
		ID:                  tracker.TicketID(issueResp.ID),
		Title:               issueResp.Name,
		DescriptionMarkdown: issueResp.DescriptionStripped,
		State:               state,
		LinkedSpecs:         specs,
		Labels:              labels,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}, nil
}

// ListPlaneIssues queries all issues for the project.
func (c *Client) ListPlaneIssues(ctx context.Context) ([]PlaneIssue, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/", c.workspaceSlug, c.projectID)
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
		return nil, fmt.Errorf("failed to list plane issues: status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var paginated struct {
		Results []PlaneIssue `json:"results"`
	}
	if err := json.Unmarshal(bodyBytes, &paginated); err == nil && len(paginated.Results) > 0 {
		return paginated.Results, nil
	}

	var directList []PlaneIssue
	if err := json.Unmarshal(bodyBytes, &directList); err == nil {
		return directList, nil
	}

	return paginated.Results, nil
}

// ListStates queries all states configured for the project.
func (c *Client) ListStates(ctx context.Context) ([]PlaneState, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/states/", c.workspaceSlug, c.projectID)
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
	if err := json.Unmarshal(bodyBytes, &paginated); err == nil && len(paginated.Results) > 0 {
		return paginated.Results, nil
	}

	var directList []PlaneState
	if err := json.Unmarshal(bodyBytes, &directList); err == nil {
		return directList, nil
	}

	return paginated.Results, nil
}

// UpdateStatus moves a Plane issue to the corresponding state.
func (c *Client) UpdateStatus(ctx context.Context, id tracker.TicketID, state tracker.Status) error {
	states, err := c.ListStates(ctx)
	if err != nil {
		return err
	}

	targetStateID := findMatchingPlaneStateFromList(states, state)
	if targetStateID == "" {
		return ErrStateNotFound
	}

	issueUUID := string(id)
	if len(issueUUID) != 36 || strings.Count(issueUUID, "-") != 4 {
		issues, err := c.ListPlaneIssues(ctx)
		if err == nil {
			for _, iss := range issues {
				if strings.Contains(iss.Name, "["+string(id)+"]") || strings.Contains(iss.Name, string(id)) {
					issueUUID = iss.ID
					break
				}
			}
		}
	}

	patchEndpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/%s/", c.workspaceSlug, c.projectID, issueUUID)
	patchBody := map[string]string{
		"state": targetStateID,
	}

	patchReq, err := c.newRequest(ctx, http.MethodPatch, patchEndpoint, patchBody)
	if err != nil {
		return err
	}

	patchResp, err := c.httpClient.Do(patchReq)
	if err != nil {
		return err
	}
	defer patchResp.Body.Close()

	if patchResp.StatusCode == http.StatusUnauthorized || patchResp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if patchResp.StatusCode == http.StatusNotFound {
		return ErrIssueNotFound
	}
	if patchResp.StatusCode >= 400 {
		return fmt.Errorf("failed to update state: status %d", patchResp.StatusCode)
	}

	return nil
}

// CreatePlaneIssue creates a new issue in Plane with rich metadata.
func (c *Client) CreatePlaneIssue(ctx context.Context, name, descHTML, stateID string, estimate *int) (*PlaneIssue, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/", c.workspaceSlug, c.projectID)
	body := map[string]interface{}{
		"name":             name,
		"description_html": descHTML,
	}
	if stateID != "" {
		body["state"] = stateID
	}
	if estimate != nil {
		body["estimate_point"] = *estimate
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
		return nil, fmt.Errorf("failed to create issue: status %d", resp.StatusCode)
	}

	var created PlaneIssue
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}
	return &created, nil
}

// UpdatePlaneIssue patches an existing issue in Plane.
func (c *Client) UpdatePlaneIssue(ctx context.Context, id, name, descHTML, stateID string, estimate *int) (*PlaneIssue, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/%s/", c.workspaceSlug, c.projectID, id)
	body := make(map[string]interface{})
	if name != "" {
		body["name"] = name
	}
	if descHTML != "" {
		body["description_html"] = descHTML
	}
	if stateID != "" {
		body["state"] = stateID
	}
	if estimate != nil {
		body["estimate_point"] = *estimate
	}

	req, err := c.newRequest(ctx, http.MethodPatch, endpoint, body)
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
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrIssueNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to patch issue: status %d", resp.StatusCode)
	}

	var updated PlaneIssue
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// ListModules queries all modules (milestones) for the project.
func (c *Client) ListModules(ctx context.Context) ([]PlaneModule, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/modules/", c.workspaceSlug, c.projectID)
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
		return nil, fmt.Errorf("failed to list modules: status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var paginated struct {
		Results []PlaneModule `json:"results"`
	}
	if err := json.Unmarshal(bodyBytes, &paginated); err == nil && len(paginated.Results) > 0 {
		return paginated.Results, nil
	}

	var directList []PlaneModule
	if err := json.Unmarshal(bodyBytes, &directList); err == nil {
		return directList, nil
	}

	return paginated.Results, nil
}

// CreateModule registers a new module (milestone) in Plane.
func (c *Client) CreateModule(ctx context.Context, name, status, desc string) (*PlaneModule, error) {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/modules/", c.workspaceSlug, c.projectID)
	if status == "" {
		status = "in-progress"
	}
	body := map[string]interface{}{
		"name":        name,
		"status":      status,
		"description": desc,
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
		return nil, fmt.Errorf("failed to create module: status %d", resp.StatusCode)
	}

	var created PlaneModule
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}
	return &created, nil
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

	req, err := c.newRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrModuleNotFound
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to add issues to module: status %d", resp.StatusCode)
	}

	return nil
}

// ListPages queries all documents/pages for the project.
func (c *Client) ListPages(ctx context.Context) ([]PlanePage, error) {
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/pages/", c.workspaceSlug, c.projectID)
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
		return nil, fmt.Errorf("failed to list pages: status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var directList []PlanePage
	if err := json.Unmarshal(bodyBytes, &directList); err == nil {
		return directList, nil
	}

	var paginated struct {
		Results []PlanePage `json:"results"`
	}
	if err := json.Unmarshal(bodyBytes, &paginated); err == nil {
		return paginated.Results, nil
	}

	return nil, nil
}

// GetPage retrieves details of a specific page.
func (c *Client) GetPage(ctx context.Context, pageID string) (*PlanePage, error) {
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/pages/%s/", c.workspaceSlug, c.projectID, pageID)
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
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPageNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to get page: status %d", resp.StatusCode)
	}

	var page PlanePage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	return &page, nil
}

// CreatePage publishes a new page document in Plane.
func (c *Client) CreatePage(ctx context.Context, name, htmlContent string) (*PlanePage, error) {
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/pages/", c.workspaceSlug, c.projectID)
	body := map[string]interface{}{
		"name":             name,
		"description_html": htmlContent,
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
		return nil, fmt.Errorf("failed to create page: status %d", resp.StatusCode)
	}

	var page PlanePage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	return &page, nil
}

// UpdatePage updates an existing page document in Plane.
func (c *Client) UpdatePage(ctx context.Context, pageID, name, htmlContent string) (*PlanePage, error) {
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/pages/%s/", c.workspaceSlug, c.projectID, pageID)
	body := make(map[string]interface{})
	if name != "" {
		body["name"] = name
	}
	if htmlContent != "" {
		body["description_html"] = htmlContent
	}

	req, err := c.newRequest(ctx, http.MethodPatch, endpoint, body)
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
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPageNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to update page: status %d", resp.StatusCode)
	}

	var page PlanePage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	return &page, nil
}

// ArchivePage archives a page in Plane (prerequisite before deletion).
func (c *Client) ArchivePage(ctx context.Context, pageID string) error {
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/pages/%s/archive/", c.workspaceSlug, c.projectID, pageID)
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrPageNotFound
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to archive page: status %d", resp.StatusCode)
	}
	return nil
}

// AddComment appends an activity comment to the Plane issue.
func (c *Client) AddComment(ctx context.Context, id tracker.TicketID, comment string) error {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/%s/comments/", c.workspaceSlug, c.projectID, id)
	body := map[string]string{
		"comment_html": fmt.Sprintf("<p>%s</p>", comment),
	}

	req, err := c.newRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrIssueNotFound
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to add comment: status %d", resp.StatusCode)
	}

	return nil
}

// LinkPullRequest creates an external link reference on the Plane issue.
func (c *Client) LinkPullRequest(ctx context.Context, id tracker.TicketID, prURL string) error {
	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/%s/links/", c.workspaceSlug, c.projectID, id)
	body := map[string]string{
		"url":   prURL,
		"title": "Pull Request: " + prURL,
	}

	req, err := c.newRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrIssueNotFound
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to link pull request: status %d", resp.StatusCode)
	}

	return nil
}

// ValidateCredentials verifies API key validity against Plane within < 1.5s.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	req, err := c.newRequest(timeoutCtx, http.MethodGet, "/api/v1/users/me/", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if timeoutCtx.Err() != nil {
			return ErrValidationTimeout
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("credential validation failed: status %d", resp.StatusCode)
	}

	return nil
}

func mapPlaneState(group, name string) tracker.Status {
	g := strings.ToLower(group)
	n := strings.ToLower(name)
	switch {
	case g == "completed" || strings.Contains(n, "done"):
		return tracker.StatusDone
	case strings.Contains(n, "review"):
		return tracker.StatusInReview
	case g == "started" || strings.Contains(n, "progress") || strings.Contains(n, "active"):
		return tracker.StatusInProgress
	case g == "unstarted" || strings.Contains(n, "todo") || strings.Contains(n, "to do"):
		return tracker.StatusTodo
	default:
		return tracker.StatusBacklog
	}
}

func findMatchingPlaneStateFromList(states []PlaneState, targetState tracker.Status) string {
	for _, s := range states {
		mapped := mapPlaneState(s.Group, s.Name)
		if mapped == targetState {
			return s.ID
		}
	}
	return ""
}

func extractLinkedSpecs(content string) []string {
	matches := specRegex.FindAllString(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}
