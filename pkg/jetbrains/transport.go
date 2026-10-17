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
