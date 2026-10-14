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
