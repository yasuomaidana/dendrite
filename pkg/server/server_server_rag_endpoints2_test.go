package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
