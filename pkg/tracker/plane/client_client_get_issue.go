package plane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
)

// GetIssueRelations queries all categorized relations (blocking, blocked_by, duplicate, relates_to) for an issue.
func (c *Client) GetIssueRelations(ctx context.Context, issueID string) (*IssueRelationsResponse, error) {
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/issues/%s/issue-relation/", c.workspaceSlug, c.projectID, issueID)
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
		return nil, fmt.Errorf("failed to get issue relations: status %d", resp.StatusCode)
	}

	var relations IssueRelationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&relations); err != nil {
		return nil, err
	}
	return &relations, nil
}

// RemoveIssueRelation removes a specific relationship between two issues.
func (c *Client) RemoveIssueRelation(ctx context.Context, issueID, relationType, relatedIssueID string) error {
	endpoint := fmt.Sprintf("/api/workspaces/%s/projects/%s/issues/%s/remove-relation/", c.workspaceSlug, c.projectID, issueID)
	body := map[string]interface{}{
		"relation_type": relationType,
		"related_issue": relatedIssueID,
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
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to remove issue relation: status %d", resp.StatusCode)
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
// AddIssueComment appends an activity comment to the Plane issue using string ID.
func (c *Client) AddIssueComment(ctx context.Context, id string, comment string) error {
	return c.AddComment(ctx, tracker.TicketID(id), comment)
}

func (c *Client) AddComment(ctx context.Context, id tracker.TicketID, comment string) error {
	issueUUID := string(id)
	if len(issueUUID) != 36 || strings.Count(issueUUID, "-") != 4 {
		if issues, err := c.ListPlaneIssues(ctx); err == nil {
			if matched := findIssueByTicketID(issues, string(id)); matched != nil {
				issueUUID = matched.ID
			}
		}
	}

	endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/%s/comments/", c.workspaceSlug, c.projectID, issueUUID)
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
	case strings.Contains(n, "review") || strings.Contains(n, "verify"):
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

// findIssueByTicketID returns the issue whose UUID is id, else the one named "[id] ...",
// else one mentioning id as a whole token. It never matches by substring, so
// "DEN-10" does not resolve to "[DEN-1014]".
func findIssueByTicketID(issues []PlaneIssue, id string) *PlaneIssue {
	prefix := "[" + id + "]"
	for i := range issues {
		if issues[i].ID == id || strings.HasPrefix(strings.TrimSpace(issues[i].Name), prefix) {
			return &issues[i]
		}
	}
	token := regexp.MustCompile(`(^|[^A-Za-z0-9-])` + regexp.QuoteMeta(id) + `($|[^A-Za-z0-9-])`)
	for i := range issues {
		if token.MatchString(issues[i].Name) {
			return &issues[i]
		}
	}
	return nil
}
