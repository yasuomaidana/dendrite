package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPlaneCommand_Create_WithDescFile(t *testing.T) {
	var mu sync.Mutex
	var receivedCreateBody map[string]interface{}
	createCallCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()

		// 1. Module listing / creation
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/modules/") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "mod-m1", "name": "M1"},
			})
			return
		}

		// 2. Issues listing (for blocker resolution)
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": "uuid-blocker-den-1", "name": "[DEN-001] Blocker Issue", "sequence_id": 1},
				},
				"next_page_results": false,
			})
			return
		}

		// 3. Issue creation POST
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/") && !strings.Contains(r.URL.Path, "/modules/") {
			createCallCount++
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &receivedCreateBody)

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          "new-issue-uuid",
				"sequence_id": 101,
				"name":        receivedCreateBody["name"],
			})
			return
		}

		// 4. Module issue linking
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/modules/mod-m1/issues/") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}

		// 5. Issue relation linking
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/issue-relation/") {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[]`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("PLANE_BASE_URL", server.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")

	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "spec.md")
	specContent := "## Architecture Header\n\nContext testing with <tag> & symbols.\n\n- Bullet 1\n- Bullet 2\n\n`inline_code`\n\n| Col A | Col B |\n|---|---|\n| Val 1 | Val 2 |\n"
	if err := os.WriteFile(specFile, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write spec.md: %v", err)
	}

	ctx := context.Background()

	// Execute create with --desc-file, --milestone, --priority, --estimate, --blocked-by
	args := []string{
		"--name", "[DEN-101] Implement Feature",
		"--desc-file", specFile,
		"--milestone", "M1",
		"--priority", "high",
		"--estimate", "5",
		"--blocked-by", "DEN-001,b23b10b0-0000-0000-0000-000000000000",
	}
	handlePlaneCommand(ctx, "create", args)

	mu.Lock()
	defer mu.Unlock()

	if createCallCount != 1 {
		t.Fatalf("expected 1 issue create POST, got %d", createCallCount)
	}

	name, _ := receivedCreateBody["name"].(string)
	if name != "[DEN-101] Implement Feature" {
		t.Errorf("unexpected issue name: %s", name)
	}

	descHTML, _ := receivedCreateBody["description_html"].(string)
	if !strings.Contains(descHTML, "<h2>Architecture Header</h2>") {
		t.Errorf("expected <h2>Architecture Header</h2> in description_html, got: %s", descHTML)
	}
	if !strings.Contains(descHTML, "&lt;tag&gt;") || !strings.Contains(descHTML, "&amp;") {
		t.Errorf("expected escaped HTML entities (&lt;tag&gt; and &amp;) in description_html, got: %s", descHTML)
	}
	if !strings.Contains(descHTML, "<li>Bullet 1</li>") {
		t.Errorf("expected <li>Bullet 1</li> in description_html, got: %s", descHTML)
	}
	if !strings.Contains(descHTML, "<code>inline_code</code>") {
		t.Errorf("expected <code>inline_code</code> in description_html, got: %s", descHTML)
	}
	if !strings.Contains(descHTML, "<table>") || !strings.Contains(descHTML, "<td>Val 1</td>") {
		t.Errorf("expected table rendering in description_html, got: %s", descHTML)
	}
}

func TestPlaneCommand_Create_EdgeCasesAndErrors(t *testing.T) {
	var mu sync.Mutex
	createCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/") && !strings.Contains(r.URL.Path, "/modules/") {
			createCalls++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`server error`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	t.Setenv("PLANE_BASE_URL", server.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")

	ctx := context.Background()

	// 1. Missing name and desc
	handlePlaneCommand(ctx, "create", []string{})

	// 2. Name provided, but empty desc
	handlePlaneCommand(ctx, "create", []string{"--name", "Ticket Without Context", "--desc", ""})

	// 3. Nonexistent desc file
	handlePlaneCommand(ctx, "create", []string{"--name", "Test", "--desc-file", "/nonexistent/path/spec.md"})

	// 4. Server error on create
	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "spec.md")
	_ = os.WriteFile(specFile, []byte("Valid ticket context"), 0644)
	handlePlaneCommand(ctx, "create", []string{"--name", "[DEN-500] Error Case", "--desc-file", specFile})

	mu.Lock()
	defer mu.Unlock()
	if createCalls != 1 {
		t.Errorf("expected 1 create call that resulted in 500, got %d", createCalls)
	}
}

func TestPlaneCommand_Update_WithDescFile(t *testing.T) {
	var mu sync.Mutex
	var patchedIssueID string
	var patchedBody map[string]interface{}
	patchCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": "uuid-den-100", "name": "[DEN-100] Old Title", "sequence_id": 100},
					{"id": "uuid-den-1000", "name": "[DEN-1000] Substring Title", "sequence_id": 1000},
				},
				"next_page_results": false,
			})
			return
		}

		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/issues/") {
			patchCalls++
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			patchedIssueID = parts[len(parts)-1]
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &patchedBody)

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          patchedIssueID,
				"name":        "[DEN-100] Old Title",
				"sequence_id": 100,
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("PLANE_BASE_URL", server.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")

	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "spec.md")
	specContent := "## Repaired Specification\n\nUpdated context containing <script> & ampersand."
	if err := os.WriteFile(specFile, []byte(specContent), 0644); err != nil {
		t.Fatalf("failed to write spec.md: %v", err)
	}

	ctx := context.Background()

	// 1. Successful update of exactly [DEN-100]
	handlePlaneCommand(ctx, "update", []string{"--ticket", "DEN-100", "--desc-file", specFile})

	mu.Lock()
	if patchCalls != 1 {
		mu.Unlock()
		t.Fatalf("expected 1 patch call, got %d", patchCalls)
	}
	if patchedIssueID != "uuid-den-100" {
		mu.Unlock()
		t.Errorf("expected patched issue to be uuid-den-100, got %s", patchedIssueID)
	}
	descHTML, _ := patchedBody["description_html"].(string)
	if !strings.Contains(descHTML, "<h2>Repaired Specification</h2>") {
		mu.Unlock()
		t.Errorf("expected <h2>Repaired Specification</h2>, got: %s", descHTML)
	}
	if !strings.Contains(descHTML, "&lt;script&gt;") || !strings.Contains(descHTML, "&amp;") {
		mu.Unlock()
		t.Errorf("expected escaped entities in description_html, got: %s", descHTML)
	}
	mu.Unlock()

	// 2. Unknown ticket ID sends no PATCH
	mu.Lock()
	patchCalls = 0
	patchedIssueID = ""
	mu.Unlock()

	handlePlaneCommand(ctx, "update", []string{"--ticket", "DEN-999", "--desc-file", specFile})

	mu.Lock()
	if patchCalls != 0 {
		mu.Unlock()
		t.Fatalf("expected 0 patch calls for unknown ticket, got %d", patchCalls)
	}
	mu.Unlock()

	// 3. Rename with an inline description
	handlePlaneCommand(ctx, "update", []string{"--ticket", "DEN-100", "--name", "[DEN-100] New Title", "--desc", "inline **context**"})

	mu.Lock()
	defer mu.Unlock()
	if patchCalls != 1 || patchedIssueID != "uuid-den-100" {
		t.Fatalf("expected one PATCH of uuid-den-100, got %d calls on %q", patchCalls, patchedIssueID)
	}
	if patchedBody["name"] != "[DEN-100] New Title" {
		t.Errorf("expected renamed ticket, got name %v", patchedBody["name"])
	}
	if descHTML, _ := patchedBody["description_html"].(string); !strings.Contains(descHTML, "<strong>context</strong>") {
		t.Errorf("expected inline Markdown rendered, got: %s", descHTML)
	}
}

func TestPlaneCommand_Update_EdgeCasesAndErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("PLANE_BASE_URL", server.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")

	ctx := context.Background()

	// Empty args
	handlePlaneCommand(ctx, "update", []string{})

	// Ticket given but no desc or name
	handlePlaneCommand(ctx, "update", []string{"--ticket", "DEN-100"})

	// Bad desc file path
	handlePlaneCommand(ctx, "update", []string{"--ticket", "DEN-100", "--desc-file", "/nonexistent/spec.md"})

	// Server error on ListPlaneIssues
	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "spec.md")
	_ = os.WriteFile(specFile, []byte("Content"), 0644)
	handlePlaneCommand(ctx, "update", []string{"--ticket", "DEN-100", "--desc-file", specFile})

	// Server error on PATCH
	patchErrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": "uuid-den-500", "name": "[DEN-500] Patch Err", "sequence_id": 500},
				},
				"next_page_results": false,
			})
			return
		}
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer patchErrServer.Close()

	t.Setenv("PLANE_BASE_URL", patchErrServer.URL)
	handlePlaneCommand(ctx, "update", []string{"--ticket", "DEN-500", "--desc-file", specFile})
}

func TestPlaneCommand_States(t *testing.T) {
	var mu sync.Mutex
	var requestedProjectID string
	var postCount int

	states := []map[string]interface{}{
		{"id": "st-1", "name": "Backlog", "group": "backlog"},
		{"id": "st-2", "name": "Todo", "group": "unstarted"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()

		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/states/") {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			requestedProjectID = parts[len(parts)-2]
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": states,
			})
			return
		}

		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/states/") {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects/") {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"id": "proj-dgit-uuid", "identifier": "DGIT"}},
			})
			return
		}

		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/states/") {
			postCount++
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			states = append(states, body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(body)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("PLANE_BASE_URL", server.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj-default")

	ctx := context.Background()

	// 1. states ensure --project <uuid>
	handlePlaneCommand(ctx, "states", []string{"ensure", "--project", "proj-custom-uuid"})

	mu.Lock()
	if requestedProjectID != "proj-custom-uuid" {
		mu.Unlock()
		t.Errorf("expected project proj-custom-uuid, got: %s", requestedProjectID)
	}
	if postCount != 4 {
		mu.Unlock()
		t.Fatalf("expected 4 POSTs (In Progress, To Verify, Done, Cancelled), got %d", postCount)
	}
	mu.Unlock()

	// 2. states ensure (defaults to client's ProjectID: proj-default, and "To Verify" now exists)
	mu.Lock()
	postCount = 0
	mu.Unlock()

	handlePlaneCommand(ctx, "states", []string{"ensure"})

	mu.Lock()
	if requestedProjectID != "proj-default" {
		mu.Unlock()
		t.Errorf("expected default project proj-default, got: %s", requestedProjectID)
	}
	if postCount != 0 {
		mu.Unlock()
		t.Errorf("expected 0 POSTs when 'To Verify' already exists, got %d", postCount)
	}
	mu.Unlock()

	// 3. reconcile by identifier, dry run and --all
	handlePlaneCommand(ctx, "states", []string{"reconcile", "--project", "DGIT", "--dry-run"})
	mu.Lock()
	if requestedProjectID != "proj-dgit-uuid" {
		t.Errorf("expected DGIT resolved to proj-dgit-uuid, got: %s", requestedProjectID)
	}
	if postCount != 0 {
		t.Errorf("dry run must not POST, got %d", postCount)
	}
	mu.Unlock()
	handlePlaneCommand(ctx, "states", []string{"reconcile", "--all"})

	// 4. Bad inputs
	handlePlaneCommand(ctx, "states", []string{})
	handlePlaneCommand(ctx, "states", []string{"invalid-subcommand"})
	handlePlaneCommand(ctx, "states", []string{"ensure", "--project"})
	handlePlaneCommand(ctx, "states", []string{"ensure", "--invalid-flag"})

	// 5. Server error
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errServer.Close()

	t.Setenv("PLANE_BASE_URL", errServer.URL)
	handlePlaneCommand(ctx, "states", []string{"ensure"})
	handlePlaneCommand(ctx, "states", []string{"reconcile", "--all"})
}

func TestPlaneCommand_DefaultAndFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("PLANE_BASE_URL", server.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")

	ctx := context.Background()
	handlePlaneCommand(ctx, "nonexistent-action", nil)
}
