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

	"Dendrite/pkg/quality"
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

// APIKeyEnv names the environment variable that holds the Plane API token. Dendrite loads it from
// $DENDRITE_HOME/.env (or the project's .env); it is never defaulted in code.
const APIKeyEnv = "PLANE_API_KEY"

// ConfigFromEnv builds the Plane client configuration from the environment and the repository's
// .dendrite/plane.json. The API token has no default: an unset PLANE_API_KEY fails with
// ErrMissingAPIToken and says where to put it.
func ConfigFromEnv() (Config, error) {
	token := strings.TrimSpace(os.Getenv(APIKeyEnv))
	if token == "" {
		return Config{}, fmt.Errorf("%w: set %s in $DENDRITE_HOME/.env (create one in the Plane UI under Profile > API tokens) or export it", ErrMissingAPIToken, APIKeyEnv)
	}

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

	// Each editable repository names its own Plane project in .dendrite/plane.json;
	// PLANE_PROJECT_ID overrides it and Dendrite Core is the fallback.
	projectID := os.Getenv("PLANE_PROJECT_ID")
	if projectID == "" {
		if cwd, err := os.Getwd(); err == nil {
			if link, _, err := LinkForDir(cwd); err == nil {
				projectID = link.ProjectID
				if os.Getenv("PLANE_WORKSPACE_SLUG") == "" && link.Workspace != "" {
					workspaceSlug = link.Workspace
				}
			}
		}
	}
	if projectID == "" {
		projectID = "e6380603-1428-4a66-995e-e06039915d52"
	}

	return Config{BaseURL: baseURL, WorkspaceSlug: workspaceSlug, ProjectID: projectID, DirectToken: token}, nil
}

// NewClientFromEnv constructs a Plane client from ConfigFromEnv.
func NewClientFromEnv() (*Client, error) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return NewClient(cfg, &http.Client{Timeout: 15 * time.Second})
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

// NewRequest builds an authenticated request for an API path ("/api/v1/...") so callers outside this
// package can make custom calls without ever reading the token themselves.
func (c *Client) NewRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Request, error) {
	return c.newRequest(ctx, method, endpoint, body)
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

// ListPlaneIssues queries all issues for the project across all pages.
func (c *Client) ListPlaneIssues(ctx context.Context) ([]PlaneIssue, error) {
	var allIssues []PlaneIssue
	cursor := ""

	for {
		endpoint := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/issues/", c.workspaceSlug, c.projectID)
		endpoint += "?per_page=100"
		if cursor != "" {
			endpoint = fmt.Sprintf("%s&cursor=%s", endpoint, cursor)
		}
		req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			return nil, ErrUnauthorized
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to list plane issues: status %d", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		var paginated struct {
			NextPageResults bool         `json:"next_page_results"`
			NextCursor      string       `json:"next_cursor"`
			Results         []PlaneIssue `json:"results"`
		}
		if err := json.Unmarshal(bodyBytes, &paginated); err == nil && len(paginated.Results) > 0 {
			allIssues = append(allIssues, paginated.Results...)
			if !paginated.NextPageResults || paginated.NextCursor == "" {
				break
			}
			cursor = paginated.NextCursor
			continue
		}

		var directList []PlaneIssue
		if err := json.Unmarshal(bodyBytes, &directList); err == nil {
			allIssues = append(allIssues, directList...)
		}
		break
	}

	return dedupeIssues(allIssues), nil
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

// CheckZeroMockGate enforces the Zero-Mock policy: a ticket cannot move to review or done while
// mocks or stubs exist in the packages it names. Every path that sets a status runs it first.
func (c *Client) CheckZeroMockGate(ctx context.Context, id tracker.TicketID, state tracker.Status) error {
	if state != tracker.StatusDone && state != tracker.StatusInReview {
		return nil
	}
	issueList, _ := c.ListPlaneIssues(ctx)
	if matchedIssue := ResolveIssue(issueList, string(id)); matchedIssue != nil {
		pkgs := quality.ExtractPackages(matchedIssue.Name + " " + matchedIssue.DescriptionHTML)
		if len(pkgs) > 0 {
			if hasMocks, mockLocs, _ := quality.ValidateZeroMockGate(".", pkgs); hasMocks {
				return fmt.Errorf("zero-mock policy gate violation: cannot transition ticket %s to %s because mocks/stubs were detected in %v: %s", id, state, pkgs, strings.Join(mockLocs, "; "))
			}
		}
	}
	return nil
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

	if err := c.CheckZeroMockGate(ctx, id, state); err != nil {
		return err
	}

	issueUUID := string(id)
	if len(issueUUID) != 36 || strings.Count(issueUUID, "-") != 4 {
		// When the list cannot be fetched the reference is sent as-is and Plane answers for it.
		if issues, err := c.ListPlaneIssues(ctx); err == nil {
			matched := ResolveIssue(issues, string(id))
			if matched == nil {
				return fmt.Errorf("%w: no ticket matches %q (use DEN-n, its Plane number, or the issue UUID)", ErrIssueNotFound, id)
			}
			issueUUID = matched.ID
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

// CreatePlaneIssue creates a new issue in Plane with rich metadata (backward compatible).
func (c *Client) CreatePlaneIssue(ctx context.Context, name, descHTML, stateID string, estimate *int) (*PlaneIssue, error) {
	return c.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{
		Name:            name,
		DescriptionHTML: descHTML,
		StateID:         stateID,
		EstimatePoint:   estimate,
	})
}
