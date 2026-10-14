package jira

import (
	"bytes"
	"context"
	"encoding/base64"
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

// Client is an Atlassian Jira REST v3 issue tracker client adhering to tracker.UnifiedIssueTracker.
type Client struct {
	baseURL    string
	email      string
	apiToken   string
	projectKey string
	httpClient *http.Client
}

// NewClient constructs a new Jira client. APIToken is retrieved exclusively from environment.
func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, ErrEmptyBaseURL
	}
	if strings.TrimSpace(cfg.Email) == "" {
		return nil, ErrEmptyEmail
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

	cleanBase := strings.TrimRight(cfg.BaseURL, "/")

	return &Client{
		baseURL:    cleanBase,
		email:      cfg.Email,
		apiToken:   token,
		projectKey: cfg.ProjectKey,
		httpClient: httpClient,
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

	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.email, c.apiToken)))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// GetIssue retrieves and normalizes an issue from Jira REST v3 into a UnifiedIssue.
func (c *Client) GetIssue(ctx context.Context, id tracker.TicketID) (*tracker.UnifiedIssue, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/rest/api/3/issue/%s", id), nil)
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
		return nil, fmt.Errorf("jira api error: status %d", resp.StatusCode)
	}

	var issueResp struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string          `json:"summary"`
			Description json.RawMessage `json:"description"`
			Status      struct {
				Name string `json:"name"`
			} `json:"status"`
			Labels  []string `json:"labels"`
			Created string   `json:"created"`
			Updated string   `json:"updated"`
		} `json:"fields"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&issueResp); err != nil {
		return nil, err
	}

	descMarkdown := ParseADFToMarkdown(issueResp.Fields.Description)
	normState := mapJiraStatus(issueResp.Fields.Status.Name)

	specs := extractLinkedSpecs(issueResp.Fields.Summary + " " + descMarkdown)

	createdAt, _ := time.Parse(time.RFC3339, issueResp.Fields.Created)
	updatedAt, _ := time.Parse(time.RFC3339, issueResp.Fields.Updated)

	return &tracker.UnifiedIssue{
		ID:                  tracker.TicketID(issueResp.Key),
		Title:               issueResp.Fields.Summary,
		DescriptionMarkdown: descMarkdown,
		State:               normState,
		LinkedSpecs:         specs,
		Labels:              issueResp.Fields.Labels,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}, nil
}

// UpdateStatus maps a normalized state to a Jira workflow transition and executes it.
func (c *Client) UpdateStatus(ctx context.Context, id tracker.TicketID, state tracker.Status) error {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/rest/api/3/issue/%s/transitions", id), nil)
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
		return fmt.Errorf("failed to fetch transitions: status %d", resp.StatusCode)
	}

	var transResp struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"transitions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&transResp); err != nil {
		return err
	}

	transitionID := findMatchingTransition(transResp.Transitions, state)
	if transitionID == "" {
		return ErrTransitionNotFound
	}

	body := map[string]interface{}{
		"transition": map[string]string{
			"id": transitionID,
		},
	}

	postReq, err := c.newRequest(ctx, http.MethodPost, fmt.Sprintf("/rest/api/3/issue/%s/transitions", id), body)
	if err != nil {
		return err
	}

	postResp, err := c.httpClient.Do(postReq)
	if err != nil {
		return err
	}
	defer postResp.Body.Close()

	if postResp.StatusCode >= 400 {
		return fmt.Errorf("transition execution failed: status %d", postResp.StatusCode)
	}

	return nil
}

// AddComment appends an ADF comment to the designated Jira issue.
func (c *Client) AddComment(ctx context.Context, id tracker.TicketID, comment string) error {
	adfBody := BuildADFFromMarkdown(comment)
	body := map[string]interface{}{
		"body": adfBody,
	}

	req, err := c.newRequest(ctx, http.MethodPost, fmt.Sprintf("/rest/api/3/issue/%s/comment", id), body)
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

// LinkPullRequest creates a remote link on the Jira issue pointing to a GitHub/GitLab Pull Request.
func (c *Client) LinkPullRequest(ctx context.Context, id tracker.TicketID, prURL string) error {
	body := map[string]interface{}{
		"object": map[string]interface{}{
			"url":   prURL,
			"title": "Pull Request: " + prURL,
			"icon": map[string]string{
				"url16x16": "https://github.githubassets.com/favicons/favicon.png",
				"title":    "Pull Request",
			},
		},
	}

	req, err := c.newRequest(ctx, http.MethodPost, fmt.Sprintf("/rest/api/3/issue/%s/remotelink", id), body)
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

// ValidateCredentials verifies connectivity and authentication with Jira within < 1.5s.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	req, err := c.newRequest(timeoutCtx, http.MethodGet, "/rest/api/3/myself", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if timeoutCtx.Err() != nil {
			return ErrCredentialsTimeout
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

func mapJiraStatus(jiraStatus string) tracker.Status {
	s := strings.ToLower(jiraStatus)
	switch {
	case strings.Contains(s, "done"), strings.Contains(s, "closed"), strings.Contains(s, "resolved"):
		return tracker.StatusDone
	case strings.Contains(s, "review"):
		return tracker.StatusInReview
	case strings.Contains(s, "progress"), strings.Contains(s, "development"), strings.Contains(s, "active"):
		return tracker.StatusInProgress
	case strings.Contains(s, "todo"), strings.Contains(s, "to do"), strings.Contains(s, "open"):
		return tracker.StatusTodo
	default:
		return tracker.StatusBacklog
	}
}

func findMatchingTransition(transitions []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}, targetState tracker.Status) string {
	targetKeywords := map[tracker.Status][]string{
		tracker.StatusTodo:       {"todo", "to do", "open", "backlog"},
		tracker.StatusInProgress: {"in progress", "start progress", "development"},
		tracker.StatusInReview:   {"review", "in review", "code review"},
		tracker.StatusDone:       {"done", "closed", "resolved"},
	}

	keywords := targetKeywords[targetState]
	for _, t := range transitions {
		tName := strings.ToLower(t.Name)
		for _, kw := range keywords {
			if strings.Contains(tName, kw) {
				return t.ID
			}
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
