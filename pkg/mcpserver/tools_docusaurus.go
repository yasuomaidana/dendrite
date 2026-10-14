package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"Dendrite/pkg/docusaurus"
)

func (s *Server) registerDocusaurusTools(deps Dependencies) {
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
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		docsDir, _ := args["docs_dir"].(string)
		if docsDir == "" {
			docsDir = "docs"
		}
		targetDir, _ := args["target_dir"].(string)
		if targetDir == "" {
			targetDir = "website"
		}
		title, _ := args["title"].(string)
		if title == "" {
			title = "Dendrite Documentation"
		}
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

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

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
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		siteDir, _ := args["site_dir"].(string)
		if siteDir == "" {
			siteDir = "website"
		}
		bundlePath, _ := args["bundle_path"].(string)

		manifest, err := svc.ExportBundle(ctx, siteDir, bundlePath)
		if err != nil {
			return nil, fmt.Errorf("failed creating sharing bundle: %w", err)
		}

		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

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
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		projRoot, _ := args["project_root"].(string)
		if projRoot == "" {
			projRoot = "."
		}
		docsDir, _ := args["docs_dir"].(string)
		if docsDir == "" {
			docsDir = "docs"
		}

		summary, err := svc.Audit(ctx, projRoot, docsDir)
		if err != nil {
			return nil, fmt.Errorf("documentation audit failed: %w", err)
		}

		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})
}
