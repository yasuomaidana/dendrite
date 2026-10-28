package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"Dendrite/pkg/mcpserver"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/synapse"

	"github.com/spf13/cobra"
)

// buildMCPServer constructs the live Dendrite Control Plane MCP server with registered default tools.
func buildMCPServer(profile mcpserver.Profile) *mcpserver.Server {
	srv := mcpserver.NewServer(mcpserver.ServerConfig{
		Name:    "Dendrite Control Plane MCP Server",
		Version: version,
	})
	mcpserver.RegisterDefaultTools(srv, mcpserver.Dependencies{
		DevStatus: "DEV_STATUS.md",
		OllamaURL: ollama.DefaultBaseURL,
	})
	return mcpserver.ApplyProfile(srv, profile)
}

// buildSynapseMCPServer constructs a scoped MCP server exposing only the synapse_* tools of one synapse.
func buildSynapseMCPServer(id string) (*mcpserver.Server, error) {
	st, err := synapse.DefaultStore()
	if err != nil {
		return nil, err
	}
	return mcpserver.NewSynapseServer(mcpserver.SynapseScope{Store: st, ID: id}, version)
}

// newInterCmd builds `dendrite inter` (alias mcp): the Model Context Protocol server over stdio.
func newInterCmd() *cobra.Command {
	cmd := newGroupCmd("inter", "Model Context Protocol (MCP) server over stdio", "cleft", "mcp", "interneuron-mcp")
	cmd.Example = "dendrite inter serve\n" +
		"dendrite inter serve --synapse web-api\n" +
		"dendrite inter tools\n" +
		"dendrite inter call dendrite_ticket_smart_select '{}'"
	var synapseID, profileFlag string
	cmd.PersistentFlags().StringVar(&synapseID, "synapse", "", "Expose only the synapse_* tools of this synapse")
	cmd.PersistentFlags().StringVar(&profileFlag, "profile", "", "Tool profile: core (RAG, graph, Plane) or full (every tool); env "+mcpserver.ProfileEnv+", default full")
	server := func() (*mcpserver.Server, error) {
		if synapseID != "" {
			return buildSynapseMCPServer(synapseID)
		}
		profile, err := mcpserver.ParseProfile(profileFlag, mcpserver.ProfileFull)
		if profileFlag == "" {
			profile, err = mcpserver.ProfileFromEnv(mcpserver.ProfileFull)
		}
		if err != nil {
			return nil, usageErrorf("%v", err)
		}
		return buildMCPServer(profile), nil
	}

	serve := &cobra.Command{
		Use:     "serve",
		Aliases: []string{"stdio"},
		Short:   "Serve MCP over stdin/stdout",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv, err := server()
			if err != nil {
				return err
			}
			if err := srv.ServeStdio(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout()); err != nil && err != io.EOF {
				return fmt.Errorf("MCP server terminated: %w", err)
			}
			return nil
		},
	}
	tools := &cobra.Command{
		Use:     "tools",
		Aliases: []string{"list", "ls"},
		Short:   "List the registered MCP tools",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv, err := server()
			if err != nil {
				return err
			}
			return runInterTools(srv, cmd.OutOrStdout())
		},
	}
	call := &cobra.Command{
		Use:   "call <tool> [arguments-json]",
		Short: "Call one MCP tool and print its result",
		Args:  rangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			callArgs := map[string]any{}
			if len(args) > 1 {
				if err := json.Unmarshal([]byte(args[1]), &callArgs); err != nil {
					return usageErrorf("arguments must be a JSON object: %v", err)
				}
			}
			srv, err := server()
			if err != nil {
				return err
			}
			return runInterCall(cmd.Context(), srv, args[0], callArgs, cmd.OutOrStdout())
		},
	}
	var probeTimeout time.Duration
	probe := &cobra.Command{
		Use:   "probe",
		Short: "Call every read-only MCP tool once and print a pass/fail matrix",
		Long: "probe calls only tools that read (never create, update or delete anything) and reports PASS, FAIL or SKIP\n" +
			"for each. Plane, Ollama and the RAG sidecar must be running for their tools to pass; a FAIL names the\n" +
			"reason. Run it against the profile you ship: tools outside the profile are SKIP.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv, err := server()
			if err != nil {
				return err
			}
			specs := mcpserver.DefaultProbeSpecs()
			if probeTimeout > 0 {
				for i := range specs {
					specs[i].Timeout = probeTimeout
				}
			}
			results := mcpserver.ProbeTools(cmd.Context(), srv, specs)
			if fails := mcpserver.WriteProbeMatrix(cmd.OutOrStdout(), results); fails > 0 {
				return fmt.Errorf("%d of %d probed MCP tools failed", fails, len(results))
			}
			return nil
		},
	}
	probe.Flags().DurationVar(&probeTimeout, "timeout", 0, "Override every probe's timeout (default: per tool, 5s unless it needs more)")
	cmd.AddCommand(serve, tools, call, probe)
	return cmd
}

func runInterTools(srv *mcpserver.Server, out io.Writer) error {
	tools := srv.ListTools()
	if globals.JSON {
		return writeJSON(out, tools)
	}
	fmt.Fprintf(out, "[+] Registered Dendrite MCP Tools (%d):\n", len(tools))
	for _, t := range tools {
		fmt.Fprintf(out, "    - %-25s: %s\n", t.Name, t.Description)
	}
	return nil
}

func runInterCall(ctx context.Context, srv *mcpserver.Server, tool string, callArgs map[string]any, out io.Writer) error {
	rawParams, err := json.Marshal(mcpserver.CallToolParams{Name: tool, Arguments: callArgs})
	if err != nil {
		return err
	}
	reqBytes, err := json.Marshal(mcpserver.JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: rawParams})
	if err != nil {
		return err
	}
	resp := srv.HandleRequest(ctx, reqBytes)
	if resp == nil {
		return fmt.Errorf("no response from MCP server")
	}
	if resp.Error != nil {
		return fmt.Errorf("MCP error (%d): %s", resp.Error.Code, resp.Error.Message)
	}
	return writeJSON(out, resp.Result)
}
