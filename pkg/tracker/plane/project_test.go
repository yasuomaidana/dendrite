package plane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

		if r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/workspaces/test-ws/projects/proj-123/states/") {
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/workspaces/test-ws/projects/proj-123/states/"), "/")
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			for i := range statesInProject {
				if statesInProject[i].ID != id {
					continue
				}
				if v, ok := body["name"].(string); ok {
					statesInProject[i].Name = v
				}
				if v, ok := body["group"].(string); ok {
					statesInProject[i].Group = v
				}
				if v, ok := body["color"].(string); ok {
					statesInProject[i].Color = v
				}
				if v, ok := body["sequence"].(float64); ok {
					statesInProject[i].Sequence = v
				}
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/test-ws/projects/proj-123/states/" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdStates = append(createdStates, body)
			newState := PlaneState{
				ID:       "st-" + body["name"].(string),
				Name:     body["name"].(string),
				Group:    body["group"].(string),
				Color:    body["color"].(string),
				Sequence: body["sequence"].(float64),
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

	// 1. First call: "To Verify", "Blocked" and "Cancelled" missing -> creates them, renames "Todo"
	created, err := client.EnsureLifecycleStates(ctx, "proj-123")
	if err != nil {
		t.Fatalf("expected EnsureLifecycleStates to succeed, got %v", err)
	}
	if len(created) != 3 || created[0] != "To Verify" || created[1] != "Blocked" || created[2] != "Cancelled" {
		t.Fatalf("expected ['To Verify' 'Blocked' 'Cancelled'], got %v", created)
	}
	if len(createdStates) != 3 {
		t.Fatalf("expected 3 POSTs to create states, got %d", len(createdStates))
	}
	if createdStates[0]["name"] != "To Verify" || createdStates[0]["group"] != "started" || createdStates[0]["color"] != "#FCB900" {
		t.Errorf("unexpected state payload: %+v", createdStates[0])
	}
	if statesInProject[1].Name != "To Do" {
		t.Errorf("expected Todo renamed to To Do, got %q", statesInProject[1].Name)
	}

	// 2. Second call: "To Verify" already exists -> creates nothing
	createdSecond, err := client.EnsureLifecycleStates(ctx, "proj-123")
	if err != nil {
		t.Fatalf("expected second EnsureLifecycleStates to succeed, got %v", err)
	}
	if len(createdSecond) != 0 {
		t.Fatalf("expected 0 created on second call, got %v", createdSecond)
	}
	if len(createdStates) != 3 {
		t.Fatalf("expected still 3 POSTs, got %d", len(createdStates))
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
	// Server returning a bare list where only a lowercase "to verify" is off-canonical
	var bareWrites []string
	bareServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			bareWrites = append(bareWrites, r.Method+" "+r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"id": "st-b", "name": "Backlog", "group": "backlog", "color": "#A3A3A3", "sequence": 15000},
			{"id": "st-t", "name": "To Do", "group": "unstarted", "color": "#3A3A3A", "sequence": 25000},
			{"id": "st-p", "name": "In Progress", "group": "started", "color": "#F59E0B", "sequence": 35000},
			{"id": "st-v", "name": "to verify", "group": "started", "color": "#fcb900", "sequence": 40000},
			{"id": "st-k", "name": "Blocked", "group": "started", "color": "#DC2626", "sequence": 42000},
			{"id": "st-d", "name": "Done", "group": "completed", "color": "#16A34A", "sequence": 45000},
			{"id": "st-c", "name": "Cancelled", "group": "cancelled", "color": "#EF4444", "sequence": 55000}
		]`))
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
	if len(bareWrites) != 1 || bareWrites[0] != "PATCH /api/v1/workspaces/ws/projects/p/states/st-v/" {
		t.Fatalf("expected only the 'to verify' name to be patched, got %v", bareWrites)
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

	if _, err := cPostUnauth.EnsureLifecycleStates(ctx, "p"); !errors.Is(err, ErrUnauthorized) {
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
