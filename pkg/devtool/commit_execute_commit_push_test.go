package devtool

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

	"Dendrite/pkg/tracker/plane"
)

func TestExecuteCommit_Push_AC4_RefuseMainAndMaster(t *testing.T) {
	ctx := context.Background()

	t.Run("RefuseMain", func(t *testing.T) {
		repoDir, _ := setupCommitRepoWithRemote(t, "main")
		_ = os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("main content"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       repoDir,
			Message:       "feat: commit on main",
			StageAll:      true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("ExecuteCommit should succeed locally: %v", err)
		}
		if res.Pushed {
			t.Errorf("expected Pushed to be false on main")
		}
		if res.PushError == nil {
			t.Fatalf("expected PushError when pushing main is refused")
		}
		if !strings.Contains(res.PushError.Error(), "pushing main directly is refused") {
			t.Errorf("expected refusal error for main, got: %v", res.PushError)
		}
		if !res.PushSkipped {
			t.Errorf("expected PushSkipped to be true")
		}
	})

	t.Run("RefuseMaster", func(t *testing.T) {
		repoDir, _ := setupCommitRepoWithRemote(t, "master")
		_ = os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("master content"), 0644)

		res, err := ExecuteCommit(ctx, CommitOptions{
			RepoDir:       repoDir,
			Message:       "feat: commit on master",
			StageAll:      true,
			UseLocalModel: false,
		})
		if err != nil {
			t.Fatalf("ExecuteCommit should succeed locally: %v", err)
		}
		if res.Pushed {
			t.Errorf("expected Pushed to be false on master")
		}
		if res.PushError == nil {
			t.Fatalf("expected PushError when pushing master is refused")
		}
		if !strings.Contains(res.PushError.Error(), "pushing master directly is refused") {
			t.Errorf("expected refusal error for master, got: %v", res.PushError)
		}
		if !res.PushSkipped {
			t.Errorf("expected PushSkipped to be true")
		}
	})
}

func TestPushAfterCommit_RemoteCommandError(t *testing.T) {
	ctx := context.Background()
	res := &CommitResult{}
	opts := CommitOptions{RepoDir: t.TempDir()}

	runGit := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = opts.RepoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	}

	err := pushAfterCommit(ctx, opts, res, runGit)
	if err != nil {
		t.Fatalf("expected nil err on remote error fallback: %v", err)
	}
	if !res.PushSkipped || res.PushReason != "no remote configured" {
		t.Errorf("expected push skipped with 'no remote configured', got %+v", res)
	}
}

func TestExtractTicketIDAndTitle(t *testing.T) {
	tests := []struct {
		name          string
		expectedID    string
		expectedTitle string
	}{
		{"[DEN-101] Fix bug in parser", "DEN-101", "Fix bug in parser"},
		{"[DEND-202] Add graph node", "DEND-202", "Add graph node"},
		{"DEN-303 Fix without brackets", "DEN-303", "Fix without brackets"},
		{"No ticket in this title", "", "No ticket in this title"},
	}

	for _, tt := range tests {
		id, title := extractTicketIDAndTitle(tt.name)
		if id != tt.expectedID || title != tt.expectedTitle {
			t.Errorf("extractTicketIDAndTitle(%q) = (%q, %q); want (%q, %q)", tt.name, id, title, tt.expectedID, tt.expectedTitle)
		}
	}
}

func TestInferScopeFromFiles(t *testing.T) {
	tests := []struct {
		files    []string
		expected string
	}{
		{[]string{"pkg/devtool/foo.go"}, "devtool"},
		{[]string{"pkg/devtool/commit.go"}, "devtool"},
		{[]string{"pkg/gateway/proxy.go"}, "gateway"},
		{[]string{"pkg/rag/search.go"}, "rag"},
		{[]string{"cmd/dendrite/ask/ask.go"}, "rag"},
		{[]string{"pkg/agent/triage/route.go"}, "triage"},
		{[]string{"pkg/inference/engine.go"}, "inference"},
		{[]string{"pkg/env/detector.go"}, "env"},
		{[]string{"cmd/dendrite/main.go"}, "cli"},
		{[]string{"docs/index.md"}, "docs"},
		{[]string{"scripts/setup.sh"}, "scripts"},
		{[]string{"unknown/file.go"}, "repo"},
		{nil, "repo"},
	}

	for _, tt := range tests {
		got := inferScopeFromFiles(tt.files)
		if got != tt.expected {
			t.Errorf("inferScopeFromFiles(%v) = %q; want %q", tt.files, got, tt.expected)
		}
	}
}

func TestInferTypeFromTitle(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"Fix login bug", "fix"},
		{"Fixing memory leak", "fix"},
		{"Doc update for readme", "docs"},
		{"Guide for setup", "docs"},
		{"Refactor auth middleware", "refactor"},
		{"Test suite additions", "test"},
		{"CI workflow update", "ci"},
		{"Pipeline configuration", "ci"},
		{"Implement new feature", "feat"},
	}

	for _, tt := range tests {
		got := inferTypeFromTitle(tt.title)
		if got != tt.expected {
			t.Errorf("inferTypeFromTitle(%q) = %q; want %q", tt.title, got, tt.expected)
		}
	}
}

func TestIsCannedOrInvalidCommitMessage_AdditionalCases(t *testing.T) {
	valid := []string{
		"style: format code",
		"build: update cargo",
		"perf: optimize query",
		"revert: rollback commit",
		"[DEN-101] style: format code",
		"[DEN-101] perf: improve speed",
	}
	for _, v := range valid {
		if isCannedOrInvalidCommitMessage(v) {
			t.Errorf("expected isCannedOrInvalidCommitMessage(%q) = false, got true", v)
		}
	}

	invalid := []string{
		"here's the commit message for you",
		"here is the commit message",
		"as an ai, here is the message",
	}
	for _, inv := range invalid {
		if !isCannedOrInvalidCommitMessage(inv) {
			t.Errorf("expected isCannedOrInvalidCommitMessage(%q) = true, got false", inv)
		}
	}
}

func TestSynthesizeDeterministicCommitMessage(t *testing.T) {
	// 1. smart != nil with title, scope, type, and > 8 staged files
	smart := &SmartTicketInfo{
		ID:    "DEN-100",
		Title: "[DEN-100] implement awesome feature",
		Scope: "gateway",
		Type:  "feat",
	}
	files10 := []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"}
	msg1 := synthesizeDeterministicCommitMessage("DEN-100", smart, "", files10)
	if !strings.HasPrefix(msg1, "feat(gateway): implement awesome feature") {
		t.Errorf("unexpected msg1: %s", msg1)
	}
	if !strings.Contains(msg1, "and 2 additional files") {
		t.Errorf("expected additional files line in msg1: %s", msg1)
	}

	// 2. smart != nil without ID in title and empty scope/type
	smartEmpty := &SmartTicketInfo{
		Title: "plain title",
	}
	msgEmpty := synthesizeDeterministicCommitMessage("", smartEmpty, "", []string{"f1"})
	if !strings.HasPrefix(msgEmpty, "feat(repo): plain title") {
		t.Errorf("unexpected msgEmpty: %s", msgEmpty)
	}

	// 3. smart == nil with diffStat
	msg2 := synthesizeDeterministicCommitMessage("", nil, "1 file changed, 10 insertions\n", nil)
	if !strings.HasPrefix(msg2, "chore: update files (1 file changed, 10 insertions)") {
		t.Errorf("unexpected msg2: %s", msg2)
	}

	// 4. smart == nil, no diffStat, but staged files present
	msg3 := synthesizeDeterministicCommitMessage("", nil, "", []string{"pkg/devtool/commit.go"})
	if !strings.HasPrefix(msg3, "feat(devtool): update devtool capabilities and synchronizations") {
		t.Errorf("unexpected msg3: %s", msg3)
	}

	// 5. smart == nil, no diffStat, no staged files
	msg4 := synthesizeDeterministicCommitMessage("", nil, "", nil)
	if msg4 != "feat(repo): autonomous repository update" {
		t.Errorf("unexpected msg4: %s", msg4)
	}
}

func TestResolveSmartTicket_GitBranch(t *testing.T) {
	isolateEnvironment(t)
	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "heal/DEN-777-something", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()

	initFile := filepath.Join(tempDir, "init.txt")
	_ = os.WriteFile(initFile, []byte("init"), 0644)
	_ = exec.Command("git", "-C", tempDir, "add", "init.txt").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "initial commit").Run()

	info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/gateway/test.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != "DEN-777" {
		t.Errorf("expected DEN-777 from branch, got %s", info.ID)
	}
	if !strings.Contains(info.Source, "Git Branch") {
		t.Errorf("expected Source to contain Git Branch, got %s", info.Source)
	}

	// Non-ticket branch falls through
	_ = exec.Command("git", "-C", tempDir, "checkout", "-b", "feature-no-id").Run()
	info2, err2 := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/gateway/test.go"})
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if info2.ID != "" {
		t.Errorf("expected empty ID for feature-no-id, got %s", info2.ID)
	}
}

func TestResolveSmartTicket_PlaneCases(t *testing.T) {
	isolateEnvironment(t)
	var handler http.HandlerFunc
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler != nil {
			handler(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	t.Setenv("PLANE_BASE_URL", ts.URL)
	t.Setenv("PLANE_API_KEY", "custom_plane_key")
	t.Setenv("PLANE_WORKSPACE_SLUG", "workspace")
	t.Setenv("PLANE_PROJECT_ID", "project")

	tempDir := t.TempDir()
	_ = exec.Command("git", "init", "-b", "develop", tempDir).Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", tempDir, "config", "user.email", "test@test.ai").Run()
	initFile := filepath.Join(tempDir, "init.txt")
	_ = os.WriteFile(initFile, []byte("init"), 0644)
	_ = exec.Command("git", "-C", tempDir, "add", "init.txt").Run()
	_ = exec.Command("git", "-C", tempDir, "commit", "-m", "init").Run()

	// Case 1: In Progress issue (includes item without ticket ID to cover continue)
	t.Run("InProgress", func(t *testing.T) {
		handler = func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"id":               "00000000-0000-0000-0000-000000000000",
						"name":             "Issue without ticket id",
						"state_slug":       "in-progress",
						"description_html": "<p>no id</p>",
					},
					{
						"id":               "11111111-1111-1111-1111-111111111111",
						"name":             "[DEN-301] Fix something in progress",
						"state":            "8e46611a-87ed-457f-8be0-3c238b37e6eb",
						"state_slug":       "in-progress",
						"description_html": "<p>some desc</p>",
					},
				},
			})
		}
		info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"file.txt"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != "DEN-301" {
			t.Errorf("expected DEN-301, got %s", info.ID)
		}
		if info.Source != "Plane v2 (In Progress)" {
			t.Errorf("expected Plane v2 (In Progress), got %s", info.Source)
		}
	})

	// Case 2: Matching staged files with score bonus for to-verify
	t.Run("DiffCorrelation", func(t *testing.T) {
		handler = func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"id":               "00000000-0000-0000-0000-000000000000",
						"name":             "Issue without ticket id",
						"state_slug":       "open",
						"description_html": "<p>touching pkg/auth/handler.go</p>",
					},
					{
						"id":               "22222222-2222-2222-2222-222222222222",
						"name":             "[DEN-302] Update auth service",
						"state":            "f36249b3-f67a-427a-ab8a-cf1693ebdada",
						"state_slug":       "to-verify",
						"description_html": "<p>touching pkg/auth/handler.go and models</p>",
					},
				},
			})
		}
		info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"pkg/auth/handler.go"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != "DEN-302" {
			t.Errorf("expected DEN-302, got %s", info.ID)
		}
		if !strings.Contains(info.Source, "Diff Correlation Score") {
			t.Errorf("expected Diff Correlation Score in source, got %s", info.Source)
		}
	})

	// Case 3: Recent To Verify when staged files don't correlate
	t.Run("RecentToVerify", func(t *testing.T) {
		handler = func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"id":               "33333333-3333-3333-3333-333333333333",
						"name":             "[DEN-303] Unrelated issue in to-verify",
						"state_slug":       "to-verify",
						"description_html": "<p>something else</p>",
					},
				},
			})
		}
		info, err := ResolveSmartTicket(context.Background(), tempDir, []string{"xyz/unmatched.txt"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != "DEN-303" {
			t.Errorf("expected DEN-303, got %s", info.ID)
		}
		if !strings.Contains(info.Source, "Plane v2") {
			t.Errorf("expected Plane v2 in source, got %s", info.Source)
		}
	})
}

func TestFetchPlaneIssues_Errors(t *testing.T) {
	isolateEnvironment(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/500") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/badjson") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not valid json"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	ctx := context.Background()

	// 1. Status 500
	t.Setenv("PLANE_BASE_URL", ts.URL+"/500")
	pClient, _ := plane.NewClientFromEnv()
	_, err := fetchPlaneIssues(ctx, pClient)
	if err == nil || !strings.Contains(err.Error(), "plane api status 500") {
		t.Errorf("expected plane api status 500, got: %v", err)
	}

	// 2. Bad JSON
	t.Setenv("PLANE_BASE_URL", ts.URL+"/badjson")
	pClient2, _ := plane.NewClientFromEnv()
	_, err2 := fetchPlaneIssues(ctx, pClient2)
	if err2 == nil {
		t.Errorf("expected decode error on bad JSON")
	}

	// 3. Invalid URL (http.NewRequest error)
	t.Setenv("PLANE_BASE_URL", "://bad url")
	pClient3, _ := plane.NewClientFromEnv()
	_, err3 := fetchPlaneIssues(ctx, pClient3)
	if err3 == nil {
		t.Errorf("expected request creation error on bad URL")
	}

	// 4. Empty API Key fallback (line 260) & connection error (line 268)
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:59999")
	t.Setenv("PLANE_API_KEY", "")
	pClient4, _ := plane.NewClientFromEnv()
	_, err4 := fetchPlaneIssues(ctx, pClient4)
	if err4 == nil {
		t.Errorf("expected connection error on closed port")
	}
}
