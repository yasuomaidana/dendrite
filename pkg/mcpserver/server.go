package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Server coordinates MCP protocol handling and tool execution.
type Server struct {
	mu          sync.RWMutex
	info        ServerInfo
	tools       map[string]Tool
	handlers    map[string]ToolHandler
	initialized bool
}

// ServerConfig configures the MCP server instance.
type ServerConfig struct {
	Name    string
	Version string
}

// NewServer initializes an MCP server instance with Dendrite branding.
func NewServer(cfg ServerConfig) *Server {
	name := cfg.Name
	if name == "" {
		name = "Dendrite Control Plane MCP Server"
	}
	version := cfg.Version
	if version == "" {
		version = "0.1.0-alpha"
	}

	return &Server{
		info: ServerInfo{
			Name:    name,
			Version: version,
		},
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
	}
}

// RegisterTool adds a tool definition and handler to the server.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.handlers[tool.Name] = handler
}

// ListTools returns all registered tools.
func (s *Server) ListTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		list = append(list, t)
	}
	return list
}

// HandleRequest processes a single JSON-RPC 2.0 request.
func (s *Server) HandleRequest(ctx context.Context, data []byte) *JSONRPCResponse {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      nil,
			Error: &JSONRPCError{
				Code:    ErrCodeParseError,
				Message: fmt.Sprintf("Failed to parse JSON-RPC request: %v", err),
			},
		}
	}

	// Notifications don't require a response if ID is nil, but we return nil or ack.
	switch req.Method {
	case "initialize":
		s.mu.Lock()
		s.initialized = true
		s.mu.Unlock()

		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Result: InitializeResult{
				ProtocolVersion: ProtocolVersion,
				ServerInfo:      s.info,
				Capabilities: ServerCapabilities{
					Tools: &ToolsCapability{
						ListChanged: false,
					},
				},
			},
		}

	case "notifications/initialized":
		// No response required for notifications
		return nil

	case "ping":
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Result:  map[string]interface{}{"status": "pong"},
		}

	case "tools/list":
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Result: ListToolsResult{
				Tools: s.ListTools(),
			},
		}

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    ErrCodeInvalidParams,
					Message: fmt.Sprintf("Invalid tools/call params: %v", err),
				},
			}
		}

		s.mu.RLock()
		handler, exists := s.handlers[params.Name]
		s.mu.RUnlock()

		if !exists {
			return &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    ErrCodeMethodNotFound,
					Message: fmt.Sprintf("Unknown tool: %s", params.Name),
				},
			}
		}

		res, err := handler(ctx, params.Arguments)
		if err != nil {
			return &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				ID:      req.ID,
				Result: CallToolResult{
					IsError: true,
					Content: []ContentItem{
						{
							Type: "text",
							Text: fmt.Sprintf("Tool error: %v", err),
						},
					},
				},
			}
		}

		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Result:  res,
		}

	default:
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    ErrCodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

// ServeStdio continuously reads line-delimited JSON-RPC requests from in and writes responses to out.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// Allow up to 10MB payloads for large AST / token payloads
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		resp := s.HandleRequest(ctx, line)
		if resp == nil {
			// Notification, no response
			continue
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("failed to encode response: %w", err)
		}

		if _, err := fmt.Fprintf(out, "%s\n", string(respBytes)); err != nil {
			return fmt.Errorf("failed to write response: %w", err)
		}
	}

	return scanner.Err()
}
