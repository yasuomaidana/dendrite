package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/docusaurus"
)

func registerDocusaurusTools(s *Server, deps Dependencies) {
	svc := docusaurus.NewService()

	// dendrite_docusaurus_generate
	s.RegisterTool(Tool{
		Name:        "dendrite_docusaurus_generate",
		Description: "Generate complete Docusaurus v3 Docs-as-Code website, sidebar taxonomy, project sharing hub, and portable manifest from markdown specifications",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"docs_dir":     map[string]interface{}{"type": "string", "description": "Source markdown documentation directory (default: docs)"},
				"target_dir":   map[string]interface{}{"type": "string", "description": "Target Docusaurus site directory (default: website)"},
				"title":        map[string]interface{}{"type": "string", "description": "Site title (default: Dendrite Documentation)"},
				"port":         map[string]interface{}{"type": "integer", "description": "Local preview port (default: 3000)"},
				"sharing_tier": map[string]interface{}{"type": "string", "description": "Distribution visibility tier: 'public', 'internal', 'federated' (default: federated)"},
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
				"site_dir":    map[string]interface{}{"type": "string", "description": "Generated Docusaurus site directory (default: website)"},
				"bundle_path": map[string]interface{}{"type": "string", "description": "Destination archive path (default: <site_dir>/dendrite-docs.tar.gz)"},
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
				"project_root": map[string]interface{}{"type": "string", "description": "Project root directory (default: .)"},
				"docs_dir":     map[string]interface{}{"type": "string", "description": "Documentation source directory (default: docs)"},
			},
		},
	}, docusaurusAuditHandler(svc))
}

// docusaurusGenerateHandler handles the dendrite_docusaurus_generate tool.
func docusaurusGenerateHandler(svc docusaurus.Service) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		docsDir := stringArgOr(args, "docs_dir", "docs")
		targetDir := stringArgOr(args, "target_dir", "website")
		title := stringArgOr(args, "title", "Dendrite Documentation")
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

		data := indentJSON(res)
		return textResult(string(data)), nil
	}
}

// docusaurusShareHandler handles the dendrite_docusaurus_share tool.
func docusaurusShareHandler(svc docusaurus.Service) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		siteDir := stringArgOr(args, "site_dir", "website")
		bundlePath, _ := args["bundle_path"].(string)

		manifest, err := svc.ExportBundle(ctx, siteDir, bundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed creating sharing bundle: %w", err)
		}

		data := indentJSON(manifest)
		return textResult(string(data)), nil
	}
}

// docusaurusAuditHandler handles the dendrite_docusaurus_audit tool.
func docusaurusAuditHandler(svc docusaurus.Service) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		projRoot := stringArgOr(args, "project_root", ".")
		docsDir := stringArgOr(args, "docs_dir", "docs")

		summary, err := svc.Audit(ctx, projRoot, docsDir)
		if err != nil {
			return nil, fmt.Errorf("documentation audit failed: %w", err)
		}

		data := indentJSON(summary)
		return textResult(string(data)), nil
	}
}
