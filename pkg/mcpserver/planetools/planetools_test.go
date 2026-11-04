package planetools_test

import (
	"Dendrite/pkg/httpscript"
	"Dendrite/pkg/mcpserver/catalog"
	"Dendrite/pkg/mcpserver/mcptest"
	"Dendrite/pkg/mcpserver/planetools"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/tracker/plane"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "Dendrite/pkg/mcpserver"
)

func TestMain(m *testing.M) { mcptest.Main(m) }

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

// healthyPlaneRules answers every read of a healthy, empty board; the project list goes last because
// its path is a prefix of the others.
func healthyPlaneRules() []mcptest.ApiRule {
	return []mcptest.ApiRule{

		mcptest.On("GET", "/states/", 200, `{"results":[]}`),
		mcptest.On("GET", "/modules/", 200, `{"results":[]}`),
		mcptest.On("GET", "/issues/", 200, `{"results":[]}`),
		mcptest.On("GET", "/projects/", 200, `{"results":[{"id":"proj","identifier":"DEN"}]}`),
		mcptest.On("GET", "/pages/", 200, `[]`),
	}
}

// lifecycleRig is a server with the lifecycle tools, a Plane board and a working copy mcptest.On branch agent/den-377.
type lifecycleRig struct {
	t     *testing.T
	s     *Server
	board *planetest.Board
	repo  string
}

// call runs a tool in the rig's working copy and decodes its JSON answer.
func (r *lifecycleRig) call(name string, args map[string]any) (map[string]any, error) {
	r.t.Helper()
	if _, has := args["worktree"]; !has {
		args["worktree"] = r.repo
	}
	h, ok := r.s.Handler(name)
	if !ok {
		r.t.Fatalf("%s is not registered", name)
	}
	res, err := h(context.Background(), args)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
		r.t.Fatalf("%s answered %q: %v", name, res.Content[0].Text, err)
	}
	return out, nil
}

var lifecycleTools = []string{
	"dendrite_ticket_claim", "dendrite_ticket_comment", "dendrite_ticket_upload_plan",
	"dendrite_ticket_get_plan", "dendrite_ticket_transition", "dendrite_ticket_finish", "dendrite_ticket_decision", "dendrite_ticket_comments", "dendrite_query_context",
}

func newLifecycleRig(t *testing.T, tickets map[string]string) *lifecycleRig {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	t.Setenv("DENDRITE_AGENT", "claude")
	t.Setenv("DENDRITE_MODEL_TIER", "7")
	board := planetest.NewBoard(t, tickets)
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	testkit.Git(t, repo, "init", "-q")
	testkit.Git(t, repo, "checkout", "-q", "-b", "agent/den-377")
	s := NewServer(ServerConfig{})
	planetools.RegisterTicketLifecycleTools(s, Dependencies{PlaneClient: client})
	return &lifecycleRig{t: t, s: s, board: board, repo: repo}
}

func newScriptedPlane(t *testing.T, rules ...mcptest.ApiRule) *mcptest.ScriptedPlane {
	t.Helper()
	return httpscript.New(t, rules...)
}

const smartIssues = `{"results":[
 {"id":"u1","name":"[DEN-001] First","sequence_id":1,"state":"st-done","priority":"none","estimate_point":3,"start_date":"2026-01-01","target_date":"2026-01-05"},
 {"id":"u2","name":"No ticket tag","sequence_id":2,"state":"","priority":""},
 {"id":"u3","name":"[DEN-003] Third","sequence_id":3,"state":"st-todo","priority":"high"}]}`

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
	catalog.RegisterDefaultTools(mcpServer, Dependencies{
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
		t.Errorf("expected DEN-011 selected mcptest.On override")
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
	// 4a. Verify rejection of empty description / context
	emptyReq, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "dendrite_plane_issue_create",
			"arguments": map[string]interface{}{
				"name":        "[DEN-100] Empty Context Ticket",
				"description": "",
			},
		},
	})
	emptyResp := mcpServer.HandleRequest(context.Background(), emptyReq)
	if emptyResp == nil {
		t.Fatalf("expected response")
	}
	resVal, ok := emptyResp.Result.(CallToolResult)
	if !ok {
		resPtr, okPtr := emptyResp.Result.(*CallToolResult)
		if !okPtr || !resPtr.IsError {
			t.Errorf("expected IsError=true when creating ticket with empty description")
		}
	} else if !resVal.IsError {
		t.Errorf("expected IsError=true when creating ticket with empty description")
	}

	createRes := callTool("dendrite_plane_issue_create", map[string]interface{}{
		"name":        "[DEN-100] New Feature",
		"description": "Detailed acceptance criteria, DoD and implementation context",
		"milestone":   "Milestone 1",
		"priority":    "high",
		"estimate":    8,
		"blocked_by":  []interface{}{"DEN-010"},
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
	catalog.RegisterDefaultTools(mcpServer, Dependencies{
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

func TestSmartSelectFromAPlaneBoardWithIncompleteIssues(t *testing.T) {
	sp := newScriptedPlane(t,
		mcptest.On("GET", "/states/", 200, `{"results":[{"id":"st-done","name":"Done","group":"completed"},{"id":"st-todo","name":"To Do","group":"unstarted"}]}`),
		mcptest.On("GET", "/issues/u2/issue-relation/", 200, `{"blocked_by":[{"id":"outside","name":"[DEN-050] external"},{"id":"u2","name":"self"}]}`),
		mcptest.On("GET", "/issues/u3/issue-relation/", 200, `{"blocked_by":[{"id":"u1","name":"[DEN-001] First"},{"id":"nobody","name":"no tag"}]}`),
		mcptest.On("GET", "/issue-relation/", 200, `{}`),
		mcptest.On("GET", "/issues/", 200, smartIssues),
	)
	srv := mcptest.ToolsWith(t, sp, "DEV_STATUS.md")
	text, isErr := mcptest.CallTool(t, srv, "dendrite_ticket_smart_select", `{}`)
	if isErr || !strings.Contains(text, `"plane_online": true`) {
		t.Fatalf("text = %s, isErr = %v", text, isErr)
	}
}

func TestSmartSelectFromTheLocalLedger(t *testing.T) {
	mcptest.NoEnvPlane(t)
	dir := t.TempDir()
	ledger := filepath.Join(dir, "DEV_STATUS.md")
	content := `- **Active Milestone**: Milestone 1: Core
- **Current Focus**: DEN-001

| Rank | Ticket | Milestone | Feature | Points | Score |
|:--:|:--|:--|:--|:--:|:--:|
| **1** | **DEN-001** | Milestone 1 | FEAT-001 | 3 SP | 9.5 / 10 |
| **2** | **DEN-002** | Milestone 1 | FEAT-002 | 8 SP | 7.5 / 10 |
| **3** | **DEN-003** | Milestone 2 | FEAT-003 | 5 SP | 2.0 / 10 |

### Milestone 1: Core
- [ ] **DEN-001**: First
- [ ] **DEN-002**: Second
### Milestone 2: Later
- [ ] **DEN-003**: Third
- [ ] **DEN-004**: Fourth
`
	if err := os.WriteFile(ledger, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := mcptest.ToolsWith(t, nil, ledger)

	if text, isErr := mcptest.CallTool(t, srv, "dendrite_ticket_smart_select", `{}`); isErr || !strings.Contains(text, `"plane_online": false`) {
		t.Fatalf("whole ledger: %s", text)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_ticket_smart_select", `{"milestone":"Milestone 2"}`); isErr || strings.Contains(text, "DEN-001") {
		t.Fatalf("the milestone filter must leave out Milestone 1: %s", text)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_ticket_smart_select", `{"ticket_id":"DEN-999"}`); !isErr || !strings.Contains(text, "smart selection failed") {
		t.Fatalf("unknown ticket: %s (error %v)", text, isErr)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_ticket_smart_select", `{"file":"`+filepath.Join(dir, "missing.md")+`"}`); !isErr || !strings.Contains(text, "Plane is not available") {
		t.Fatalf("missing ledger: %s (error %v)", text, isErr)
	}
}

func TestRelationLinkAndIssueCreateRejectBadInput(t *testing.T) {
	mcptest.NoEnvPlane(t)
	srv := mcptest.ToolsWith(t, nil, "DEV_STATUS.md")
	mcptest.RunToolFailures(t, srv, []mcptest.ToolFailure{
		{Name: "relation link without a type", Tool: "dendrite_plane_relation_link", Args: `{"issue_id":"DEN-1"}`, Env: nil, Want: "all required"},
		{Name: "relation link without a client", Tool: "dendrite_plane_relation_link", Args: `{"issue_id":"DEN-1","relation_type":"blocked_by","target_issue_id":"DEN-2"}`, Env: nil, Want: "plane client error"},
		{Name: "create without a name", Tool: "dendrite_plane_issue_create", Args: `{"name":"  "}`, Env: nil, Want: "missing required 'name'"},
		{Name: "create without a description", Tool: "dendrite_plane_issue_create", Args: `{"name":"[DEN-9] x","description":" "}`, Env: nil, Want: "cannot be empty"},
		{Name: "create without a client", Tool: "dendrite_plane_issue_create", Args: `{"name":"[DEN-9] x","context":"real context"}`, Env: nil, Want: "plane client error"},
	})
}

func TestRelationLinkReportsEachWayItCanFail(t *testing.T) {
	uuid := "11111111-2222-3333-4444-555555555555"
	args := func(a, b string) string {
		return `{"issue_id":"` + a + `","relation_type":"blocked_by","target_issue_id":"` + b + `"}`
	}
	t.Run("the issue list cannot be read", func(t *testing.T) {
		sp := newScriptedPlane(t, mcptest.On("GET", "/issues/", 500, ""))
		if text, isErr := mcptest.CallTool(t, mcptest.ToolsWith(t, sp, ""), "dendrite_plane_relation_link", args("DEN-1", uuid)); !isErr {
			t.Fatalf("text = %s", text)
		}
	})
	t.Run("the target ticket cannot be found", func(t *testing.T) {
		sp := newScriptedPlane(t, mcptest.On("GET", "/issues/", 200, `{"results":[]}`))
		if text, isErr := mcptest.CallTool(t, mcptest.ToolsWith(t, sp, ""), "dendrite_plane_relation_link", args(uuid, "DEN-404")); !isErr || !strings.Contains(text, "not found in Plane") {
			t.Fatalf("text = %s (error %v)", text, isErr)
		}
	})
	t.Run("the relation is refused", func(t *testing.T) {
		sp := newScriptedPlane(t, mcptest.On("POST", "/issue-relation/", 500, ""))
		if text, isErr := mcptest.CallTool(t, mcptest.ToolsWith(t, sp, ""), "dendrite_plane_relation_link", args(uuid, "99999999-2222-3333-4444-555555555555")); !isErr || !strings.Contains(text, "failed to create relation") {
			t.Fatalf("text = %s (error %v)", text, isErr)
		}
	})
}

func TestIssueCreatePlacesTheTicketInAModuleAndLinksItsBlockers(t *testing.T) {
	uuid := "11111111-2222-3333-4444-555555555555"
	sp := newScriptedPlane(t,
		mcptest.On("GET", "/modules/", 200, `{"results":[{"id":"m1","name":"Milestone 1: Core"}]}`),
		mcptest.On("POST", "/modules/", 201, `{"id":"m-new","name":"Brand new"}`),
		mcptest.On("GET", "/issues/", 200, `{"results":[{"id":"u9","name":"[DEN-9] Blocker"}]}`),
		mcptest.On("POST", "/issues/", 201, `{"id":"created","name":"[DEN-100] x","sequence_id":100}`),
	)
	srv := mcptest.ToolsWith(t, sp, "")

	text, isErr := mcptest.CallTool(t, srv, "dendrite_plane_issue_create",
		`{"name":"[DEN-100] x","description":"details","milestone":"Brand new","blocked_by":["DEN-9","`+uuid+`","",7,"unknown"]}`)
	if isErr || !strings.Contains(text, `"module_id": "m-new"`) || !strings.Contains(text, `"blockers_count": 2`) || !strings.Contains(text, `"priority": "medium"`) {
		t.Fatalf("text = %s (error %v)", text, isErr)
	}
	if text, isErr = mcptest.CallTool(t, srv, "dendrite_plane_issue_create", `{"name":"[DEN-101] y","description":"details","milestone":"core","estimate":8,"start_date":"2026-01-01"}`); isErr || !strings.Contains(text, `"module_id": "m1"`) {
		t.Fatalf("existing module: %s", text)
	}

	failing := newScriptedPlane(t, mcptest.On("POST", "/issues/", 500, "no"))
	if text, isErr = mcptest.CallTool(t, mcptest.ToolsWith(t, failing, ""), "dendrite_plane_issue_create", `{"name":"[DEN-102] z","description":"details"}`); !isErr || !strings.Contains(text, "failed to create issue") {
		t.Fatalf("a refused create: %s (error %v)", text, isErr)
	}
}

func TestSmartSelectReportsALedgerItCannotScan(t *testing.T) {
	mcptest.NoEnvPlane(t)
	ledger := filepath.Join(t.TempDir(), "DEV_STATUS.md")
	if err := os.WriteFile(ledger, []byte(strings.Repeat("x", 70*1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := mcptest.CallTool(t, mcptest.ToolsWith(t, nil, ledger), "dendrite_ticket_smart_select", `{}`); !isErr || !strings.Contains(text, "failed to parse dev status") {
		t.Fatalf("text = %s (error %v)", text, isErr)
	}
}

func TestPlaneTicketSetResolvesRefsAndPatchesState(t *testing.T) {
	var patched, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/"):
			_, _ = w.Write([]byte(`{"results":[
				{"id":"iss-fresh","name":"Fresh ticket","sequence_id":324,"state":"st-todo"},
				{"id":"iss-old","name":"[DEN-1014] legacy","sequence_id":300,"state":"st-todo"}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/states/"):
			_, _ = w.Write([]byte(`{"results":[{"id":"st-todo","name":"To Do","group":"unstarted"},{"id":"st-prog","name":"In Progress","group":"started"}]}`))
		case r.Method == http.MethodPatch:
			b, _ := io.ReadAll(r.Body)
			patched, body = r.URL.Path, string(b)
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	client, err := plane.NewClient(plane.Config{BaseURL: srv.URL, WorkspaceSlug: "ws", ProjectID: "proj", DirectToken: "k"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(ServerConfig{})
	planetools.RegisterPlaneTicketSetTool(s, Dependencies{PlaneClient: client})
	h, ok := s.Handler("dendrite_plane_ticket_set")
	if !ok {
		t.Fatal("tool not registered")
	}

	if _, err := h(context.Background(), map[string]any{"ticket": "DEN-324", "status": "in progress"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(patched, "/issues/iss-fresh/") || !strings.Contains(body, "st-prog") {
		t.Errorf("patched %q with %q", patched, body)
	}

	_, err = h(context.Background(), map[string]any{"ticket": "DEN-999", "status": "done"})
	if err == nil || !strings.Contains(err.Error(), "DEN-999") {
		t.Errorf("unknown ticket error = %v", err)
	}
	if _, err = h(context.Background(), map[string]any{"ticket": "DEN-324", "status": "bogus"}); err == nil {
		t.Error("bogus status accepted")
	}
}

func TestCoreProfileIncludesPlaneTicketSet(t *testing.T) {
	if !ProfileCore.InProfile("dendrite_plane_ticket_set") {
		t.Fatal("core must expose dendrite_plane_ticket_set")
	}
}

func TestLifecycleToolsAreInTheCoreProfile(t *testing.T) {
	src := mcptest.NewDefaultServerForProfileTest()
	core := ApplyProfile(src, ProfileCore)
	listed := map[string]bool{}
	for _, tool := range core.ListTools() {
		listed[tool.Name] = true
	}
	for _, name := range append(lifecycleTools, "dendrite_ticket_smart_select") {
		if !listed[name] {
			t.Errorf("core profile lacks %s", name)
		}
		if _, ok := core.Handler(name); !ok {
			t.Errorf("core tool %s lost its handler", name)
		}
	}
}

func TestLifecycleToolsRunATicketFromClaimToHandoff(t *testing.T) {
	r := newLifecycleRig(t, map[string]string{"DEN-377": "To Do"})
	r.board.SetDescription("DEN-377", "<p>edit pkg/ticketlock/lifecycle.go</p><ul><li>[AC-1] tools listed</li></ul>")

	claim, err := r.call("dendrite_ticket_claim", map[string]any{"ticket": "DEN-377", "agent": "claude", "model_tier": "9"})
	if err != nil {
		t.Fatal(err)
	}
	if claim["ticket"] != "DEN-377" || claim["state"] != "In Progress" || claim["branch"] != "agent/den-377" {
		t.Errorf("claim = %v", claim)
	}
	if got := r.board.LabelNames("DEN-377"); strings.Join(got, ",") != "agent:claude,tier:9" {
		t.Errorf("labels = %v", got)
	}

	if _, err := r.call("dendrite_ticket_comment", map[string]any{"ticket": "DEN-377", "kind": "progress", "body": "lock done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.call("dendrite_ticket_comment", map[string]any{"ticket": "DEN-377", "kind": "shout", "body": "x"}); err == nil {
		t.Error("an unknown comment kind is refused")
	}

	plan, err := r.call("dendrite_ticket_upload_plan", map[string]any{"ticket": "DEN-377", "markdown": "- [x] lock\n- [ ] hooks"})
	if err != nil {
		t.Fatal(err)
	}
	if plan["version"] != float64(1) || plan["pending_steps"] != float64(1) || plan["next_step"] != "hooks" {
		t.Errorf("upload = %v", plan)
	}
	got, err := r.call("dendrite_ticket_get_plan", map[string]any{"ticket": "DEN-377"})
	if err != nil || got["markdown"] != "- [x] lock\n- [ ] hooks" || got["source"] != "plane" {
		t.Errorf("get_plan = %v, err = %v", got, err)
	}

	ctx, err := r.call("dendrite_query_context", map[string]any{"ticket": "DEN-377"})
	if err != nil {
		t.Fatal(err)
	}
	if acs, _ := ctx["acceptance_criteria"].([]any); len(acs) != 1 || acs[0] != "[AC-1] tools listed" {
		t.Errorf("context = %v", ctx)
	}

	// AC-2: blocked without the three sections is refused and the ticket stays put.
	if _, err := r.call("dendrite_ticket_transition", map[string]any{"ticket": "DEN-377", "outcome": "blocked"}); err == nil || !strings.Contains(err.Error(), "needs a handoff") {
		t.Errorf("blocked without a handoff: %v", err)
	}
	if _, err := r.call("dendrite_ticket_transition", map[string]any{"ticket": "DEN-377", "outcome": "blocked", "handoff": map[string]any{"completed": "a", "challenges": "b"}}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("blocked with two sections: %v", err)
	}
	if r.board.State("DEN-377") != "In Progress" {
		t.Errorf("state = %s", r.board.State("DEN-377"))
	}
	res, err := r.call("dendrite_ticket_transition", map[string]any{"ticket": "DEN-377", "outcome": "blocked",
		"handoff": map[string]any{"completed": "lock", "challenges": "none", "missing": "hooks"}})
	if err != nil || res["state"] != "Blocked" {
		t.Errorf("blocked with a handoff: %v, %v", res, err)
	}
	if _, err := r.call("dendrite_ticket_transition", map[string]any{"ticket": "DEN-377", "outcome": "done"}); err == nil || !strings.Contains(err.Error(), "chronos finish") {
		t.Errorf("done must point to finish: %v", err)
	}
}

func TestLifecycleToolsValidateTheirArguments(t *testing.T) {
	r := newLifecycleRig(t, map[string]string{"DEN-377": "To Do"})
	for _, name := range lifecycleTools {
		if _, err := r.call(name, map[string]any{}); err == nil || !strings.Contains(err.Error(), "ticket is required") {
			t.Errorf("%s without a ticket: %v", name, err)
		}
	}
	if _, err := r.call("dendrite_ticket_claim", map[string]any{"ticket": "DEN-999"}); err == nil {
		t.Error("an unknown ticket is an error")
	}
	if _, err := r.call("dendrite_ticket_get_plan", map[string]any{"ticket": "DEN-377"}); err == nil || !strings.Contains(err.Error(), "no plan") {
		t.Errorf("a ticket without a plan: %v", err)
	}
	if _, err := r.call("dendrite_ticket_claim", map[string]any{"ticket": "DEN-377", "steal": true}); err != nil {
		t.Errorf("steal=true mcptest.On a free ticket: %v", err)
	}
}

func TestLifecycleToolsUseTheServersDirectoryByDefault(t *testing.T) {
	r := newLifecycleRig(t, map[string]string{"DEN-377": "To Do"})
	h, _ := r.s.Handler("dendrite_ticket_get_plan")
	if _, err := h(context.Background(), map[string]any{"ticket": "DEN-377", "worktree": "  "}); err == nil || !strings.Contains(err.Error(), "no plan") {
		t.Errorf("err = %v: a blank worktree falls back to the current directory", err)
	}
}

func TestLifecycleToolsNeedAPlaneClient(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("PLANE_BASE_URL", "")
	s := NewServer(ServerConfig{})
	planetools.RegisterTicketLifecycleTools(s, Dependencies{})
	h, _ := s.Handler("dendrite_ticket_get_plan")
	if _, err := h(context.Background(), map[string]any{"ticket": "DEN-1"}); err == nil || !strings.Contains(err.Error(), "plane client error") {
		t.Errorf("err = %v", err)
	}
}

func TestFinishToolBlocksATicketWhoseGateCannotRun(t *testing.T) {
	r := newLifecycleRig(t, map[string]string{"DEN-377": "To Do"})
	if _, err := r.call("dendrite_ticket_claim", map[string]any{"ticket": "DEN-377"}); err != nil {
		t.Fatal(err)
	}
	out, err := r.call("dendrite_ticket_finish", map[string]any{"ticket": "DEN-377"})
	if err != nil {
		t.Fatal(err)
	}
	if out["state"] != "Blocked" || r.board.State("DEN-377") != "Blocked" {
		t.Errorf("a working copy with no toolchain has no passing gate: %v, Plane %s", out, r.board.State("DEN-377"))
	}
	if got := out["gate"].(map[string]any)["report"]; !strings.Contains(got.(string), "could not run") {
		t.Errorf("gate = %v", out["gate"])
	}
}

func TestDecisionAndCommentsTools(t *testing.T) {
	r := newLifecycleRig(t, map[string]string{"DEN-377": "To Do"})
	if _, err := r.call("dendrite_ticket_claim", map[string]any{"ticket": "DEN-377"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.call("dendrite_ticket_decision", map[string]any{"ticket": "DEN-377", "question": "Which?", "answer": "A", "note": "n", "options": []any{"A", "B", 7}}); err != nil {
		t.Fatal(err)
	}
	out, err := r.call("dendrite_ticket_comments", map[string]any{"ticket": "DEN-377", "kind": "decision", "last": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	rows := out["comments"].([]any)
	text := rows[0].(map[string]any)["text"].(string)
	if len(rows) != 1 || !strings.Contains(text, "Options: A | B") || !strings.Contains(text, "Note: n") {
		t.Errorf("comments = %v", out)
	}
	if _, err := r.call("dendrite_ticket_decision", map[string]any{"ticket": "DEN-999", "question": "q", "answer": "a"}); err == nil {
		t.Error("an unknown ticket is an error")
	}
}

func TestTools_PlaneDAG(t *testing.T) {
	_, planeClient := createTestPlaneServer(t)

	tmpDir := t.TempDir()
	devStatusFile := filepath.Join(tmpDir, "DEV_STATUS.md")
	mdContent := "# Development Status\n\n- **Active Milestone**: Milestone 1\n\n### Milestone 1: Core Foundation\n\n- [ ] **DEN-101**: Setup (`pkg/mcpserver`) [State: To Do, Priority: High]\n- [ ] **DEN-102**: Feature (`pkg/mcpserver`) [State: To Do, Priority: Medium]\n"
	_ = os.WriteFile(devStatusFile, []byte(mdContent), 0644)

	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{
		DevStatus:   devStatusFile,
		PlaneClient: planeClient,
	})

	// 1. dendrite_ticket_smart_select (with planeClient)
	selReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_ticket_smart_select", "arguments": {"ticket_id": "DEN-101"}}}`
	selResp1 := srv.HandleRequest(context.Background(), []byte(selReq1))
	if selResp1 == nil || selResp1.Error != nil {
		t.Fatalf("smart select with plane failed: %v", selResp1)
	}

	// 2. dendrite_plane_ticket_smart_select (alias, with milestone filter)
	selReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_plane_ticket_smart_select", "arguments": {"milestone": "Milestone 1"}}}`
	selResp2 := srv.HandleRequest(context.Background(), []byte(selReq2))
	if selResp2 == nil || selResp2.Error != nil {
		t.Fatalf("plane smart select failed: %v", selResp2)
	}

	// 3. Fallback to markdown file when planeClient is nil (Plane pinned to an unreachable address)
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:1")
	srvFallback := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srvFallback, Dependencies{DevStatus: devStatusFile})
	selReq3 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_ticket_smart_select", "arguments": {"file": %q}}}`, devStatusFile)
	selResp3 := srvFallback.HandleRequest(context.Background(), []byte(selReq3))
	if selResp3 == nil || selResp3.Error != nil {
		t.Fatalf("smart select fallback to markdown failed: %v", selResp3)
	}

	// 4. Missing both Plane and file (error)
	missingFile := filepath.Join(tmpDir, "NON_EXISTENT.md")
	selReq4 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_ticket_smart_select", "arguments": {"file": %q}}}`, missingFile)
	selResp4 := srvFallback.HandleRequest(context.Background(), []byte(selReq4))
	isErr := false
	switch res := selResp4.Result.(type) {
	case CallToolResult:
		isErr = res.IsError
	case *CallToolResult:
		isErr = res != nil && res.IsError
	}
	if !isErr {
		t.Fatalf("expected error when file missing and plane offline, got: %#v", selResp4)
	}

	// 5. dendrite_plane_relation_link: missing arguments
	relReq1 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_plane_relation_link", "arguments": {"issue_id": "DEN-101"}}}`
	relResp1 := srv.HandleRequest(context.Background(), []byte(relReq1))
	if res, ok := relResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing relation_type, got: %#v", relResp1)
	}

	// 6. dendrite_plane_relation_link: valid
	relReq2 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_plane_relation_link", "arguments": {"issue_id": "DEN-101", "relation_type": "blocking", "target_issue_id": "DEN-101"}}}`
	relResp2 := srv.HandleRequest(context.Background(), []byte(relReq2))
	if relResp2 == nil || relResp2.Error != nil {
		t.Fatalf("plane relation link failed: %v", relResp2)
	}

	// 7. dendrite_plane_issue_create: missing name error
	issReq1 := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_plane_issue_create", "arguments": {"description": "desc"}}}`
	issResp1 := srv.HandleRequest(context.Background(), []byte(issReq1))
	if res, ok := issResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing name in issue_create, got: %#v", issResp1)
	}

	// 8. dendrite_plane_issue_create: empty context error
	issReq2 := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_plane_issue_create", "arguments": {"name": "DEN-999", "description": ""}}}`
	issResp2 := srv.HandleRequest(context.Background(), []byte(issReq2))
	if res, ok := issResp2.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for empty description, got: %#v", issResp2)
	}

	// 9. dendrite_plane_issue_create: valid
	issReq3 := `{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": {"name": "dendrite_plane_issue_create", "arguments": {"name": "[DEN-103] Real Issue", "description": "Comprehensive feature specification with detailed acceptance criteria and DoD.", "milestone": "M1", "priority": "high", "estimate": 5, "blocked_by": ["DEN-101"]}}}`
	issResp3 := srv.HandleRequest(context.Background(), []byte(issReq3))
	if issResp3 == nil || issResp3.Error != nil {
		t.Fatalf("plane issue create failed: %v", issResp3)
	}
}

func TestPlaneTicketSetNeedsArgumentsAndAClient(t *testing.T) {
	mcptest.NoEnvPlane(t)
	srv := mcptest.ToolsWith(t, nil, "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_plane_ticket_set", `{"ticket":" ","status":"done"}`); !isErr || !strings.Contains(text, "are required") {
		t.Errorf("blank ticket: %q (error %v)", text, isErr)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_plane_ticket_set", `{"ticket":"DEN-1","status":"done"}`); !isErr || !strings.Contains(text, "plane client error") {
		t.Errorf("no client: %q (error %v)", text, isErr)
	}
}

func TestTools_DendriteEnvStatus(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

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
	catalog.RegisterDefaultTools(srv, Dependencies{
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
	catalog.RegisterDefaultTools(srvNoPlane, Dependencies{DevStatus: missingFile})
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
	catalog.RegisterDefaultTools(srv, Dependencies{
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
	catalog.RegisterDefaultTools(srvNoPlane, Dependencies{DevStatus: missingFile})
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
	catalog.RegisterDefaultTools(srv, Dependencies{PlaneClient: planeClient})

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
	catalog.RegisterDefaultTools(srvBad, Dependencies{PlaneClient: badClient})
	respBad := srvBad.HandleRequest(context.Background(), []byte(req))
	if res, ok := respBad.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error from dendrite_plane_issues with 500 server, got: %#v", respBad)
	}
}

func TestTools_DendriteOllamaProbe(t *testing.T) {
	ollamaSrv := createTestOllamaServer(t)
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{OllamaURL: ollamaSrv.URL})

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
	catalog.RegisterDefaultTools(srv, Dependencies{})

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
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// Valid scenarios
	for _, sc := range []string{"local-hybrid", "cloud-burst", "full-local"} {
		mcptest.AssertToolCallSuccess(t, srv, "dendrite_gateway_render", fmt.Sprintf(`{"scenario": %q}`, sc))
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
	catalog.RegisterDefaultTools(srv, Dependencies{})

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

func TestTicketUpdateFallsBackToPlane(t *testing.T) {
	sp := newScriptedPlane(t,
		mcptest.On("GET", "/states/", 200, `{"results":[{"id":"st-todo","name":"To Do","group":"unstarted"},{"id":"st-prog","name":"In Progress","group":"started"},{"id":"st-done","name":"Done","group":"completed"}]}`),
		mcptest.On("GET", "/issues/", 200, `{"results":[{"id":"11111111-2222-3333-4444-555555555555","name":"[DEN-5] work","sequence_id":5,"state":"st-todo"}]}`),
		mcptest.On("PATCH", "/issues/", 200, `{}`),
	)
	srv := mcptest.ToolsWith(t, sp, filepath.Join(t.TempDir(), "missing.md"))
	text, isErr := mcptest.CallTool(t, srv, "dendrite_ticket_update", `{"ticket_id":"DEN-5","status":"in_progress"}`)
	if isErr || !strings.Contains(text, "in Plane service") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestStatusToolsWithoutPlaneOrWithOddArguments(t *testing.T) {
	mcptest.NoEnvPlane(t)
	srv := mcptest.ToolsWith(t, nil, "")
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_plane_issues", `{}`); !isErr || !strings.Contains(text, "plane client error") {
		t.Errorf("plane issues: %q (error %v)", text, isErr)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_topology_advisor", `{"nodes":" , ,"}`); isErr {
		t.Errorf("a list of blank nodes falls back to the local one: %s", text)
	}
}

func TestGatewayUpdateReportsAConfigItCannotWrite(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := mcptest.ToolsWith(t, nil, "")
	args := `{"file":"` + filepath.Join(blocker, "sub", "litellm.yaml") + `","dry_run":false,"use_container_host":false}`
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_gateway_update", args); !isErr || !strings.Contains(text, "gateway update failed") {
		t.Fatalf("text = %q (error %v)", text, isErr)
	}
}

func TestTools_DendritePlaneReplicateAndPush(t *testing.T) {
	_, planeClient := createTestPlaneServer(t)

	tmpDir := t.TempDir()
	devStatusFile := filepath.Join(tmpDir, "DEV_STATUS.md")

	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{PlaneClient: planeClient})

	// 1. dendrite_plane_replicate (dry_run: true)
	repReq1 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_plane_replicate", "arguments": {"file": "%s", "dry_run": true}}}`, devStatusFile)
	resp1 := srv.HandleRequest(context.Background(), []byte(repReq1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("plane replicate dry run failed: %v", resp1)
	}

	// 2. dendrite_plane_replicate (dry_run: false)
	repReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_plane_replicate", "arguments": {"file": "%s", "dry_run": false}}}`, devStatusFile)
	resp2 := srv.HandleRequest(context.Background(), []byte(repReq2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("plane replicate real run failed: %v", resp2)
	}

	// 3. dendrite_plane_push_status with missing file
	missingFile := filepath.Join(tmpDir, "NON_EXISTENT.md")
	pushReq1 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_plane_push_status", "arguments": {"file": "%s"}}}`, missingFile)
	respPush1 := srv.HandleRequest(context.Background(), []byte(pushReq1))
	if res, ok := respPush1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error pushing non-existent file, got: %#v", respPush1)
	}

	// 4. dendrite_plane_push_status with existing file
	pushReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_plane_push_status", "arguments": {"file": "%s"}}}`, devStatusFile)
	respPush2 := srv.HandleRequest(context.Background(), []byte(pushReq2))
	if respPush2 == nil || respPush2.Error != nil {
		t.Fatalf("plane push status failed: %v", respPush2)
	}

	// 5. dendrite_plane_doc_sync with different directions
	docsDir := filepath.Join(tmpDir, "docs")
	_ = os.MkdirAll(docsDir, 0755)
	_ = os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide\nHello world\n"), 0644)

	directions := []string{"to-plane", "from-plane", "bidirectional", "default"}
	for i, d := range directions {
		syncReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": %d, "method": "tools/call", "params": {"name": "dendrite_plane_doc_sync", "arguments": {"dir": "%s", "direction": "%s"}}}`, 10+i, docsDir, d)
		respSync := srv.HandleRequest(context.Background(), []byte(syncReq))
		if respSync == nil || respSync.Error != nil {
			t.Fatalf("doc sync direction %s failed: %v", d, respSync)
		}
	}
}

func TestTools_DendriteJetbrainsSetup(t *testing.T) {
	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	tmpDir := t.TempDir()

	// project_only = true
	req1 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_jetbrains_setup", "arguments": {"project_only": true, "project_dir": %q}}}`, tmpDir)
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("jetbrains project_only setup failed: %v", resp1)
	}

	// global_only = true
	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_jetbrains_setup", "arguments": {"global_only": true, "project_dir": %q}}}`, tmpDir)
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("jetbrains global_only setup failed: %v", resp2)
	}
}

func TestTools_DendriteCommit(t *testing.T) {
	repoDir := createTestGitRepo(t)
	t.Chdir(repoDir)

	srv := NewServer(ServerConfig{})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	// Modify a file so there is a diff
	_ = os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test Repo Modified\n"), 0644)

	// 1. Dry run
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_commit", "arguments": {"ticket_id": "DEN-098", "message": "feat: update readme", "files": ["README.md"], "dry_run": true, "stage_all": false, "use_local_model": false}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("commit dry run failed: %v", resp1)
	}

	// 2. Real commit
	req2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_commit", "arguments": {"ticket_id": "DEN-098", "message": "feat: update readme real", "stage_all": true, "dry_run": false, "use_local_model": false}}}`
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("real commit failed: %v", resp2)
	}
}

func TestPlaneSyncToolsReportEachFailure(t *testing.T) {
	dir := t.TempDir()
	ledger := filepath.Join(dir, "DEV_STATUS.md")
	if err := os.WriteFile(ledger, []byte("### Milestone 1: A\n- [ ] **DEN-1**: one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	healthy := healthyPlaneRules()
	with := func(first mcptest.ApiRule) []mcptest.ApiRule { return append([]mcptest.ApiRule{first}, healthy...) }

	cases := []struct {
		name  string
		rules []mcptest.ApiRule
		tool  string
		args  string
		want  string
	}{
		{"replicate: Plane cannot be read", with(mcptest.On("GET", "/issues/", 500, "")), "dendrite_plane_replicate", `{"dry_run":true}`, "replication failed"},
		{"replicate: the file cannot be written", healthy, "dendrite_plane_replicate", `{"file":"` + filepath.Join(dir, "no", "such", "dir", "out.md") + `"}`, "failed to write"},
		{"push: the ledger is missing", healthy, "dendrite_plane_push_status", `{"file":"` + filepath.Join(dir, "missing.md") + `"}`, "failed to read"},
		{"push: Plane refuses the states", with(mcptest.On("GET", "/states/", 500, "")), "dendrite_plane_push_status", `{"file":"` + ledger + `"}`, "push to plane failed"},
		{"docs: Plane cannot list pages", with(mcptest.On("GET", "/pages/", 500, "")), "dendrite_plane_doc_sync", `{"direction":"pull","dir":"` + dir + `"}`, "documentation sync failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := mcptest.ToolsWith(t, newScriptedPlane(t, c.rules...), "")
			if text, isErr := mcptest.CallTool(t, srv, c.tool, c.args); !isErr || !strings.Contains(text, c.want) {
				t.Fatalf("text = %q (error %v), want %q", text, isErr, c.want)
			}
		})
	}
}

func TestPlaneSyncToolsNeedAPlaneClient(t *testing.T) {
	mcptest.NoEnvPlane(t)
	srv := mcptest.ToolsWith(t, nil, "")
	for _, tool := range []string{"dendrite_plane_replicate", "dendrite_plane_push_status", "dendrite_plane_doc_sync"} {
		if text, isErr := mcptest.CallTool(t, srv, tool, `{}`); !isErr || !strings.Contains(text, "plane client error") {
			t.Errorf("%s: %q (error %v)", tool, text, isErr)
		}
	}
}

func TestPlaneDocSyncAcceptsEveryDirection(t *testing.T) {
	dir := t.TempDir()
	srv := mcptest.ToolsWith(t, newScriptedPlane(t, healthyPlaneRules()...), "")
	for _, direction := range []string{"to-plane", "push", "from-plane", "pull", "anything else", ""} {
		if text, isErr := mcptest.CallTool(t, srv, "dendrite_plane_doc_sync", `{"direction":"`+direction+`","dir":"`+dir+`"}`); isErr {
			t.Errorf("%q: %s", direction, text)
		}
	}
}

func TestJetBrainsAndCommitToolsReportFailures(t *testing.T) {
	srv := mcptest.ToolsWith(t, nil, "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_jetbrains_setup", `{"project_dir":"`+file+`","project_only":true}`); !isErr || !strings.Contains(text, "jetbrains setup failed") {
		t.Errorf("jetbrains: %q (error %v)", text, isErr)
	}

	t.Chdir(t.TempDir()) // not a git repository
	if text, isErr := mcptest.CallTool(t, srv, "dendrite_commit", `{"message":"m","files":["a.go",7],"stage_all":false,"use_local_model":false,"dry_run":true}`); !isErr || !strings.Contains(text, "commit failed") {
		t.Errorf("commit: %q (error %v)", text, isErr)
	}
}

func TestDendriteGatewayUpdate_Tool(t *testing.T) {
	srv := NewServer(ServerConfig{
		Name:    "Test MCP Server",
		Version: "1.0.0",
	})
	catalog.RegisterDefaultTools(srv, Dependencies{})

	callReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_gateway_update", "arguments": {"dry_run": true, "file": "/tmp/litellm_test_mcp.yaml", "use_container_host": false}}}`
	resp := srv.HandleRequest(context.Background(), []byte(callReq))
	if resp == nil || resp.Error != nil {
		t.Fatalf("unexpected error calling dendrite_gateway_update: %v", resp)
	}

	callRes, ok := resp.Result.(*CallToolResult)
	if !ok || len(callRes.Content) == 0 {
		t.Fatalf("expected non-empty CallToolResult, got: %T", resp.Result)
	}

	var report map[string]interface{}
	if err := json.Unmarshal([]byte(callRes.Content[0].Text), &report); err != nil {
		t.Fatalf("failed to decode report json: %v", err)
	}

	if dryRun, ok := report["dry_run"].(bool); !ok || !dryRun {
		t.Errorf("expected dry_run to be true in report, got %v", report["dry_run"])
	}

	if totalModels, ok := report["total_models_configured"].(float64); !ok || totalModels == 0 {
		t.Errorf("expected positive total_models_configured, got %v", report["total_models_configured"])
	}
}
