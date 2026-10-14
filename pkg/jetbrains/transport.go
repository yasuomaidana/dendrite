package jetbrains

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// Transport abstracts the physical JSON-RPC 2.0 communication medium with JetBrains MCP server.
type Transport interface {
	Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error)
	Close() error
}

// MockTransport provides an in-memory transport implementation for deterministic testing.
type MockTransport struct {
	mu           sync.Mutex
	handlers     map[string]func(req *JSONRPCRequest) (*JSONRPCResponse, error)
	sentRequests []*JSONRPCRequest
	closed       bool
}

// NewMockTransport creates a mock transport initialized with standard JetBrains handlers.
func NewMockTransport() *MockTransport {
	m := &MockTransport{
		handlers:     make(map[string]func(req *JSONRPCRequest) (*JSONRPCResponse, error)),
		sentRequests: make([]*JSONRPCRequest, 0),
	}
	m.registerDefaultHandlers()
	return m
}

// RegisterHandler sets a custom response handler for a given JSON-RPC method.
func (m *MockTransport) RegisterHandler(method string, handler func(req *JSONRPCRequest) (*JSONRPCResponse, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[method] = handler
}

// Send dispatches request to the registered handler or returns mock default.
func (m *MockTransport) Send(_ context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrNotConnected
	}

	m.sentRequests = append(m.sentRequests, req)

	if handler, exists := m.handlers[req.Method]; exists {
		return handler(req)
	}

	return nil, fmt.Errorf("no mock handler registered for method: %s", req.Method)
}

// Close marks the mock transport as closed.
func (m *MockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// GetSentRequests returns a copy of all requests received by the mock.
func (m *MockTransport) GetSentRequests() []*JSONRPCRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]*JSONRPCRequest, len(m.sentRequests))
	copy(res, m.sentRequests)
	return res
}

func (m *MockTransport) registerDefaultHandlers() {
	m.handlers["initialize"] = func(req *JSONRPCRequest) (*JSONRPCResponse, error) {
		initResult := InitializeResult{
			ProtocolVersion: DefaultProtocolVersion,
			ServerInfo: ServerInfo{
				Name:    "JetBrains GoLand MCP Server",
				Version: "2024.3",
			},
			Capabilities: ServerCapabilities{
				Tools:     &ToolsCapability{ListChanged: true},
				Resources: &ResourcesCapability{Subscribe: false},
			},
		}
		resBytes, _ := json.Marshal(initResult)
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resBytes,
		}, nil
	}

	m.handlers["notifications/initialized"] = func(req *JSONRPCRequest) (*JSONRPCResponse, error) {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}, nil
	}

	m.handlers["ping"] = func(req *JSONRPCRequest) (*JSONRPCResponse, error) {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{}`),
		}, nil
	}

	m.handlers["tools/list"] = func(req *JSONRPCRequest) (*JSONRPCResponse, error) {
		listRes := ListToolsResult{
			Tools: []Tool{
				{
					Name:        "open_file_in_editor",
					Description: "Opens the specified file in the active JetBrains editor window.",
				},
				{
					Name:        "get_file_problems",
					Description: "Retrieves inspection problems and syntax errors for a file.",
				},
				{
					Name:        "search_symbol",
					Description: "Searches for symbols matching query across project index.",
				},
				{
					Name:        "build_project",
					Description: "Compiles and builds the currently open project in JetBrains.",
				},
				{
					Name:        "execute_run_configuration",
					Description: "Runs a specified run/test configuration inside the IDE.",
				},
				{
					Name:        "lint_files",
					Description: "Lints specified file paths using JetBrains inspections.",
				},
			},
		}
		resBytes, _ := json.Marshal(listRes)
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resBytes,
		}, nil
	}

	m.handlers["tools/call"] = func(req *JSONRPCRequest) (*JSONRPCResponse, error) {
		paramsBytes, _ := json.Marshal(req.Params)
		var callParams CallToolParams
		if err := json.Unmarshal(paramsBytes, &callParams); err != nil {
			return nil, err
		}

		var callRes CallToolResult
		switch callParams.Name {
		case "open_file_in_editor":
			callRes = CallToolResult{
				Content: []ContentItem{{Type: "text", Text: "File opened successfully"}},
			}
		case "get_file_problems":
			problems := []Problem{
				{File: "main.go", Line: 12, Column: 5, Severity: "warning", Description: "Unused variable: count"},
			}
			pBytes, _ := json.Marshal(problems)
			callRes = CallToolResult{
				Content: []ContentItem{{Type: "text", Text: string(pBytes)}},
			}
		case "search_symbol":
			symbols := []Symbol{
				{Name: "NewClient", Kind: "function", File: "pkg/jetbrains/client.go", Line: 30},
			}
			sBytes, _ := json.Marshal(symbols)
			callRes = CallToolResult{
				Content: []ContentItem{{Type: "text", Text: string(sBytes)}},
			}
		case "build_project":
			callRes = CallToolResult{
				Content: []ContentItem{{Type: "text", Text: "Build completed successfully in 1.45s"}},
			}
		case "execute_run_configuration":
			configName := "Test"
			if v, ok := callParams.Arguments["configuration_name"].(string); ok {
				configName = v
			}
			callRes = CallToolResult{
				Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("Run configuration '%s' finished with exit code 0", configName)}},
			}
		case "lint_files":
			callRes = CallToolResult{
				Content: []ContentItem{{Type: "text", Text: "[]"}},
			}
		default:
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    -32601,
					Message: fmt.Sprintf("tool '%s' not found", callParams.Name),
				},
			}, nil
		}

		resBytes, _ := json.Marshal(callRes)
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resBytes,
		}, nil
	}
}

// HTTPTransport implements Transport over HTTP/SSE REST connection to JetBrains IDE.
type HTTPTransport struct {
	client  *http.Client
	baseURL string
}

// NewHTTPTransport initializes an HTTP transport pointing to JetBrains built-in webserver.
func NewHTTPTransport(baseURL string, client *http.Client) *HTTPTransport {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if baseURL == "" {
		baseURL = "http://localhost:63342/api/mcp"
	}
	return &HTTPTransport{
		client:  client,
		baseURL: baseURL,
	}
}

// Send posts JSON-RPC payload to HTTP endpoint.
func (h *HTTPTransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpRes, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute http request: %w", err)
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode < 200 || httpRes.StatusCode >= 300 {
		body, _ := io.ReadAll(httpRes.Body)
		return nil, fmt.Errorf("jetbrains http error status %d: %s", httpRes.StatusCode, string(body))
	}

	var rpcRes JSONRPCResponse
	if err := json.NewDecoder(httpRes.Body).Decode(&rpcRes); err != nil {
		return nil, fmt.Errorf("failed to decode json-rpc response: %w", err)
	}

	return &rpcRes, nil
}

// Close cleans up HTTP transport.
func (h *HTTPTransport) Close() error {
	return nil
}

// StdioTransport executes a subprocess (e.g. MCP wrapper CLI) and communicates over stdio.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
}

// NewStdioTransport starts an external command and creates a stdio JSON-RPC transport.
func NewStdioTransport(command string, args ...string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to pipe stdin: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("failed to pipe stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdoutPipe.Close()
		return nil, fmt.Errorf("failed to start stdio command: %w", err)
	}

	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
	}, nil
}

// Send writes JSON-RPC request to stdin and reads newline-delimited response from stdout.
func (s *StdioTransport) Send(_ context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	reqData = append(reqData, '\n')

	if _, err := s.stdin.Write(reqData); err != nil {
		return nil, fmt.Errorf("failed to write to stdio stdin: %w", err)
	}

	line, err := s.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read from stdio stdout: %w", err)
	}

	var res JSONRPCResponse
	if err := json.Unmarshal(bytes.TrimSpace(line), &res); err != nil {
		return nil, fmt.Errorf("failed to parse json-rpc response line: %w", err)
	}

	return &res, nil
}

// Close terminates the subprocess.
func (s *StdioTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Kill()
	}
	return nil
}
