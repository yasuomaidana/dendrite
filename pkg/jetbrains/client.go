package jetbrains

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Client coordinates JSON-RPC 2.0 communication with JetBrains Model Context Protocol server.
type Client struct {
	transport          Transport
	clientInfo         ClientInfo
	protocolVersion    string
	connected          bool
	serverInfo         *ServerInfo
	serverCapabilities *ServerCapabilities
	reqCounter         uint64
	mu                 sync.RWMutex
}

// Option configures Client parameters.
type Option func(*Client)

// WithClientInfo configures custom client identification.
func WithClientInfo(name, version string) Option {
	return func(c *Client) {
		c.clientInfo = ClientInfo{Name: name, Version: version}
	}
}

// WithProtocolVersion overrides the MCP protocol version string.
func WithProtocolVersion(ver string) Option {
	return func(c *Client) {
		c.protocolVersion = ver
	}
}

// NewClient initializes a new JetBrains MCP client with specified transport.
func NewClient(transport Transport, opts ...Option) (*Client, error) {
	if transport == nil {
		return nil, ErrNilTransport
	}

	c := &Client{
		transport:       transport,
		protocolVersion: DefaultProtocolVersion,
		clientInfo: ClientInfo{
			Name:    "dendrite-jetbrains-bridge",
			Version: "0.1.0",
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// nextID generates an incremental JSON-RPC request ID.
func (c *Client) nextID() uint64 {
	return atomic.AddUint64(&c.reqCounter, 1)
}

// Connect initiates the MCP handshake with JetBrains IDE.
func (c *Client) Connect(ctx context.Context) (*InitializeResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil, ErrAlreadyConnected
	}

	initParams := InitializeParams{
		ProtocolVersion: c.protocolVersion,
		ClientInfo:      c.clientInfo,
		Capabilities: ClientCapabilities{
			Roots: &RootsCapability{ListChanged: true},
		},
	}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "initialize",
		Params:  initParams,
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%w: %s (code %d)", ErrInvalidResponse, resp.Error.Message, resp.Error.Code)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		return nil, fmt.Errorf("%w: failed to decode initialize result: %s", ErrInvalidResponse, err)
	}

	// Send notification acknowledging initialization
	notifyReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	_, _ = c.transport.Send(ctx, notifyReq)

	c.connected = true
	c.serverInfo = &initResult.ServerInfo
	c.serverCapabilities = &initResult.Capabilities

	return &initResult, nil
}

// Disconnect gracefully shuts down the connection to JetBrains.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false
	c.serverInfo = nil
	c.serverCapabilities = nil

	if c.transport != nil {
		return c.transport.Close()
	}
	return nil
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetServerInfo returns details about the connected JetBrains IDE.
func (c *Client) GetServerInfo() *ServerInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.serverInfo == nil {
		return nil
	}
	cpy := *c.serverInfo
	return &cpy
}

// GetServerCapabilities returns capabilities advertised by the IDE.
func (c *Client) GetServerCapabilities() *ServerCapabilities {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.serverCapabilities == nil {
		return nil
	}
	cpy := *c.serverCapabilities
	return &cpy
}

// ListTools discovers all tools exposed by the JetBrains MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if !c.IsConnected() {
		return nil, ErrNotConnected
	}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "tools/list",
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidResponse, resp.Error.Message)
	}

	var res ListToolsResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("%w: failed to decode tools list: %s", ErrInvalidResponse, err)
	}

	return res.Tools, nil
}

// CallTool invokes an MCP tool on JetBrains IDE with given arguments.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	if !c.IsConnected() {
		return nil, ErrNotConnected
	}
	if name == "" {
		return nil, ErrEmptyToolName
	}

	if args == nil {
		args = make(map[string]interface{})
	}

	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "tools/call",
		Params: CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("tool call network failure: %w", err)
	}

	if resp.Error != nil {
		if resp.Error.Code == -32601 {
			return nil, fmt.Errorf("%w: %s", ErrToolNotFound, resp.Error.Message)
		}
		return nil, fmt.Errorf("%w: %s (code %d)", ErrExecutionFailed, resp.Error.Message, resp.Error.Code)
	}

	var callRes CallToolResult
	if err := json.Unmarshal(resp.Result, &callRes); err != nil {
		return nil, fmt.Errorf("%w: failed to decode tool result: %s", ErrInvalidResponse, err)
	}

	if callRes.IsError {
		errMsg := "unknown tool execution error"
		if len(callRes.Content) > 0 && callRes.Content[0].Text != "" {
			errMsg = callRes.Content[0].Text
		}
		return &callRes, fmt.Errorf("%w: %s", ErrExecutionFailed, errMsg)
	}

	return &callRes, nil
}

// Ping measures roundtrip latency to the JetBrains MCP server.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	if !c.IsConnected() {
		return 0, ErrNotConnected
	}

	start := time.Now()
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "ping",
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("ping failed: %w", err)
	}

	if resp.Error != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidResponse, resp.Error.Message)
	}

	return time.Since(start), nil
}
