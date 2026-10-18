package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/node/migrate"
)

func registerMigrateTools(s *Server, deps Dependencies) {
	// dendrite_service_move
	s.RegisterTool(Tool{
		Name:        "dendrite_service_move",
		Description: "Migrates containerized Dendrite services (Plane v2, LiteLLM, Database, Penpot, or 'debian' bundle) to a remote Debian SSH server with zero-credential authorization, ephemeral streaming, health verification, and atomic cutover",
		InputSchema: map[string]interface{}{
			"type": "object",
			"required": []string{
				"target_host",
			},
			"properties": map[string]interface{}{
				"services": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "List of services to migrate (e.g. ['plane'], ['plane', 'litellm', 'database'], or ['debian'])",
				},
				"target_host": map[string]interface{}{
					"type":        "string",
					"description": "Target remote SSH host (e.g. 'user@debian-server' or '192.168.1.50')",
				},
				"ssh_port": map[string]interface{}{
					"type":        "integer",
					"description": "SSH port on target host (default: 22)",
				},
				"ssh_user": map[string]interface{}{
					"type":        "string",
					"description": "SSH username (default: derived from target_host or deployer)",
				},
				"target_ram_mb": map[string]interface{}{
					"type":        "integer",
					"description": "Target server RAM in MB (default: 4096 for 4 GB)",
				},
				"target_disk_gb": map[string]interface{}{
					"type":        "integer",
					"description": "Target server Disk in GB (default: 1000 for 1 TB)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, simulates validation without transferring data or modifying containers",
				},
				"prune_local": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, safely prunes local containers after successful remote health check (default: true)",
				},
				"timeout_seconds": map[string]interface{}{
					"type":        "integer",
					"description": "Remote health check timeout in seconds (default: 120)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		targetHost, _ := args["target_host"].(string)
		if targetHost == "" {
			return nil, fmt.Errorf("target_host is required")
		}

		var services []string
		if rawSvc, ok := args["services"].([]interface{}); ok {
			for _, item := range rawSvc {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					services = append(services, strings.TrimSpace(s))
				}
			}
		}
		if len(services) == 0 {
			services = []string{"debian"}
		}

		sshPort := 22
		if p, ok := args["ssh_port"].(float64); ok && p > 0 {
			sshPort = int(p)
		}
		sshUser, _ := args["ssh_user"].(string)

		targetRAM := uint64(4096)
		if r, ok := args["target_ram_mb"].(float64); ok && r > 0 {
			targetRAM = uint64(r)
		}
		targetDisk := uint64(1000)
		if d, ok := args["target_disk_gb"].(float64); ok && d > 0 {
			targetDisk = uint64(d)
		}

		dryRun, _ := args["dry_run"].(bool)
		pruneLocal := true
		if p, ok := args["prune_local"].(bool); ok {
			pruneLocal = p
		}

		timeout := 120 * time.Second
		if t, ok := args["timeout_seconds"].(float64); ok && t > 0 {
			timeout = time.Duration(t) * time.Second
		}

		migrator := migrate.NewMigrator(nil, nil)
		req := migrate.MigrationRequest{
			Services:     services,
			TargetHost:   targetHost,
			SSHPort:      sshPort,
			SSHUser:      sshUser,
			TargetRAMMB:  targetRAM,
			TargetDiskGB: targetDisk,
			AutoApprove:  true, // MCP invocations have confirmed programmatic consent
			DryRun:       dryRun,
			PruneLocal:   pruneLocal,
			Timeout:      timeout,
		}

		res, err := migrator.Migrate(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// dendrite_service_plan
	s.RegisterTool(Tool{
		Name:        "dendrite_service_plan",
		Description: "Evaluates hardware capacity (RAM and Disk) and returns which services can live on a Debian server (e.g. 4 GB RAM with 1 TB Disk)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ram_mb": map[string]interface{}{
					"type":        "integer",
					"description": "Server RAM in MB (default: 4096)",
				},
				"disk_gb": map[string]interface{}{
					"type":        "integer",
					"description": "Server Disk in GB (default: 1000)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		ramMB := uint64(4096)
		if r, ok := args["ram_mb"].(float64); ok && r > 0 {
			ramMB = uint64(r)
		}
		diskGB := uint64(1000)
		if d, ok := args["disk_gb"].(float64); ok && d > 0 {
			diskGB = uint64(d)
		}

		plan := migrate.EvaluateDebianServer(ramMB, diskGB)
		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return nil, err
		}

		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})
}
