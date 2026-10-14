package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type mockHijacker struct {
	*httptest.ResponseRecorder
	serverConn net.Conn
	clientConn net.Conn
	rw         *bufio.ReadWriter
}

func newMockHijacker() *mockHijacker {
	sConn, cConn := net.Pipe()
	return &mockHijacker{
		ResponseRecorder: httptest.NewRecorder(),
		serverConn:       sConn,
		clientConn:       cConn,
		rw:               bufio.NewReadWriter(bufio.NewReader(sConn), bufio.NewWriter(sConn)),
	}
}

func (m *mockHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.serverConn, m.rw, nil
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

	// Test WebSocket with mock hijacker
	hijacker := newMockHijacker()
	defer hijacker.serverConn.Close()
	defer hijacker.clientConn.Close()

	wsReq := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/ws", nil)
	wsReq.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	go func() {
		// Continuously drain clientConn
		buf := make([]byte, 1024)
		for {
			if _, err := hijacker.clientConn.Read(buf); err != nil {
				return
			}
		}
	}()

	hub.HandleWebSocket(hijacker, wsReq)

	// Broadcast another frame to write into websocket conn
	hub.Broadcast(testFrame)

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
