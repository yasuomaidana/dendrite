package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTools_Pool(t *testing.T) {
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	tmpDir := t.TempDir()

	// 1. dendrite_pool_resolve without directories (empty pool)
	req1 := `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_pool_resolve", "arguments": {}}}`
	resp1 := srv.HandleRequest(context.Background(), []byte(req1))
	if resp1 == nil || resp1.Error != nil {
		t.Fatalf("pool_resolve empty failed: %v", resp1)
	}

	// 2. dendrite_pool_resolve with directories
	projA := filepath.Join(tmpDir, "projA")
	projB := filepath.Join(tmpDir, "projB")
	_ = os.MkdirAll(projA, 0755)
	_ = os.MkdirAll(projB, 0755)
	_ = os.WriteFile(filepath.Join(projA, "go.mod"), []byte("module projA\n\ngo 1.25\n"), 0644)
	_ = os.WriteFile(filepath.Join(projB, "go.mod"), []byte("module projB\n\ngo 1.25\n"), 0644)

	req2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_pool_resolve", "arguments": {"directories": [%q, %q]}}}`, projA, projB)
	resp2 := srv.HandleRequest(context.Background(), []byte(req2))
	if resp2 == nil || resp2.Error != nil {
		t.Fatalf("pool_resolve with dirs failed: %v", resp2)
	}

	// 3. dendrite_pool_list
	req3 := `{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_pool_list", "arguments": {}}}`
	resp3 := srv.HandleRequest(context.Background(), []byte(req3))
	if resp3 == nil || resp3.Error != nil {
		t.Fatalf("pool_list failed: %v", resp3)
	}

	// 4. dendrite_pool_add: empty directories (error)
	req4a := `{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_pool_add", "arguments": {"directories": []}}}`
	resp4a := srv.HandleRequest(context.Background(), []byte(req4a))
	if res, ok := resp4a.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for empty directories in pool_add, got: %#v", resp4a)
	}

	// 5. dendrite_pool_add: valid directories
	req4b := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_pool_add", "arguments": {"directories": [%q]}}}`, projA)
	resp4b := srv.HandleRequest(context.Background(), []byte(req4b))
	if resp4b == nil {
		t.Fatal("expected response for pool_add")
	}

	// 6. dendrite_pool_switch
	req5 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_pool_switch", "arguments": {"target_project_dir": %q, "dependency_name": "projB", "mode": "published", "version": "v1.0.0"}}}`, projA)
	resp5 := srv.HandleRequest(context.Background(), []byte(req5))
	if resp5 == nil {
		t.Fatal("expected response for pool_switch")
	}

	// 7. dendrite_pool_split
	splitTarget := filepath.Join(tmpDir, "splitProj")
	req6 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_pool_split", "arguments": {"source_project_dir": %q, "module_subpath": "sub", "new_member_name": "splitProj", "target_directory": %q, "wire_local_mode": false, "register_in_pool": false}}}`, projA, splitTarget)
	resp6 := srv.HandleRequest(context.Background(), []byte(req6))
	if resp6 == nil {
		t.Fatal("expected response for pool_split")
	}
}

func TestTools_Query(t *testing.T) {
	// The reuse audit records ADRs under docs/adr relative to the CWD; keep them out of the package dir.
	t.Chdir(t.TempDir())
	srv := NewServer(ServerConfig{})
	srv.RegisterDefaultTools(Dependencies{})

	tmpDir := t.TempDir()

	// 1. dendrite_query_slice
	sliceReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "dendrite_query_slice", "arguments": {"sources_path": %q, "entrypoint_symbol": "main", "intent": "extract core", "direction": "forward", "output_dir": %q}}}`, tmpDir, filepath.Join(tmpDir, "slice_out"))
	sliceResp := srv.HandleRequest(context.Background(), []byte(sliceReq))
	if sliceResp == nil {
		t.Fatal("expected response for query_slice")
	}

	// 2. dendrite_query_swap
	swapReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "dendrite_query_swap", "arguments": {"sources_path": %q, "from_subsystem": "opengl", "to_subsystem": "wgpu", "target_language": "rust", "output_dir": %q}}}`, tmpDir, filepath.Join(tmpDir, "swap_out"))
	swapResp := srv.HandleRequest(context.Background(), []byte(swapReq))
	if swapResp == nil {
		t.Fatal("expected response for query_swap")
	}

	// 3. dendrite_query_transpile
	transReq := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "dendrite_query_transpile", "arguments": {"sources_artifact": %q, "target_language": "rust", "no_std": true, "audit_library_reuse": true, "prompt": "safe rust", "output_dir": %q}}}`, tmpDir, filepath.Join(tmpDir, "trans_out"))
	transResp := srv.HandleRequest(context.Background(), []byte(transReq))
	if transResp == nil {
		t.Fatal("expected response for query_transpile")
	}

	// 4. dendrite_query_fuse: < 2 sources (error)
	fuseReq1 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "dendrite_query_fuse", "arguments": {"sources": [%q], "fused_member_name": "fused", "target_directory": %q}}}`, tmpDir, filepath.Join(tmpDir, "fuse_out"))
	fuseResp1 := srv.HandleRequest(context.Background(), []byte(fuseReq1))
	if res, ok := fuseResp1.Result.(CallToolResult); !ok || !res.IsError {
		t.Fatalf("expected error for < 2 sources in query_fuse, got: %#v", fuseResp1)
	}

	// 5. dendrite_query_fuse: valid sources
	fuseReq2 := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": {"name": "dendrite_query_fuse", "arguments": {"sources": [%q, %q], "fused_member_name": "fused", "target_directory": %q, "target_language": "go", "register_in_pool": false}}}`, tmpDir, tmpDir, filepath.Join(tmpDir, "fuse_out"))
	fuseResp2 := srv.HandleRequest(context.Background(), []byte(fuseReq2))
	if fuseResp2 == nil {
		t.Fatal("expected response for query_fuse")
	}

	// 6. dendrite_query_reuse_audit
	reuseReq := `{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "dendrite_query_reuse_audit", "arguments": {"capability": "linear_algebra", "language": "rust", "ticket_id": "DEN-001", "intent": "matrix ops"}}}`
	reuseResp := srv.HandleRequest(context.Background(), []byte(reuseReq))
	if reuseResp == nil {
		t.Fatal("expected response for query_reuse_audit")
	}

	// 7. dendrite_ref_list
	refListReq := `{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "dendrite_ref_list", "arguments": {}}}`
	refListResp := srv.HandleRequest(context.Background(), []byte(refListReq))
	if refListResp == nil {
		t.Fatal("expected response for ref_list")
	}

	// 8. dendrite_ref_add (with name and without name)
	refAddReq1 := `{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": {"name": "dendrite_ref_add", "arguments": {"id": "ref-wgpu", "name": "wgpu-rs", "repository": "https://github.com/gfx-rs/wgpu", "language": "rust", "capabilities": ["graphics", "compute"], "license": "MIT"}}}`
	refAddResp1 := srv.HandleRequest(context.Background(), []byte(refAddReq1))
	if refAddResp1 == nil {
		t.Fatal("expected response for ref_add with name")
	}

	refAddReq2 := `{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": {"name": "dendrite_ref_add", "arguments": {"id": "ref-turso", "repository": "https://github.com/tursodatabase/libsql"}}}`
	refAddResp2 := srv.HandleRequest(context.Background(), []byte(refAddReq2))
	if refAddResp2 == nil {
		t.Fatal("expected response for ref_add without name")
	}
}
