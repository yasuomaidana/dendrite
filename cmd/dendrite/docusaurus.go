package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"Dendrite/pkg/docgen/reflect"
	"Dendrite/pkg/docusaurus"
)

func printDocusaurusUsage() {
	fmt.Println("Usage: dendrite doc docusaurus <generate|init|share|audit|serve|build> [flags...]")
	fmt.Println("")
	fmt.Println("Subcommands:")
	fmt.Println("  generate [--reference] [--docs <dir>] [--out <dir>] [--title <t>] [--tier <public|internal|federated>]")
	fmt.Println("           Synthesize complete Docusaurus site or live reference documentation (--reference)")
	fmt.Println("  init [--out <dir>]")
	fmt.Println("           Scaffold a blank Docusaurus project skeleton")
	fmt.Println("  share [--site <dir>] [--out <path>]")
	fmt.Println("           Export portable documentation archive and compute SHA-256 manifest")
	fmt.Println("  audit [--strict] [--root <dir>] [--docs <dir>]")
	fmt.Println("           Audit documentation coverage, frontmatter, and feature traceability (--strict for registry enforcement)")
	fmt.Println("  serve [--site <dir>] [--port <port>]")
	fmt.Println("           Start embedded live preview and sharing portal HTTP server")
	fmt.Println("  build [--site <dir>]")
	fmt.Println("           Execute production npm build in target documentation directory")
}

func handleDocusaurusCommand(ctx context.Context, args []string) {
	if len(args) == 0 {
		printDocusaurusUsage()
		return
	}

	svc := docusaurus.NewService()
	action := args[0]
	subArgs := args[1:]

	switch action {
	case "generate":
		handleDocusaurusGenerate(ctx, svc, subArgs)
	case "init":
		handleDocusaurusInit(ctx, svc, subArgs)
	case "share":
		handleDocusaurusShare(ctx, svc, subArgs)
	case "audit":
		handleDocusaurusAudit(ctx, svc, subArgs)
	case "serve":
		handleDocusaurusServe(ctx, svc, subArgs)
	case "build":
		handleDocusaurusBuild(ctx, svc, subArgs)
	default:
		fmt.Printf("[-] Unknown docusaurus action: %s\n\n", action)
		printDocusaurusUsage()
	}
}

func handleDocusaurusGenerate(ctx context.Context, svc docusaurus.Service, args []string) {
	isReference := false
	docsDir := "docs"
	targetDir := "website"
	title := "Dendrite Documentation"
	port := 3000
	tier := docusaurus.SharingTierFederated

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reference":
			isReference = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				docsDir = args[i+1]
				i++
			}
		case "--docs":
			if i+1 < len(args) {
				docsDir = args[i+1]
				i++
			}
		case "--out", "--target":
			if i+1 < len(args) {
				targetDir = args[i+1]
				i++
			}
		case "--title":
			if i+1 < len(args) {
				title = args[i+1]
				i++
			}
		case "--port":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					port = p
				}
				i++
			}
		case "--tier":
			if i+1 < len(args) {
				tier = docusaurus.SharingTier(args[i+1])
				i++
			}
		}
	}

	if isReference {
		reg := buildRegistry()
		srv := buildMCPServer()
		fmt.Printf("[+] Generating reference documentation into '%s'...\n", docsDir)
		res, err := reflect.GenerateReferenceDocs(docsDir, reg, srv)
		if err != nil {
			fmt.Printf("[-] Failed to generate reference docs: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[✓] Reference documentation generated successfully (%d files: %d CLI, %d MCP)\n",
			res.TotalWritten, len(res.CLIFilesWritten), len(res.MCPFilesWritten))
		return
	}

	fmt.Printf("[+] Synthesizing Docusaurus documentation from '%s' into '%s'...\n", docsDir, targetDir)
	cfg, err := docusaurus.NewSiteConfigBuilder().
		WithTitle(title).
		WithPort(port).
		WithSharingTier(tier).
		Build()
	if err != nil {
		fmt.Printf("[-] Configuration error: %v\n", err)
		return
	}

	result, err := svc.Generate(ctx, docsDir, targetDir, cfg)
	if err != nil {
		fmt.Printf("[-] Generation failed: %v\n", err)
		return
	}

	fmt.Printf("[✓] Docusaurus documentation and Project Sharing Hub generated successfully in %dms!\n", result.DurationMs)
	fmt.Printf("    - Target Directory: %s\n", result.SiteDirectory)
	fmt.Printf("    - Total Documents:  %d\n", result.TotalDocs)
	fmt.Printf("    - Categories:       %d\n", result.CategoriesCount)
	fmt.Printf("    - Sharing Manifest: %s\n", result.ManifestPath)
	fmt.Printf("    - Portable Bundle:  %s\n", result.BundlePath)
	if result.Audit != nil {
		fmt.Printf("    - Compliance Score: %.1f%%\n", result.Audit.ComplianceScore*100)
	}
}

func handleDocusaurusInit(ctx context.Context, svc docusaurus.Service, args []string) {
	targetDir := "website"
	if len(args) > 0 && args[0] != "" {
		targetDir = args[0]
	}

	fmt.Printf("[+] Initializing blank Docusaurus site layout in '%s'...\n", targetDir)
	if err := svc.Init(ctx, targetDir); err != nil {
		fmt.Printf("[-] Init failed: %v\n", err)
		return
	}
	fmt.Println("[✓] Docusaurus skeleton initialized.")
}

func handleDocusaurusShare(ctx context.Context, svc docusaurus.Service, args []string) {
	siteDir := "website"
	bundlePath := ""

	for i := 0; i < len(args); i++ {
		if (args[i] == "--site" || args[i] == "--dir") && i+1 < len(args) {
			siteDir = args[i+1]
			i++
		} else if (args[i] == "--out" || args[i] == "--bundle") && i+1 < len(args) {
			bundlePath = args[i+1]
			i++
		}
	}

	fmt.Printf("[+] Creating standalone documentation sharing bundle for '%s'...\n", siteDir)
	manifest, err := svc.ExportBundle(ctx, siteDir, bundlePath)
	if err != nil {
		fmt.Printf("[-] Failed exporting sharing bundle: %v\n", err)
		return
	}

	fmt.Printf("[✓] Successfully exported documentation sharing bundle for '%s' (v%s)!\n", manifest.ProjectName, manifest.Version)
	fmt.Printf("    - Documents:  %d\n", manifest.TotalDocuments)
	fmt.Printf("    - Categories: %d\n", len(manifest.Categories))
	fmt.Printf("    - Commit:     %s (%s)\n", manifest.CommitSHA, manifest.Branch)
}

func handleDocusaurusAudit(ctx context.Context, svc docusaurus.Service, args []string) {
	strict := false
	isReferenceAudit := false
	projectRoot := "."
	docsDir := "docs"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--strict":
			strict = true
			isReferenceAudit = true
		case "--reference":
			isReferenceAudit = true
		case "--root":
			if i+1 < len(args) {
				projectRoot = args[i+1]
				i++
			}
		case "--docs":
			if i+1 < len(args) {
				docsDir = args[i+1]
				i++
			}
		}
	}

	if isReferenceAudit {
		reg := buildRegistry()
		srv := buildMCPServer()
		rep, err := reflect.AuditReferenceDocs(docsDir, reg, srv)
		if err != nil {
			fmt.Printf("[-] Reference audit error: %v\n", err)
			if strict {
				os.Exit(1)
			}
			return
		}
		if !rep.Passed {
			fmt.Println("==================================================")
			fmt.Println("       REFERENCE DOCUMENTATION AUDIT FAILED       ")
			fmt.Println("==================================================")
			for _, item := range rep.Items {
				fmt.Printf("  [-] [%s] %s (%s): %s\n", strings.ToUpper(item.Reason), item.Type, item.Name, item.Path)
			}
			fmt.Println("==================================================")
			fmt.Printf("Total: %d missing, %d stale\n", rep.MissingCount, rep.StaleCount)
			if strict {
				os.Exit(1)
			}
			return
		}
		fmt.Println("[✓] Reference documentation audit passed:")
		fmt.Printf("    - %d total items checked\n", rep.TotalChecked)
		fmt.Println("    - 0 missing, 0 stale")
		return
	}

	fmt.Println("[+] Running Docusaurus & Project Sharing Documentation Audit...")
	summary, err := svc.Audit(ctx, projectRoot, docsDir)
	if err != nil {
		fmt.Printf("[-] Audit error: %v\n", err)
		return
	}

	fmt.Println("==================================================")
	fmt.Println("    DOCUSAURUS DOCUMENTATION AUDIT SCORECARD      ")
	fmt.Println("==================================================")
	fmt.Printf("Total Features Discovered:     %d\n", summary.TotalFeatures)
	fmt.Printf("Features with Acceptance Spec: %d\n", summary.DocumentedFeatures)
	fmt.Printf("Sharing Readiness:             %t\n", summary.SharingPrepared)
	if len(summary.UndocumentedModules) > 0 {
		fmt.Printf("Undocumented Modules (%d):     %s\n", len(summary.UndocumentedModules), summary.UndocumentedModules)
	}
	if len(summary.Recommendations) > 0 {
		fmt.Println("\nRecommendations:")
		for _, rec := range summary.Recommendations {
			fmt.Printf("  * %s\n", rec)
		}
	}
	fmt.Println("==================================================")
}

func handleDocusaurusServe(ctx context.Context, svc docusaurus.Service, args []string) {
	siteDir := "website"
	port := 3000

	for i := 0; i < len(args); i++ {
		if args[i] == "--site" && i+1 < len(args) {
			siteDir = args[i+1]
			i++
		} else if args[i] == "--port" && i+1 < len(args) {
			if p, err := strconv.Atoi(args[i+1]); err == nil {
				port = p
			}
			i++
		}
	}

	fmt.Printf("[+] Launching Docusaurus documentation and sharing preview server on :%d...\n", port)
	inst, err := svc.Serve(ctx, siteDir, port)
	if err != nil {
		fmt.Printf("[-] Server start failed: %v\n", err)
		return
	}
	defer inst.Shutdown(ctx)

	fmt.Printf("[✓] Serving site at http://localhost:%d (Press Ctrl+C to terminate)\n", port)
	fmt.Printf("    - Health:   http://localhost:%d/api/v1/health\n", port)
	fmt.Printf("    - Manifest: http://localhost:%d/api/v1/sharing-manifest\n", port)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\n[+] Shutting down preview server...")
}

func handleDocusaurusBuild(ctx context.Context, svc docusaurus.Service, args []string) {
	siteDir := "website"
	if len(args) > 0 && args[0] != "" {
		siteDir = args[0]
	}

	fmt.Printf("[+] Building production Docusaurus static assets in '%s'...\n", siteDir)
	if err := svc.Build(ctx, siteDir); err != nil {
		fmt.Printf("[-] Build failed: %v\n", err)
		return
	}
	fmt.Println("[✓] Production build completed.")
}
