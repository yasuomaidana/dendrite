package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTools_DendriteTelemetryAndInference(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

	// dendrite_telemetry
	tReq := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_telemetry", "arguments": {}}}`
	tResp := srv.HandleRequest(context.Background(), []byte(tReq))
	if tResp == nil || tResp.Error != nil {
		t.Fatalf("telemetry error: %v", tResp)
	}

	// dendrite_inference_list
	lReq := `{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_inference_list", "arguments": {}}}`
	lResp := srv.HandleRequest(context.Background(), []byte(lReq))
	if lResp == nil || lResp.Error != nil {
		t.Fatalf("inference list error: %v", lResp)
	}

	// dendrite_inference_probe all
	pReq1 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {}}}`
	pResp1 := srv.HandleRequest(context.Background(), []byte(pReq1))
	if pResp1 == nil || pResp1.Error != nil {
		t.Fatalf("inference probe all error: %v", pResp1)
	}

	// dendrite_inference_probe single engine
	pReq2 := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {"engine_id": "oxillama-rust"}}}`
	pResp2 := srv.HandleRequest(context.Background(), []byte(pReq2))
	if pResp2 == nil || pResp2.Error != nil {
		t.Fatalf("inference probe single error: %v", pResp2)
	}

	// dendrite_inference_probe non-existent engine
	pReq3 := `{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_inference_probe", "arguments": {"engine_id": "non-existent-engine"}}}`
	pResp3 := srv.HandleRequest(context.Background(), []byte(pReq3))
	if res, ok := pResp3.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for non-existent engine, got: %#v", pResp3)
	}
}

func TestTools_DendritePlaneReplicateAndPush(t *testing.T) {
	_, planeClient := createTestPlaneServer(t)

	tmpDir := t.TempDir()
	devStatusFile := filepath.Join(tmpDir, "DEV_STATUS.md")

	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{PlaneClient: planeClient})

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
	RegisterDefaultTools(srv, Dependencies{})

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
	RegisterDefaultTools(srv, Dependencies{})

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
