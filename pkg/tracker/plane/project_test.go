package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_CreateAndListProjects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/test-ws/projects/" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			proj := PlaneProject{
				ID:          "proj-123",
				Name:        body["name"].(string),
				Identifier:  body["identifier"].(string),
				Description: body["description"].(string),
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(proj)
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/test-ws/projects/" {
			projects := []PlaneProject{
				{
					ID:         "proj-123",
					Name:       "Oxipix",
					Identifier: "OXI",
				},
				{
					ID:         "proj-124",
					Name:       "Carla-Rust",
					Identifier: "CARLA",
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": projects,
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client, err := NewClient(Config{
		BaseURL:       ts.URL,
		WorkspaceSlug: "test-ws",
		ProjectID:     "proj-123",
		DirectToken:   "secret-token",
	}, nil)
	if err != nil {
		t.Fatalf("failed to construct client: %v", err)
	}

	ctx := context.Background()
	created, err := client.CreateProject(ctx, "Carla Sensor RS", "CSEN", "Sensor suite in 100% Rust")
	if err != nil {
		t.Fatalf("expected project creation success, got %v", err)
	}
	if created.ID != "proj-123" || created.Identifier != "CSEN" {
		t.Errorf("unexpected project output: %+v", created)
	}

	projects, err := client.ListProjects(ctx)
	if err != nil {
		t.Fatalf("expected list projects success, got %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestClient_EnsureLifecycleStates(t *testing.T) {
	var createdStates []map[string]interface{}
	statesInProject := []PlaneState{
		{ID: "st-1", Name: "Backlog", Group: "backlog"},
		{ID: "st-2", Name: "Todo", Group: "unstarted"},
		{ID: "st-3", Name: "In Progress", Group: "started"},
		{ID: "st-4", Name: "Done", Group: "completed"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/test-ws/projects/proj-123/states/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": statesInProject,
			})
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/test-ws/projects/proj-123/states/" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdStates = append(createdStates, body)
			newState := PlaneState{
				ID:    "st-verify",
				Name:  body["name"].(string),
				Group: body["group"].(string),
			}
			statesInProject = append(statesInProject, newState)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(newState)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client, err := NewClient(Config{
		BaseURL:       ts.URL,
		WorkspaceSlug: "test-ws",
		ProjectID:     "proj-123",
		DirectToken:   "secret-token",
	}, nil)
	if err != nil {
		t.Fatalf("failed to construct client: %v", err)
	}

	ctx := context.Background()

	// 1. First call: "To Verify" missing -> creates it
	created, err := client.EnsureLifecycleStates(ctx, "proj-123")
	if err != nil {
		t.Fatalf("expected EnsureLifecycleStates to succeed, got %v", err)
	}
	if len(created) != 1 || created[0] != "To Verify" {
		t.Fatalf("expected ['To Verify'], got %v", created)
	}
	if len(createdStates) != 1 {
		t.Fatalf("expected 1 POST to create state, got %d", len(createdStates))
	}
	if createdStates[0]["name"] != "To Verify" || createdStates[0]["group"] != "started" || createdStates[0]["color"] != "#F59E0B" {
		t.Errorf("unexpected state payload: %+v", createdStates[0])
	}

	// 2. Second call: "To Verify" already exists -> creates nothing
	createdSecond, err := client.EnsureLifecycleStates(ctx, "proj-123")
	if err != nil {
		t.Fatalf("expected second EnsureLifecycleStates to succeed, got %v", err)
	}
	if len(createdSecond) != 0 {
		t.Fatalf("expected 0 created on second call, got %v", createdSecond)
	}
	if len(createdStates) != 1 {
		t.Fatalf("expected still 1 POST, got %d", len(createdStates))
	}

	// 3. Omitting projectID defaults to c.projectID
	createdDefault, err := client.EnsureLifecycleStates(ctx, "")
	if err != nil {
		t.Fatalf("expected default project EnsureLifecycleStates to succeed, got %v", err)
	}
	if len(createdDefault) != 0 {
		t.Fatalf("expected 0 created for default project, got %v", createdDefault)
	}
}

func TestClient_EnsureLifecycleStates_BareListAndErrors(t *testing.T) {
	// Server returning bare list with lowercase "to verify"
	bareServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id": "st-v", "name": "to verify", "group": "started"}]`))
	}))
	defer bareServer.Close()

	client, _ := NewClient(Config{
		BaseURL:       bareServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, bareServer.Client())

	ctx := context.Background()
	created, err := client.EnsureLifecycleStates(ctx, "p")
	if err != nil {
		t.Fatalf("unexpected error on bare list: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("expected no states created when case-insensitive 'to verify' exists, got %v", created)
	}

	// Empty project with empty client project
	emptyClient, _ := NewClient(Config{
		BaseURL:       bareServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, nil)
	emptyClient.projectID = ""
	if _, err := emptyClient.EnsureLifecycleStates(ctx, ""); err != ErrEmptyProjectID {
		t.Errorf("expected ErrEmptyProjectID, got: %v", err)
	}

	// 401 unauthorized on GET
	unauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthServer.Close()

	cUnauth, _ := NewClient(Config{
		BaseURL:       unauthServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, unauthServer.Client())

	if _, err := cUnauth.EnsureLifecycleStates(ctx, "p"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on GET 401, got: %v", err)
	}

	// 500 error on GET
	err500Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer err500Server.Close()

	c500, _ := NewClient(Config{
		BaseURL:       err500Server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, err500Server.Client())

	if _, err := c500.EnsureLifecycleStates(ctx, "p"); err == nil {
		t.Errorf("expected error on GET 500")
	}

	// POST 401 unauthorized
	postUnauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer postUnauthServer.Close()

	cPostUnauth, _ := NewClient(Config{
		BaseURL:       postUnauthServer.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, postUnauthServer.Client())

	if _, err := cPostUnauth.EnsureLifecycleStates(ctx, "p"); err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized on POST 401, got: %v", err)
	}

	// POST 500 internal error
	post500Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer post500Server.Close()

	cPost500, _ := NewClient(Config{
		BaseURL:       post500Server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, post500Server.Client())

	if _, err := cPost500.EnsureLifecycleStates(ctx, "p"); err == nil {
		t.Errorf("expected error on POST 500")
	}
}
