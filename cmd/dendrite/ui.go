package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"Dendrite/pkg/server"
	"Dendrite/pkg/ui/penpot"

	"github.com/spf13/cobra"
)

const defaultPenpotEndpoint = "http://localhost:9000"

// newCortexCmd builds `dendrite cortex` (alias ui): the web cockpit, Penpot sync and the creative canvas.
func newCortexCmd() *cobra.Command {
	cmd := newGroupCmd("cortex", "Embedded Web Control Cockpit and telemetry server", "cortex",
		"ui", "cortex-cockpit")
	cmd.Example = "dendrite cortex serve --port 3456\n" +
		"dendrite cortex sync --file web/src/Button.svelte --ticket DEN-810\n" +
		"dendrite cortex canvas dispatch --action WRITE_PLAN --prompt \"Q4 roadmap\""
	cmd.AddCommand(newCortexServeCmd(), newCortexSyncCmd(), newCortexCanvasCmd())
	return cmd
}

func newCortexServeCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:     "serve",
		Aliases: []string{"start"},
		Short:   "Run the web cockpit and telemetry server in the foreground (Ctrl+C stops it)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runCortexServe(ctx, port, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVar(&port, "port", 3456, "Listen port")
	return cmd
}

func runCortexServe(ctx context.Context, port int, out io.Writer) error {
	reg, _ := newProjectRegistry()
	srv := server.NewServer(server.ServerConfig{Host: "127.0.0.1", Port: port}, reg, server.NewTelemetryHub())
	start := time.Now()
	if err := srv.Start(); err != nil {
		return fmt.Errorf("start Dendrite Web UI: %w", err)
	}
	defer func() { _ = srv.Shutdown(context.Background()) }()
	fmt.Fprintf(out, "[✓] Dendrite Web UI listening at http://127.0.0.1:%d (Started in %v)\n", port, time.Since(start))
	fmt.Fprintf(out, "    - WebSocket Telemetry: ws://127.0.0.1:%d/api/v1/telemetry/ws\n", port)
	fmt.Fprintf(out, "    - REST API Snapshot:   http://127.0.0.1:%d/api/v1/telemetry/snapshot\n", port)
	fmt.Fprintf(out, "    - Prometheus Metrics:  http://127.0.0.1:%d/metrics\n", port)
	fmt.Fprintln(out, "[+] Press Ctrl+C to stop.")
	<-ctx.Done()
	return nil
}

func newCortexSyncCmd() *cobra.Command {
	opts := penpot.SyncOptions{TicketID: "DEN-810", PenpotEndpoint: defaultPenpotEndpoint, CommentPlane: true}
	cmd := &cobra.Command{
		Use:     "sync",
		Aliases: []string{"penpot"},
		Short:   "Extract a UI component's tokens and props and sync them to Penpot and the Plane ticket",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCortexSync(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addFileFlag(cmd, &opts.FilePath, "", "UI component file")
	addTicketFlag(cmd, &opts.TicketID, opts.TicketID)
	cmd.Flags().StringVarP(&opts.PenpotEndpoint, "endpoint", "e", opts.PenpotEndpoint, "Penpot URL")
	addDryRunFlag(cmd, &opts.DryRun)
	requireFlags(cmd, "file")
	return cmd
}

func runCortexSync(ctx context.Context, opts penpot.SyncOptions, out io.Writer) error {
	res, err := penpot.NewSynchronizer(nil).Sync(ctx, opts)
	if err != nil {
		return fmt.Errorf("UI synchronization: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, res)
	}
	fmt.Fprintf(out, "[✓] Successfully synchronized UI component '%s' (%s) with Penpot & Plane\n", res.Component.Name, res.Component.Framework)
	fmt.Fprintf(out, "    * Design Tokens:   %d tokens extracted\n", len(res.Component.Tokens))
	fmt.Fprintf(out, "    * Component Props: %s\n", strings.Join(res.Component.Props, ", "))
	fmt.Fprintf(out, "    * Penpot Frame:    %s\n", res.PenpotFrameURL)
	fmt.Fprintf(out, "    * Plane Synced:    %v (Ticket: %s)\n", res.PlaneSynced, res.TicketID)
	fmt.Fprintf(out, "    * Service Status:  %s\n", res.Notes)
	return nil
}
