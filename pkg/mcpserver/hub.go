package mcpserver

import hub "github.com/yasuomaidana/dendrite-mcp-hub/pkg/mcpserver"

// The MCP protocol server, its wire types and the tool registry live in
// dendrite-mcp-hub; this package registers Dendrite's control-plane tools into it.
type (
	Server              = hub.Server
	ServerConfig        = hub.ServerConfig
	ServerInfo          = hub.ServerInfo
	Tool                = hub.Tool
	ToolHandler         = hub.ToolHandler
	CallToolParams      = hub.CallToolParams
	CallToolResult      = hub.CallToolResult
	ContentItem         = hub.ContentItem
	ListToolsResult     = hub.ListToolsResult
	InitializeResult    = hub.InitializeResult
	JSONRPCRequest      = hub.JSONRPCRequest
	JSONRPCResponse     = hub.JSONRPCResponse
	JSONRPCError        = hub.JSONRPCError
	ServerCapabilities  = hub.ServerCapabilities
	ToolsCapability     = hub.ToolsCapability
	ResourcesCapability = hub.ResourcesCapability
	PromptsCapability   = hub.PromptsCapability
)

// Protocol constants re-exported from dendrite-mcp-hub.
const (
	ProtocolVersion       = hub.ProtocolVersion
	JSONRPCVersion        = hub.JSONRPCVersion
	ErrCodeParseError     = hub.ErrCodeParseError
	ErrCodeInvalidRequest = hub.ErrCodeInvalidRequest
	ErrCodeMethodNotFound = hub.ErrCodeMethodNotFound
	ErrCodeInvalidParams  = hub.ErrCodeInvalidParams
	ErrCodeInternalError  = hub.ErrCodeInternalError
)

// Errors re-exported from dendrite-mcp-hub.
var (
	ErrServerClosed       = hub.ErrServerClosed
	ErrToolNotFound       = hub.ErrToolNotFound
	ErrInvalidRequestType = hub.ErrInvalidRequestType
)

// Dendrite's identity for the control-plane MCP server.
const (
	defaultName    = "Dendrite Control Plane MCP Server"
	defaultVersion = "0.1.0-alpha"
)

// NewServer creates a hub MCP server branded as the Dendrite control plane when cfg
// leaves the name or version empty.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Name == "" {
		cfg.Name = defaultName
	}
	if cfg.Version == "" {
		cfg.Version = defaultVersion
	}
	return hub.NewServer(cfg)
}
