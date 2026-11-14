package plane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

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
		Priority:        prio,
		StartDate:       &sDate,
		TargetDate:      &tDate,
		EstimatePoint:   &est,
		ModuleID:        "mod-1",
		BlockedBy:       []string{"iss-0"},
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
