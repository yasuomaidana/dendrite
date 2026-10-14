package linear

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
	if err != ErrMissingAPIToken {
		t.Errorf("expected ErrMissingAPIToken, got: %v", err)
	}

	_, err = NewClient(Config{APITokenEnv: "UNSET_LINEAR_KEY"}, nil)
	if err != ErrMissingAPIToken {
		t.Errorf("expected ErrMissingAPIToken for unset env, got: %v", err)
	}

	os.Setenv("TEST_LINEAR_KEY", "lin_api_12345")
	defer os.Unsetenv("TEST_LINEAR_KEY")

	client, err := NewClient(Config{
		APITokenEnv: "TEST_LINEAR_KEY",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	if client.endpoint != DefaultEndpoint {
		t.Errorf("expected default endpoint, got: %s", client.endpoint)
	}
}

func TestClient_Operations(t *testing.T) {
	os.Setenv("MOCK_LINEAR_KEY", "lin_secret")
	defer os.Unsetenv("MOCK_LINEAR_KEY")

	var lastQuery string
	var lastVars map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "lin_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req graphQLRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		lastQuery = req.Query
		lastVars = req.Variables

		switch {
		case strings.Contains(req.Query, "query Viewer"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data": {"viewer": {"id": "usr-1", "email": "dev@dendrite.ai"}}}`))

		case strings.Contains(req.Query, "query GetIssue"):
			id, _ := req.Variables["id"].(string)
			if id == "LIN-404" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"errors": [{"message": "Entity not found"}]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			data := `{
				"data": {
					"issue": {
						"id": "uuid-1",
						"identifier": "LIN-100",
						"title": "Build Linear Strategy FEAT-008",
						"description": "Reflects ADR-005 and FEAT-008 specs",
						"state": {"id": "st-1", "name": "In Progress", "type": "started"},
						"labels": {"nodes": [{"name": "api"}]},
						"createdAt": "2026-10-14T00:00:00Z",
						"updatedAt": "2026-10-14T01:00:00Z"
					}
				}
			}`
			_, _ = w.Write([]byte(data))

		case strings.Contains(req.Query, "query GetWorkflowStates"):
			w.WriteHeader(http.StatusOK)
			data := `{
				"data": {
					"workflowStates": {
						"nodes": [
							{"id": "st-todo", "name": "Todo", "type": "unstarted"},
							{"id": "st-prog", "name": "In Progress", "type": "started"},
							{"id": "st-rev", "name": "Review", "type": "started"},
							{"id": "st-done", "name": "Done", "type": "completed"}
						]
					}
				}
			}`
			_, _ = w.Write([]byte(data))

		case strings.Contains(req.Query, "mutation UpdateIssueState"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data": {"issueUpdate": {"success": true}}}`))

		case strings.Contains(req.Query, "mutation AddComment"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data": {"commentCreate": {"success": true}}}`))

		case strings.Contains(req.Query, "mutation LinkPR"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data": {"attachmentCreate": {"success": true}}}`))

		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint:    server.URL,
		APITokenEnv: "MOCK_LINEAR_KEY",
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
	issue, err := client.GetIssue(ctx, "LIN-100")
	if err != nil {
		t.Fatalf("GetIssue failed: %v", err)
	}
	if issue.ID != "LIN-100" || issue.State != tracker.StatusInProgress {
		t.Errorf("unexpected issue data: %+v", issue)
	}
	if len(issue.LinkedSpecs) != 2 {
		t.Errorf("expected 2 linked specs (FEAT-008, ADR-005), got: %v", issue.LinkedSpecs)
	}

	// 3. Get non-existent issue
	_, err = client.GetIssue(ctx, "LIN-404")
	if err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound, got: %v", err)
	}

	// 4. Update status
	if err := client.UpdateStatus(ctx, "LIN-100", tracker.StatusDone); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if lastVars["stateId"] != "st-done" {
		t.Errorf("expected stateId st-done, got: %v", lastVars["stateId"])
	}

	// 5. Add comment
	if err := client.AddComment(ctx, "LIN-100", "Autonomous commit generated"); err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}
	if lastVars["body"] != "Autonomous commit generated" {
		t.Errorf("expected comment body, got: %v", lastVars["body"])
	}

	// 6. Link PR
	if err := client.LinkPullRequest(ctx, "LIN-100", "https://github.com/org/repo/pull/50"); err != nil {
		t.Fatalf("LinkPullRequest failed: %v", err)
	}
	if lastVars["url"] != "https://github.com/org/repo/pull/50" {
		t.Errorf("expected pr url, got: %v", lastVars["url"])
	}
	_ = lastQuery
}

func TestClient_ErrorBranches(t *testing.T) {
	os.Setenv("ERR_LINEAR_KEY", "bad-key")
	defer os.Unsetenv("ERR_LINEAR_KEY")

	// 401 Unauthorized server
	unauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthServer.Close()

	client, _ := NewClient(Config{
		Endpoint:    unauthServer.URL,
		APITokenEnv: "ERR_LINEAR_KEY",
	}, unauthServer.Client())

	ctx := context.Background()
	if err := client.ValidateCredentials(ctx); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
	if _, err := client.GetIssue(ctx, "LIN-1"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on GetIssue, got: %v", err)
	}
	if err := client.UpdateStatus(ctx, "LIN-1", tracker.StatusDone); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on UpdateStatus, got: %v", err)
	}
	if err := client.AddComment(ctx, "LIN-1", "test"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on AddComment, got: %v", err)
	}
	if err := client.LinkPullRequest(ctx, "LIN-1", "http://pr"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on LinkPullRequest, got: %v", err)
	}

	// 500 error server
	err500Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer err500Server.Close()

	err500Client, _ := NewClient(Config{
		Endpoint:    err500Server.URL,
		APITokenEnv: "ERR_LINEAR_KEY",
	}, err500Server.Client())

	if err := err500Client.ValidateCredentials(ctx); err == nil {
		t.Errorf("expected error on 500 status")
	}

	// Missing state server
	noStateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": {"workflowStates": {"nodes": []}}}`))
	}))
	defer noStateServer.Close()

	noStateClient, _ := NewClient(Config{
		Endpoint:    noStateServer.URL,
		APITokenEnv: "ERR_LINEAR_KEY",
	}, noStateServer.Client())

	if err := noStateClient.UpdateStatus(ctx, "LIN-1", tracker.StatusDone); err != ErrStateNotFound {
		t.Errorf("expected ErrStateNotFound, got: %v", err)
	}
}

func TestMapLinearState(t *testing.T) {
	cases := []struct {
		stateType string
		name      string
		exp       tracker.Status
	}{
		{"completed", "Closed", tracker.StatusDone},
		{"started", "Code Review", tracker.StatusInReview},
		{"started", "In Progress", tracker.StatusInProgress},
		{"unstarted", "To Do", tracker.StatusTodo},
		{"backlog", "Triage", tracker.StatusBacklog},
	}

	for _, c := range cases {
		out := mapLinearState(c.stateType, c.name)
		if out != c.exp {
			t.Errorf("mapLinearState(%q, %q) = %v; want %v", c.stateType, c.name, out, c.exp)
		}
	}
}
