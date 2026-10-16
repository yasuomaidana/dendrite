package plane

import (
	"context"
	"errors"
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
		{"started", "To Verify", tracker.StatusInReview},
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

func TestClient_ExtendedOperations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id": "iss-1", "name": "Direct Issue", "sequence_id": 1}]`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "iss-new", "name": "Created Issue", "sequence_id": 2}`))

		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/issues/iss-1/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "iss-1", "name": "Patched Issue"}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/states/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id": "st-1", "name": "Done", "group": "completed"}]`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/modules/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id": "mod-1", "name": "Module 1"}]`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/modules/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "mod-2", "name": "Created Module"}`))

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/modules/mod-1/issues/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [{"id": "p-1", "name": "Page 1"}]}`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pages/p-1/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "p-1", "name": "Page 1", "description_html": "<p>Content</p>"}`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pages/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "p-2", "name": "New Page"}`))

		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/pages/p-1/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "p-1", "name": "Updated Page"}`))

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/pages/p-1/archive/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "archived"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		DirectToken:   "token-direct",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	if client.BaseURL() != server.URL {
		t.Errorf("expected base url %s, got %s", server.URL, client.BaseURL())
	}
	if client.WorkspaceSlug() != "ws" {
		t.Errorf("expected workspace ws, got %s", client.WorkspaceSlug())
	}
	if client.ProjectID() != "proj" {
		t.Errorf("expected project proj, got %s", client.ProjectID())
	}

	ctx := context.Background()

	// ListPlaneIssues direct
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil || len(issues) != 1 {
		t.Errorf("ListPlaneIssues failed: %v, count: %d", err, len(issues))
	}

	// CreatePlaneIssue
	est := 5
	createdIss, err := client.CreatePlaneIssue(ctx, "New Issue", "<p>desc</p>", "st-1", &est)
	if err != nil || createdIss.ID != "iss-new" {
		t.Errorf("CreatePlaneIssue failed: %v", err)
	}

	// UpdatePlaneIssue
	patchedIss, err := client.UpdatePlaneIssue(ctx, "iss-1", "Patched Issue", "<p>new</p>", "st-1", &est)
	if err != nil || patchedIss.Name != "Patched Issue" {
		t.Errorf("UpdatePlaneIssue failed: %v", err)
	}

	// ListStates direct
	states, err := client.ListStates(ctx)
	if err != nil || len(states) != 1 {
		t.Errorf("ListStates failed: %v", err)
	}

	// ListModules & CreateModule & AddIssuesToModule
	modules, err := client.ListModules(ctx)
	if err != nil || len(modules) != 1 {
		t.Errorf("ListModules failed: %v", err)
	}
	newMod, err := client.CreateModule(ctx, "Created Module", "planned", "desc")
	if err != nil || newMod.ID != "mod-2" {
		t.Errorf("CreateModule failed: %v", err)
	}
	err = client.AddIssuesToModule(ctx, "mod-1", []string{"iss-1"})
	if err != nil {
		t.Errorf("AddIssuesToModule failed: %v", err)
	}
	err = client.AddIssuesToModule(ctx, "mod-1", nil)
	if err != nil {
		t.Errorf("AddIssuesToModule with empty slice failed: %v", err)
	}

	// Pages operations
	pages, err := client.ListPages(ctx)
	if err != nil || len(pages) != 1 {
		t.Errorf("ListPages failed: %v, count: %d", err, len(pages))
	}
	pg, err := client.GetPage(ctx, "p-1")
	if err != nil || pg.ID != "p-1" {
		t.Errorf("GetPage failed: %v", err)
	}
	createdPg, err := client.CreatePage(ctx, "New Page", "<p>Content</p>")
	if err != nil || createdPg.ID != "p-2" {
		t.Errorf("CreatePage failed: %v", err)
	}
	updatedPg, err := client.UpdatePage(ctx, "p-1", "Updated Page", "<p>Updated</p>")
	if err != nil || updatedPg.Name != "Updated Page" {
		t.Errorf("UpdatePage failed: %v", err)
	}
	err = client.ArchivePage(ctx, "p-1")
	if err != nil {
		t.Errorf("ArchivePage failed: %v", err)
	}
}

func TestNewClientFromEnv(t *testing.T) {
	os.Setenv("PLANE_BASE_URL", "http://127.0.0.1:8090")
	os.Setenv("PLANE_WORKSPACE_SLUG", "custom-ws")
	os.Setenv("PLANE_PROJECT_ID", "custom-proj")
	os.Setenv("PLANE_API_KEY", "custom-key")
	defer func() {
		os.Unsetenv("PLANE_BASE_URL")
		os.Unsetenv("PLANE_WORKSPACE_SLUG")
		os.Unsetenv("PLANE_PROJECT_ID")
		os.Unsetenv("PLANE_API_KEY")
	}()

	client, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv failed: %v", err)
	}
	if client.BaseURL() != "http://127.0.0.1:8090" {
		t.Errorf("unexpected base url: %s", client.BaseURL())
	}
	if client.WorkspaceSlug() != "custom-ws" {
		t.Errorf("unexpected ws: %s", client.WorkspaceSlug())
	}
	if client.ProjectID() != "custom-proj" {
		t.Errorf("unexpected proj: %s", client.ProjectID())
	}
}

func TestClient_ExtendedErrors(t *testing.T) {
	// 401 unauthorized
	unauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthServer.Close()

	cUnauth, _ := NewClient(Config{
		BaseURL:       unauthServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "bad",
	}, unauthServer.Client())

	ctx := context.Background()
	if _, err := cUnauth.ListPlaneIssues(ctx); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on ListPlaneIssues")
	}
	if _, err := cUnauth.CreatePlaneIssue(ctx, "n", "d", "s", nil); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on CreatePlaneIssue")
	}
	if _, err := cUnauth.UpdatePlaneIssue(ctx, "id", "n", "d", "s", nil); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on UpdatePlaneIssue")
	}
	if _, err := cUnauth.ListStates(ctx); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on ListStates")
	}
	if _, err := cUnauth.ListModules(ctx); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on ListModules")
	}
	if _, err := cUnauth.CreateModule(ctx, "n", "st", "d"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on CreateModule")
	}
	if err := cUnauth.AddIssuesToModule(ctx, "m", []string{"i"}); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on AddIssuesToModule")
	}
	if _, err := cUnauth.ListPages(ctx); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on ListPages")
	}
	if _, err := cUnauth.GetPage(ctx, "p"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on GetPage")
	}
	if _, err := cUnauth.CreatePage(ctx, "n", "h"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on CreatePage")
	}
	if _, err := cUnauth.UpdatePage(ctx, "p", "n", "h"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on UpdatePage")
	}
	if err := cUnauth.ArchivePage(ctx, "p"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on ArchivePage")
	}

	// 404 not found
	err404Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer err404Server.Close()

	c404, _ := NewClient(Config{
		BaseURL:       err404Server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, err404Server.Client())

	if _, err := c404.UpdatePlaneIssue(ctx, "id", "n", "d", "s", nil); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on UpdatePlaneIssue")
	}
	if err := c404.AddIssuesToModule(ctx, "m", []string{"i"}); err != ErrModuleNotFound {
		t.Errorf("expected ErrModuleNotFound on AddIssuesToModule")
	}
	if _, err := c404.GetPage(ctx, "p"); err != ErrPageNotFound {
		t.Errorf("expected ErrPageNotFound on GetPage")
	}
	if _, err := c404.UpdatePage(ctx, "p", "n", "h"); err != ErrPageNotFound {
		t.Errorf("expected ErrPageNotFound on UpdatePage")
	}
	if err := c404.ArchivePage(ctx, "p"); err != ErrPageNotFound {
		t.Errorf("expected ErrPageNotFound on ArchivePage")
	}
}

func TestClient_GanttAndRelationOperations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/issues/iss-1/issue-relation/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[{"id": "rel-1", "relation_type": "blocked_by"}]`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/issues/iss-1/issue-relation/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"blocking": [], "blocked_by": [{"id": "iss-2", "relation_type": "blocked_by"}], "duplicate": [], "relates_to": []}`))

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/issues/iss-1/remove-relation/"):
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/modules/mod-1/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "mod-1", "name": "Patched Module", "start_date": "2026-10-15"}`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/modules/mod-1/module-issues/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [{"id": "iss-1", "name": "Module Issue 1"}]}`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/modules/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "mod-new", "name": "Gantt Module", "start_date": "2026-10-15", "target_date": "2026-10-25"}`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "iss-rich", "name": "Rich Issue", "priority": "high", "start_date": "2026-10-15"}`))

		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/issues/iss-1/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "iss-1", "name": "Patched Rich", "priority": "urgent"}`))

		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		DirectToken:   "sec-key",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. CreatePlaneIssueWithDetails
	sDate := "2026-10-15"
	tDate := "2026-10-20"
	est := 5
	prio := "high"
	iss, err := client.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{
		Name:            "Rich Issue",
		DescriptionHTML: "<p>Rich Issue Context</p>",
		Priority:      prio,
		StartDate:     &sDate,
		TargetDate:    &tDate,
		EstimatePoint: &est,
		ModuleID:      "mod-1",
		BlockedBy:     []string{"iss-0"},
	})
	if err != nil || iss.ID != "iss-rich" {
		t.Fatalf("CreatePlaneIssueWithDetails failed: %v", err)
	}

	// 2. UpdatePlaneIssueWithDetails
	prioUrgent := "urgent"
	updatedIss, err := client.UpdatePlaneIssueWithDetails(ctx, "iss-1", UpdateIssueRequest{
		Priority: prioUrgent,
	})
	if err != nil || updatedIss.Priority != "urgent" {
		t.Fatalf("UpdatePlaneIssueWithDetails failed: %v", err)
	}

	// 3. CreateModuleWithDates & UpdateModule
	mod, err := client.CreateModuleWithDates(ctx, "Gantt Module", "in-progress", "desc", &sDate, &tDate)
	if err != nil || mod.ID != "mod-new" {
		t.Fatalf("CreateModuleWithDates failed: %v", err)
	}
	upMod, err := client.UpdateModule(ctx, "mod-1", UpdateModuleRequest{
		Name:      "Patched Module",
		StartDate: &sDate,
	})
	if err != nil || upMod.ID != "mod-1" {
		t.Fatalf("UpdateModule failed: %v", err)
	}

	// 4. ListModuleIssues
	mIssues, err := client.ListModuleIssues(ctx, "mod-1")
	if err != nil || len(mIssues) != 1 {
		t.Fatalf("ListModuleIssues failed: %v", err)
	}

	// 5. AddIssueRelation, GetIssueRelations, RemoveIssueRelation
	if err := client.AddIssueRelation(ctx, "iss-1", "blocked_by", []string{"iss-2"}); err != nil {
		t.Fatalf("AddIssueRelation failed: %v", err)
	}
	if err := client.AddIssueRelation(ctx, "iss-1", "blocked_by", nil); err != nil {
		t.Fatalf("AddIssueRelation empty slice should be nil: %v", err)
	}
	rels, err := client.GetIssueRelations(ctx, "iss-1")
	if err != nil || len(rels.BlockedBy) != 1 {
		t.Fatalf("GetIssueRelations failed: %v", err)
	}
	if err := client.RemoveIssueRelation(ctx, "iss-1", "blocked_by", "iss-2"); err != nil {
		t.Fatalf("RemoveIssueRelation failed: %v", err)
	}
}

func TestClient_GanttAndRelationErrors(t *testing.T) {
	unauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthServer.Close()

	cUnauth, _ := NewClient(Config{
		BaseURL:       unauthServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		DirectToken:   "tok",
	}, unauthServer.Client())

	ctx := context.Background()

	if _, err := cUnauth.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{Name: "iss", DescriptionHTML: "<p>Context</p>"}); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on CreatePlaneIssueWithDetails")
	}
	if _, err := cUnauth.UpdatePlaneIssueWithDetails(ctx, "iss-1", UpdateIssueRequest{Name: "iss"}); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on UpdatePlaneIssueWithDetails")
	}
	if _, err := cUnauth.CreateModuleWithDates(ctx, "m", "p", "d", nil, nil); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on CreateModuleWithDates")
	}
	if _, err := cUnauth.UpdateModule(ctx, "mod-1", UpdateModuleRequest{Name: "m"}); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on UpdateModule")
	}
	if _, err := cUnauth.ListModuleIssues(ctx, "mod-1"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on ListModuleIssues")
	}
	if err := cUnauth.AddIssueRelation(ctx, "iss-1", "blocked_by", []string{"iss-2"}); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on AddIssueRelation")
	}
	if _, err := cUnauth.GetIssueRelations(ctx, "iss-1"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on GetIssueRelations")
	}
	if err := cUnauth.RemoveIssueRelation(ctx, "iss-1", "blocked_by", "iss-2"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on RemoveIssueRelation")
	}

	// 404 server
	notFoundServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer notFoundServer.Close()

	c404, _ := NewClient(Config{
		BaseURL:       notFoundServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		DirectToken:   "tok",
	}, notFoundServer.Client())

	if _, err := c404.UpdateModule(ctx, "mod-1", UpdateModuleRequest{Name: "m"}); err != ErrModuleNotFound {
		t.Errorf("expected ErrModuleNotFound on UpdateModule")
	}
	if _, err := c404.GetIssueRelations(ctx, "iss-1"); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on GetIssueRelations")
	}
	if _, err := c404.UpdatePlaneIssueWithDetails(ctx, "iss-1", UpdateIssueRequest{Name: "iss"}); err != ErrIssueNotFound {
		t.Errorf("expected ErrIssueNotFound on UpdatePlaneIssueWithDetails")
	}
}

func TestClient_CreatePlaneIssue_EmptyContext(t *testing.T) {
	client, err := NewClient(Config{
		BaseURL:       "http://localhost",
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		DirectToken:   "tok",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	// Test empty description string
	if _, err := client.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{Name: "iss", DescriptionHTML: ""}); !errors.Is(err, ErrEmptyTicketContext) {
		t.Errorf("expected ErrEmptyTicketContext for empty string, got: %v", err)
	}
	// Test empty HTML tags
	if _, err := client.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{Name: "iss", DescriptionHTML: "<div><p></p></div>"}); !errors.Is(err, ErrEmptyTicketContext) {
		t.Errorf("expected ErrEmptyTicketContext for empty HTML tags, got: %v", err)
	}
	// Test whitespace only
	if _, err := client.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{Name: "iss", DescriptionHTML: "   \t\n  "}); !errors.Is(err, ErrEmptyTicketContext) {
		t.Errorf("expected ErrEmptyTicketContext for whitespace, got: %v", err)
	}
	// Test empty name
	if _, err := client.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{Name: "", DescriptionHTML: "<p>valid</p>"}); err == nil {
		t.Errorf("expected error for empty name")
	}
}
