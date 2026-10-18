package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/server"
	"Dendrite/pkg/ui/penpot"
	"Dendrite/pkg/workspace"
)

func handleUICommand(ctx context.Context, args []string) {
	if len(args) > 0 && (args[0] == "sync" || args[0] == "penpot") {
		subArgs := args[1:]
		if len(args) > 1 && args[0] == "penpot" && args[1] == "sync" {
			subArgs = args[2:]
		}

		ticketID := "DEN-810"
		filePath := ""
		endpoint := "http://localhost:9000"
		dryRun := false

		for i := 0; i < len(subArgs); i++ {
			switch subArgs[i] {
			case "--ticket", "-t":
				if i+1 < len(subArgs) {
					ticketID = subArgs[i+1]
					i++
				}
			case "--file", "-f":
				if i+1 < len(subArgs) {
					filePath = subArgs[i+1]
					i++
				}
			case "--endpoint", "-e":
				if i+1 < len(subArgs) {
					endpoint = subArgs[i+1]
					i++
				}
			case "--dry-run":
				dryRun = true
			}
		}

		if filePath == "" {
			fmt.Println("[-] Usage: dendrite ui sync --file <path> [--ticket <id>] [--endpoint <url>] [--dry-run]")
			return
		}

		syncer := penpot.NewSynchronizer(nil)
		res, err := syncer.Sync(ctx, penpot.SyncOptions{
			TicketID:       ticketID,
			FilePath:       filePath,
			PenpotEndpoint: endpoint,
			DryRun:         dryRun,
			CommentPlane:   true,
		})
		if err != nil {
			fmt.Printf("[-] UI synchronization failed: %v\n", err)
			return
		}

		fmt.Printf("[✓] Successfully synchronized UI component '%s' (%s) with Penpot & Plane\n", res.Component.Name, res.Component.Framework)
		fmt.Printf("    * Design Tokens:   %d tokens extracted\n", len(res.Component.Tokens))
		fmt.Printf("    * Component Props: %s\n", strings.Join(res.Component.Props, ", "))
		fmt.Printf("    * Penpot Frame:    %s\n", res.PenpotFrameURL)
		fmt.Printf("    * Plane Synced:    %v (Ticket: %s)\n", res.PlaneSynced, res.TicketID)
		fmt.Printf("    * Service Status:  %s\n", res.Notes)
		return
	}

	port := 3456
	daemonMode := false
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" && i+1 < len(args) {
			if p, err := strconv.Atoi(args[i+1]); err == nil {
				port = p
			}
			i++
		} else if args[i] == "--daemon" || args[i] == "-d" {
			daemonMode = true
		}
	}

	reg, _ := workspace.NewRegistry("", "")
	hub := server.NewTelemetryHub()

	srv := server.NewServer(server.ServerConfig{
		Host: "127.0.0.1",
		Port: port,
	}, reg, hub)

	start := time.Now()
	if err := srv.Start(); err != nil {
		fmt.Printf("[-] Failed to start Dendrite Web UI: %v\n", err)
		return
	}
	duration := time.Since(start)

	fmt.Printf("[✓] Dendrite Web UI listening at http://127.0.0.1:%d (Started in %v)\n", port, duration)
	fmt.Printf("    - WebSocket Telemetry: ws://127.0.0.1:%d/api/v1/telemetry/ws\n", port)
	fmt.Printf("    - REST API Snapshot:   http://127.0.0.1:%d/api/v1/telemetry/snapshot\n", port)
	fmt.Printf("    - Prometheus Metrics:  http://127.0.0.1:%d/metrics\n", port)

	if daemonMode {
		fmt.Println("[+] Running in daemon mode. Press Ctrl+C to terminate.")
		select {}
	}
}
