package jetbrains_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"Dendrite/pkg/jetbrains"
)

func TestClient_Lifecycle_And_Tools(t *testing.T) {
	ctx := context.Background()
	mockTransport := jetbrains.NewMockTransport()

	client, err := jetbrains.NewClient(mockTransport,
		jetbrains.WithClientInfo("test-agent", "1.0.0"),
		jetbrains.WithProtocolVersion(jetbrains.DefaultProtocolVersion),
	)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}

	if client.IsConnected() {
		t.Fatal("expected client to not be connected initially")
	}

	// 1. Connect Handshake
	initRes, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	if !client.IsConnected() {
		t.Fatal("expected client to be connected after Connect()")
	}

	if initRes.ProtocolVersion != jetbrains.DefaultProtocolVersion {
		t.Errorf("expected protocol version %s, got %s", jetbrains.DefaultProtocolVersion, initRes.ProtocolVersion)
	}

	if client.GetServerInfo() == nil || client.GetServerInfo().Name != "JetBrains GoLand MCP Server" {
		t.Errorf("unexpected server info: %+v", client.GetServerInfo())
	}

	if client.GetServerCapabilities() == nil || client.GetServerCapabilities().Tools == nil {
		t.Error("expected server capabilities to include tools")
	}

	// Connect again should return ErrAlreadyConnected
	if _, err := client.Connect(ctx); !errors.Is(err, jetbrains.ErrAlreadyConnected) {
		t.Errorf("expected ErrAlreadyConnected, got: %v", err)
	}

	// 2. Ping
	latency, err := client.Ping(ctx)
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if latency < 0 {
		t.Errorf("invalid ping latency: %v", latency)
	}

	// 3. List Tools
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected non-empty tool list")
	}

	foundOpen := false
	for _, tool := range tools {
		if tool.Name == "open_file_in_editor" {
			foundOpen = true
			break
		}
	}
	if !foundOpen {
		t.Error("expected 'open_file_in_editor' tool in list")
	}

	// 4. Call Tool - Happy Path
	callRes, err := client.CallTool(ctx, "open_file_in_editor", map[string]interface{}{
		"file_path": "main.go",
		"line":      10,
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if len(callRes.Content) == 0 || callRes.Content[0].Text != "File opened successfully" {
		t.Errorf("unexpected call tool content: %+v", callRes.Content)
	}

	// 5. Call Tool - Unknown Tool
	_, err = client.CallTool(ctx, "non_existent_tool", nil)
	if !errors.Is(err, jetbrains.ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}

	// 6. Call Tool - Empty Name
	_, err = client.CallTool(ctx, "", nil)
	if !errors.Is(err, jetbrains.ErrEmptyToolName) {
		t.Errorf("expected ErrEmptyToolName, got: %v", err)
	}

	// 7. Disconnect
	if err := client.Disconnect(); err != nil {
		t.Fatalf("disconnect failed: %v", err)
	}
	if client.IsConnected() {
		t.Fatal("expected client to be disconnected")
	}

	// Double disconnect should be idempotent
	if err := client.Disconnect(); err != nil {
		t.Errorf("expected no error on duplicate disconnect, got: %v", err)
	}

	// Operations after disconnect should fail
	if _, err := client.ListTools(ctx); !errors.Is(err, jetbrains.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
	if _, err := client.CallTool(ctx, "open_file_in_editor", nil); !errors.Is(err, jetbrains.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
	if _, err := client.Ping(ctx); !errors.Is(err, jetbrains.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestClient_NilTransport(t *testing.T) {
	_, err := jetbrains.NewClient(nil)
	if !errors.Is(err, jetbrains.ErrNilTransport) {
		t.Errorf("expected ErrNilTransport, got: %v", err)
	}
}

func TestClient_ErrorResponses(t *testing.T) {
	ctx := context.Background()
	mock := jetbrains.NewMockTransport()

	// Handler returning JSON-RPC error
	mock.RegisterHandler("initialize", func(req *jetbrains.JSONRPCRequest) (*jetbrains.JSONRPCResponse, error) {
		return &jetbrains.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jetbrains.JSONRPCError{
				Code:    -32000,
				Message: "handshake rejected",
			},
		}, nil
	})

	client, err := jetbrains.NewClient(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.Connect(ctx)
	if !errors.Is(err, jetbrains.ErrInvalidResponse) {
		t.Errorf("expected ErrInvalidResponse on handshake rejection, got: %v", err)
	}
}

func TestClient_ListTools_And_Ping_Errors(t *testing.T) {
	ctx := context.Background()
	mock := jetbrains.NewMockTransport()

	client, err := jetbrains.NewClient(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, _ = client.Connect(ctx)

	// ListTools returns JSON-RPC error
	mock.RegisterHandler("tools/list", func(req *jetbrains.JSONRPCRequest) (*jetbrains.JSONRPCResponse, error) {
		return &jetbrains.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jetbrains.JSONRPCError{
				Code:    -32603,
				Message: "failed listing tools",
			},
		}, nil
	})

	if _, err := client.ListTools(ctx); !errors.Is(err, jetbrains.ErrInvalidResponse) {
		t.Errorf("expected ErrInvalidResponse from ListTools error, got: %v", err)
	}

	// Ping returns JSON-RPC error
	mock.RegisterHandler("ping", func(req *jetbrains.JSONRPCRequest) (*jetbrains.JSONRPCResponse, error) {
		return &jetbrains.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jetbrains.JSONRPCError{
				Code:    -32000,
				Message: "ping failed",
			},
		}, nil
	})

	if _, err := client.Ping(ctx); !errors.Is(err, jetbrains.ErrInvalidResponse) {
		t.Errorf("expected ErrInvalidResponse from Ping error, got: %v", err)
	}
}

func TestClient_ToolExecutionFailure(t *testing.T) {
	ctx := context.Background()
	mock := jetbrains.NewMockTransport()

	mock.RegisterHandler("tools/call", func(req *jetbrains.JSONRPCRequest) (*jetbrains.JSONRPCResponse, error) {
		res := jetbrains.CallToolResult{
			IsError: true,
			Content: []jetbrains.ContentItem{{Type: "text", Text: "Internal compilation failure"}},
		}
		data, _ := json.Marshal(res)
		return &jetbrains.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  data,
		}, nil
	})

	client, err := jetbrains.NewClient(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, _ = client.Connect(ctx)

	_, err = client.CallTool(ctx, "build_project", nil)
	if !errors.Is(err, jetbrains.ErrExecutionFailed) {
		t.Errorf("expected ErrExecutionFailed, got: %v", err)
	}
}

func TestBridge_HighLevelMethods(t *testing.T) {
	ctx := context.Background()
	mock := jetbrains.NewMockTransport()
	client, err := jetbrains.NewClient(mock)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	if _, err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	bridge := jetbrains.NewBridge(client)
	if bridge.GetClient() != client {
		t.Error("bridge.GetClient() mismatch")
	}

	// 1. OpenFile
	if err := bridge.OpenFile(ctx, "pkg/jetbrains/bridge.go", 15, 2); err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	// Line and col clamping (< 1)
	if err := bridge.OpenFile(ctx, "pkg/jetbrains/bridge.go", 0, 0); err != nil {
		t.Fatalf("OpenFile with line/col 0 failed: %v", err)
	}
	if err := bridge.OpenFile(ctx, "", 1, 1); !errors.Is(err, jetbrains.ErrEmptyFilePath) {
		t.Errorf("expected ErrEmptyFilePath, got: %v", err)
	}

	// 2. GetFileProblems
	problems, err := bridge.GetFileProblems(ctx, "main.go")
	if err != nil {
		t.Fatalf("GetFileProblems failed: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("expected problems for main.go")
	}
	if problems[0].Severity != "warning" {
		t.Errorf("expected severity 'warning', got %s", problems[0].Severity)
	}
	if _, err := bridge.GetFileProblems(ctx, "   "); !errors.Is(err, jetbrains.ErrEmptyFilePath) {
		t.Errorf("expected ErrEmptyFilePath, got: %v", err)
	}

	// 3. SearchSymbol
	symbols, err := bridge.SearchSymbol(ctx, "NewClient")
	if err != nil {
		t.Fatalf("SearchSymbol failed: %v", err)
	}
	if len(symbols) == 0 || symbols[0].Name != "NewClient" {
		t.Errorf("unexpected symbols result: %+v", symbols)
	}

	// 4. BuildProject
	buildRes, err := bridge.BuildProject(ctx)
	if err != nil {
		t.Fatalf("BuildProject failed: %v", err)
	}
	if !buildRes.Success {
		t.Error("expected build to succeed")
	}

	// 5. ExecuteRunConfiguration
	output, err := bridge.ExecuteRunConfiguration(ctx, "UnitTestSuite")
	if err != nil {
		t.Fatalf("ExecuteRunConfiguration failed: %v", err)
	}
	if output == "" {
		t.Error("expected non-empty output from run configuration")
	}
	if _, err := bridge.ExecuteRunConfiguration(ctx, ""); err == nil {
		t.Error("expected error for empty configuration name")
	}

	// 6. LintFiles
	lintProblems, err := bridge.LintFiles(ctx, []string{"main.go"})
	if err != nil {
		t.Fatalf("LintFiles failed: %v", err)
	}
	if len(lintProblems) != 0 {
		t.Errorf("expected 0 lint problems for mock, got %d", len(lintProblems))
	}
}

func TestBridge_EmptyAndFallbackContent(t *testing.T) {
	ctx := context.Background()
	mock := jetbrains.NewMockTransport()

	mock.RegisterHandler("tools/call", func(req *jetbrains.JSONRPCRequest) (*jetbrains.JSONRPCResponse, error) {
		paramsBytes, _ := json.Marshal(req.Params)
		var callParams jetbrains.CallToolParams
		_ = json.Unmarshal(paramsBytes, &callParams)

		switch callParams.Name {
		case "get_file_problems":
			// Plaintext non-JSON fallback
			res := jetbrains.CallToolResult{
				Content: []jetbrains.ContentItem{{Type: "text", Text: "Syntax error on line 5"}},
			}
			b, _ := json.Marshal(res)
			return &jetbrains.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: b}, nil
		case "search_symbol":
			// Empty content
			res := jetbrains.CallToolResult{Content: []jetbrains.ContentItem{}}
			b, _ := json.Marshal(res)
			return &jetbrains.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: b}, nil
		case "lint_files":
			// Non-JSON or empty
			res := jetbrains.CallToolResult{Content: []jetbrains.ContentItem{{Type: "text", Text: ""}}}
			b, _ := json.Marshal(res)
			return &jetbrains.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: b}, nil
		default:
			return nil, errors.New("unhandled")
		}
	})

	client, err := jetbrains.NewClient(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, _ = client.Connect(ctx)
	bridge := jetbrains.NewBridge(client)

	// Plain text problems fallback
	probs, err := bridge.GetFileProblems(ctx, "test.go")
	if err != nil {
		t.Fatalf("GetFileProblems failed: %v", err)
	}
	if len(probs) != 1 || probs[0].Description != "Syntax error on line 5" {
		t.Errorf("unexpected fallback problems: %+v", probs)
	}

	// Empty symbols
	syms, err := bridge.SearchSymbol(ctx, "Missing")
	if err != nil {
		t.Fatalf("SearchSymbol failed: %v", err)
	}
	if len(syms) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(syms))
	}

	// Empty lint files
	lints, err := bridge.LintFiles(ctx, []string{"test.go"})
	if err != nil {
		t.Fatalf("LintFiles failed: %v", err)
	}
	if len(lints) != 0 {
		t.Errorf("expected 0 lints, got %d", len(lints))
	}
}

func TestHTTPTransport_SendAndDecode(t *testing.T) {
	ctx := context.Background()

	// Spin up test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req jetbrains.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		res := jetbrains.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"status":"ok"}`),
		}
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer server.Close()

	// Test default parameters constructor
	defaultTransport := jetbrains.NewHTTPTransport("", nil)
	_ = defaultTransport.Close()

	httpTransport := jetbrains.NewHTTPTransport(server.URL, server.Client())
	defer func() { _ = httpTransport.Close() }()

	req := &jetbrains.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test_method",
	}

	resp, err := httpTransport.Send(ctx, req)
	if err != nil {
		t.Fatalf("http transport Send failed: %v", err)
	}
	if string(resp.Result) != `{"status":"ok"}` {
		t.Errorf("unexpected result: %s", string(resp.Result))
	}
}

func TestHTTPTransport_ErrorStatus(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	httpTransport := jetbrains.NewHTTPTransport(server.URL, server.Client())
	req := &jetbrains.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "fail"}

	_, err := httpTransport.Send(ctx, req)
	if err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}

func TestStdioTransport_Echo(t *testing.T) {
	ctx := context.Background()

	// Use 'cat' on mac/unix as a loopback stdio JSON-RPC process
	stdio, err := jetbrains.NewStdioTransport("cat")
	if err != nil {
		t.Fatalf("failed to start stdio cat: %v", err)
	}
	defer func() { _ = stdio.Close() }()

	req := &jetbrains.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "echo",
	}

	resp, err := stdio.Send(ctx, req)
	if err != nil {
		t.Fatalf("stdio Send failed: %v", err)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
}

func TestMockTransport_SentRequests(t *testing.T) {
	mock := jetbrains.NewMockTransport()
	ctx := context.Background()

	req := &jetbrains.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      100,
		Method:  "ping",
	}
	_, err := mock.Send(ctx, req)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	sent := mock.GetSentRequests()
	if len(sent) != 1 || sent[0].Method != "ping" {
		t.Errorf("unexpected sent requests: %+v", sent)
	}

	_ = mock.Close()
	_, err = mock.Send(ctx, req)
	if !errors.Is(err, jetbrains.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected after close, got: %v", err)
	}
}
