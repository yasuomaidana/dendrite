package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/ollama"
	"Dendrite/pkg/tracker/plane"
)

// helper to spin up an httptest.Server simulating Plane API routes.
func createTestPlaneServer(t *testing.T) (*httptest.Server, *plane.Client) {
	mux := http.NewServeMux()

	// Issues collection
	mux.HandleFunc("/api/v1/workspaces/dendrite-ws/projects/proj-1/issues/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]plane.PlaneIssue{
				{
					ID:                  "issue-1",
					SequenceID:          101,
					Name:                "[DEN-101] Test Issue",
					State:               "state-todo",
					DescriptionStripped: "Target Package: pkg/mcpserver",
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(plane.PlaneIssue{
				ID:         "issue-new",
				SequenceID: 102,
				Name:       "[DEN-102] Created Issue",
				State:      "state-todo",
			})
			return
		}
	})

	// Single issue item
	mux.HandleFunc("/api/v1/workspaces/dendrite-ws/projects/proj-1/issues/issue-1/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(plane.PlaneIssue{
			ID:         "issue-1",
			SequenceID: 101,
			Name:       "[DEN-101] Test Issue",
			State:      "state-done",
		})
	})

	// States collection
	mux.HandleFunc("/api/v1/workspaces/dendrite-ws/projects/proj-1/states/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]plane.PlaneState{
			{ID: "state-todo", Name: "Todo", Group: "unstarted"},
			{ID: "state-done", Name: "Done", Group: "completed"},
			{ID: "state-prog", Name: "In Progress", Group: "started"},
			{ID: "state-rev", Name: "To Verify", Group: "started"},
		})
	})

	// Modules collection
	mux.HandleFunc("/api/v1/workspaces/dendrite-ws/projects/proj-1/modules/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(plane.PlaneModule{
				ID:     "mod-new",
				Name:   "New Module",
				Status: "in-progress",
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]plane.PlaneModule{
			{ID: "mod-1", Name: "M1", Status: "in-progress"},
		})
	})

	// Module issues
	mux.HandleFunc("/api/v1/workspaces/dendrite-ws/projects/proj-1/modules/mod-1/module-issues/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]plane.PlaneIssue{})
	})

	// Projects collection
	mux.HandleFunc("/api/v1/workspaces/dendrite-ws/projects/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]plane.PlaneProject{
			{ID: "proj-1", Identifier: "DEN", Name: "Dendrite"},
		})
	})

	// Pages collection
	mux.HandleFunc("/api/workspaces/dendrite-ws/projects/proj-1/pages/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(plane.PlanePage{
				ID:   "page-new",
				Name: "Page",
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]plane.PlanePage{})
	})

	// Issue relations
	mux.HandleFunc("/api/workspaces/dendrite-ws/projects/proj-1/issues/issue-1/issue-relation/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := plane.NewClient(plane.Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "dendrite-ws",
		ProjectID:     "proj-1",
		DirectToken:   "token-xyz",
	}, server.Client())
	if err != nil {
		t.Fatalf("failed to create plane client: %v", err)
	}

	return server, client
}

// helper to spin up an httptest.Server simulating Ollama probe endpoint.
func createTestOllamaServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ollama is running"))
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []ollama.ModelInfo{
				{Name: "qwen2.5-coder:7b", Size: 4500000000},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// helper to initialize a temporary git repo.
func createTestGitRepo(t *testing.T) string {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v (%s)", strings.Join(args, " "), err, string(out))
		}
	}
	run("init")
	run("config", "user.name", "TestUser")
	run("config", "user.email", "test@example.com")
	run("config", "commit.gpgsign", "false")

	file := filepath.Join(dir, "README.md")
	if err := os.WriteFile(file, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial commit")
	return dir
}

func TestTools_DendriteEnvStatus(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	callReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_env_status", "arguments": {}}}`
	resp := srv.HandleRequest(context.Background(), []byte(callReq))
	if resp == nil || resp.Error != nil {
		t.Fatalf("dendrite_env_status failed: %v", resp)
	}
}

func TestTools_DendriteTicketList(t *testing.T) {
	_, planeClient := createTestPlaneServer(t)

	tmpDir := t.TempDir()
	devStatusFile := filepath.Join(tmpDir, "DEV_STATUS.md")
	mdContent := "# Development Status\n\n- **Active Milestone**: Milestone 1\n\n### Milestone 1: Core Foundation\n\n- [ ] **DEN-001**: Initial Setup (`pkg/mcpserver`) [State: To Do, Priority: High]\n"
	if err := os.WriteFile(devStatusFile, []byte(mdContent), 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{
		DevStatus:   devStatusFile,
		PlaneClient: planeClient,
	})

	// 1. Success with markdown file
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_ticket_list", "arguments": {"file": "` + devStatusFile + `"}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("expected success with markdown file, got: %v", resp1)
	}

	// 2. Markdown file missing, fallback to Plane
	missingFile := filepath.Join(tmpDir, "NON_EXISTENT.md")
	req2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_ticket_list", "arguments": {"file": "` + missingFile + `"}}}`
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("expected fallback to Plane issues, got: %v", resp2)
	}

	// 3. Markdown missing and Plane client nil / error
	srvNoPlane := NewServer(ServerConfig{})
	RegisterDefaultTools(srvNoPlane, Dependencies{DevStatus: missingFile})
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:0")
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_ticket_list", "arguments": {"file": "` + missingFile + `"}}}`
	resp3 := srvNoPlane.HandleRequest(context.Background(), []byte(req3))
	if resp3 == nil {
		t.Fatal("expected response")
	}
	callRes, ok := resp3.Result.(CallToolResult)
	if !ok || !callRes.IsError {
		t.Fatalf("expected error result when both markdown and plane fail, got: %#v", resp3.Result)
	}
}

func TestTools_DendriteTicketUpdate(t *testing.T) {
	_, planeClient := createTestPlaneServer(t)

	tmpDir := t.TempDir()
	devStatusFile := filepath.Join(tmpDir, "DEV_STATUS.md")
	mdContent := "# Development Status\n\n- **Active Milestone**: Milestone 1\n\n### Milestone 1: Core Foundation\n\n- [ ] **DEN-001**: Initial Setup (`pkg/mcpserver`) [State: To Do, Priority: High]\n"
	if err := os.WriteFile(devStatusFile, []byte(mdContent), 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{
		DevStatus:   devStatusFile,
		PlaneClient: planeClient,
	})

	// Missing ticket_id
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_ticket_update", "arguments": {"status": "DONE"}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if res, ok := resp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Errorf("expected error for missing ticket_id, got: %#v", resp1)
	}

	// Missing status
	req2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_ticket_update", "arguments": {"ticket_id": "DEN-001"}}}`
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if res, ok := resp2.Result.(CallToolResult); !ok || !res.IsError {
		t.Errorf("expected error for missing status, got: %#v", resp2)
	}

	// Success with markdown file
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_ticket_update", "arguments": {"ticket_id": "DEN-001", "status": "IN_PROGRESS", "file": "` + devStatusFile + `"}}}`
	resp3 := srv.HandleRequest(context.Background(), []byte(req3))
	if resp3 == nil || resp3.Error != nil {
		t.Fatalf("expected successful ticket update in markdown, got: %v", resp3)
	}

	// Markdown fails, fallback to Plane succeeds
	missingFile := filepath.Join(tmpDir, "NON_EXISTENT.md")
	req4 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_ticket_update", "arguments": {"ticket_id": "DEN-101", "status": "DONE", "file": "` + missingFile + `"}}}`
	resp4 := srv.HandleRequest(context.Background(), []byte(req4))
	if resp4 == nil || resp4.Error != nil {
		t.Fatalf("expected fallback to Plane status update, got: %v", resp4)
	}

	// Both fail
	srvNoPlane := NewServer(ServerConfig{})
	RegisterDefaultTools(srvNoPlane, Dependencies{DevStatus: missingFile})
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:0")
	req5 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_ticket_update", "arguments": {"ticket_id": "DEN-999", "status": "DONE", "file": "` + missingFile + `"}}}`
	resp5 := srvNoPlane.HandleRequest(context.Background(), []byte(req5))
	if res, ok := resp5.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error when both markdown and plane fail, got: %#v", resp5)
	}
}

func TestTools_DendritePlaneIssues(t *testing.T) {
	_, planeClient := createTestPlaneServer(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{PlaneClient: planeClient})

	// Success
	req := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_plane_issues", "arguments": {}}}`
	resp := srv.HandleRequest(context.Background(), []byte(req))
	if resp == nil || resp.Error != nil {
		t.Fatalf("dendrite_plane_issues failed: %v", resp)
	}

	// Error when server fails
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badServer.Close()
	badClient, _ := plane.NewClient(plane.Config{
		BaseURL:       badServer.URL,
		WorkspaceSlug: "dendrite-ws",
		ProjectID:     "proj-1",
		DirectToken:   "tok",
	}, badServer.Client())

	srvBad := NewServer(ServerConfig{})
	RegisterDefaultTools(srvBad, Dependencies{PlaneClient: badClient})
	respBad := srvBad.HandleRequest(context.Background(), []byte(req))
	if res, ok := respBad.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error from dendrite_plane_issues with 500 server, got: %#v", respBad)
	}
}

func TestTools_DendriteOllamaProbe(t *testing.T) {
	ollamaSrv := createTestOllamaServer(t)
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{OllamaURL: ollamaSrv.URL})

	// 1. Success with custom URL
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_ollama_probe", "arguments": {"url": "` + ollamaSrv.URL + `"}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("expected ollama probe success, got: %v", resp1)
	}

	// 2. Success with default URL
	req2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_ollama_probe", "arguments": {}}}`
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("expected ollama probe success with default url, got: %v", resp2)
	}

	// 3. Error with unreachable URL
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_ollama_probe", "arguments": {"url": "http://127.0.0.1:0"}}}`
	resp3 := srv.HandleRequest(context.Background(), []byte(req3))
	if res, ok := resp3.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error with unreachable url, got: %#v", resp3)
	}
}

func TestTools_DendriteTopologyAdvisor(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// 1. With nodes string containing '='
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_topology_advisor", "arguments": {"nodes": "local=localhost"}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("expected success with nodes, got: %v", resp1)
	}

	// 2. With node without '='
	req2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_topology_advisor", "arguments": {"nodes": "localhost"}}}`
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("expected success with single node, got: %v", resp2)
	}

	// 3. With empty arguments (defaults to local=localhost)
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_topology_advisor", "arguments": {}}}`
	resp3 := srv.HandleRequest(context.Background(), []byte(req3))
	if resp3 == nil || resp3.Error != nil {
		t.Fatalf("expected success with empty args, got: %v", resp3)
	}
}

func TestTools_DendriteGatewayRender(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// Valid scenarios
	for _, sc := range []string{"local-hybrid", "cloud-burst", "full-local"} {
		req := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_gateway_render", "arguments": {"scenario": "%s"}}}`, sc)
		resp := srv.HandleRequest(context.Background(), []byte(req))
		if resp == nil || resp.Error != nil {
			t.Fatalf("scenario %s failed: %v", sc, resp)
		}
	}

	// Invalid scenario
	reqInvalid := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_gateway_render", "arguments": {"scenario": "unknown-scenario"}}}`
	respInvalid := srv.HandleRequest(context.Background(), []byte(reqInvalid))
	if res, ok := respInvalid.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for invalid scenario, got: %#v", respInvalid)
	}
}

func TestTools_DendriteGatewayUpdate(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "litellm_config.yaml")

	// 1. Dry run
	req1 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_gateway_update", "arguments": {"file": "%s", "dry_run": true, "use_container_host": false}}}`, cfgPath)
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("dry_run gateway update failed: %v", resp1)
	}

	// 2. Real update
	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_gateway_update", "arguments": {"file": "%s", "dry_run": false, "use_container_host": true}}}`, cfgPath)
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("real gateway update failed: %v", resp2)
	}
}

func TestTools_DendriteAgentRun(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// 1. Invalid agent type
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_agent_run", "arguments": {"agent_type": "invalid_agent", "task": "test"}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if res, ok := resp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for invalid agent type, got: %#v", resp1)
	}

	// 2. Multiple valid agent types (model default branches, sandbox, spend_limit)
	agents := []string{"aider", "openhands", "opencode", "agy", "copilot"}
	for _, a := range agents {
		req := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_agent_run", "arguments": {"agent_type": "%s", "task": "run chore", "ticket_id": "DEN-001", "sandbox": true, "spend_limit": 1.50}}}`, a)
		resp := srv.HandleRequest(context.Background(), []byte(req))
		if resp == nil {
			t.Fatalf("expected response for agent %s", a)
		}
	}
}
