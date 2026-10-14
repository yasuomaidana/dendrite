package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
)

var specRegex = regexp.MustCompile(`\b(FEAT-\d+|ADR-\d+)\b`)

// Client is a Linear GraphQL API client adhering to tracker.UnifiedIssueTracker.
type Client struct {
	endpoint   string
	apiKey     string
	teamKey    string
	httpClient *http.Client
}

// NewClient constructs a new Linear GraphQL tracker client.
func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(cfg.APITokenEnv) == "" {
		return nil, ErrMissingAPIToken
	}

	token := strings.TrimSpace(os.Getenv(cfg.APITokenEnv))
	if token == "" {
		return nil, ErrMissingAPIToken
	}

	endpoint := cfg.Endpoint
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	return &Client{
		endpoint:   endpoint,
		apiKey:     token,
		teamKey:    cfg.TeamKey,
		httpClient: httpClient,
	}, nil
}

type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) executeQuery(ctx context.Context, query string, variables map[string]interface{}) (json.RawMessage, error) {
	reqBody := graphQLRequest{
		Query:     query,
		Variables: variables,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("linear api error: status %d", resp.StatusCode)
	}

	var gqlResp graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, err
	}

	if len(gqlResp.Errors) > 0 {
		msg := gqlResp.Errors[0].Message
		if strings.Contains(strings.ToLower(msg), "not found") {
			return nil, ErrIssueNotFound
		}
		if strings.Contains(strings.ToLower(msg), "authentication") {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("graphql error: %s", msg)
	}

	return gqlResp.Data, nil
}

// GetIssue queries an issue from Linear and maps it into a UnifiedIssue.
func (c *Client) GetIssue(ctx context.Context, id tracker.TicketID) (*tracker.UnifiedIssue, error) {
	query := `query GetIssue($id: String!) {
		issue(id: $id) {
			id
			identifier
			title
			description
			state {
				id
				name
				type
			}
			labels {
				nodes {
					name
				}
			}
			createdAt
			updatedAt
		}
	}`

	data, err := c.executeQuery(ctx, query, map[string]interface{}{"id": string(id)})
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Issue *struct {
			ID          string `json:"id"`
			Identifier  string `json:"identifier"`
			Title       string `json:"title"`
			Description string `json:"description"`
			State       struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"state"`
			Labels struct {
				Nodes []struct {
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"labels"`
			CreatedAt string `json:"createdAt"`
			UpdatedAt string `json:"updatedAt"`
		} `json:"issue"`
	}

	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Issue == nil {
		return nil, ErrIssueNotFound
	}

	iss := parsed.Issue
	state := mapLinearState(iss.State.Type, iss.State.Name)

	var labels []string
	for _, l := range iss.Labels.Nodes {
		labels = append(labels, l.Name)
	}

	specs := extractLinkedSpecs(iss.Title + " " + iss.Description)
	createdAt, _ := time.Parse(time.RFC3339, iss.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, iss.UpdatedAt)

	return &tracker.UnifiedIssue{
		ID:                  tracker.TicketID(iss.Identifier),
		Title:               iss.Title,
		DescriptionMarkdown: iss.Description,
		State:               state,
		LinkedSpecs:         specs,
		Labels:              labels,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}, nil
}

// UpdateStatus moves a Linear issue to the target state.
func (c *Client) UpdateStatus(ctx context.Context, id tracker.TicketID, state tracker.Status) error {
	statesQuery := `query GetWorkflowStates {
		workflowStates {
			nodes {
				id
				name
				type
			}
		}
	}`

	data, err := c.executeQuery(ctx, statesQuery, nil)
	if err != nil {
		return err
	}

	var parsed struct {
		WorkflowStates struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"nodes"`
		} `json:"workflowStates"`
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}

	targetStateID := findMatchingLinearState(parsed.WorkflowStates.Nodes, state)
	if targetStateID == "" {
		return ErrStateNotFound
	}

	mutation := `mutation UpdateIssueState($id: String!, $stateId: String!) {
		issueUpdate(id: $id, input: { stateId: $stateId }) {
			success
		}
	}`

	_, err = c.executeQuery(ctx, mutation, map[string]interface{}{
		"id":      string(id),
		"stateId": targetStateID,
	})
	return err
}

// AddComment posts a comment to the Linear issue.
func (c *Client) AddComment(ctx context.Context, id tracker.TicketID, comment string) error {
	mutation := `mutation AddComment($id: String!, $body: String!) {
		commentCreate(input: { issueId: $id, body: $body }) {
			success
		}
	}`

	_, err := c.executeQuery(ctx, mutation, map[string]interface{}{
		"id":   string(id),
		"body": comment,
	})
	return err
}

// LinkPullRequest attaches an external link or comment with PR URL to the Linear issue.
func (c *Client) LinkPullRequest(ctx context.Context, id tracker.TicketID, prURL string) error {
	mutation := `mutation LinkPR($id: String!, $url: String!, $title: String!) {
		attachmentCreate(input: { issueId: $id, url: $url, title: $title }) {
			success
		}
	}`

	_, err := c.executeQuery(ctx, mutation, map[string]interface{}{
		"id":    string(id),
		"url":   prURL,
		"title": "Pull Request",
	})
	return err
}

// ValidateCredentials validates the Linear API key within < 1.5s.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	query := `query Viewer {
		viewer {
			id
			email
		}
	}`

	_, err := c.executeQuery(timeoutCtx, query, nil)
	if err != nil {
		if timeoutCtx.Err() != nil {
			return ErrValidationTimeout
		}
		return err
	}
	return nil
}

func mapLinearState(stateType, name string) tracker.Status {
	t := strings.ToLower(stateType)
	n := strings.ToLower(name)
	switch {
	case t == "completed" || strings.Contains(n, "done"):
		return tracker.StatusDone
	case strings.Contains(n, "review"):
		return tracker.StatusInReview
	case t == "started" || strings.Contains(n, "progress") || strings.Contains(n, "development"):
		return tracker.StatusInProgress
	case t == "unstarted" || strings.Contains(n, "todo") || strings.Contains(n, "to do"):
		return tracker.StatusTodo
	default:
		return tracker.StatusBacklog
	}
}

func findMatchingLinearState(states []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}, target tracker.Status) string {
	for _, s := range states {
		mapped := mapLinearState(s.Type, s.Name)
		if mapped == target {
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
