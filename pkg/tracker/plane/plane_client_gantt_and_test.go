package plane

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

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

func TestClient_UpdateStatus_ZeroMockRejection(t *testing.T) {
	// Hermetic fixture: a throwaway git repository whose pkg/uitest ships a production mock type.
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "pkg", "uitest"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package uitest\n\ntype " + "Mock" + "UIDriver struct{}\n"
	if err := os.WriteFile(filepath.Join(repo, "pkg", "uitest", "driver.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	t.Chdir(repo)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/states/" {
			data := `[
				{"id": "st-todo", "name": "To Do", "group": "unstarted"},
				{"id": "st-done", "name": "Done", "group": "completed"}
			]`
			_, _ = w.Write([]byte(data))
			return
		}
		if r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/issues/" {
			data := `[
				{"id": "iss-mock-1", "name": "[DEN-038] Automated Test Synthesizer (pkg/uitest)", "description_html": "<div>Needs Playwright</div>"}
			]`
			_, _ = w.Write([]byte(data))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:       ts.URL,
		WorkspaceSlug: "ws1",
		ProjectID:     "p1",
		DirectToken:   "tok-1",
	}
	client, err := NewClient(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.UpdateStatus(context.Background(), "DEN-038", tracker.StatusDone)
	if err == nil {
		t.Fatalf("expected zero-mock gate rejection on DEN-038, got nil")
	}
	if !strings.Contains(err.Error(), "zero-mock policy gate violation") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestClient_UpdateStatus_ExactIDResolution(t *testing.T) {
	var patchedIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/states/" {
			data := `[
				{"id": "st-todo", "name": "To Do", "group": "unstarted"},
				{"id": "st-done", "name": "Done", "group": "completed"}
			]`
			_, _ = w.Write([]byte(data))
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/issues/" {
			// DEN-900 appears FIRST in list; DEN-90 appears second
			data := `[
				{"id": "uuid-den-900", "name": "[DEN-900] Substring ticket", "sequence_id": 900},
				{"id": "uuid-den-90", "name": "[DEN-90] Target ticket", "sequence_id": 90}
			]`
			_, _ = w.Write([]byte(data))
			return
		}

		if r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/workspaces/ws1/projects/p1/issues/") {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			issueID := parts[len(parts)-1]
			patchedIDs = append(patchedIDs, issueID)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "` + issueID + `"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client, err := NewClient(Config{
		BaseURL:       ts.URL,
		WorkspaceSlug: "ws1",
		ProjectID:     "p1",
		DirectToken:   "tok-1",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// 1. UpdateStatus with "DEN-90": must match "[DEN-90]" and NOT "[DEN-900]"
	if err := client.UpdateStatus(ctx, "DEN-90", tracker.StatusDone); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	if len(patchedIDs) != 1 {
		t.Fatalf("expected exactly 1 patch, got %d (%v)", len(patchedIDs), patchedIDs)
	}
	if patchedIDs[0] != "uuid-den-90" {
		t.Errorf("expected patched issue to be uuid-den-90, got: %s", patchedIDs[0])
	}

	// 2. Fallback: non-bracketed issue name
	patchedIDs = nil
	fallbackTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/states/" {
			_, _ = w.Write([]byte(`[{"id": "st-done", "name": "Done", "group": "completed"}]`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws1/projects/p1/issues/" {
			_, _ = w.Write([]byte(`[{"id": "uuid-plain-90", "name": "DEN-90 without brackets", "sequence_id": 90}]`))
			return
		}
		if r.Method == http.MethodPatch {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			issueID := parts[len(parts)-1]
			patchedIDs = append(patchedIDs, issueID)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "` + issueID + `"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fallbackTS.Close()

	fallbackClient, err := NewClient(Config{
		BaseURL:       fallbackTS.URL,
		WorkspaceSlug: "ws1",
		ProjectID:     "p1",
		DirectToken:   "tok-1",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := fallbackClient.UpdateStatus(ctx, "DEN-90", tracker.StatusDone); err != nil {
		t.Fatalf("UpdateStatus fallback failed: %v", err)
	}
	if len(patchedIDs) != 1 || patchedIDs[0] != "uuid-plain-90" {
		t.Errorf("expected fallback to match uuid-plain-90, got: %v", patchedIDs)
	}
}
