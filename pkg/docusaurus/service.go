package docusaurus

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Service defines high-level operations for Docusaurus documentation and project sharing.
type Service interface {
	Init(ctx context.Context, targetDir string) error
	Generate(ctx context.Context, srcDocsDir, targetDir string, cfg *SiteConfig) (*GenerateResult, error)
	ExportBundle(ctx context.Context, siteDir, bundlePath string) (*SharingManifest, error)
	Serve(ctx context.Context, siteDir string, port int) (*ServerInstance, error)
	Audit(ctx context.Context, projectRoot, docsDir string) (*AgentAuditSummary, error)
	Build(ctx context.Context, siteDir string) error
}

type docusaurusService struct{}

// NewService instantiates a default DocusaurusService.
func NewService() Service {
	return &docusaurusService{}
}

// Init creates a fresh scaffold in targetDir.
func (s *docusaurusService) Init(ctx context.Context, targetDir string) error {
	cfg, err := NewSiteConfigBuilder().Build()
	if err != nil {
		return err
	}
	return Scaffold(targetDir, cfg)
}

// Generate orchestrates scaffolding, document ingestion, frontmatter normalization, and manifest synthesis.
func (s *docusaurusService) Generate(ctx context.Context, srcDocsDir, targetDir string, cfg *SiteConfig) (*GenerateResult, error) {
	startTime := time.Now()

	if cfg == nil {
		var err error
		cfg, err = NewSiteConfigBuilder().Build()
		if err != nil {
			return nil, err
		}
	}

	if err := Scaffold(targetDir, cfg); err != nil {
		return nil, fmt.Errorf("scaffold failed: %w", err)
	}

	transformer := NewDocumentTransformer(srcDocsDir, targetDir)
	items, auditReport, err := transformer.Transform()
	if err != nil {
		return nil, fmt.Errorf("document transformation failed: %w", err)
	}

	manifest, err := GenerateSharingManifest(targetDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("manifest generation failed: %w", err)
	}

	manifestPath := filepath.Join(targetDir, "sharing-manifest.json")
	bundlePath := filepath.Join(targetDir, fmt.Sprintf("%s-docs.tar.gz", cfg.ProjectName))
	_ = ExportSharingArchive(targetDir, bundlePath)

	duration := time.Since(startTime).Milliseconds()

	return &GenerateResult{
		SiteDirectory:   targetDir,
		DocsDirectory:   srcDocsDir,
		TotalDocs:       len(items),
		CategoriesCount: len(manifest.Categories),
		ManifestPath:    manifestPath,
		BundlePath:      bundlePath,
		Audit:           auditReport,
		DurationMs:      duration,
	}, nil
}

// ExportBundle compresses target site and verifies manifest.
func (s *docusaurusService) ExportBundle(ctx context.Context, siteDir, bundlePath string) (*SharingManifest, error) {
	if siteDir == "" {
		return nil, ErrEmptyOutputDir
	}
	if bundlePath == "" {
		bundlePath = filepath.Join(siteDir, "dendrite-docs.tar.gz")
	}

	cfg, err := NewSiteConfigBuilder().Build()
	if err != nil {
		return nil, err
	}

	manifest, err := GenerateSharingManifest(siteDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed generating manifest: %w", err)
	}

	if err := ExportSharingArchive(siteDir, bundlePath); err != nil {
		return nil, fmt.Errorf("failed exporting archive: %w", err)
	}

	return manifest, nil
}

// Serve starts the embedded HTTP preview and sharing portal.
func (s *docusaurusService) Serve(ctx context.Context, siteDir string, port int) (*ServerInstance, error) {
	return StartPreviewServer(siteDir, port)
}

// Audit runs agent evaluation on project documentation.
func (s *docusaurusService) Audit(ctx context.Context, projectRoot, docsDir string) (*AgentAuditSummary, error) {
	agent := NewDocusaurusAgent(projectRoot, docsDir)
	return agent.Audit(ctx)
}

// Build executes npm install and npm run build in siteDir if npm exists.
func (s *docusaurusService) Build(ctx context.Context, siteDir string) error {
	if _, err := os.Stat(filepath.Join(siteDir, "package.json")); err != nil {
		return fmt.Errorf("package.json not found in %s", siteDir)
	}

	installCmd := exec.CommandContext(ctx, "npm", "install")
	installCmd.Dir = siteDir
	if out, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm install failed: %s (%w)", string(out), err)
	}

	buildCmd := exec.CommandContext(ctx, "npm", "run", "build")
	buildCmd.Dir = siteDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm run build failed: %s (%w)", string(out), err)
	}

	return nil
}
