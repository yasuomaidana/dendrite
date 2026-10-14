package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

func TestNewClient_Validation(t *testing.T) {
	_, err := NewClient(Config{}, nil)
	if err != ErrEmptyBaseURL {
		t.Errorf("expected ErrEmptyBaseURL, got: %v", err)
	}

	_, err = NewClient(Config{BaseURL: "https://plane.so"}, nil)
	if err != ErrEmptyWorkspace {
		t.Errorf("expected ErrEmptyWorkspace, got: %v", err)
	}

	_, err = NewClient(Config{BaseURL: "https://plane.so", WorkspaceSlug: "my-org"}, nil)
	if err != ErrEmptyProjectID {
		t.Errorf("expected ErrEmptyProjectID, got: %v", err)
	}

	_, err = NewClient(Config{BaseURL: "https://plane.so", WorkspaceSlug: "my-org", ProjectID: "proj-1"}, nil)
	if err != ErrMissingAPIToken {
		t.Errorf("expected ErrMissingAPIToken, got: %v", err)
	}

	_, err = NewClient(Config{BaseURL: "https://plane.so", WorkspaceSlug: "my-org", ProjectID: "proj-1", APITokenEnv: "UNSET_PLANE_KEY"}, nil)
	if err != ErrMissingAPIToken {
		t.Errorf("expected ErrMissingAPIToken for unset env, got: %v", err)
	}

	os.Setenv("TEST_PLANE_KEY", "plane-api-key-xyz")
	defer os.Unsetenv("TEST_PLANE_KEY")

	client, err := NewClient(Config{
		BaseURL:       "https://plane.so/",
		WorkspaceSlug: "my-org",
		ProjectID:     "proj-1",
		APITokenEnv:   "TEST_PLANE_KEY",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	if client.baseURL != "https://plane.so" {
		t.Errorf("expected trailing slash to be stripped, got: %s", client.baseURL)
	}
}

func TestClient_Operations(t *testing.T) {
	os.Setenv("MOCK_PLANE_KEY", "key-12345")
	defer os.Unsetenv("MOCK_PLANE_KEY")

	var lastPatchReq map[string]string
	var lastCommentReq map[string]string
	var lastLinkReq map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("x-api-key")
		if apiKey != "key-12345" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/me/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "usr-1", "email": "dev@dendrite.ai"}`))

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/issues/ISSUE-1/":
			w.WriteHeader(http.StatusOK)
			data := `{
				"id": "ISSUE-1",
				"name": "Implement Plane Adapter FEAT-008",
				"description_stripped": "Specification references FEAT-008 and ADR-002",
				"state_detail": {"name": "In Progress", "group": "started"},
				"label_details": [{"name": "backend"}],
				"created_at": "2026-10-14T00:00:00Z",
				"updated_at": "2026-10-14T01:00:00Z"
			}`
			_, _ = w.Write([]byte(data))

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/issues/ISSUE-404/":
			w.WriteHeader(http.StatusNotFound)

		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/states/":
			w.WriteHeader(http.StatusOK)
			data := `[
				{"id": "st-todo", "name": "To Do", "group": "unstarted"},
				{"id": "st-prog", "name": "In Progress", "group": "started"},
				{"id": "st-rev", "name": "Review", "group": "started"},
				{"id": "st-done", "name": "Done", "group": "completed"}
			]`
			_, _ = w.Write([]byte(data))

		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/issues/ISSUE-1/":
			_ = json.NewDecoder(r.Body).Decode(&lastPatchReq)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "ISSUE-1"}`))

		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/issues/ISSUE-1/comments/":
			_ = json.NewDecoder(r.Body).Decode(&lastCommentReq)
			w.WriteHeader(http.StatusCreated)

		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/issues/ISSUE-1/links/":
			_ = json.NewDecoder(r.Body).Decode(&lastLinkReq)
			w.WriteHeader(http.StatusCreated)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws1",
		ProjectID:     "p1",
		APITokenEnv:   "MOCK_PLANE_KEY",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Validate credentials
	if err := client.ValidateCredentials(ctx); err != nil {
		t.Fatalf("ValidateCredentials failed: %v", err)
	}

	// 2. Get issue
	issue, err := client.GetIssue(ctx, "ISSUE-1")
	if err != nil {
		t.Fatalf("GetIssue failed: %v", err)
	}
	if issue.ID != "ISSUE-1" || issue.State != tracker.StatusInProgress {
		t.Errorf("unexpected issue details: %+v", issue)
	}
	if len(issue.LinkedSpecs) != 2 {
		t.Errorf("expected 2 linked specs (FEAT-008, ADR-002), got: %v", issue.LinkedSpecs)
	}

	// 3. Get non-existent
	_, err = client.GetIssue(ctx, "ISSUE-404")
	if err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound, got: %v", err)
	}

	// 4. Update status
	if err := client.UpdateStatus(ctx, "ISSUE-1", tracker.StatusDone); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if lastPatchReq["state"] != "st-done" {
		t.Errorf("expected target state st-done, got: %s", lastPatchReq["state"])
	}

	// 5. Add comment
	if err := client.AddComment(ctx, "ISSUE-1", "Test comment"); err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}
	if !strings.Contains(lastCommentReq["comment_html"], "Test comment") {
		t.Errorf("expected comment body in HTML, got: %v", lastCommentReq)
	}

	// 6. Link PR
	if err := client.LinkPullRequest(ctx, "ISSUE-1", "https://github.com/org/repo/pull/1"); err != nil {
		t.Fatalf("LinkPullRequest failed: %v", err)
	}
	if lastLinkReq["url"] != "https://github.com/org/repo/pull/1" {
		t.Errorf("expected pr URL in link req, got: %v", lastLinkReq)
	}
}

func TestClient_ErrorBranches(t *testing.T) {
	os.Setenv("ERR_PLANE_KEY", "bad-key")
	defer os.Unsetenv("ERR_PLANE_KEY")

	// 401 Unauthorized server
	unauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthServer.Close()

	client, _ := NewClient(Config{
		BaseURL:       unauthServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		APITokenEnv:   "ERR_PLANE_KEY",
	}, unauthServer.Client())

	ctx := context.Background()
	if err := client.ValidateCredentials(ctx); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
	if _, err := client.GetIssue(ctx, "ISSUE-1"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on GetIssue, got: %v", err)
	}
	if err := client.UpdateStatus(ctx, "ISSUE-1", tracker.StatusDone); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on UpdateStatus, got: %v", err)
	}
	if err := client.AddComment(ctx, "ISSUE-1", "msg"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on AddComment, got: %v", err)
	}
	if err := client.LinkPullRequest(ctx, "ISSUE-1", "url"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on LinkPullRequest, got: %v", err)
	}

	// 500 Internal error server
	err500Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer err500Server.Close()

	err500Client, _ := NewClient(Config{
		BaseURL:       err500Server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		APITokenEnv:   "ERR_PLANE_KEY",
	}, err500Server.Client())

	if err := err500Client.ValidateCredentials(ctx); err == nil {
		t.Errorf("expected error on 500 status in ValidateCredentials")
	}
	if _, err := err500Client.GetIssue(ctx, "ISSUE-1"); err == nil {
		t.Errorf("expected error on 500 status in GetIssue")
	}
	if err := err500Client.UpdateStatus(ctx, "ISSUE-1", tracker.StatusDone); err == nil {
		t.Errorf("expected error on 500 status in UpdateStatus")
	}
	if err := err500Client.AddComment(ctx, "ISSUE-1", "msg"); err == nil {
		t.Errorf("expected error on 500 status in AddComment")
	}
	if err := err500Client.LinkPullRequest(ctx, "ISSUE-1", "url"); err == nil {
		t.Errorf("expected error on 500 status in LinkPullRequest")
	}

	// 404 server for comments and links
	err404Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/states/") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id": "st-done", "name": "Done", "group": "completed"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer err404Server.Close()

	err404Client, _ := NewClient(Config{
		BaseURL:       err404Server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		APITokenEnv:   "ERR_PLANE_KEY",
	}, err404Server.Client())

	if err := err404Client.UpdateStatus(ctx, "ISSUE-1", tracker.StatusDone); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on 404 patch, got: %v", err)
	}
	if err := err404Client.AddComment(ctx, "ISSUE-1", "msg"); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on 404 comment, got: %v", err)
	}
	if err := err404Client.LinkPullRequest(ctx, "ISSUE-1", "url"); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on 404 link, got: %v", err)
	}

	// Missing state server
	missingStateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer missingStateServer.Close()

	missingStateClient, _ := NewClient(Config{
		BaseURL:       missingStateServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		APITokenEnv:   "ERR_PLANE_KEY",
	}, missingStateServer.Client())

	if err := missingStateClient.UpdateStatus(ctx, "ISSUE-1", tracker.StatusDone); err != ErrStateNotFound {
		t.Errorf("expected ErrStateNotFound, got: %v", err)
	}
}

func TestMapPlaneState(t *testing.T) {
	cases := []struct {
		group string
		name  string
		exp   tracker.Status
	}{
		{"completed", "Closed", tracker.StatusDone},
		{"started", "In Code Review", tracker.StatusInReview},
		{"started", "Active Coding", tracker.StatusInProgress},
		{"unstarted", "To Do List", tracker.StatusTodo},
		{"backlog", "Icebox", tracker.StatusBacklog},
	}

	for _, c := range cases {
		out := mapPlaneState(c.group, c.name)
		if out != c.exp {
			t.Errorf("mapPlaneState(%q, %q) = %v; want %v", c.group, c.name, out, c.exp)
		}
	}
}
