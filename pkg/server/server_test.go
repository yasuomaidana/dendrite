package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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

// dialWebSocket serves handler on a real httptest server, performs the
// WebSocket upgrade handshake over TCP and returns a reader positioned at the
// first frame. The connection is closed when the test ends.
func dialWebSocket(t *testing.T, handler http.HandlerFunc) *bufio.Reader {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial websocket server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, "GET /api/v1/telemetry/ws HTTP/1.1\r\n"+
		"Host: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"); err != nil {
		t.Fatalf("write upgrade request: %v", err)
	}
	r := bufio.NewReader(conn)
	status, err := r.ReadString('\n')
	if err != nil || !strings.Contains(status, "101") {
		t.Fatalf("expected 101 Switching Protocols, got %q (err %v)", status, err)
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read upgrade headers: %v", err)
		}
		if line == "\r\n" {
			return r
		}
	}
}

func TestServer_LifecycleAndEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.json")
	dataDir := filepath.Join(tempDir, "data")
	reg, err := workspace.NewRegistry(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}

	p1Dir := filepath.Join(tempDir, "proj-1")
	_ = os.MkdirAll(p1Dir, 0755)

	customHub := NewTelemetryHub()
	srv := NewServer(ServerConfig{
		Host: "127.0.0.1",
		Port: 0, // OS assigned ephemeral port
	}, reg, customHub)

	if srv.GetHub() != customHub {
		t.Errorf("expected GetHub to return customHub")
	}

	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	addr := srv.Addr()
	if addr == "" {
		t.Fatal("expected non-empty server address")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	baseURL := fmt.Sprintf("http://%s", addr)

	// 1. GET /
	resp, err := client.Get(baseURL + "/")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / failed: %v, status: %v", err, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Dendrite Control Plane") {
		t.Errorf("unexpected HTML body: %s", string(body))
	}

	// 2. GET /notfound -> 404
	resp404, _ := client.Get(baseURL + "/notfound")
	if resp404.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown path, got: %d", resp404.StatusCode)
	}

	// 3. GET /api/v1/health
	respHealth, err := client.Get(baseURL + "/api/v1/health")
	if err != nil || respHealth.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/health failed: %v", err)
	}
	var healthData map[string]interface{}
	_ = json.NewDecoder(respHealth.Body).Decode(&healthData)
	if healthData["status"] != "healthy" {
		t.Errorf("expected healthy status, got: %v", healthData)
	}

	// 4. POST /api/v1/workspaces
	addBody := `{"name": "proj-alpha", "path": "` + p1Dir + `"}`
	respAdd, err := client.Post(baseURL+"/api/v1/workspaces", "application/json", strings.NewReader(addBody))
	if err != nil || respAdd.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/workspaces failed: %v", err)
	}

	// 5. GET /api/v1/workspaces
	respList, err := client.Get(baseURL + "/api/v1/workspaces")
	if err != nil || respList.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/workspaces failed: %v", err)
	}
	var listData map[string]interface{}
	_ = json.NewDecoder(respList.Body).Decode(&listData)
	wsList := listData["workspaces"].([]interface{})
	if len(wsList) != 1 {
		t.Errorf("expected 1 workspace in list, got: %d", len(wsList))
	}

	// 6. POST /api/v1/workspaces/switch
	switchBody := `{"name": "proj-alpha"}`
	respSwitch, err := client.Post(baseURL+"/api/v1/workspaces/switch", "application/json", strings.NewReader(switchBody))
	if err != nil || respSwitch.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/workspaces/switch failed: %v", err)
	}

	// 7. GET /api/v1/topology
	respTop, err := client.Get(baseURL + "/api/v1/topology")
	if err != nil || respTop.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/topology failed: %v", err)
	}

	// 8. POST /api/v1/topology
	topBody := `{"scenario": "azure-burst"}`
	respTopPost, err := client.Post(baseURL+"/api/v1/topology", "application/json", strings.NewReader(topBody))
	if err != nil || respTopPost.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/topology failed: %v", err)
	}

	// 9. GET /api/v1/telemetry/snapshot
	respSnap, err := client.Get(baseURL + "/api/v1/telemetry/snapshot")
	if err != nil || respSnap.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/telemetry/snapshot failed: %v", err)
	}
	var snap TelemetryFrame
	_ = json.NewDecoder(respSnap.Body).Decode(&snap)
	if snap.ActiveWorkspace != "proj-alpha" || snap.ActiveScenario != "azure-burst" {
		t.Errorf("unexpected snapshot data: %+v", snap)
	}

	// 10. GET /metrics
	respMetrics, err := client.Get(baseURL + "/metrics")
	if err != nil || respMetrics.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics failed: %v, status: %v", err, respMetrics.StatusCode)
	}
	metricsBody, _ := io.ReadAll(respMetrics.Body)
	if !strings.Contains(string(metricsBody), "dendrite_soc_temperature_celsius") {
		t.Errorf("expected prometheus metrics, got:\n%s", string(metricsBody))
	}
}

func TestServer_AuthAndErrors(t *testing.T) {
	srv := NewServer(ServerConfig{
		Host:      "127.0.0.1",
		Port:      0,
		AuthToken: "super-secret-token",
	}, nil, nil)

	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	baseURL := fmt.Sprintf("http://%s", srv.Addr())
	client := &http.Client{Timeout: 2 * time.Second}

	// 1. Unauthorized
	respUnauth, _ := client.Get(baseURL + "/api/v1/health")
	if respUnauth.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got: %d", respUnauth.StatusCode)
	}

	// 2. Authorized via Bearer header
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer super-secret-token")
	respAuth, err := client.Do(req)
	if err != nil || respAuth.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK with Bearer token, got: %v", err)
	}

	// 3. Authorized via query param
	respQuery, err := client.Get(baseURL + "/api/v1/health?token=super-secret-token")
	if err != nil || respQuery.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK with query param, got: %v", err)
	}

	// 4. Test OPTIONS preflight
	reqOpt, _ := http.NewRequest(http.MethodOptions, baseURL+"/api/v1/health", nil)
	respOpt, _ := client.Do(reqOpt)
	if respOpt.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK on OPTIONS, got: %d", respOpt.StatusCode)
	}

	// 5. Test invalid JSON in workspace switch
	reqInvalid, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/workspaces/switch?token=super-secret-token", strings.NewReader("{invalid"))
	respInvalid, _ := client.Do(reqInvalid)
	if respInvalid.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on invalid JSON, got: %d", respInvalid.StatusCode)
	}
}

func TestServer_HandlerBranches(t *testing.T) {
	// Server with nil registry
	srvNoReg := NewServer(ServerConfig{Port: 0}, nil, nil)
	rec := httptest.NewRecorder()

	// GET workspaces with nil registry
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	srvNoReg.handleWorkspaces(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with empty list, got: %d", rec.Code)
	}

	// Switch workspace with nil registry
	recSwitch := httptest.NewRecorder()
	reqSwitch := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/switch", strings.NewReader(`{"name": "foo"}`))
	srvNoReg.handleWorkspaceSwitch(recSwitch, reqSwitch)
	if recSwitch.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when registry is nil, got: %d", recSwitch.Code)
	}

	// Method not allowed tests
	recMethod := httptest.NewRecorder()
	srvNoReg.handleWorkspaces(recMethod, httptest.NewRequest(http.MethodPut, "/api/v1/workspaces", nil))
	if recMethod.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on PUT workspaces, got: %d", recMethod.Code)
	}

	recSwitchMethod := httptest.NewRecorder()
	srvNoReg.handleWorkspaceSwitch(recSwitchMethod, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/switch", nil))
	if recSwitchMethod.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on GET switch, got: %d", recSwitchMethod.Code)
	}

	recTopMethod := httptest.NewRecorder()
	srvNoReg.handleTopology(recTopMethod, httptest.NewRequest(http.MethodDelete, "/api/v1/topology", nil))
	if recTopMethod.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on DELETE topology, got: %d", recTopMethod.Code)
	}

	// Invalid scenario POST
	recTopBad := httptest.NewRecorder()
	srvNoReg.handleTopology(recTopBad, httptest.NewRequest(http.MethodPost, "/api/v1/topology", strings.NewReader(`{"scenario": "unknown-scenario"}`)))
	if recTopBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad scenario, got: %d", recTopBad.Code)
	}

	// Metrics endpoint test
	if srvNoReg.Exporter() == nil {
		t.Errorf("expected default exporter to be non-nil")
	}
	srvNoReg.SetExporter(nil)
	if srvNoReg.Exporter() != nil {
		t.Errorf("expected nil exporter after SetExporter(nil)")
	}
	recMetricsErr := httptest.NewRecorder()
	srvNoReg.handleMetrics(recMetricsErr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recMetricsErr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when exporter is nil, got: %d", recMetricsErr.Code)
	}
}

func TestTelemetryHub_BroadcastAndFrames(t *testing.T) {
	hub := NewTelemetryHub()

	ch, unsub := hub.Subscribe()
	defer unsub()

	testFrame := TelemetryFrame{
		Timestamp:       time.Now(),
		ActiveScenario:  "local-hybrid",
		SoCTemperatureC: 55.0,
		ThermalAlert:    false,
		TokenSpend24h:   5.0,
	}

	hub.Broadcast(testFrame)

	select {
	case received := <-ch:
		if received.ActiveScenario != "local-hybrid" || received.SoCTemperatureC != 55.0 {
			t.Errorf("unexpected frame received: %+v", received)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for broadcast frame")
	}

	// Test writeWSTextFrame payloads of various sizes
	var bufSmall bytes.Buffer
	if err := writeWSTextFrame(&bufSmall, []byte("small payload")); err != nil {
		t.Fatalf("small payload frame failed: %v", err)
	}

	var bufMedium bytes.Buffer
	mediumPayload := make([]byte, 500)
	if err := writeWSTextFrame(&bufMedium, mediumPayload); err != nil {
		t.Fatalf("medium payload frame failed: %v", err)
	}

	var bufLarge bytes.Buffer
	largePayload := make([]byte, 70000)
	if err := writeWSTextFrame(&bufLarge, largePayload); err != nil {
		t.Fatalf("large payload frame failed: %v", err)
	}

	// Test WebSocket bad request without header
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/ws", nil)
	hub.HandleWebSocket(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request without WS key, got: %d", rec.Code)
	}

	// Test WebSocket upgrade over a real TCP connection
	wsReader := dialWebSocket(t, hub.HandleWebSocket)

	// Broadcast another frame to write into websocket conn
	hub.Broadcast(testFrame)
	if op, err := wsReader.ReadByte(); err != nil || op != 0x81 {
		t.Errorf("expected text frame opcode 0x81, got %#x (err %v)", op, err)
	}

	// Test SSE streaming with context cancel
	ctx, cancel := context.WithCancel(context.Background())
	reqSSE := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/events", nil).WithContext(ctx)
	recSSE := httptest.NewRecorder()

	go func() {
		time.Sleep(50 * time.Millisecond)
		hub.Broadcast(testFrame)
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	hub.HandleSSE(recSSE, reqSSE)
	if !strings.Contains(recSSE.Body.String(), "data: ") {
		t.Errorf("expected SSE data in stream, got: %s", recSSE.Body.String())
	}
}

func TestServer_ZeroCostEndpoints(t *testing.T) {
	srv := NewServer(ServerConfig{Host: "127.0.0.1", Port: 0}, nil, nil)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	baseURL := fmt.Sprintf("http://%s", srv.Addr())
	client := &http.Client{Timeout: 3 * time.Second}

	// 1. GET /api/v1/inference/zero-cost
	resp, err := client.Get(baseURL + "/api/v1/inference/zero-cost")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/inference/zero-cost failed: %v, status: %v", err, resp.StatusCode)
	}
	var details map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if details["total_active"].(float64) <= 0 {
		t.Errorf("expected positive total_active models, got: %v", details["total_active"])
	}

	// 2. POST /api/v1/inference/zero-cost (add custom model)
	addReq := `{"name": "test-api-model", "is_prefix": false}`
	respAdd, err := client.Post(baseURL+"/api/v1/inference/zero-cost", "application/json", strings.NewReader(addReq))
	if err != nil || respAdd.StatusCode != http.StatusCreated {
		t.Fatalf("POST add failed: %v, status: %v", err, respAdd.StatusCode)
	}

	// 3. GET /api/v1/inference/zero-cost/test
	respTest, err := client.Get(baseURL + "/api/v1/inference/zero-cost/test?model=test-api-model")
	if err != nil || respTest.StatusCode != http.StatusOK {
		t.Fatalf("GET test failed: %v, status: %v", err, respTest.StatusCode)
	}
	var testResult map[string]interface{}
	_ = json.NewDecoder(respTest.Body).Decode(&testResult)
	if testResult["zero_cost"] != true {
		t.Errorf("expected test-api-model to be zero cost, got: %v", testResult)
	}

	// 4. Test commercial model via /test
	respComm, err := client.Get(baseURL + "/api/v1/inference/zero-cost/test?model=copilot-sonnet")
	if err != nil || respComm.StatusCode != http.StatusOK {
		t.Fatalf("GET test commercial failed: %v, status: %v", err, respComm.StatusCode)
	}
	var commResult map[string]interface{}
	_ = json.NewDecoder(respComm.Body).Decode(&commResult)
	if commResult["zero_cost"] != false {
		t.Errorf("expected copilot-sonnet to NOT be zero cost, got: %v", commResult)
	}

	// 5. DELETE /api/v1/inference/zero-cost
	delReq, _ := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/inference/zero-cost", strings.NewReader(`{"name": "test-api-model"}`))
	delReq.Header.Set("Content-Type", "application/json")
	respDel, err := client.Do(delReq)
	if err != nil || respDel.StatusCode != http.StatusOK {
		t.Fatalf("DELETE failed: %v, status: %v", err, respDel.StatusCode)
	}
}

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

func TestServer_RAGEndpointsExtended(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	docFile := filepath.Join(tmpDir, "arch.md")
	docContent := "# Architecture\nDendrite RAG server provides context retrieval."
	if err := os.WriteFile(docFile, []byte(docContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := ServerConfig{Port: 0, Host: "127.0.0.1"}
	s := NewServer(cfg, nil, nil)
	rec := httptest.NewRecorder()

	// 1. GET /api/v1/rag/ask -> 405
	reqAskGet := httptest.NewRequest(http.MethodGet, "/api/v1/rag/ask", nil)
	s.handleRAGAsk(rec, reqAskGet)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 on GET ask, got: %d", rec.Code)
	}

	// 2. POST /api/v1/rag/ask bad JSON -> 400
	recBadJSON := httptest.NewRecorder()
	reqAskBadJSON := httptest.NewRequest(http.MethodPost, "/api/v1/rag/ask", strings.NewReader("{invalid"))
	s.handleRAGAsk(recBadJSON, reqAskBadJSON)
	if recBadJSON.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid JSON in ask, got: %d", recBadJSON.Code)
	}

	// 3. POST /api/v1/rag/ask failure (via unreachable server_url) -> 500
	recAskErr := httptest.NewRecorder()
	askErrBody := fmt.Sprintf(`{"question": "How to run?", "project_path": "%s", "server_url": "http://127.0.0.1:1"}`, tmpDir)
	reqAskErr := httptest.NewRequest(http.MethodPost, "/api/v1/rag/ask", strings.NewReader(askErrBody))
	s.handleRAGAsk(recAskErr, reqAskErr)
	if recAskErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on ask failure, got: %d", recAskErr.Code)
	}

	// 4. POST /api/v1/rag/ask success via test gateway server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-test",
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "Answer from test gateway"}},
			},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer ts.Close()
	t.Setenv("LITELLM_PROXY_URL", ts.URL)

	recAskSuccess := httptest.NewRecorder()
	askBody := fmt.Sprintf(`{"question": "How to run?", "project_path": "%s", "top_k": 2, "paid_model": "test-paid", "force_paid": true}`, tmpDir)
	reqAskSuccess := httptest.NewRequest(http.MethodPost, "/api/v1/rag/ask", strings.NewReader(askBody))
	s.handleRAGAsk(recAskSuccess, reqAskSuccess)
	if recAskSuccess.Code != http.StatusOK {
		t.Errorf("expected 200 on successful ask, got: %d (%s)", recAskSuccess.Code, recAskSuccess.Body.String())
	}

	// 5. POST /api/v1/rag/retrieve bad JSON -> 400
	recRetBadJSON := httptest.NewRecorder()
	reqRetBadJSON := httptest.NewRequest(http.MethodPost, "/api/v1/rag/retrieve", strings.NewReader("{invalid"))
	s.handleRAGRetrieve(recRetBadJSON, reqRetBadJSON)
	if recRetBadJSON.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid JSON in retrieve, got: %d", recRetBadJSON.Code)
	}

	// 6. POST /api/v1/rag/retrieve with defaults (ProjectPath == "", TopK <= 0, MinScore <= 0)
	// Because t.Chdir(tmpDir) was called, default project_path "." refers to tmpDir
	recRetDefaults := httptest.NewRecorder()
	reqRetDefaults := httptest.NewRequest(http.MethodPost, "/api/v1/rag/retrieve", strings.NewReader(`{"query": "architecture"}`))
	s.handleRAGRetrieve(recRetDefaults, reqRetDefaults)
	if recRetDefaults.Code != http.StatusOK {
		t.Errorf("expected 200 on retrieve defaults, got: %d", recRetDefaults.Code)
	}

	// 7. POST /api/v1/rag/retrieve with nonexistent project path -> 500
	recRetPathErr := httptest.NewRecorder()
	badPath := filepath.Join(tmpDir, "nonexistent-dir")
	reqRetPathErr := httptest.NewRequest(http.MethodPost, "/api/v1/rag/retrieve", strings.NewReader(fmt.Sprintf(`{"query": "architecture", "project_path": "%s"}`, badPath)))
	s.handleRAGRetrieve(recRetPathErr, reqRetPathErr)
	if recRetPathErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on nonexistent path retrieve, got: %d", recRetPathErr.Code)
	}

	// 8. POST /api/v1/rag/retrieve with canceled context on single file -> 500
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recRetCancel := httptest.NewRecorder()
	reqRetCancel := httptest.NewRequest(http.MethodPost, "/api/v1/rag/retrieve", strings.NewReader(fmt.Sprintf(`{"query": "architecture", "project_path": "%s"}`, docFile))).WithContext(ctx)
	s.handleRAGRetrieve(recRetCancel, reqRetCancel)
	if recRetCancel.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on canceled retrieve, got: %d", recRetCancel.Code)
	}
}

func TestTelemetryHub_ExtendedBranches(t *testing.T) {
	// 1. Subscribe when lastFrame != nil
	hub1 := NewTelemetryHub()
	initialFrame := TelemetryFrame{
		ActiveScenario:  "local-hybrid",
		SoCTemperatureC: 50.0,
	}
	hub1.Broadcast(initialFrame)

	ch, unsub := hub1.Subscribe()
	select {
	case received := <-ch:
		if received.SoCTemperatureC != 50.0 {
			t.Errorf("expected initial frame, got: %+v", received)
		}
	default:
		t.Errorf("expected immediate frame from Subscribe when lastFrame != nil")
	}

	// Unsubscribe twice
	unsub()
	unsub()

	// 2. Channel overflow on Broadcast
	hub2 := NewTelemetryHub()
	overflowCh, unsubOverflow := hub2.Subscribe()
	defer unsubOverflow()
	for i := 0; i < 35; i++ {
		hub2.Broadcast(TelemetryFrame{SoCTemperatureC: float64(i)})
	}
	if len(overflowCh) != 32 {
		t.Errorf("expected channel to cap at 32 items, got: %d", len(overflowCh))
	}

	// 3. Broadcast with broken WebSocket conn (exercises error on writeWSTextFrame, close, delete)
	hubBroken := NewTelemetryHub()
	sConn, cConn := net.Pipe()
	_ = sConn.Close()
	_ = cConn.Close()
	hubBroken.mu.Lock()
	hubBroken.wsConns[sConn] = struct{}{}
	hubBroken.mu.Unlock()

	hubBroken.Broadcast(TelemetryFrame{ActiveScenario: "azure-burst"})
	hubBroken.mu.RLock()
	connsLeft := len(hubBroken.wsConns)
	hubBroken.mu.RUnlock()
	if connsLeft != 0 {
		t.Errorf("expected broken conn to be removed from wsConns, got %d left", connsLeft)
	}

	// 4. Broadcast with math.NaN() (exercises json.Marshal error)
	hubNaN := NewTelemetryHub()
	hubNaN.Broadcast(TelemetryFrame{SoCTemperatureC: math.NaN()})

	// 5. HandleWebSocket with non-hijacker recorder -> 500
	hubWS := NewTelemetryHub()
	recNonHijack := httptest.NewRecorder()
	reqWS := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/ws", nil)
	reqWS.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	hubWS.HandleWebSocket(recNonHijack, reqWS)
	if recNonHijack.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when hijacker unsupported, got: %d", recNonHijack.Code)
	}

	// 6. HandleWebSocket with failing Hijack() -> 500
	failHijacker := &testHijackFailRecorder{ResponseRecorder: httptest.NewRecorder()}
	hubWS.HandleWebSocket(failHijacker, reqWS)
	if failHijacker.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on failing hijack, got: %d", failHijacker.Code)
	}

	// 7. HandleWebSocket with small buffer on closed conn -> exercises bufrw.WriteString error
	pipeS, pipeC := net.Pipe()
	_ = pipeS.Close()
	_ = pipeC.Close()
	smallBufHijacker := &testSmallBufHijacker{
		ResponseRecorder: httptest.NewRecorder(),
		serverConn:       pipeS,
		rw:               bufio.NewReadWriter(bufio.NewReader(pipeS), bufio.NewWriterSize(pipeS, 10)),
	}
	hubWS.HandleWebSocket(smallBufHijacker, reqWS)

	// 8. HandleWebSocket with lastFrame containing math.NaN() -> exercises json.Marshal failure
	hubWSNaN := NewTelemetryHub()
	hubWSNaN.lastFrame = &TelemetryFrame{SoCTemperatureC: math.NaN()}
	_ = dialWebSocket(t, hubWSNaN.HandleWebSocket)

	// 9. HandleSSE with non-flusher -> 500
	hubSSE := NewTelemetryHub()
	nonFlusher := &testNonFlusherRecorder{}
	hubSSE.HandleSSE(nonFlusher, httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/events", nil))
	if nonFlusher.code != http.StatusInternalServerError {
		t.Errorf("expected 500 on non-flusher SSE, got: %d", nonFlusher.code)
	}

	// 10. HandleSSE with math.NaN() frame and normal frame
	hubSSENaN := NewTelemetryHub()
	recSSE := httptest.NewRecorder()
	ctxSSE, cancelSSE := context.WithCancel(context.Background())
	reqSSE := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/events", nil).WithContext(ctxSSE)
	sseDone := make(chan struct{})
	go func() {
		defer close(sseDone)
		hubSSENaN.HandleSSE(recSSE, reqSSE)
	}()

	for {
		hubSSENaN.mu.Lock()
		subCount := len(hubSSENaN.subscribers)
		hubSSENaN.mu.Unlock()
		if subCount > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	hubSSENaN.Broadcast(TelemetryFrame{SoCTemperatureC: math.NaN()})
	hubSSENaN.Broadcast(TelemetryFrame{ActiveScenario: "nan-test-scenario", SoCTemperatureC: 50.0})
	time.Sleep(20 * time.Millisecond)
	cancelSSE()

	select {
	case <-sseDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE to finish")
	}
	if !strings.Contains(recSSE.Body.String(), "nan-test-scenario") {
		t.Errorf("expected nan-test-scenario in SSE output, got: %s", recSSE.Body.String())
	}

	// 11. HandleSSE exits when subscriber channel is closed
	hubSSEClose := NewTelemetryHub()
	recSSEClose := httptest.NewRecorder()
	reqSSEClose := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/events", nil)
	sseCloseDone := make(chan struct{})
	go func() {
		defer close(sseCloseDone)
		hubSSEClose.HandleSSE(recSSEClose, reqSSEClose)
	}()

	for {
		hubSSEClose.mu.Lock()
		subCount := len(hubSSEClose.subscribers)
		hubSSEClose.mu.Unlock()
		if subCount > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	hubSSEClose.mu.Lock()
	for ch := range hubSSEClose.subscribers {
		close(ch)
		delete(hubSSEClose.subscribers, ch)
		break
	}
	hubSSEClose.mu.Unlock()

	select {
	case <-sseCloseDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE to exit on channel close")
	}

	// 12. writeWSTextFrame error branches
	failWriterHeader := &testFailingWriter{allowedBytes: 0}
	if err := writeWSTextFrame(failWriterHeader, []byte("payload")); err == nil {
		t.Errorf("expected error writing header to failWriterHeader, got nil")
	}
	failWriterPayload := &testFailingWriter{allowedBytes: 2}
	if err := writeWSTextFrame(failWriterPayload, []byte("payload")); err == nil {
		t.Errorf("expected error writing payload to failWriterPayload, got nil")
	}
}
