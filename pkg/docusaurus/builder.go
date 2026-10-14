package docusaurus

import (
	"fmt"
	"strings"
	"time"
)

// SiteConfigBuilder implements an idiomatic typed builder for SiteConfig.
type SiteConfigBuilder struct {
	config SiteConfig
	errs   []string
}

// NewSiteConfigBuilder instantiates a builder with safe production defaults.
func NewSiteConfigBuilder() *SiteConfigBuilder {
	return &SiteConfigBuilder{
		config: SiteConfig{
			Title:            "Dendrite Documentation",
			Tagline:          "Autonomous AI Development & Distributed Infrastructure Control Plane",
			URL:              "https://dendrite.local",
			BaseURL:          "/",
			OrganizationName: "dendrite",
			ProjectName:      "dendrite",
			GitRepo:          "https://github.com/yasuomaidana/Dendrite",
			Favicon:          "img/favicon.ico",
			EnableMermaid:    true,
			EnableSearch:     true,
			Port:             3000,
			SharingTier:      SharingTierFederated,
			Federation:       make([]FederationPartner, 0),
		},
	}
}

// WithTitle sets the documentation website title.
func (b *SiteConfigBuilder) WithTitle(title string) *SiteConfigBuilder {
	b.config.Title = strings.TrimSpace(title)
	return b
}

// WithTagline sets the website tagline.
func (b *SiteConfigBuilder) WithTagline(tagline string) *SiteConfigBuilder {
	b.config.Tagline = strings.TrimSpace(tagline)
	return b
}

// WithURL sets the public host URL.
func (b *SiteConfigBuilder) WithURL(url string) *SiteConfigBuilder {
	b.config.URL = strings.TrimRight(strings.TrimSpace(url), "/")
	return b
}

// WithBaseURL sets the base URL path.
func (b *SiteConfigBuilder) WithBaseURL(baseURL string) *SiteConfigBuilder {
	clean := "/" + strings.Trim(baseURL, "/") + "/"
	if clean == "//" {
		clean = "/"
	}
	b.config.BaseURL = clean
	return b
}

// WithProject sets project and organization identifiers.
func (b *SiteConfigBuilder) WithProject(org, project string) *SiteConfigBuilder {
	b.config.OrganizationName = strings.TrimSpace(org)
	b.config.ProjectName = strings.TrimSpace(project)
	return b
}

// WithGitRepo sets the repository URL.
func (b *SiteConfigBuilder) WithGitRepo(repo string) *SiteConfigBuilder {
	b.config.GitRepo = strings.TrimSpace(repo)
	return b
}

// WithMermaid configures Mermaid diagram rendering support.
func (b *SiteConfigBuilder) WithMermaid(enabled bool) *SiteConfigBuilder {
	b.config.EnableMermaid = enabled
	return b
}

// WithPort sets local development and preview port.
func (b *SiteConfigBuilder) WithPort(port int) *SiteConfigBuilder {
	if port <= 0 || port > 65535 {
		b.errs = append(b.errs, fmt.Sprintf("invalid port: %d (must be between 1 and 65535)", port))
		return b
	}
	b.config.Port = port
	return b
}

// WithSharingTier defines distribution visibility scope.
func (b *SiteConfigBuilder) WithSharingTier(tier SharingTier) *SiteConfigBuilder {
	b.config.SharingTier = tier
	return b
}

// AddFederationPartner registers a connected ecosystem project.
func (b *SiteConfigBuilder) AddFederationPartner(p FederationPartner) *SiteConfigBuilder {
	if p.ProjectName != "" {
		b.config.Federation = append(b.config.Federation, p)
	}
	return b
}

// Build validates fields and constructs an immutable SiteConfig.
func (b *SiteConfigBuilder) Build() (*SiteConfig, error) {
	if len(b.errs) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, strings.Join(b.errs, "; "))
	}
	if b.config.Title == "" {
		return nil, ErrEmptySiteName
	}
	if b.config.ProjectName == "" {
		return nil, fmt.Errorf("%w: project name cannot be empty", ErrInvalidConfig)
	}

	cfg := b.config
	return &cfg, nil
}

// ManifestBuilder assists in creating verifiable SharingManifest instances.
type ManifestBuilder struct {
	manifest SharingManifest
}

// NewManifestBuilder initializes a SharingManifest builder with current timestamp.
func NewManifestBuilder(project, version string) *ManifestBuilder {
	return &ManifestBuilder{
		manifest: SharingManifest{
			ManifestVersion: "1.0.0",
			ProjectName:     project,
			Version:         version,
			ExportedAt:      time.Now().UTC(),
			Generator:       "Dendrite Docusaurus Engine v1.0.0",
			SharingTier:     SharingTierFederated,
			Categories:      make([]string, 0),
			Federation:      make([]FederationPartner, 0),
			FileChecksums:   make(map[string]string),
		},
	}
}

// WithGitMetadata records commit and branch provenance.
func (mb *ManifestBuilder) WithGitMetadata(commit, branch, commitDate string) *ManifestBuilder {
	mb.manifest.CommitSHA = commit
	mb.manifest.Branch = branch
	mb.manifest.CommitDate = commitDate
	return mb
}

// WithDescription sets project description.
func (mb *ManifestBuilder) WithDescription(desc string) *ManifestBuilder {
	mb.manifest.Description = desc
	return mb
}

// WithRepository sets the git repository URL.
func (mb *ManifestBuilder) WithRepository(repo string) *ManifestBuilder {
	mb.manifest.Repository = repo
	return mb
}

// WithCategories records discovered doc categories.
func (mb *ManifestBuilder) WithCategories(categories []string) *ManifestBuilder {
	mb.manifest.Categories = categories
	return mb
}

// WithChecksums sets the map of file SHA-256 hashes.
func (mb *ManifestBuilder) WithChecksums(checksums map[string]string) *ManifestBuilder {
	mb.manifest.FileChecksums = checksums
	mb.manifest.TotalDocuments = len(checksums)
	return mb
}

// WithFederation registers linked distributed codebase partners.
func (mb *ManifestBuilder) WithFederation(partners []FederationPartner) *ManifestBuilder {
	mb.manifest.Federation = partners
	return mb
}

// WithContainerImage specifies container reference for deployment.
func (mb *ManifestBuilder) WithContainerImage(image string) *ManifestBuilder {
	mb.manifest.ContainerImage = image
	return mb
}

// Build finalizes and returns the SharingManifest.
func (mb *ManifestBuilder) Build() (*SharingManifest, error) {
	if mb.manifest.ProjectName == "" {
		return nil, ErrEmptySiteName
	}
	m := mb.manifest
	return &m, nil
}
