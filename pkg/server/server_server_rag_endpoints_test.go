package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/workspace"
)

func TestServer_RAGEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	docFile := filepath.Join(tmpDir, "arch.md")
	docContent := "# Architecture\nDendrite RAG server provides context retrieval."
	_ = os.WriteFile(docFile, []byte(docContent), 0644)

	cfg := ServerConfig{Port: 0, Host: "127.0.0.1"}
	s := NewServer(cfg, nil, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() { _ = s.Shutdown(context.Background()) }()

	baseURL := fmt.Sprintf("http://%s", s.listener.Addr().String())
	client := &http.Client{Timeout: 5 * time.Second}

	// 1. POST /api/v1/rag/retrieve
	retReq := fmt.Sprintf(`{"query": "Architecture context retrieval", "project_path": "%s", "top_k": 2}`, tmpDir)
	respRet, err := client.Post(baseURL+"/api/v1/rag/retrieve", "application/json", strings.NewReader(retReq))
	if err != nil || respRet.StatusCode != http.StatusOK {
		t.Fatalf("POST retrieve failed: %v, status: %v", err, respRet.StatusCode)
	}

	var chunks []map[string]interface{}
	if err := json.NewDecoder(respRet.Body).Decode(&chunks); err != nil {
		t.Fatalf("failed to decode retrieve response: %v", err)
	}
	if len(chunks) == 0 {
		t.Errorf("expected at least 1 retrieved chunk, got 0")
	}

	// 2. Bad requests (method not allowed & missing params)
	respGet, _ := client.Get(baseURL + "/api/v1/rag/retrieve")
	if respGet.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on GET retrieve, got: %d", respGet.StatusCode)
	}

	respBadQuery, _ := client.Post(baseURL+"/api/v1/rag/retrieve", "application/json", strings.NewReader(`{"query": ""}`))
	if respBadQuery.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 on empty query, got: %d", respBadQuery.StatusCode)
	}

	respBadAsk, _ := client.Post(baseURL+"/api/v1/rag/ask", "application/json", strings.NewReader(`{"question": ""}`))
	if respBadAsk.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 on empty question, got: %d", respBadAsk.StatusCode)
	}
}

func TestServer_GraphEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	src := "package demo\n\nfunc LocateBinary(name string) string { return name }\n"
	_ = os.MkdirAll(filepath.Join(tmpDir, "pkg", "demo"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "pkg", "demo", "demo.go"), []byte(src), 0644)

	cfg := ServerConfig{Port: 0, Host: "127.0.0.1"}
	s := NewServer(cfg, nil, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() { _ = s.Shutdown(context.Background()) }()

	baseURL := fmt.Sprintf("http://%s", s.listener.Addr().String())
	client := &http.Client{Timeout: 5 * time.Second}

	// 1. POST /api/v1/graph/communities
	commReq := fmt.Sprintf(`{"project_path": "%s", "limit": 10}`, tmpDir)
	respComm, err := client.Post(baseURL+"/api/v1/graph/communities", "application/json", strings.NewReader(commReq))
	if err != nil || respComm.StatusCode != http.StatusOK {
		t.Fatalf("POST communities failed: %v, status: %v", err, respComm.StatusCode)
	}
	var commRes map[string]interface{}
	if err := json.NewDecoder(respComm.Body).Decode(&commRes); err != nil {
		t.Fatalf("failed to decode communities response: %v", err)
	}
	if _, ok := commRes["communities"]; !ok {
		t.Errorf("expected 'communities' key in response, got %v", commRes)
	}

	// 2. POST /api/v1/graph/god_nodes
	godReq := fmt.Sprintf(`{"project_path": "%s", "min_in_degree": 21}`, tmpDir)
	respGod, err := client.Post(baseURL+"/api/v1/graph/god_nodes", "application/json", strings.NewReader(godReq))
	if err != nil || respGod.StatusCode != http.StatusOK {
		t.Fatalf("POST god_nodes failed: %v, status: %v", err, respGod.StatusCode)
	}
	var godRes map[string]interface{}
	if err := json.NewDecoder(respGod.Body).Decode(&godRes); err != nil {
		t.Fatalf("failed to decode god_nodes response: %v", err)
	}
	if _, ok := godRes["nodes"]; !ok {
		t.Errorf("expected 'nodes' key in response, got %v", godRes)
	}

	// 3. POST /api/v1/graph/path
	pathReq := fmt.Sprintf(`{"project_path": "%s", "from": "Run", "to": "LocateBinary", "kinds": ["call"]}`, tmpDir)
	respPath, err := client.Post(baseURL+"/api/v1/graph/path", "application/json", strings.NewReader(pathReq))
	if err != nil || respPath.StatusCode != http.StatusOK {
		t.Fatalf("POST path failed: %v, status: %v", err, respPath.StatusCode)
	}
	var pathRes map[string]interface{}
	if err := json.NewDecoder(respPath.Body).Decode(&pathRes); err != nil {
		t.Fatalf("failed to decode path response: %v", err)
	}
	if pathRes["found"] != true {
		t.Errorf("expected found: true, got %v", pathRes["found"])
	}

	// 4. POST /api/v1/graph/explain
	explainReq := fmt.Sprintf(`{"project_path": "%s", "target": "LocateBinary"}`, tmpDir)
	respExplain, err := client.Post(baseURL+"/api/v1/graph/explain", "application/json", strings.NewReader(explainReq))
	if err != nil || respExplain.StatusCode != http.StatusOK {
		t.Fatalf("POST explain failed: %v, status: %v", err, respExplain.StatusCode)
	}
	var explainRes map[string]interface{}
	if err := json.NewDecoder(respExplain.Body).Decode(&explainRes); err != nil {
		t.Fatalf("failed to decode explain response: %v", err)
	}
	if _, ok := explainRes["matches"]; !ok {
		t.Errorf("expected 'matches' key in response, got %v", explainRes)
	}

	// 5. Bad requests
	respGet, _ := client.Get(baseURL + "/api/v1/graph/communities")
	if respGet.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on GET communities, got: %d", respGet.StatusCode)
	}

	respBadPath, _ := client.Post(baseURL+"/api/v1/graph/path", "application/json", strings.NewReader(`{"from": ""}`))
	if respBadPath.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 on empty path targets, got: %d", respBadPath.StatusCode)
	}

	respBadExplain, _ := client.Post(baseURL+"/api/v1/graph/explain", "application/json", strings.NewReader(`{"target": ""}`))
	if respBadExplain.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 on empty explain target, got: %d", respBadExplain.StatusCode)
	}
}

type testHijackFailRecorder struct {
	*httptest.ResponseRecorder
}

func (h *testHijackFailRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("hijack failure")
}

type testSmallBufHijacker struct {
	*httptest.ResponseRecorder
	serverConn net.Conn
	rw         *bufio.ReadWriter
}

func (h *testSmallBufHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.serverConn, h.rw, nil
}

type testNonFlusherRecorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (r *testNonFlusherRecorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

func (r *testNonFlusherRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *testNonFlusherRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

type testFailingWriter struct {
	allowedBytes int
	written      int
}

func (w *testFailingWriter) Write(p []byte) (int, error) {
	if w.written+len(p) > w.allowedBytes {
		return 0, errors.New("write limit exceeded")
	}
	w.written += len(p)
	return len(p), nil
}

func TestServer_ConfigAndLifecycleBranches(t *testing.T) {
	// 1. NewServer with explicit host, custom read timeout, port > 0, hub == nil
	s := NewServer(ServerConfig{
		Host:        "127.0.0.1",
		Port:        8080,
		ReadTimeout: 10 * time.Second,
	}, nil, nil)

	// Addr() before Start()
	if addr := s.Addr(); addr != "127.0.0.1:8080" {
		t.Errorf("expected 127.0.0.1:8080, got: %s", addr)
	}

	// 2. NewServer with empty host exercises cfg.Host == "" -> "127.0.0.1"
	sDefaultHost := NewServer(ServerConfig{
		Host: "",
		Port: 9090,
	}, nil, nil)
	if sDefaultHost.cfg.Host != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got: %s", sDefaultHost.cfg.Host)
	}

	// 3. Start() failure on invalid address
	sBad := NewServer(ServerConfig{
		Host: "999.999.999.999",
		Port: 80,
	}, nil, nil)
	if err := sBad.Start(); err == nil {
		t.Errorf("expected error starting server with invalid host, got nil")
	}

	// 4. Shutdown on nil server
	sNil := &Server{}
	if err := sNil.Shutdown(context.Background()); err != nil {
		t.Errorf("expected nil error on Shutdown with nil http.Server, got: %v", err)
	}
}

func TestServer_WorkspaceHandlersExtended(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.json")
	dataDir := filepath.Join(tempDir, "data")
	reg, err := workspace.NewRegistry(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}

	p1Dir := filepath.Join(tempDir, "proj-1")
	_ = os.MkdirAll(p1Dir, 0755)

	srv := NewServer(ServerConfig{Port: 0}, reg, nil)

	// 1. POST /api/v1/workspaces with bad JSON -> 400
	recBadJSON := httptest.NewRecorder()
	reqBadJSON := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", strings.NewReader("{invalid"))
	srv.handleWorkspaces(recBadJSON, reqBadJSON)
	if recBadJSON.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad JSON in workspaces POST, got: %d", recBadJSON.Code)
	}

	// 2. POST /api/v1/workspaces with invalid project -> 400
	recBadAdd := httptest.NewRecorder()
	reqBadAdd := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", strings.NewReader(`{"name": "", "path": ""}`))
	srv.handleWorkspaces(recBadAdd, reqBadAdd)
	if recBadAdd.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid project add, got: %d", recBadAdd.Code)
	}

	// 3. Add project and switch, then GET /api/v1/workspaces when active is present
	entry, err := reg.AddProject("proj-alpha", p1Dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.SwitchProject(entry.Name); err != nil {
		t.Fatal(err)
	}

	recGetActive := httptest.NewRecorder()
	reqGetActive := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	srv.handleWorkspaces(recGetActive, reqGetActive)
	if recGetActive.Code != http.StatusOK {
		t.Errorf("expected 200 on workspaces GET, got: %d", recGetActive.Code)
	}
	var res map[string]interface{}
	_ = json.NewDecoder(recGetActive.Body).Decode(&res)
	if res["active"] != "proj-alpha" {
		t.Errorf("expected active project 'proj-alpha', got: %v", res["active"])
	}

	// 4. POST /api/v1/workspaces/switch with non-existent project -> 400
	recSwitchErr := httptest.NewRecorder()
	reqSwitchErr := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/switch", strings.NewReader(`{"name": "non-existent-proj"}`))
	srv.handleWorkspaceSwitch(recSwitchErr, reqSwitchErr)
	if recSwitchErr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on switch to non-existent project, got: %d", recSwitchErr.Code)
	}
}

func TestServer_TopologyHandlersExtended(t *testing.T) {
	srv := NewServer(ServerConfig{Port: 0}, nil, nil)

	// 1. POST /api/v1/topology with bad JSON -> 400
	recBadJSON := httptest.NewRecorder()
	reqBadJSON := httptest.NewRequest(http.MethodPost, "/api/v1/topology", strings.NewReader("{invalid"))
	srv.handleTopology(recBadJSON, reqBadJSON)
	if recBadJSON.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad JSON in topology POST, got: %d", recBadJSON.Code)
	}

	// 2. POST /api/v1/topology with valid scenario but invalid tailscale endpoint in activeTop -> 400
	srv.activeTop.TailscaleEndpoints.ControlPlane = "invalid host with spaces"
	recBadEndpoint := httptest.NewRecorder()
	reqBadEndpoint := httptest.NewRequest(http.MethodPost, "/api/v1/topology", strings.NewReader(`{"scenario": "local-hybrid"}`))
	srv.handleTopology(recBadEndpoint, reqBadEndpoint)
	if recBadEndpoint.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid endpoint topology config, got: %d", recBadEndpoint.Code)
	}

	// 3. GET /api/v1/telemetry/snapshot with nil registry
	recSnap := httptest.NewRecorder()
	reqSnap := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/snapshot", nil)
	srv.handleTelemetrySnapshot(recSnap, reqSnap)
	if recSnap.Code != http.StatusOK {
		t.Errorf("expected 200 on snapshot with nil registry, got: %d", recSnap.Code)
	}
	var frame TelemetryFrame
	_ = json.NewDecoder(recSnap.Body).Decode(&frame)
	if frame.ActiveWorkspace != "" {
		t.Errorf("expected empty active workspace for nil registry, got: %s", frame.ActiveWorkspace)
	}
}

func TestServer_ZeroCostHandlersExtended(t *testing.T) {
	srv := NewServer(ServerConfig{Port: 0}, nil, nil)

	// 1. POST bad JSON -> 400
	recPostBad := httptest.NewRecorder()
	srv.handleZeroCost(recPostBad, httptest.NewRequest(http.MethodPost, "/api/v1/inference/zero-cost", strings.NewReader("{bad")))
	if recPostBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad JSON in zero-cost POST, got: %d", recPostBad.Code)
	}

	// 2. POST empty name -> 400
	recPostEmpty := httptest.NewRecorder()
	srv.handleZeroCost(recPostEmpty, httptest.NewRequest(http.MethodPost, "/api/v1/inference/zero-cost", strings.NewReader(`{"name": "   "}`)))
	if recPostEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on empty name in zero-cost POST, got: %d", recPostEmpty.Code)
	}

	// 3. POST with is_prefix: true and sync_env: true -> 201
	recPostPrefix := httptest.NewRecorder()
	srv.handleZeroCost(recPostPrefix, httptest.NewRequest(http.MethodPost, "/api/v1/inference/zero-cost", strings.NewReader(`{"name": "custom-ext/", "is_prefix": true, "sync_env": true}`)))
	if recPostPrefix.Code != http.StatusCreated {
		t.Errorf("expected 201 on prefix addition, got: %d", recPostPrefix.Code)
	}

	// 4. DELETE bad JSON -> 400
	recDelBad := httptest.NewRecorder()
	srv.handleZeroCost(recDelBad, httptest.NewRequest(http.MethodDelete, "/api/v1/inference/zero-cost", strings.NewReader("{bad")))
	if recDelBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad JSON in zero-cost DELETE, got: %d", recDelBad.Code)
	}

	// 5. DELETE empty name -> 400
	recDelEmpty := httptest.NewRecorder()
	srv.handleZeroCost(recDelEmpty, httptest.NewRequest(http.MethodDelete, "/api/v1/inference/zero-cost", strings.NewReader(`{"name": "   "}`)))
	if recDelEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on empty name in zero-cost DELETE, got: %d", recDelEmpty.Code)
	}

	// 6. DELETE with is_prefix: true and sync_env: true -> 200
	recDelPrefix := httptest.NewRecorder()
	srv.handleZeroCost(recDelPrefix, httptest.NewRequest(http.MethodDelete, "/api/v1/inference/zero-cost", strings.NewReader(`{"name": "custom-ext/", "is_prefix": true, "sync_env": true}`)))
	if recDelPrefix.Code != http.StatusOK {
		t.Errorf("expected 200 on prefix removal, got: %d", recDelPrefix.Code)
	}

	// 7. PATCH -> 200
	recPatch := httptest.NewRecorder()
	srv.handleZeroCost(recPatch, httptest.NewRequest(http.MethodPatch, "/api/v1/inference/zero-cost", nil))
	if recPatch.Code != http.StatusOK {
		t.Errorf("expected 200 on PATCH zero-cost, got: %d", recPatch.Code)
	}

	// 8. Method not allowed (PUT) -> 405
	recPut := httptest.NewRecorder()
	srv.handleZeroCost(recPut, httptest.NewRequest(http.MethodPut, "/api/v1/inference/zero-cost", nil))
	if recPut.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on PUT zero-cost, got: %d", recPut.Code)
	}

	// 9. ZeroCostTest POST with body -> 200
	recTestPost := httptest.NewRecorder()
	srv.handleZeroCostTest(recTestPost, httptest.NewRequest(http.MethodPost, "/api/v1/inference/zero-cost/test", strings.NewReader(`{"model": "test-api-model"}`)))
	if recTestPost.Code != http.StatusOK {
		t.Errorf("expected 200 on zero-cost/test POST, got: %d", recTestPost.Code)
	}

	// 10. ZeroCostTest POST with invalid JSON body -> 400
	recTestBadPost := httptest.NewRecorder()
	srv.handleZeroCostTest(recTestBadPost, httptest.NewRequest(http.MethodPost, "/api/v1/inference/zero-cost/test", strings.NewReader("{bad")))
	if recTestBadPost.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad JSON in zero-cost/test POST, got: %d", recTestBadPost.Code)
	}

	// 11. ZeroCostTest GET without model -> 400
	recTestEmptyGet := httptest.NewRecorder()
	srv.handleZeroCostTest(recTestEmptyGet, httptest.NewRequest(http.MethodGet, "/api/v1/inference/zero-cost/test", nil))
	if recTestEmptyGet.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on empty model in zero-cost/test GET, got: %d", recTestEmptyGet.Code)
	}
}
