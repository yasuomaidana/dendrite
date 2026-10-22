package mcpserver

import (
	"context"
	"fmt"
	"testing"
)

func TestTools_CanvasAndRAG(t *testing.T) {
	srv := NewServer(ServerConfig{})
	RegisterDefaultTools(srv, Dependencies{})

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
