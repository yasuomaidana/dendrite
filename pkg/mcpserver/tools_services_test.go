package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTools_Store(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	// 1. dendrite_store_status
	stReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_store_status", "arguments": {"project": "proj-store"}}}`
	stResp := srv.HandleRequest(context.Background(), []byte(stReq))
	if stResp == nil || stResp.Error != nil {
		t.Fatalf("store_status failed: %v", stResp)
	}

	// 2. dendrite_store_commit_chunk (type: "file", is_final: false)
	chunkReq1 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_store_commit_chunk", "arguments": {"project": "proj-store", "chunk_id": "chunk-1", "identifier": "main.go", "data": "package main", "type": "file", "is_final": false}}}`
	chunkResp1 := srv.HandleRequest(context.Background(), []byte(chunkReq1))
	if chunkResp1 == nil || chunkResp1.Error != nil {
		t.Fatalf("commit_chunk 1 failed: %v", chunkResp1)
	}

	// 3. dendrite_store_commit_chunk (type: "", is_final: true)
	chunkReq2 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_store_commit_chunk", "arguments": {"project": "proj-store", "chunk_id": "chunk-2", "identifier": "helper.go", "data": "package helper", "is_final": true}}}`
	chunkResp2 := srv.HandleRequest(context.Background(), []byte(chunkReq2))
	if chunkResp2 == nil || chunkResp2.Error != nil {
		t.Fatalf("commit_chunk 2 failed: %v", chunkResp2)
	}

	// 4. dendrite_store_query_partial
	qpReq := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_store_query_partial", "arguments": {"project": "proj-store"}}}`
	qpResp := srv.HandleRequest(context.Background(), []byte(qpReq))
	if qpResp == nil || qpResp.Error != nil {
		t.Fatalf("query_partial failed: %v", qpResp)
	}

	// 5. dendrite_store_inspect
	inspReq := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_store_inspect", "arguments": {"store_id": "proj-store"}}}`
	inspResp := srv.HandleRequest(context.Background(), []byte(inspReq))
	if inspResp == nil {
		t.Fatal("expected response for store_inspect")
	}

	// 6. dendrite_store_rm (force: false and force: true)
	rmReq1 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_store_rm", "arguments": {"store_id": "proj-store", "force": false}}}`
	rmResp1 := srv.HandleRequest(context.Background(), []byte(rmReq1))
	if rmResp1 == nil {
		t.Fatal("expected response for store_rm force:false")
	}

	rmReq2 := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_store_rm", "arguments": {"store_id": "proj-store", "force": true}}}`
	rmResp2 := srv.HandleRequest(context.Background(), []byte(rmReq2))
	if rmResp2 == nil {
		t.Fatal("expected response for store_rm force:true")
	}
}

func TestTools_Scheduler(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	// 1. dendrite_analyze_policy: sprint_before_reset invalid duration error
	polReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_analyze_policy", "arguments": {"sprint_before_reset": "invalid-dur"}}}`
	polResp1 := srv.HandleRequest(context.Background(), []byte(polReq1))
	if res, ok := polResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for invalid duration in policy, got: %#v", polResp1)
	}

	// 2. dendrite_analyze_policy: valid options
	polReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_analyze_policy", "arguments": {"night_window": "23:00-06:00", "weekly_reset": "SUN 23:59", "sprint_before_reset": "48h", "reserve_floor": 0.20, "allow_zero": true, "mode": "auto"}}}`
	polResp2 := srv.HandleRequest(context.Background(), []byte(polReq2))
	if polResp2 == nil || polResp2.Error != nil {
		t.Fatalf("analyze_policy failed: %v", polResp2)
	}

	// 3. dendrite_analyze_status
	statReq := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_analyze_status", "arguments": {"remaining_quota": 0.75}}}`
	statResp := srv.HandleRequest(context.Background(), []byte(statReq))
	if statResp == nil || statResp.Error != nil {
		t.Fatalf("analyze_status failed: %v", statResp)
	}

	// 4. dendrite_analyze_trigger: missing chunk_id error
	trigReq1 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {}}}`
	trigResp1 := srv.HandleRequest(context.Background(), []byte(trigReq1))
	if res, ok := trigResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing chunk_id, got: %#v", trigResp1)
	}

	// 5. dendrite_analyze_trigger: simulate_night and simulate_sprint
	trigReq2 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {"chunk_id": "chunk-101", "simulate_night": true}}}`
	trigResp2 := srv.HandleRequest(context.Background(), []byte(trigReq2))
	if trigResp2 == nil || trigResp2.Error != nil {
		t.Fatalf("analyze_trigger simulate_night failed: %v", trigResp2)
	}

	trigReq3 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_analyze_trigger", "arguments": {"chunk_id": "chunk-102", "simulate_sprint": true}}}`
	trigResp3 := srv.HandleRequest(context.Background(), []byte(trigReq3))
	if trigResp3 == nil || trigResp3.Error != nil {
		t.Fatalf("analyze_trigger simulate_sprint failed: %v", trigResp3)
	}

	// 6. dendrite_analyze_switch
	swReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_analyze_switch", "arguments": {"allow_zero": true, "mode": "always"}}}`
	swResp := srv.HandleRequest(context.Background(), []byte(swReq))
	if swResp == nil || swResp.Error != nil {
		t.Fatalf("analyze_switch failed: %v", swResp)
	}

	// 7. dendrite_tokens_status
	tokReq := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_tokens_status", "arguments": {"model": "gemini-3.8-flash-high", "probe": false}}}`
	tokResp := srv.HandleRequest(context.Background(), []byte(tokReq))
	if tokResp == nil || tokResp.Error != nil {
		t.Fatalf("tokens_status failed: %v", tokResp)
	}
}

func TestTools_Contract(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	// 1. Missing schema_dirs error
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_contract_drift_check", "arguments": {}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if res, ok := resp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing schema_dirs, got: %#v", resp1)
	}

	// 2. Valid contract drift check
	tmpDir := t.TempDir()
	schemaDir := filepath.Join(tmpDir, "schemas")
	_ = os.MkdirAll(schemaDir, 0755)
	_ = os.WriteFile(filepath.Join(schemaDir, "test.proto"), []byte(`syntax = "proto3"; message Test { string id = 1; }`), 0644)

	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_contract_drift_check", "arguments": {"schema_dirs": [%q], "code_dirs": {"go": %q}}}}`, schemaDir, tmpDir)
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("contract_drift_check failed: %v", resp2)
	}
}

func TestTools_Story(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	// 1. dendrite_story_fork: missing story_id
	fReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_story_fork", "arguments": {"variants": ["v1", "v2"]}}}`
	fResp1 := srv.HandleRequest(context.Background(), []byte(fReq1))
	if res, ok := fResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing story_id in story_fork, got: %#v", fResp1)
	}

	// 2. dendrite_story_fork: < 2 variants
	fReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_story_fork", "arguments": {"story_id": "DEN-064", "variants": ["v1"]}}}`
	fResp2 := srv.HandleRequest(context.Background(), []byte(fReq2))
	if res, ok := fResp2.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for < 2 variants in story_fork, got: %#v", fResp2)
	}

	// 3. dendrite_story_benchmark: missing story_id
	bReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_story_benchmark", "arguments": {}}}`
	bResp1 := srv.HandleRequest(context.Background(), []byte(bReq1))
	if res, ok := bResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing story_id in story_benchmark, got: %#v", bResp1)
	}

	// 4. dendrite_story_resolve: missing story_id or winner
	rReq1 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_story_resolve", "arguments": {"story_id": "DEN-064"}}}`
	rResp1 := srv.HandleRequest(context.Background(), []byte(rReq1))
	if res, ok := rResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing winner in story_resolve, got: %#v", rResp1)
	}

	// 5. dendrite_story_abort: missing story_id
	aReq1 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_story_abort", "arguments": {}}}`
	aResp1 := srv.HandleRequest(context.Background(), []byte(aReq1))
	if res, ok := aResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing story_id in story_abort, got: %#v", aResp1)
	}
}

func TestTools_PlaneDAG(t *testing.T) {
	_, planeClient := createTestPlaneServer(t)

	tmpDir := t.TempDir()
	devStatusFile := filepath.Join(tmpDir, "DEV_STATUS.md")
	mdContent := "# Development Status\n\n- **Active Milestone**: Milestone 1\n\n### Milestone 1: Core Foundation\n\n- [ ] **DEN-101**: Setup (`pkg/mcpserver`) [State: To Do, Priority: High]\n- [ ] **DEN-102**: Feature (`pkg/mcpserver`) [State: To Do, Priority: Medium]\n"
	_ = os.WriteFile(devStatusFile, []byte(mdContent), 0644)

	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{
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
	srvFallback.RegisterDefaultTools(Dependencies{DevStatus: devStatusFile})
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

// chdirToScratchRepo runs the rest of the test inside a throwaway git checkout (on a
// feature branch, with an origin) so tools that act on "." (branch guard, audit,
// remote sync) can never switch, rebase or commit in the real repository.
func chdirToScratchRepo(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	origin, repo := filepath.Join(root, "origin.git"), filepath.Join(root, "repo")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(root, "init", "-q", "--bare", "-b", "develop", origin)
	git(root, "clone", "-q", origin, repo)
	git(repo, "checkout", "-q", "-b", "develop")
	git(repo, "commit", "-q", "--allow-empty", "-m", "init")
	git(repo, "push", "-q", "-u", "origin", "develop")
	git(repo, "checkout", "-q", "-b", "feature/x")
	t.Chdir(repo)
}

func TestTools_Audit(t *testing.T) {
	chdirToScratchRepo(t)
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	// 1. dendrite_vcs_guard (STANDARD and DEGRADED_BUDGET)
	gReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_vcs_guard", "arguments": {"mode": "STANDARD"}}}`
	gResp1 := srv.HandleRequest(context.Background(), []byte(gReq1))
	if gResp1 == nil || gResp1.Error != nil {
		t.Fatalf("vcs_guard STANDARD failed: %v", gResp1)
	}

	gReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_vcs_guard", "arguments": {"mode": "DEGRADED_BUDGET"}}}`
	gResp2 := srv.HandleRequest(context.Background(), []byte(gReq2))
	if gResp2 == nil || gResp2.Error != nil {
		t.Fatalf("vcs_guard DEGRADED_BUDGET failed: %v", gResp2)
	}

	// 2. dendrite_quality_audit: missing pkg error
	qaReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_quality_audit", "arguments": {}}}`
	qaResp1 := srv.HandleRequest(context.Background(), []byte(qaReq1))
	if res, ok := qaResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing pkg, got: %#v", qaResp1)
	}

	// 3. dendrite_quality_audit: valid
	qaReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_quality_audit", "arguments": {"pkg": "pkg/mcpserver"}}}`
	qaResp2 := srv.HandleRequest(context.Background(), []byte(qaReq2))
	if qaResp2 == nil {
		t.Fatal("expected response for quality_audit")
	}

	// 4. dendrite_audit_changes
	acReq := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_audit_changes", "arguments": {"dry_run": true, "max_tokens": 2048, "model": "qwen3.8:27b"}}}`
	acResp := srv.HandleRequest(context.Background(), []byte(acReq))
	if acResp == nil {
		t.Fatal("expected response for audit_changes")
	}

	// 5. dendrite_hitl_sync
	hitlReq := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_hitl_sync", "arguments": {}}}`
	hitlResp := srv.HandleRequest(context.Background(), []byte(hitlReq))
	if hitlResp == nil {
		t.Fatal("expected response for hitl_sync")
	}

	// 6. dendrite_remote_sync
	rsReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_remote_sync", "arguments": {"branch": "develop"}}}`
	rsResp := srv.HandleRequest(context.Background(), []byte(rsReq))
	if rsResp == nil {
		t.Fatal("expected response for remote_sync")
	}

	// 7. dendrite_github_sync
	ghReq := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_github_sync", "arguments": {"repo": "yasuomaidana/dendrite"}}}`
	ghResp := srv.HandleRequest(context.Background(), []byte(ghReq))
	if ghResp == nil {
		t.Fatal("expected response for github_sync")
	}
}

func TestTools_InferenceAndPenpot(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	// 1. dendrite_inference_migrate_model: missing model error
	mReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_inference_migrate_model", "arguments": {}}}`
	mResp1 := srv.HandleRequest(context.Background(), []byte(mReq1))
	if res, ok := mResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing model, got: %#v", mResp1)
	}

	// 2. dendrite_inference_migrate_model: valid
	mReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_inference_migrate_model", "arguments": {"model": "qwen2.5:7b", "source_engine": "ollama", "target_engine": "oxillama", "delete_source": false, "dry_run": true}}}`
	mResp2 := srv.HandleRequest(context.Background(), []byte(mReq2))
	if mResp2 == nil {
		t.Fatal("expected response for migrate_model")
	}

	// 3. dendrite_ui_penpot_sync: missing file error
	pReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_ui_penpot_sync", "arguments": {}}}`
	pResp1 := srv.HandleRequest(context.Background(), []byte(pReq1))
	if res, ok := pResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing file, got: %#v", pResp1)
	}

	// 4. dendrite_ui_penpot_sync: valid
	pReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_ui_penpot_sync", "arguments": {"file": "Button.svelte", "ticket_id": "DEN-810", "dry_run": true, "comment_plane": false}}}`
	pResp2 := srv.HandleRequest(context.Background(), []byte(pReq2))
	if pResp2 == nil {
		t.Fatal("expected response for ui_penpot_sync")
	}
}

func TestTools_MigrateGreetingEnv(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	// 1. dendrite_service_move: missing target_host error
	smReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {}}}`
	smResp1 := srv.HandleRequest(context.Background(), []byte(smReq1))
	if res, ok := smResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing target_host, got: %#v", smResp1)
	}

	// 2. dendrite_service_move: valid dry_run
	smReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_service_move", "arguments": {"target_host": "127.0.0.1", "services": ["plane"], "ssh_port": 22, "target_ram_mb": 4096, "target_disk_gb": 500, "dry_run": true, "prune_local": false, "timeout_seconds": 60}}}`
	smResp2 := srv.HandleRequest(context.Background(), []byte(smReq2))
	if smResp2 == nil {
		t.Fatal("expected response for service_move")
	}

	// 3. dendrite_service_plan
	planReq := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_service_plan", "arguments": {"ram_mb": 4096, "disk_gb": 1000}}}`
	planResp := srv.HandleRequest(context.Background(), []byte(planReq))
	if planResp == nil || planResp.Error != nil {
		t.Fatalf("service_plan failed: %v", planResp)
	}

	// 4. dendrite_pcontext_greeting_generate
	gReq := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_pcontext_greeting_generate", "arguments": {"topic": "Refactoring", "decision": "architecture", "milestone": "M1", "milestone_achieved": true, "event": "Launch", "days_until_event": 5, "unfinished_step": "test", "specific_challenge": "speed", "curiosity_angle": "wasm", "days_since_last_active": 2, "turn_count": 3, "high_stress": false, "low_pressure_hobby": true, "options": ["A", "B"], "recent_styles": ["G-01"]}}}`
	gResp := srv.HandleRequest(context.Background(), []byte(gReq))
	if gResp == nil || gResp.Error != nil {
		t.Fatalf("greeting_generate failed: %v", gResp)
	}

	// 5. dendrite_env_prepare (dry_run: true)
	tmpDir := t.TempDir()
	envReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_env_prepare", "arguments": {"project_path": %q, "default_tool": "copilot", "enforce_cicd": true, "dry_run": true}}}`, tmpDir)
	envResp := srv.HandleRequest(context.Background(), []byte(envReq))
	if envResp == nil || envResp.Error != nil {
		t.Fatalf("env_prepare failed: %v", envResp)
	}
}

func TestTools_CanvasAndRAG(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	// 1. dendrite_canvas_action_dispatch: missing action error
	cReq1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_canvas_action_dispatch", "arguments": {"prompt": "write test"}}}`
	cResp1 := srv.HandleRequest(context.Background(), []byte(cReq1))
	if res, ok := cResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing action, got: %#v", cResp1)
	}

	// 2. dendrite_canvas_action_dispatch: valid
	cReq2 := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_canvas_action_dispatch", "arguments": {"action": "WRITE_PLAN", "mode": "DRAFT", "title": "Test Plan", "prompt": "Outline tasks"}}}`
	cResp2 := srv.HandleRequest(context.Background(), []byte(cReq2))
	if cResp2 == nil || cResp2.Error != nil {
		t.Fatalf("canvas action dispatch failed: %v", cResp2)
	}

	// 3. dendrite_canvas_penpot_bridge subactions
	// relate
	subReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "relate", "ticket_id": "DEN-100", "feature_name": "Canvas", "file_path": "App.svelte"}}}`
	subResp1 := srv.HandleRequest(context.Background(), []byte(subReq1))
	if subResp1 == nil || subResp1.Error != nil {
		t.Fatalf("penpot bridge relate failed: %v", subResp1)
	}

	// sync
	subReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "sync", "ticket_id": "DEN-100", "file_path": "App.svelte", "dry_run": true}}}`
	subResp2 := srv.HandleRequest(context.Background(), []byte(subReq2))
	if subResp2 == nil || subResp2.Error != nil {
		t.Fatalf("penpot bridge sync failed: %v", subResp2)
	}

	// update_state
	subReq3 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "update_state", "ticket_id": "DEN-100", "target_state": "Done", "push_to_plane": false}}}`
	subResp3 := srv.HandleRequest(context.Background(), []byte(subReq3))
	if subResp3 == nil || subResp3.Error != nil {
		t.Fatalf("penpot bridge update_state failed: %v", subResp3)
	}

	// list
	subReq4 := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "list"}}}`
	subResp4 := srv.HandleRequest(context.Background(), []byte(subReq4))
	if subResp4 == nil || subResp4.Error != nil {
		t.Fatalf("penpot bridge list failed: %v", subResp4)
	}

	// unknown subaction error
	subReq5 := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_canvas_penpot_bridge", "arguments": {"subaction": "invalid_subaction"}}}`
	subResp5 := srv.HandleRequest(context.Background(), []byte(subReq5))
	if res, ok := subResp5.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for unknown subaction, got: %#v", subResp5)
	}

	// 4. dendrite_rag_ask: missing question error
	tmpDir := t.TempDir()
	rReq1 := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_rag_ask", "arguments": {}}}`
	rResp1 := srv.HandleRequest(context.Background(), []byte(rReq1))
	if res, ok := rResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing question in rag_ask, got: %#v", rResp1)
	}

	// dendrite_rag_ask: valid question
	rReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": {"name": "dendrite_rag_ask", "arguments": {"question": "How does server work?", "project_path": %q, "auto_approve_paid": false, "top_k": 2}}}`, tmpDir)
	rResp2 := srv.HandleRequest(context.Background(), []byte(rReq2))
	if rResp2 == nil {
		t.Fatal("expected response for rag_ask")
	}

	// 5. dendrite_rag_retrieve: missing query error
	retReq1 := `{"jsonrpc": "2.0", "id": 10, "method": "tools/call", "params": {"name": "dendrite_rag_retrieve", "arguments": {}}}`
	retResp1 := srv.HandleRequest(context.Background(), []byte(retReq1))
	if res, ok := retResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for missing query in rag_retrieve, got: %#v", retResp1)
	}

	// dendrite_rag_retrieve: valid query
	retReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 11, "method": "tools/call", "params": {"name": "dendrite_rag_retrieve", "arguments": {"query": "Server", "project_path": %q, "top_k": 2, "min_score": 0.001}}}`, tmpDir)
	retResp2 := srv.HandleRequest(context.Background(), []byte(retReq2))
	if retResp2 == nil {
		t.Fatal("expected response for rag_retrieve")
	}
}
