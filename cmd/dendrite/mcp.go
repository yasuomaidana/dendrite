package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/mcpserver"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/synapse"
)

// buildMCPServer constructs the live Dendrite Control Plane MCP server with registered default tools.
func buildMCPServer() *mcpserver.Server {
	srv := mcpserver.NewServer(mcpserver.ServerConfig{
		Name:    "Dendrite Control Plane MCP Server",
		Version: version,
	})
	srv.RegisterDefaultTools(mcpserver.Dependencies{
		DevStatus: "DEV_STATUS.md",
		OllamaURL: ollama.DefaultBaseURL,
	})
	return srv
}

// buildSynapseMCPServer constructs a scoped MCP server exposing only the synapse_* tools of one synapse.
func buildSynapseMCPServer(id string) (*mcpserver.Server, error) {
	st, err := synapse.DefaultStore()
	if err != nil {
		return nil, err
	}
	return mcpserver.NewSynapseServer(mcpserver.SynapseScope{Store: st, ID: id}, version)
}

// extractSynapseFlag removes "--synapse <id>" / "--synapse=<id>" from args and returns the id.
func extractSynapseFlag(args []string) (string, []string, error) {
	var id string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--synapse":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--synapse requires a synapse id")
			}
			id = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--synapse="):
			id = strings.TrimPrefix(args[i], "--synapse=")
		default:
			rest = append(rest, args[i])
		}
	}
	return id, rest, nil
}

func handleMCPCommand(ctx context.Context, action string, args []string) {
	synapseID, args, err := extractSynapseFlag(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
		os.Exit(1)
	}
	srv := buildMCPServer()
	if synapseID != "" {
		if srv, err = buildSynapseMCPServer(synapseID); err != nil {
			fmt.Fprintf(os.Stderr, "[-] %v\n", err)
			os.Exit(1)
		}
	}

	switch action {
	case "serve", "stdio":
		if err := srv.ServeStdio(ctx, os.Stdin, os.Stdout); err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "[-] MCP Server terminated: %v\n", err)
		}

	case "tools", "list":
		tools := srv.ListTools()
		fmt.Printf("[+] Registered Dendrite MCP Tools (%d):\n", len(tools))
		for _, t := range tools {
			fmt.Printf("    - %-25s: %s\n", t.Name, t.Description)
		}

	case "call":
		if len(args) == 0 {
			fmt.Println("[-] Please specify a tool name. Usage: dendrite mcp call <tool> [arguments-json]")
			return
		}
		toolName := args[0]
		callArgs := make(map[string]interface{})
		if len(args) > 1 {
			if err := json.Unmarshal([]byte(args[1]), &callArgs); err != nil {
				fmt.Printf("[-] Failed to parse arguments JSON: %v\n", err)
				return
			}
		}

		callReq := mcpserver.JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}
		rawParams, _ := json.Marshal(mcpserver.CallToolParams{
			Name:      toolName,
			Arguments: callArgs,
		})
		callReq.Params = rawParams

		reqBytes, _ := json.Marshal(callReq)
		resp := srv.HandleRequest(ctx, reqBytes)
		if resp == nil {
			fmt.Println("[-] No response from MCP server")
			return
		}
		if resp.Error != nil {
			fmt.Printf("[-] MCP Error (%d): %s\n", resp.Error.Code, resp.Error.Message)
			return
		}

		resData, _ := json.MarshalIndent(resp.Result, "", "  ")
		fmt.Println(string(resData))

	default:
		fmt.Printf("[-] Unknown MCP action: %s. Use 'serve', 'tools', or 'call'.\n", action)
	}
}
