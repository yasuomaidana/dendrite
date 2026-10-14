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
	if strings.TrimSpace(cfg.APITokenEnv) == "" {
		return nil, ErrMissingAPIToken
	}

	token := strings.TrimSpace(os.Getenv(cfg.APITokenEnv))
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

// UpdateStatus moves a Plane issue to the corresponding state.
func (c *Client) UpdateStatus(ctx context.Context, id tracker.TicketID, state tracker.Status) error {
	statesEndpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/states/", c.workspaceSlug, c.projectID)
	req, err := c.newRequest(ctx, http.MethodGet, statesEndpoint, nil)
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
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to fetch states: status %d", resp.StatusCode)
	}

	var statesList []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Group string `json:"group"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&statesList); err != nil {
		return err
	}

	targetStateID := findMatchingPlaneState(statesList, state)
	if targetStateID == "" {
		return ErrStateNotFound
	}

	patchEndpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/%s/", c.workspaceSlug, c.projectID, id)
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

	if patchResp.StatusCode == http.StatusNotFound {
		return ErrIssueNotFound
	}
	if patchResp.StatusCode >= 400 {
		return fmt.Errorf("failed to update state: status %d", patchResp.StatusCode)
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

func findMatchingPlaneState(states []struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}, targetState tracker.Status) string {
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
