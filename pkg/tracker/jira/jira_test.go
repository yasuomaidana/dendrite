package jira

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

func TestADF_ParsingAndBuilding(t *testing.T) {
	// Empty
	if res := ParseADFToMarkdown(nil); res != "" {
		t.Errorf("expected empty string for nil ADF, got: %s", res)
	}

	// Plain string
	plainJSON := []byte(`"Simple issue description"`)
	if res := ParseADFToMarkdown(plainJSON); res != "Simple issue description" {
		t.Errorf("expected unquoted string, got: %s", res)
	}

	// Complex ADF
	adfJSON := `{
		"type": "doc",
		"version": 1,
		"content": [
			{
				"type": "heading",
				"attrs": {"level": 2},
				"content": [{"type": "text", "text": "Architecture FEAT-008"}]
			},
			{
				"type": "paragraph",
				"content": [
					{"type": "text", "text": "This is "},
					{"type": "text", "text": "bold", "marks": [{"type": "strong"}]},
					{"type": "text", "text": " and "},
					{"type": "text", "text": "italic", "marks": [{"type": "em"}]},
					{"type": "text", "text": " and "},
					{"type": "text", "text": "code", "marks": [{"type": "code"}]},
					{"type": "text", "text": " and "},
					{"type": "text", "text": "link", "marks": [{"type": "link", "attrs": {"href": "https://example.com"}}]}
				]
			},
			{
				"type": "bulletList",
				"content": [
					{
						"type": "listItem",
						"content": [{"type": "paragraph", "content": [{"type": "text", "text": "Bullet item"}]}]
					}
				]
			},
			{
				"type": "orderedList",
				"content": [
					{
						"type": "listItem",
						"content": [{"type": "paragraph", "content": [{"type": "text", "text": "Ordered item"}]}]
					}
				]
			},
			{
				"type": "codeBlock",
				"attrs": {"language": "go"},
				"content": [{"type": "text", "text": "package main"}]
			},
			{
				"type": "blockquote",
				"content": [{"type": "text", "text": "A quote"}]
			}
		]
	}`

	parsed := ParseADFToMarkdown([]byte(adfJSON))
	if !strings.Contains(parsed, "## Architecture FEAT-008") {
		t.Errorf("missing heading in markdown: %s", parsed)
	}
	if !strings.Contains(parsed, "**bold**") || !strings.Contains(parsed, "*italic*") || !strings.Contains(parsed, "`code`") {
		t.Errorf("missing text formatting in markdown: %s", parsed)
	}
	if !strings.Contains(parsed, "[link](https://example.com)") {
		t.Errorf("missing link in markdown: %s", parsed)
	}
	if !strings.Contains(parsed, "- Bullet item") {
		t.Errorf("missing bullet list in markdown: %s", parsed)
	}
	if !strings.Contains(parsed, "1. Ordered item") {
		t.Errorf("missing ordered list in markdown: %s", parsed)
	}
	if !strings.Contains(parsed, "```go\npackage main\n```") {
		t.Errorf("missing code block in markdown: %s", parsed)
	}
	if !strings.Contains(parsed, "> A quote") {
		t.Errorf("missing blockquote in markdown: %s", parsed)
	}

	// Build ADF from markdown
	built := BuildADFFromMarkdown("Paragraph 1\n\nParagraph 2")
	if built["type"] != "doc" {
		t.Errorf("expected doc type, got: %v", built["type"])
	}
	contentList, ok := built["content"].([]map[string]interface{})
	if !ok || len(contentList) != 2 {
		t.Errorf("expected 2 paragraphs in built ADF, got: %d", len(contentList))
	}
}

func TestNewClient_Validation(t *testing.T) {
	_, err := NewClient(Config{}, nil)
	if err != ErrEmptyBaseURL {
		t.Errorf("expected ErrEmptyBaseURL, got: %v", err)
	}

	_, err = NewClient(Config{BaseURL: "https://jira.com"}, nil)
	if err != ErrEmptyEmail {
		t.Errorf("expected ErrEmptyEmail, got: %v", err)
	}

	_, err = NewClient(Config{BaseURL: "https://jira.com", Email: "dev@dendrite.ai"}, nil)
	if err != ErrMissingAPIToken {
		t.Errorf("expected ErrMissingAPIToken, got: %v", err)
	}

	_, err = NewClient(Config{BaseURL: "https://jira.com", Email: "dev@dendrite.ai", APITokenEnv: "UNSET_VAR_XYZ"}, nil)
	if err != ErrMissingAPIToken {
		t.Errorf("expected ErrMissingAPIToken for unset env, got: %v", err)
	}

	os.Setenv("TEST_JIRA_TOKEN", "secret-token")
	defer os.Unsetenv("TEST_JIRA_TOKEN")

	client, err := NewClient(Config{
		BaseURL:     "https://jira.com/",
		Email:       "dev@dendrite.ai",
		APITokenEnv: "TEST_JIRA_TOKEN",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	if client.baseURL != "https://jira.com" {
		t.Errorf("expected trailing slash to be removed, got: %s", client.baseURL)
	}
}

func TestClient_Operations(t *testing.T) {
	os.Setenv("MOCK_JIRA_TOKEN", "token-12345")
	defer os.Unsetenv("MOCK_JIRA_TOKEN")

	var lastTransitionReq map[string]interface{}
	var lastCommentReq map[string]interface{}
	var lastRemoteLinkReq map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Basic ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/myself":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accountId": "acc-123", "displayName": "Developer"}`))

		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/DEN-100":
			w.WriteHeader(http.StatusOK)
			data := `{
				"key": "DEN-100",
				"fields": {
					"summary": "Implement Feature FEAT-008 & ADR-001",
					"description": "Plain description text referencing FEAT-008",
					"status": {"name": "In Progress"},
					"labels": ["backend", "go"],
					"created": "2026-10-14T00:00:00Z",
					"updated": "2026-10-14T01:00:00Z"
				}
			}`
			_, _ = w.Write([]byte(data))

		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/DEN-404":
			w.WriteHeader(http.StatusNotFound)

		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issue/DEN-100/transitions":
			w.WriteHeader(http.StatusOK)
			trans := `{
				"transitions": [
					{"id": "11", "name": "To Do"},
					{"id": "21", "name": "In Progress"},
					{"id": "31", "name": "In Review"},
					{"id": "41", "name": "Done"}
				]
			}`
			_, _ = w.Write([]byte(trans))

		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue/DEN-100/transitions":
			_ = json.NewDecoder(r.Body).Decode(&lastTransitionReq)
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue/DEN-100/comment":
			_ = json.NewDecoder(r.Body).Decode(&lastCommentReq)
			w.WriteHeader(http.StatusCreated)

		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue/DEN-100/remotelink":
			_ = json.NewDecoder(r.Body).Decode(&lastRemoteLinkReq)
			w.WriteHeader(http.StatusCreated)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:     server.URL,
		Email:       "user@company.com",
		APITokenEnv: "MOCK_JIRA_TOKEN",
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
	issue, err := client.GetIssue(ctx, "DEN-100")
	if err != nil {
		t.Fatalf("GetIssue failed: %v", err)
	}
	if issue.ID != "DEN-100" || issue.State != tracker.StatusInProgress {
		t.Errorf("unexpected issue data: %+v", issue)
	}
	if len(issue.LinkedSpecs) != 2 {
		t.Errorf("expected 2 linked specs (FEAT-008, ADR-001), got: %v", issue.LinkedSpecs)
	}

	// 3. Get non-existent issue
	_, err = client.GetIssue(ctx, "DEN-404")
	if err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound, got: %v", err)
	}

	// 4. Update status
	if err := client.UpdateStatus(ctx, "DEN-100", tracker.StatusDone); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if lastTransitionReq == nil {
		t.Fatal("expected transition POST request")
	}
	transObj := lastTransitionReq["transition"].(map[string]interface{})
	if transObj["id"] != "41" {
		t.Errorf("expected transition ID 41 for Done, got: %v", transObj["id"])
	}

	// 5. Add comment
	if err := client.AddComment(ctx, "DEN-100", "Autonomous commit applied"); err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}
	if lastCommentReq == nil {
		t.Fatal("expected comment request")
	}

	// 6. Link pull request
	if err := client.LinkPullRequest(ctx, "DEN-100", "https://github.com/org/repo/pull/42"); err != nil {
		t.Fatalf("LinkPullRequest failed: %v", err)
	}
	if lastRemoteLinkReq == nil {
		t.Fatal("expected remote link request")
	}
}

func TestClient_ErrorBranches(t *testing.T) {
	os.Setenv("ERR_TOKEN", "err-token")
	defer os.Unsetenv("ERR_TOKEN")

	// Server returning 401
	unauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthServer.Close()

	client, err := NewClient(Config{
		BaseURL:     unauthServer.URL,
		Email:       "user@company.com",
		APITokenEnv: "ERR_TOKEN",
	}, unauthServer.Client())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	if err := client.ValidateCredentials(ctx); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
	if _, err := client.GetIssue(ctx, "DEN-1"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on GetIssue, got: %v", err)
	}
	if err := client.UpdateStatus(ctx, "DEN-1", tracker.StatusDone); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on UpdateStatus, got: %v", err)
	}
	if err := client.AddComment(ctx, "DEN-1", "comment"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on AddComment, got: %v", err)
	}
	if err := client.LinkPullRequest(ctx, "DEN-1", "http://pr"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on LinkPullRequest, got: %v", err)
	}

	// Server returning 500 error
	err500Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer err500Server.Close()

	err500Client, _ := NewClient(Config{
		BaseURL:     err500Server.URL,
		Email:       "user@company.com",
		APITokenEnv: "ERR_TOKEN",
	}, err500Server.Client())

	if err := err500Client.ValidateCredentials(ctx); err == nil {
		t.Errorf("expected error on 500 status in ValidateCredentials")
	}
	if _, err := err500Client.GetIssue(ctx, "DEN-1"); err == nil {
		t.Errorf("expected error on 500 status in GetIssue")
	}
	if err := err500Client.UpdateStatus(ctx, "DEN-1", tracker.StatusDone); err == nil {
		t.Errorf("expected error on 500 status in UpdateStatus")
	}
	if err := err500Client.AddComment(ctx, "DEN-1", "test"); err == nil {
		t.Errorf("expected error on 500 status in AddComment")
	}
	if err := err500Client.LinkPullRequest(ctx, "DEN-1", "http://pr"); err == nil {
		t.Errorf("expected error on 500 status in LinkPullRequest")
	}

	// Server returning 404 for transitions, comments, and links
	err404Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer err404Server.Close()

	err404Client, _ := NewClient(Config{
		BaseURL:     err404Server.URL,
		Email:       "user@company.com",
		APITokenEnv: "ERR_TOKEN",
	}, err404Server.Client())

	if err := err404Client.UpdateStatus(ctx, "DEN-1", tracker.StatusDone); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on 404 transitions, got: %v", err)
	}
	if err := err404Client.AddComment(ctx, "DEN-1", "test"); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on 404 comment, got: %v", err)
	}
	if err := err404Client.LinkPullRequest(ctx, "DEN-1", "http://pr"); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on 404 link, got: %v", err)
	}

	// Server returning no matching transition
	noTransServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/transitions") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"transitions": []}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer noTransServer.Close()

	noTransClient, _ := NewClient(Config{
		BaseURL:     noTransServer.URL,
		Email:       "user@company.com",
		APITokenEnv: "ERR_TOKEN",
	}, noTransServer.Client())

	if err := noTransClient.UpdateStatus(ctx, "DEN-1", tracker.StatusDone); err != ErrTransitionNotFound {
		t.Errorf("expected ErrTransitionNotFound, got: %v", err)
	}
}

func TestMapJiraStatus(t *testing.T) {
	cases := []struct {
		in  string
		exp tracker.Status
	}{
		{"Done", tracker.StatusDone},
		{"Closed", tracker.StatusDone},
		{"Resolved", tracker.StatusDone},
		{"Code Review", tracker.StatusInReview},
		{"In Progress", tracker.StatusInProgress},
		{"Development", tracker.StatusInProgress},
		{"To Do", tracker.StatusTodo},
		{"Open", tracker.StatusTodo},
		{"Random Backlog State", tracker.StatusBacklog},
	}

	for _, c := range cases {
		out := mapJiraStatus(c.in)
		if out != c.exp {
			t.Errorf("mapJiraStatus(%q) = %v; want %v", c.in, out, c.exp)
		}
	}
}
