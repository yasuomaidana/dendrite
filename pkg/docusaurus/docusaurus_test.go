package docusaurus

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSiteConfigBuilder(t *testing.T) {
	t.Run("DefaultBuilderSuccess", func(t *testing.T) {
		cfg, err := NewSiteConfigBuilder().Build()
		if err != nil {
			t.Fatalf("expected valid default config, got error: %v", err)
		}
		if cfg.Title != "Dendrite Documentation" {
			t.Errorf("expected default title 'Dendrite Documentation', got %q", cfg.Title)
		}
		if cfg.Port != 3000 {
			t.Errorf("expected default port 3000, got %d", cfg.Port)
		}
		if !cfg.EnableMermaid {
			t.Errorf("expected mermaid enabled by default")
		}
	})

	t.Run("CustomBuilderValues", func(t *testing.T) {
		cfg, err := NewSiteConfigBuilder().
			WithTitle("Custom Portal").
			WithTagline("Enterprise Knowledge Hub").
			WithURL("https://example.com").
			WithBaseURL("/").
			WithProject("acme", "core-api").
			WithGitRepo("https://github.com/acme/core-api").
			WithMermaid(true).
			WithPort(8080).
			WithSharingTier(SharingTierInternal).
			AddFederationPartner(FederationPartner{
				ProjectName: "docula",
				Repository:  "https://github.com/yasuomaidana/docula",
			}).
			Build()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Title != "Custom Portal" {
			t.Errorf("expected custom title, got %q", cfg.Title)
		}
		if cfg.BaseURL != "/" {
			t.Errorf("expected normalized baseURL '/', got %q", cfg.BaseURL)
		}
		if cfg.GitRepo != "https://github.com/acme/core-api" {
			t.Errorf("expected GitRepo set, got %q", cfg.GitRepo)
		}
		if len(cfg.Federation) != 1 {
			t.Errorf("expected 1 federation partner, got %d", len(cfg.Federation))
		}
	})

	t.Run("InvalidPortValidation", func(t *testing.T) {
		_, err := NewSiteConfigBuilder().WithPort(70000).Build()
		if err == nil {
			t.Fatal("expected error for invalid port > 65535, got nil")
		}
	})

	t.Run("EmptySiteNameValidation", func(t *testing.T) {
		_, err := NewSiteConfigBuilder().WithTitle("").Build()
		if err == nil {
			t.Fatal("expected error for empty site title, got nil")
		}
	})

	t.Run("EmptyProjectNameValidation", func(t *testing.T) {
		_, err := NewSiteConfigBuilder().WithProject("acme", "").Build()
		if err == nil {
			t.Fatal("expected error for empty project name, got nil")
		}
	})
}

func TestManifestBuilder(t *testing.T) {
	mb := NewManifestBuilder("dendrite", "1.2.0")
	manifest, err := mb.
		WithGitMetadata("commit123", "feature-branch", "2026-10-14").
		WithDescription("Test project description").
		WithRepository("https://github.com/test/repo").
		WithCategories([]string{"features", "architecture"}).
		WithChecksums(map[string]string{"doc1.md": "sha256hash"}).
		WithFederation([]FederationPartner{
			{ProjectName: "partner1", Repository: "https://github.com/test/partner1"},
		}).
		WithContainerImage("dendrite/test:latest").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.ProjectName != "dendrite" || manifest.Version != "1.2.0" {
		t.Errorf("unexpected project or version: %s / %s", manifest.ProjectName, manifest.Version)
	}
	if manifest.CommitSHA != "commit123" {
		t.Errorf("expected commit123, got %s", manifest.CommitSHA)
	}
	if manifest.TotalDocuments != 1 {
		t.Errorf("expected 1 total doc, got %d", manifest.TotalDocuments)
	}
	if len(manifest.Federation) != 1 {
		t.Errorf("expected 1 federation entry, got %d", len(manifest.Federation))
	}

	// Empty project name error
	emptyMB := NewManifestBuilder("", "1.0.0")
	if _, err := emptyMB.Build(); err == nil {
		t.Errorf("expected error for empty project name in manifest builder")
	}
}

func TestScaffold(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "docusaurus-scaffold-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg, err := NewSiteConfigBuilder().Build()
	if err != nil {
		t.Fatalf("failed to build default config: %v", err)
	}

	err = Scaffold(tempDir, cfg)
	if err != nil {
		t.Fatalf("scaffold failed: %v", err)
	}

	expectedFiles := []string{
		"package.json",
		"docusaurus.config.ts",
		"sidebars.ts",
		"src/css/custom.css",
		"src/pages/index.tsx",
		"src/pages/share.tsx",
		"Dockerfile",
		"docker-compose.share.yaml",
		"nginx.conf",
		"static/img/logo.svg",
	}

	for _, f := range expectedFiles {
		fullPath := filepath.Join(tempDir, f)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected file missing: %s", f)
		}
	}

	// Nil config error
	if err := Scaffold(tempDir, nil); err == nil {
		t.Errorf("expected error when scaffolding with nil config")
	}
}

func TestDocumentTransformer(t *testing.T) {
	// Test validation
	invalidT := NewDocumentTransformer("", "")
	if _, _, err := invalidT.Transform(); err == nil {
		t.Errorf("expected error for empty source dir")
	}
	invalidT2 := NewDocumentTransformer("somedir", "")
	if _, _, err := invalidT2.Transform(); err == nil {
		t.Errorf("expected error for empty target dir")
	}

	srcDir, err := os.MkdirTemp("", "docusaurus-src-*")
	if err != nil {
		t.Fatalf("failed to create temp src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)

	destDir, err := os.MkdirTemp("", "docusaurus-dest-*")
	if err != nil {
		t.Fatalf("failed to create temp dest dir: %v", err)
	}
	defer os.RemoveAll(destDir)

	// Create sample markdown documents
	featContent := `---
id: FEAT-999
title: "Test Feature Spec"
sidebar_position: 10
tags: [test, spec]
---

# FEAT-999: Test Feature Spec

## 1. User Story
As a user, I want automated documentation.

## 2. Acceptance Criteria
- [x] AC-1: Generates clean markdown.
`
	archContent := `# ADR-001 Architecture Decision
System architecture details.
`
	guideContent := `Quickstart instructions without title.`
	cliContent := `# CLI Reference Guide
CLI command reference.
`
	qualityContent := `# Quality Scorecard
Scorecard metrics.
`
	featWithExtras := `---
id: FEAT-888
title: "Advanced Spec"
description: "Detailed description"
sidebar_label: "Adv Spec"
slug: "/features/advanced"
sidebar_position: 15
---

# Advanced Spec

Detailed body text.
`

	_ = os.WriteFile(filepath.Join(srcDir, "FEAT-999-sample.md"), []byte(featContent), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "ADR-001-sample.md"), []byte(archContent), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "quickstart-guide.md"), []byte(guideContent), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "cli-commands.md"), []byte(cliContent), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "quality-scorecard.md"), []byte(qualityContent), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "FEAT-888-advanced.md"), []byte(featWithExtras), 0644)
	diagramStr := "flowchart LR\nA --> B\n"
	_ = os.WriteFile(filepath.Join(srcDir, "diagram.md"), []byte(diagramStr), 0644)
	mixedStr := "```\nsome code\n```\n\nflowchart TD\nX --> Y\n\n# Heading\n"
	_ = os.WriteFile(filepath.Join(srcDir, "mixed.md"), []byte(mixedStr), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "unnamed-spec.md"), []byte(`User Story: story
Acceptance Criteria: ac`), 0644)

	transformer := NewDocumentTransformer(srcDir, destDir)
	items, report, err := transformer.Transform()
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}

	if len(items) != 9 {
		t.Errorf("expected 6 transformed items, got %d", len(items))
	}
	if report.CheckedDocuments != 9 {
		t.Errorf("expected 6 checked documents, got %d", report.CheckedDocuments)
	}

	// Verify categories
	featuresDir := filepath.Join(destDir, "docs", "features")
	if _, err := os.Stat(filepath.Join(featuresDir, "_category_.json")); os.IsNotExist(err) {
		t.Errorf("expected _category_.json in features dir")
	}

	sharingDir := filepath.Join(destDir, "docs", "project-sharing")
	if _, err := os.Stat(filepath.Join(sharingDir, "project-sharing-overview.md")); os.IsNotExist(err) {
		t.Errorf("expected project-sharing-overview.md generated")
	}
}

func TestManifestAndBundle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "docusaurus-bundle-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	docsDir := filepath.Join(tempDir, "docs")
	_ = os.MkdirAll(docsDir, 0755)
	_ = os.WriteFile(filepath.Join(docsDir, "sample.md"), []byte("# Sample Doc\nHello world"), 0644)

	// Nil config error
	if _, err := GenerateSharingManifest(tempDir, nil); err == nil {
		t.Errorf("expected error for nil config in GenerateSharingManifest")
	}

	cfg, _ := NewSiteConfigBuilder().Build()
	manifest, err := GenerateSharingManifest(tempDir, cfg)
	if err != nil {
		t.Fatalf("generate manifest failed: %v", err)
	}

	if manifest.TotalDocuments != 1 {
		t.Errorf("expected 1 document in manifest, got %d", manifest.TotalDocuments)
	}
	if len(manifest.FileChecksums) != 1 {
		t.Errorf("expected 1 checksum in manifest, got %d", len(manifest.FileChecksums))
	}

	bundlePath := filepath.Join(tempDir, "test-docs.tar.gz")
	err = ExportSharingArchive(tempDir, bundlePath)
	if err != nil {
		t.Fatalf("export archive failed: %v", err)
	}

	if info, err := os.Stat(bundlePath); err != nil || info.Size() == 0 {
		t.Fatalf("bundle archive was not created or is empty")
	}

	// Test invalid export archive paths
	_, _ = calculateSHA256("non-existent-file-xyz")
	if err := ExportSharingArchive("", ""); err == nil {
		t.Errorf("expected error for empty paths in ExportSharingArchive")
	}
}

func TestServerLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "docusaurus-server-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manifestBytes, _ := json.Marshal(map[string]string{"project": "dendrite"})
	_ = os.WriteFile(filepath.Join(tempDir, "sharing-manifest.json"), manifestBytes, 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "dendrite-docs.tar.gz"), []byte("dummy-archive"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "index.html"), []byte("<html><body>Hello Docusaurus</body></html>"), 0644)

	// Test invalid start
	if _, err := StartPreviewServer("", 0); err == nil {
		t.Errorf("expected error when starting server with empty site dir")
	}

	srv, err := StartPreviewServer(tempDir, 39999)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	if srv.Addr() == "" {
		t.Errorf("expected non-empty Addr()")
	}

	time.Sleep(50 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get("http://localhost:39999/api/v1/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Test manifest endpoint
	respManifest, err := http.Get("http://localhost:39999/api/v1/sharing-manifest")
	if err != nil {
		t.Fatalf("manifest query failed: %v", err)
	}
	defer respManifest.Body.Close()
	if respManifest.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for manifest, got %d", respManifest.StatusCode)
	}

	// Test download-bundle endpoint
	respBundle, err := http.Get("http://localhost:39999/api/v1/download-bundle")
	if err != nil {
		t.Fatalf("bundle download failed: %v", err)
	}
	defer respBundle.Body.Close()
	if respBundle.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for bundle download, got %d", respBundle.StatusCode)
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}
	// Redundant shutdown should succeed cleanly
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("redundant shutdown error: %v", err)
	}
}

func TestAgentAndService(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "docusaurus-svc-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	docsDir := filepath.Join(tempDir, "docs")
	pkgDir := filepath.Join(tempDir, "pkg", "samplepkg")
	_ = os.MkdirAll(docsDir, 0755)
	_ = os.MkdirAll(pkgDir, 0755)

	featContent := `# FEAT-001: Sample Feature
## 3. Acceptance Criteria
- [x] AC-1: Pass
`
	_ = os.WriteFile(filepath.Join(docsDir, "FEAT-001-sample.md"), []byte(featContent), 0644)

	// Test agent with empty docsDir
	agentDefault := NewDocusaurusAgent(tempDir, "")
	agentNonExistent := NewDocusaurusAgent("/non/existent/path/999", "/non/existent/docs/999")
	ctx := context.Background()
	_, _ = agentNonExistent.Audit(ctx)
	if agentDefault == nil {
		t.Fatalf("expected valid agent")
	}

	svc := NewService()

	// Test Init
	initDir := filepath.Join(tempDir, "init-site")
	if err := svc.Init(ctx, initDir); err != nil {
		t.Fatalf("service init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(initDir, "package.json")); os.IsNotExist(err) {
		t.Errorf("package.json missing after service.Init")
	}

	// Test Audit
	summary, err := svc.Audit(ctx, tempDir, docsDir)
	if err != nil {
		t.Fatalf("agent audit failed: %v", err)
	}
	if summary.TotalFeatures != 1 {
		t.Errorf("expected 1 feature in audit, got %d", summary.TotalFeatures)
	}

	// Test Generate
	siteDir := filepath.Join(tempDir, "site")
	res, err := svc.Generate(ctx, docsDir, siteDir, nil)
	if err != nil {
		t.Fatalf("service generate failed: %v", err)
	}
	if res.TotalDocs != 1 {
		t.Errorf("expected 1 total doc, got %d", res.TotalDocs)
	}
	if _, err := os.Stat(res.ManifestPath); os.IsNotExist(err) {
		t.Errorf("manifest file was not generated: %s", res.ManifestPath)
	}
	if _, err := os.Stat(res.BundlePath); os.IsNotExist(err) {
		t.Errorf("bundle file was not generated: %s", res.BundlePath)
	}

	// Test ExportBundle with custom and empty bundlePath
	bundleDest := filepath.Join(tempDir, "exported-bundle.tar.gz")
	manifest, err := svc.ExportBundle(ctx, siteDir, bundleDest)
	if err != nil {
		t.Fatalf("service ExportBundle failed: %v", err)
	}
	if manifest.ProjectName != "dendrite" {
		t.Errorf("unexpected project in exported manifest: %s", manifest.ProjectName)
	}
	// Default bundle path
	_, err = svc.ExportBundle(ctx, siteDir, "")
	if err != nil {
		t.Fatalf("service ExportBundle with empty bundlePath failed: %v", err)
	}
	// Empty siteDir error
	if _, err := svc.ExportBundle(ctx, "", ""); err == nil {
		t.Errorf("expected error for empty siteDir in ExportBundle")
	}

	// Test Serve
	serverInst, err := svc.Serve(ctx, siteDir, 39998)
	if err != nil {
		t.Fatalf("service Serve failed: %v", err)
	}
	_ = serverInst.Shutdown(ctx)

	// Test Build with missing package.json error handling
	emptyDir := filepath.Join(tempDir, "empty")
	_ = os.MkdirAll(emptyDir, 0755)
	if err := svc.Build(ctx, emptyDir); err == nil {
		t.Errorf("expected error when building directory without package.json")
	}
}
