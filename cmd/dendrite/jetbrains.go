package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/jetbrains"

	"github.com/spf13/cobra"
)

// jetbrainsMCPURL is the IDE's built-in MCP endpoint; tests point it at a local server.
var jetbrainsMCPURL = "http://localhost:63342/api/mcp"

func getJetBrainsClient(ctx context.Context) (*jetbrains.Client, error) {
	transport := jetbrains.NewHTTPTransport(jetbrainsMCPURL, &http.Client{Timeout: 1500 * time.Millisecond})
	client, err := jetbrains.NewClient(transport)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	if _, err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect to the JetBrains IDE MCP server at %s (is the IDE running?): %w", jetbrainsMCPURL, err)
	}
	return client, nil
}

// withIDE connects to the IDE and runs fn with the client and its bridge.
func withIDE(ctx context.Context, fn func(*jetbrains.Client, *jetbrains.Bridge) error) error {
	client, err := getJetBrainsClient(ctx)
	if err != nil {
		return err
	}
	return fn(client, jetbrains.NewBridge(client))
}

// newBridgeCmd builds `dendrite bridge` (alias jetbrains): the JetBrains IDE MCP integration.
func newBridgeCmd() *cobra.Command {
	cmd := newGroupCmd("bridge", "JetBrains IDE MCP protocol integration and PSI symbol indexer", "cleft",
		"jetbrains", "neuromuscular-bridge", "jb")
	cmd.Example = "dendrite bridge setup\n" +
		"dendrite bridge probe\n" +
		"dendrite bridge open cmd/dendrite/root.go --line 40\n" +
		"dendrite bridge symbols newRootCmd"

	probe := &cobra.Command{
		Use:   "probe",
		Short: "Connect to the IDE and list its MCP tools",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIDE(cmd.Context(), func(c *jetbrains.Client, _ *jetbrains.Bridge) error {
				return runBridgeProbe(cmd.Context(), c, cmd.OutOrStdout())
			})
		},
	}

	var line int
	open := &cobra.Command{
		Use:   "open <file>",
		Short: "Open a file in the IDE editor",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if line < 1 {
				return usageErrorf("--line must be 1 or more")
			}
			return withIDE(cmd.Context(), func(_ *jetbrains.Client, b *jetbrains.Bridge) error {
				if err := b.OpenFile(cmd.Context(), args[0], line, 1); err != nil {
					return fmt.Errorf("open file: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Opened %s at line %d in JetBrains editor\n", args[0], line)
				return nil
			})
		},
	}
	open.Flags().IntVarP(&line, "line", "l", 1, "Line to place the caret on")

	problems := &cobra.Command{
		Use:   "problems <file>",
		Short: "Show the IDE's inspection results for a file",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIDE(cmd.Context(), func(_ *jetbrains.Client, b *jetbrains.Bridge) error {
				probs, err := b.GetFileProblems(cmd.Context(), args[0])
				if err != nil {
					return fmt.Errorf("get problems: %w", err)
				}
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, probs)
				}
				fmt.Fprintf(out, "[+] JetBrains Diagnostics for %s (%d found):\n", args[0], len(probs))
				for _, p := range probs {
					fmt.Fprintf(out, "    [%s] Line %d:%d - %s\n", p.Severity, p.Line, p.Column, p.Description)
				}
				return nil
			})
		},
	}

	symbols := &cobra.Command{
		Use:   "symbols [query]",
		Short: "Search the IDE's symbol index",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) == 1 {
				query = args[0]
			}
			return withIDE(cmd.Context(), func(_ *jetbrains.Client, b *jetbrains.Bridge) error {
				syms, err := b.SearchSymbol(cmd.Context(), query)
				if err != nil {
					return fmt.Errorf("symbol search: %w", err)
				}
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, syms)
				}
				fmt.Fprintf(out, "[+] Matching Symbols (%d):\n", len(syms))
				for _, s := range syms {
					fmt.Fprintf(out, "    - %s (%s) -> %s:%d\n", s.Name, s.Kind, s.File, s.Line)
				}
				return nil
			})
		},
	}

	build := &cobra.Command{
		Use:   "build",
		Short: "Build the project in the IDE",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIDE(cmd.Context(), func(_ *jetbrains.Client, b *jetbrains.Bridge) error {
				out := cmd.OutOrStdout()
				fmt.Fprintln(out, "[+] Triggering IDE project build...")
				res, err := b.BuildProject(cmd.Context())
				if err != nil {
					return fmt.Errorf("build: %w", err)
				}
				if !res.Success {
					return fmt.Errorf("build completed with errors in %v", res.Duration)
				}
				fmt.Fprintf(out, "[✓] Project build succeeded in %v\n", res.Duration)
				return nil
			})
		},
	}

	run := &cobra.Command{
		Use:   "run <configuration>",
		Short: "Run an IDE run configuration",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withIDE(cmd.Context(), func(_ *jetbrains.Client, b *jetbrains.Bridge) error {
				fmt.Fprintf(cmd.OutOrStdout(), "[+] Running IDE configuration '%s'...\n", args[0])
				res, err := b.ExecuteRunConfiguration(cmd.Context(), args[0])
				if err != nil {
					return fmt.Errorf("run configuration: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] %s\n", res)
				return nil
			})
		},
	}

	cmd.AddCommand(probe, open, problems, symbols, build, run, newBridgeSetupCmd())
	return cmd
}

func runBridgeProbe(ctx context.Context, client *jetbrains.Client, out io.Writer) error {
	fmt.Fprintln(out, "[+] Probing JetBrains IDE MCP Server...")
	if info := client.GetServerInfo(); info != nil {
		fmt.Fprintf(out, "  [✓] Connected: %s (v%s)\n", info.Name, info.Version)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	fmt.Fprintf(out, "  [✓] Discovered Tools (%d):\n", len(tools))
	for _, t := range tools {
		fmt.Fprintf(out, "      - %-25s: %s\n", t.Name, t.Description)
	}
	return nil
}

func newBridgeSetupCmd() *cobra.Command {
	var projectOnly, globalOnly bool
	cmd := &cobra.Command{
		Use:     "setup",
		Aliases: []string{"setup-mcp", "configure"},
		Short:   "Write the IDE's MCP config, run configurations and external tools (no running IDE needed)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := jetbrains.SetupOptions{
				ConfigureProjectMCP:    !globalOnly,
				ConfigureGlobalMCP:     !projectOnly,
				ConfigureRunConfigs:    !globalOnly,
				ConfigureExternalTools: !globalOnly,
			}
			return runBridgeSetup(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&projectOnly, "project-only", false, "Only write this project's configuration")
	cmd.Flags().BoolVar(&globalOnly, "global-only", false, "Only write the global MCP configuration")
	cmd.MarkFlagsMutuallyExclusive("project-only", "global-only")
	return cmd
}

func runBridgeSetup(opts jetbrains.SetupOptions, out io.Writer) error {
	fmt.Fprintln(out, "[+] Configuring JetBrains IDE Toolset for Dendrite MCP & Autonomous Agents...")
	res, err := jetbrains.SetupJetBrainsToolset(opts)
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	fmt.Fprintln(out, "[✓] JetBrains Toolset configured successfully!")
	for _, group := range []struct {
		title string
		items []string
	}{
		{"Created Configurations", res.FilesCreated},
		{"Updated Configurations", res.FilesUpdated},
		{"Registered External Tools", res.ToolsAdded},
	} {
		if len(group.items) == 0 {
			continue
		}
		fmt.Fprintf(out, "[+] %s:\n", group.title)
		for _, item := range group.items {
			fmt.Fprintf(out, "    - %s\n", item)
		}
	}
	fmt.Fprintln(out, "[+] JetBrains MCP Integration Active:")
	fmt.Fprintln(out, "    - Project MCP: .ai/mcp/mcp.json")
	fmt.Fprintln(out, "    - Global MCP:  ~/.ai/mcp/mcp.json")
	fmt.Fprintln(out, "    - Run Configs: .idea/runConfigurations/ (MCP Server, Aider, Antigravity, Web UI)")
	fmt.Fprintln(out, "    - Toolset:     .idea/tools/External_Tools.xml")
	return nil
}
