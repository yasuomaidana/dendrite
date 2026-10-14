package jetbrains

import (
	"encoding/json"
	"errors"
	"time"
)

// Standard domain errors for JetBrains MCP client.
var (
	ErrNotConnected            = errors.New("jetbrains mcp client is not connected")
	ErrAlreadyConnected        = errors.New("jetbrains mcp client is already connected")
	ErrNilTransport            = errors.New("transport cannot be nil")
	ErrToolNotFound            = errors.New("requested tool was not found on jetbrains mcp server")
	ErrExecutionFailed         = errors.New("jetbrains mcp tool execution failed")
	ErrInvalidResponse         = errors.New("received invalid json-rpc response from jetbrains")
	ErrEmptyToolName           = errors.New("tool name cannot be empty")
	ErrEmptyFilePath           = errors.New("file path cannot be empty")
	ErrProtocolVersionMismatch = errors.New("unsupported protocol version returned by server")
)

// DefaultProtocolVersion defines the standard MCP specification version.
const DefaultProtocolVersion = "2024-11-05"

// JSONRPCRequest represents a JSON-RPC 2.0 request envelope.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response envelope.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents standard JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ClientInfo describes the MCP client identity.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeParams holds parameters sent during the MCP initialize handshake.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
	Capabilities    ClientCapabilities `json:"capabilities"`
}

// ClientCapabilities declares optional capabilities supported by the client.
type ClientCapabilities struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
}

// RootsCapability indicates root folder tracking support.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability indicates LLM sampling support.
type SamplingCapability struct{}

// ServerInfo describes the remote JetBrains server identity.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities lists capabilities supported by the JetBrains MCP server.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
}

// ToolsCapability indicates tool calling support.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates resource reading support.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability indicates prompt template support.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// InitializeResult holds server info and capabilities after handshake.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
}

// Tool represents a registered MCP tool exposed by JetBrains.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ListToolsResult represents the payload returned by tools/list.
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolParams holds arguments for tools/call.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ContentItem holds a single content payload (text, image, or resource) returned by a tool.
type ContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// CallToolResult represents the response payload for tools/call.
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// Problem represents an IDE diagnostic or inspection problem.
type Problem struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// Symbol represents an IDE code symbol found across project index.
type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Container string `json:"container,omitempty"`
}

// BuildResult represents IDE project build execution summary.
type BuildResult struct {
	Success  bool          `json:"success"`
	Errors   []string      `json:"errors,omitempty"`
	Duration time.Duration `json:"duration"`
}
