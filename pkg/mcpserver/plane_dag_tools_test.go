package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"Dendrite/pkg/tracker/plane"
)

func TestMCP_PlaneDAGTools(t *testing.T) {
	// Setup mock Plane HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [
				{"id": "iss-1", "name": "[DEN-010] Foundation Core", "sequence_id": 10, "state": "st-todo", "priority": "low"},
				{"id": "iss-2", "name": "[DEN-011] Urgent Feature", "sequence_id": 11, "state": "st-todo", "priority": "urgent"}
			]}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/states/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [
				{"id": "st-done", "name": "Done", "group": "completed"},
				{"id": "st-todo", "name": "To Do", "group": "unstarted"}
			]}`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/issues/iss-2/issue-relation/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"blocking": [], "blocked_by": [{"id": "iss-1", "name": "[DEN-010] Foundation Core"}], "duplicate": [], "relates_to": []}`))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/issue-relation/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"blocking": [], "blocked_by": [], "duplicate": [], "relates_to": []}`))

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/issue-relation/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[{"id": "rel-1", "relation_type": "blocked_by"}]`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": "iss-new", "name": "[DEN-100] New Feature", "sequence_id": 100, "priority": "high"}`))

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/modules/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results": [{"id": "mod-1", "name": "Milestone 1"}]}`))

		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	planeClient, err := plane.NewClient(plane.Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		DirectToken:   "sec-key",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	mcpServer := NewServer(ServerConfig{
		Name:    "Dendrite Test Server",
		Version: "1.0.0",
	})
	mcpServer.RegisterDefaultTools(Dependencies{
		PlaneClient: planeClient,
		DevStatus:   "DEV_STATUS.md",
	})

	callTool := func(toolName string, args map[string]interface{}) *CallToolResult {
		reqBytes, _ := json.Marshal(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      toolName,
				"arguments": args,
			},
		})
		resp := mcpServer.HandleRequest(context.Background(), reqBytes)
		if resp == nil || resp.Error != nil {
			t.Fatalf("tool %s failed: %v", toolName, resp)
		}
		res, ok := resp.Result.(*CallToolResult)
		if !ok {
			t.Fatalf("expected *CallToolResult, got: %T", resp.Result)
		}
		return res
	}

	// 1. Test dendrite_ticket_smart_select
	// In mock data: DEN-010 (Low, Todo) blocks DEN-011 (Urgent, Todo).
	// Because DEN-010 unblocks Urgent DEN-011, DEN-010 should be selected first!
	selectRes := callTool("dendrite_ticket_smart_select", map[string]interface{}{})
	var selectOutput struct {
		Status    string `json:"status"`
		Selection struct {
			SelectedTicket struct {
				ID string `json:"id"`
			} `json:"selected_ticket"`
			IsBlocked bool `json:"is_blocked"`
		} `json:"selection"`
	}
	if err := json.Unmarshal([]byte(selectRes.Content[0].Text), &selectOutput); err != nil {
		t.Fatalf("failed to parse smart select json: %v", err)
	}
	if selectOutput.Selection.SelectedTicket.ID != "DEN-010" {
		t.Errorf("expected DEN-010 to be selected, got %s", selectOutput.Selection.SelectedTicket.ID)
	}

	// 2. Test user override in dendrite_ticket_smart_select
	overrideRes := callTool("dendrite_ticket_smart_select", map[string]interface{}{
		"ticket_id": "DEN-011",
	})
	var overrideOutput struct {
		Selection struct {
			SelectedTicket struct {
				ID string `json:"id"`
			} `json:"selected_ticket"`
			IsBlocked bool `json:"is_blocked"`
		} `json:"selection"`
	}
	_ = json.Unmarshal([]byte(overrideRes.Content[0].Text), &overrideOutput)
	if overrideOutput.Selection.SelectedTicket.ID != "DEN-011" {
		t.Errorf("expected DEN-011 selected on override")
	}
	if !overrideOutput.Selection.IsBlocked {
		t.Errorf("expected DEN-011 to be flagged as blocked by DEN-010")
	}

	// 3. Test dendrite_plane_relation_link
	linkRes := callTool("dendrite_plane_relation_link", map[string]interface{}{
		"issue_id":        "DEN-011",
		"relation_type":   "blocked_by",
		"target_issue_id": "DEN-010",
	})
	if !strings.Contains(linkRes.Content[0].Text, "gantt_connected") {
		t.Errorf("expected gantt_connected in relation link output: %s", linkRes.Content[0].Text)
	}

	// 4. Test dendrite_plane_issue_create
	createRes := callTool("dendrite_plane_issue_create", map[string]interface{}{
		"name":       "[DEN-100] New Feature",
		"milestone":  "Milestone 1",
		"priority":   "high",
		"estimate":   8,
		"blocked_by": []interface{}{"DEN-010"},
	})
	if !strings.Contains(createRes.Content[0].Text, "gantt_plan_placed") {
		t.Errorf("expected gantt_plan_placed in issue create output: %s", createRes.Content[0].Text)
	}
}

func TestMCP_SmartSelect_LocalFallback(t *testing.T) {
	// Point to unreachable port to force fallback to local markdown
	os.Setenv("PLANE_BASE_URL", "http://127.0.0.1:54321")
	defer os.Unsetenv("PLANE_BASE_URL")

	tmpFile := "test_dev_status_tmp.md"
	content := fmt.Sprintf("# Test Dev Status\n- **Active Milestone**: Milestone 1\n- **Current Focus**: DEN-001\n\n### Milestone 1: Core\n- [ ] **DEN-001**: Task 1 (`pkg/a`)\n- [ ] **DEN-002**: Task 2 (`pkg/b`)\n")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	mcpServer := NewServer(ServerConfig{})
	mcpServer.RegisterDefaultTools(Dependencies{
		PlaneClient: nil, // force fallback
		DevStatus:   tmpFile,
	})

	reqBytes, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "dendrite_ticket_smart_select",
			"arguments": map[string]interface{}{
				"file": tmpFile,
			},
		},
	})
	resp := mcpServer.HandleRequest(context.Background(), reqBytes)
	if resp == nil || resp.Error != nil {
		t.Fatalf("local fallback failed: %v", resp)
	}
	res, ok := resp.Result.(*CallToolResult)
	if !ok {
		t.Fatalf("expected *CallToolResult, got: %T", resp.Result)
	}
	if !strings.Contains(res.Content[0].Text, "DEN-001") {
		t.Errorf("expected DEN-001 in selection output: %s", res.Content[0].Text)
	}
}
