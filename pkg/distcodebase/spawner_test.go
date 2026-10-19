package distcodebase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/workspace"
)

func TestProjectSpawner_Spawn_WithPlaneLifecycleStates(t *testing.T) {
	tempBase := t.TempDir()

	statesInProject := []plane.PlaneState{
		{ID: "st-backlog", Name: "Backlog", Group: "backlog"},
		{ID: "st-todo", Name: "Todo", Group: "unstarted"},
		{ID: "st-prog", Name: "In Progress", Group: "started"},
		{ID: "st-done", Name: "Done", Group: "completed"},
	}
	var createdStateBodies []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. CreateProject POST
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/test-ws/projects/" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          "proj-spawn-123",
				"name":        body["name"],
				"identifier":  body["identifier"],
				"description": body["description"],
			})
			return
		}

		// 2. States GET
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/test-ws/projects/proj-spawn-123/states/" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": statesInProject,
			})
			return
		}

		// 3. States PATCH (EnsureLifecycleStates -> Todo renamed / recolored)
		if r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/workspaces/test-ws/projects/proj-spawn-123/states/") {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 4. States POST (EnsureLifecycleStates -> To Verify, Cancelled)
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/test-ws/projects/proj-spawn-123/states/" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdStateBodies = append(createdStateBodies, body)
			newState := plane.PlaneState{
				ID:    "st-" + body["name"].(string),
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
	defer server.Close()

	planeClient, err := plane.NewClient(plane.Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "test-ws",
		ProjectID:     "default-proj",
		DirectToken:   "secret-token",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create plane client: %v", err)
	}

	wsReg, err := workspace.NewRegistry(filepath.Join(tempBase, ".dendrite"), filepath.Join(tempBase, "ws.json"))
	if err != nil {
		t.Fatalf("failed to create workspace registry: %v", err)
	}

	spawner := NewProjectSpawner(planeClient, wsReg)
	targetDir := filepath.Join(tempBase, "projects", "dcore")

	ctx := context.Background()
	res, err := spawner.Spawn(ctx, SpawnRequest{
		ProjectName:        "DCORE",
		TargetDir:          targetDir,
		Language:           "rust",
		CreatePlaneProject: true,
		InitGit:            true,
		Identifier:         "DCORE",
		Description:        "Core distributed system",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if res.PlaneProjectID != "proj-spawn-123" {
		t.Errorf("expected PlaneProjectID proj-spawn-123, got: %s", res.PlaneProjectID)
	}
	if res.PlaneError != "" {
		t.Errorf("expected empty PlaneError, got: %s", res.PlaneError)
	}
	if len(createdStateBodies) != 2 {
		t.Fatalf("expected 2 states created (To Verify, Cancelled), got %d", len(createdStateBodies))
	}
	if createdStateBodies[0]["name"] != "To Verify" || createdStateBodies[0]["group"] != "started" {
		t.Errorf("unexpected created state: %+v", createdStateBodies[0])
	}

	// Verify the project now has Backlog, a To Do-group state, In Progress, To Verify and Done
	hasBacklog, hasTodo, hasInProgress, hasToVerify, hasDone := false, false, false, false, false
	for _, st := range statesInProject {
		switch {
		case strings.EqualFold(st.Name, "Backlog"):
			hasBacklog = true
		case strings.EqualFold(st.Name, "Todo") || st.Group == "unstarted":
			hasTodo = true
		case strings.EqualFold(st.Name, "In Progress") && st.Group == "started":
			hasInProgress = true
		case strings.EqualFold(st.Name, "To Verify") && st.Group == "started":
			hasToVerify = true
		case strings.EqualFold(st.Name, "Done") && st.Group == "completed":
			hasDone = true
		}
	}
	if !hasBacklog || !hasTodo || !hasInProgress || !hasToVerify || !hasDone {
		t.Errorf("project states incomplete: Backlog=%v, Todo=%v, InProgress=%v, ToVerify=%v, Done=%v",
			hasBacklog, hasTodo, hasInProgress, hasToVerify, hasDone)
	}
}

func TestProjectSpawner_Spawn_EnsureStatesError(t *testing.T) {
	tempBase := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/projects/") {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   "proj-err-123",
				"name": "DRAG",
			})
			return
		}
		// States endpoint returns 500
		if strings.Contains(r.URL.Path, "/states/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	planeClient, _ := plane.NewClient(plane.Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, nil)

	spawner := NewProjectSpawner(planeClient, nil)
	targetDir := filepath.Join(tempBase, "projects", "drag")

	res, err := spawner.Spawn(context.Background(), SpawnRequest{
		ProjectName:        "DRAG",
		TargetDir:          targetDir,
		CreatePlaneProject: true,
	})
	if err != nil {
		t.Fatalf("unexpected error from Spawn: %v", err)
	}
	if res.PlaneProjectID != "proj-err-123" {
		t.Errorf("expected PlaneProjectID proj-err-123, got: %s", res.PlaneProjectID)
	}
	if !strings.Contains(res.PlaneError, "ensure states:") {
		t.Errorf("expected PlaneError to contain 'ensure states:', got: %s", res.PlaneError)
	}
}

func TestProjectSpawner_Spawn_CreateProjectError(t *testing.T) {
	tempBase := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	planeClient, _ := plane.NewClient(plane.Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "tok",
	}, nil)

	spawner := NewProjectSpawner(planeClient, nil)
	targetDir := filepath.Join(tempBase, "projects", "dgit")

	res, err := spawner.Spawn(context.Background(), SpawnRequest{
		ProjectName:        "DGIT",
		TargetDir:          targetDir,
		CreatePlaneProject: true,
	})
	if err != nil {
		t.Fatalf("unexpected error from Spawn: %v", err)
	}
	if !strings.Contains(res.PlaneError, "identifier DGIT:") {
		t.Errorf("expected PlaneError to contain 'identifier DGIT:', got: %s", res.PlaneError)
	}
}

func TestProjectSpawner_Spawn_ScaffoldingAndValidation(t *testing.T) {
	tempBase := t.TempDir()
	spawner := NewProjectSpawner(nil, nil)
	ctx := context.Background()

	// Empty name
	_, err := spawner.Spawn(ctx, SpawnRequest{ProjectName: ""})
	if err != ErrEmptyProjectName {
		t.Errorf("expected ErrEmptyProjectName, got: %v", err)
	}

	// Directory exists and not empty
	existingDir := filepath.Join(tempBase, "existing")
	_ = os.MkdirAll(existingDir, 0755)
	_ = os.WriteFile(filepath.Join(existingDir, "dummy.txt"), []byte("data"), 0644)
	_, err = spawner.Spawn(ctx, SpawnRequest{
		ProjectName: "Existing",
		TargetDir:   existingDir,
	})
	if err != ErrProjectDirExists {
		t.Errorf("expected ErrProjectDirExists, got: %v", err)
	}

	// Test languages scaffolding
	languages := []string{"go", "typescript", "python", "rust"}
	for _, lang := range languages {
		dir := filepath.Join(tempBase, "lang-"+lang)
		res, err := spawner.Spawn(ctx, SpawnRequest{
			ProjectName: "Test-" + lang,
			TargetDir:   dir,
			Language:    lang,
		})
		if err != nil {
			t.Fatalf("failed to spawn %s: %v", lang, err)
		}
		if res.Status != "created_and_initialized" {
			t.Errorf("unexpected status: %s", res.Status)
		}
	}

	// Default target dir when empty
	res, err := spawner.Spawn(ctx, SpawnRequest{
		ProjectName: "SubDefault",
		TargetDir:   filepath.Join(tempBase, "auto-default"),
	})
	if err != nil {
		t.Fatalf("failed to spawn default dir: %v", err)
	}
	if res.ProjectPath == "" {
		t.Errorf("expected non-empty project path")
	}

	// Test SpawnParallel
	reqs := []SpawnRequest{
		{ProjectName: "Par1", TargetDir: filepath.Join(tempBase, "par1")},
		{ProjectName: "Par2", TargetDir: filepath.Join(tempBase, "par2")},
	}
	results, err := spawner.SpawnParallel(ctx, reqs)
	if err != nil {
		t.Fatalf("SpawnParallel failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results from parallel spawn, got: %d", len(results))
	}

	// Test SpawnParallel error
	errReqs := []SpawnRequest{
		{ProjectName: ""},
	}
	_, err = spawner.SpawnParallel(ctx, errReqs)
	if err == nil {
		t.Errorf("expected error from SpawnParallel with empty project name")
	}
}
