package opstools

import (
	"Dendrite/pkg/docusaurus"
	"Dendrite/pkg/inference"
	"Dendrite/pkg/node/migrate"
	"Dendrite/pkg/perf"
	"Dendrite/pkg/telemetry"
	"context"
	"fmt"
	"strings"
	"time"

	. "Dendrite/pkg/mcpserver"
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
				"target_host":     TypedProp("string", "Target remote SSH host (e.g. 'user@debian-server' or '192.168.1.50')"),
				"ssh_port":        TypedProp("integer", "SSH port on target host (default: 22)"),
				"ssh_user":        TypedProp("string", "SSH username (default: derived from target_host or deployer)"),
				"target_ram_mb":   TypedProp("integer", "Target server RAM in MB (default: 4096 for 4 GB)"),
				"target_disk_gb":  TypedProp("integer", "Target server Disk in GB (default: 1000 for 1 TB)"),
				"dry_run":         TypedProp("boolean", "If true, simulates validation without transferring data or modifying containers"),
				"prune_local":     TypedProp("boolean", "If true, safely prunes local containers after successful remote health check (default: true)"),
				"timeout_seconds": TypedProp("integer", "Remote health check timeout in seconds (default: 120)"),
			},
		},
	}, serviceMoveHandler())

	// dendrite_service_plan
	s.RegisterTool(Tool{
		Name:        "dendrite_service_plan",
		Description: "Evaluates hardware capacity (RAM and Disk) and returns which services can live on a Debian server (e.g. 4 GB RAM with 1 TB Disk)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ram_mb":  TypedProp("integer", "Server RAM in MB (default: 4096)"),
				"disk_gb": TypedProp("integer", "Server Disk in GB (default: 1000)"),
			},
		},
	}, servicePlanHandler())
}

// serviceMoveHandler handles the dendrite_service_move tool.
func serviceMoveHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		targetHost, _ := args["target_host"].(string)
		if targetHost == "" {
			return nil, fmt.Errorf("target_host is required")
		}
		services := TrimmedStrings(args["services"])
		if len(services) == 0 {
			services = []string{"debian"}
		}
		sshUser, _ := args["ssh_user"].(string)
		dryRun, _ := args["dry_run"].(bool)

		res, err := migrate.NewMigrator(nil, nil).Migrate(ctx, migrate.MigrationRequest{
			Services:     services,
			TargetHost:   targetHost,
			SSHPort:      int(FloatArgOr(args, "ssh_port", 22)),
			SSHUser:      sshUser,
			TargetRAMMB:  uint64(FloatArgOr(args, "target_ram_mb", 4096)),
			TargetDiskGB: uint64(FloatArgOr(args, "target_disk_gb", 1000)),
			AutoApprove:  true, // MCP invocations have confirmed programmatic consent
			DryRun:       dryRun,
			PruneLocal:   BoolArgOr(args, "prune_local", true),
			Timeout:      time.Duration(FloatArgOr(args, "timeout_seconds", 120)) * time.Second,
		})
		if err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}
		return TextResult(string(IndentJSON(res))), nil
	}
}

// servicePlanHandler handles the dendrite_service_plan tool.
func servicePlanHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		ramMB := uint64(4096)
		if r, ok := args["ram_mb"].(float64); ok && r > 0 {
			ramMB = uint64(r)
		}
		diskGB := uint64(1000)
		if d, ok := args["disk_gb"].(float64); ok && d > 0 {
			diskGB = uint64(d)
		}

		plan := migrate.EvaluateDebianServer(ramMB, diskGB)
		data := IndentJSON(plan)

		return TextResult(string(data)), nil
	}
}

func registerPerfTools(s *Server, deps Dependencies) {
	// 20. dendrite_bench_run
	s.RegisterTool(Tool{
		Name:        "dendrite_bench_run",
		Description: "Execute an automated k6 or synthetic performance benchmark measuring TTFT, TPS, gateway cascade failover latency, or sliding window budget concurrency",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mode":              TypedProp("string", "Benchmark mode: 'inference', 'gateway', 'stress', 'matrix' (default: 'inference')"),
				"engine_id":         TypedProp("string", "Inference engine identifier (e.g. 'oxillama', 'ollama-metal', 'vllm-cluster')"),
				"model":             TypedProp("string", "Model identifier (default: 'qwen2.5-coder-7b')"),
				"endpoint":          TypedProp("string", "Target endpoint URL (default: auto-resolved from registered engine)"),
				"failover_endpoint": TypedProp("string", "Secondary endpoint URL for failover testing"),
				"virtual_users":     TypedProp("integer", "Number of concurrent virtual users (default: 5)"),
				"duration_seconds":  TypedProp("integer", "Benchmark duration in seconds (default: 10)"),
				"fault_injection":   TypedProp("string", "Simulated fault injection: 'none', 'rate-limit-429', 'node-glitch-503'"),
			},
		},
	}, benchRunHandler())
}

// benchTargetURL is the endpoint a benchmark hits: the one given, else the engine's own, else a local default.
func benchTargetURL(args map[string]interface{}, engineID string) string {
	if u, _ := args["endpoint"].(string); u != "" {
		return u
	}
	if eng, err := inference.DefaultManager().GetEngine(engineID); err == nil {
		return eng.Config().Endpoint
	}
	return "http://localhost:8080/v1/chat/completions"
}

// benchDuration is how long a benchmark runs: the seconds asked for, otherwise five.
func benchDuration(args map[string]interface{}) time.Duration {
	secs := PositiveIntArg(args, "duration_seconds")
	if secs == 0 {
		secs = 5
	}
	return time.Duration(secs) * time.Second
}

// benchFault maps the fault_injection argument to a fault kind.
func benchFault(name string) perf.FaultInjectionType {
	switch name {
	case "rate-limit-429":
		return perf.FaultRateLimit429
	case "node-glitch-503":
		return perf.FaultNodeGlitch503
	}
	return perf.FaultNone
}

// benchRunHandler handles the dendrite_bench_run tool.
func benchRunHandler() ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		mode := perf.BenchmarkMode(strings.ToLower(StringArgOr(args, "mode", "inference")))
		engineID := StringArgOr(args, "engine_id", "oxillama")
		vus := perf.VirtualUsers(PositiveIntArg(args, "virtual_users"))
		if vus <= 0 {
			vus = 5
		}
		if mode == "stress" {
			res, _ := perf.StressBudgetLimiter(ctx, perf.GatewayStressConfig{VirtualUsers: vus, TotalSpends: int(vus) * 20})
			return TextResult(string(IndentJSON(res))), nil
		}
		failoverURL, _ := args["failover_endpoint"].(string)
		faultStr, _ := args["fault_injection"].(string)
		session := perf.NewSession(perf.BenchmarkConfig{
			Mode:              mode,
			TargetEndpoint:    benchTargetURL(args, engineID),
			FailoverEndpoint:  failoverURL,
			EngineID:          engineID,
			Model:             StringArgOr(args, "model", "qwen2.5-coder-7b"),
			VirtualUsers:      vus,
			Duration:          benchDuration(args),
			FaultInjection:    benchFault(faultStr),
			ThermalMonitoring: true,
		})
		prov, _ := session.Provision(perf.DefaultRunner(telemetry.NewHostThermalSensor())) // fails only for a nil runner or an empty endpoint, and neither is possible here
		completed, err := prov.Run(ctx)
		if err != nil {
			return nil, fmt.Errorf("benchmark execution failed: %w", err)
		}
		summary := completed.Summary()
		return &CallToolResult{
			Content: []ContentItem{
				{Type: "text", Text: string(IndentJSON(summary))},
				{Type: "text", Text: "\n" + perf.FormatMarkdownScorecard(summary)},
			},
		}, nil
	}
}

func registerDocusaurusTools(s *Server, deps Dependencies) {
	svc := docusaurus.NewService()

	// dendrite_docusaurus_generate
	s.RegisterTool(Tool{
		Name:        "dendrite_docusaurus_generate",
		Description: "Generate complete Docusaurus v3 Docs-as-Code website, sidebar taxonomy, project sharing hub, and portable manifest from markdown specifications",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"docs_dir":     TypedProp("string", "Source markdown documentation directory (default: docs)"),
				"target_dir":   TypedProp("string", "Target Docusaurus site directory (default: website)"),
				"title":        TypedProp("string", "Site title (default: Dendrite Documentation)"),
				"port":         TypedProp("integer", "Local preview port (default: 3000)"),
				"sharing_tier": TypedProp("string", "Distribution visibility tier: 'public', 'internal', 'federated' (default: federated)"),
			},
		},
	}, docusaurusGenerateHandler(svc))

	// dendrite_docusaurus_share
	s.RegisterTool(Tool{
		Name:        "dendrite_docusaurus_share",
		Description: "Package Docusaurus documentation into a portable distribution bundle (.tar.gz) with cryptographic SHA-256 sharing-manifest.json",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"site_dir":    TypedProp("string", "Generated Docusaurus site directory (default: website)"),
				"bundle_path": TypedProp("string", "Destination archive path (default: <site_dir>/dendrite-docs.tar.gz)"),
			},
		},
	}, docusaurusShareHandler(svc))

	// dendrite_docusaurus_audit
	s.RegisterTool(Tool{
		Name:        "dendrite_docusaurus_audit",
		Description: "Audit documentation completeness, frontmatter compliance, and feature traceability against codebase packages and Plane tickets",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_root": TypedProp("string", "Project root directory (default: .)"),
				"docs_dir":     TypedProp("string", "Documentation source directory (default: docs)"),
			},
		},
	}, docusaurusAuditHandler(svc))
}

// docusaurusGenerateHandler handles the dendrite_docusaurus_generate tool.
func docusaurusGenerateHandler(svc docusaurus.Service) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		docsDir := StringArgOr(args, "docs_dir", "docs")
		targetDir := StringArgOr(args, "target_dir", "website")
		title := StringArgOr(args, "title", "Dendrite Documentation")
		portFloat, _ := args["port"].(float64)
		port := int(portFloat)
		if port <= 0 {
			port = 3000
		}
		tierStr, _ := args["sharing_tier"].(string)
		tier := docusaurus.SharingTierFederated
		if tierStr != "" {
			tier = docusaurus.SharingTier(strings.ToLower(tierStr))
		}

		cfgBuilder := docusaurus.NewSiteConfigBuilder().
			WithTitle(title).
			WithPort(port).
			WithSharingTier(tier)

		cfg, err := cfgBuilder.Build()
		if err != nil {
			return nil, fmt.Errorf("invalid site configuration: %w", err)
		}

		res, err := svc.Generate(ctx, docsDir, targetDir, cfg)
		if err != nil {
			return nil, fmt.Errorf("docusaurus generation failed: %w", err)
		}

		data := IndentJSON(res)
		return TextResult(string(data)), nil
	}
}

// docusaurusShareHandler handles the dendrite_docusaurus_share tool.
func docusaurusShareHandler(svc docusaurus.Service) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		siteDir := StringArgOr(args, "site_dir", "website")
		bundlePath, _ := args["bundle_path"].(string)

		manifest, err := svc.ExportBundle(ctx, siteDir, bundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed creating sharing bundle: %w", err)
		}

		data := IndentJSON(manifest)
		return TextResult(string(data)), nil
	}
}

// docusaurusAuditHandler handles the dendrite_docusaurus_audit tool.
func docusaurusAuditHandler(svc docusaurus.Service) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		projRoot := StringArgOr(args, "project_root", ".")
		docsDir := StringArgOr(args, "docs_dir", "docs")

		summary, err := svc.Audit(ctx, projRoot, docsDir)
		if err != nil {
			return nil, fmt.Errorf("documentation audit failed: %w", err)
		}

		data := IndentJSON(summary)
		return TextResult(string(data)), nil
	}
}
