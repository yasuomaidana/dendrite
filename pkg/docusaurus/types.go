package docusaurus

import (
	"errors"
	"time"
)

// Typed domain errors for docusaurus operations.
var (
	ErrInvalidConfig      = errors.New("invalid docusaurus configuration")
	ErrEmptySiteName      = errors.New("site name cannot be empty")
	ErrEmptyDocDir        = errors.New("documentation source directory cannot be empty")
	ErrEmptyOutputDir     = errors.New("output directory cannot be empty")
	ErrScaffoldFailed     = errors.New("failed to scaffold docusaurus project")
	ErrBundleExportFailed = errors.New("failed to export documentation sharing bundle")
	ErrDocNotFound        = errors.New("specified document not found")
	ErrServerStartFailed  = errors.New("failed to start documentation preview server")
)

// SharingTier defines the distribution scope for project documentation.
type SharingTier string

const (
	SharingTierPublic    SharingTier = "public"
	SharingTierInternal  SharingTier = "internal"
	SharingTierFederated SharingTier = "federated"
	SharingTierPrivate   SharingTier = "private"
)

// DocCategoryType represents the documentation taxonomy hierarchy.
type DocCategoryType string

const (
	CategoryOverview     DocCategoryType = "overview"
	CategoryArchitecture DocCategoryType = "architecture"
	CategoryFeatures     DocCategoryType = "features"
	CategoryGuides       DocCategoryType = "guides"
	CategoryCLI          DocCategoryType = "cli"
	CategoryQuality      DocCategoryType = "quality"
	CategorySharing      DocCategoryType = "project-sharing"
)

// Frontmatter represents YAML frontmatter metadata for Docusaurus Docs-as-Code pages.
type Frontmatter struct {
	ID              string   `yaml:"id" json:"id"`
	Title           string   `yaml:"title" json:"title"`
	Description     string   `yaml:"description,omitempty" json:"description,omitempty"`
	SidebarLabel    string   `yaml:"sidebar_label,omitempty" json:"sidebar_label,omitempty"`
	SidebarPosition int      `yaml:"sidebar_position,omitempty" json:"sidebar_position,omitempty"`
	Tags            []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Slug            string   `yaml:"slug,omitempty" json:"slug,omitempty"`
}

// DocItem represents an individual ingested and normalized documentation document.
type DocItem struct {
	Frontmatter  Frontmatter
	Category     DocCategoryType
	SourcePath   string
	RelativePath string
	Content      string
	WordCount    int
	HasMermaid   bool
}

// CategoryConfig represents the Docusaurus `_category_.json` schema.
type CategoryConfig struct {
	Label       string                 `json:"label"`
	Position    int                    `json:"position"`
	Link        map[string]interface{} `json:"link,omitempty"`
	Collapsible bool                   `json:"collapsible"`
	Collapsed   bool                   `json:"collapsed"`
	ClassName   string                 `json:"className,omitempty"`
}

// FederationPartner describes a cross-project repository linked in the sharing hub.
type FederationPartner struct {
	ProjectName  string   `json:"project_name"`
	Repository   string   `json:"repository"`
	DocsURL      string   `json:"docs_url"`
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies"`
	Tier         string   `json:"tier"`
}

// SharingManifest records cryptographic checksums, metadata, and ecosystem linkage.
type SharingManifest struct {
	ManifestVersion string              `json:"manifest_version"`
	ProjectName     string              `json:"project_name"`
	Version         string              `json:"version"`
	Description     string              `json:"description"`
	Repository      string              `json:"repository"`
	CommitSHA       string              `json:"commit_sha"`
	CommitDate      string              `json:"commit_date"`
	Branch          string              `json:"branch"`
	ExportedAt      time.Time           `json:"exported_at"`
	Generator       string              `json:"generator"`
	SharingTier     SharingTier         `json:"sharing_tier"`
	TotalDocuments  int                 `json:"total_documents"`
	Categories      []string            `json:"categories"`
	Federation      []FederationPartner `json:"federation"`
	FileChecksums   map[string]string   `json:"file_checksums"`
	ContainerImage  string              `json:"container_image,omitempty"`
}

// SiteConfig represents complete configuration for synthesizing Docusaurus v3 sites.
type SiteConfig struct {
	Title            string              `json:"title"`
	Tagline          string              `json:"tagline"`
	URL              string              `json:"url"`
	BaseURL          string              `json:"base_url"`
	OrganizationName string              `json:"organization_name"`
	ProjectName      string              `json:"project_name"`
	GitRepo          string              `json:"git_repo"`
	Favicon          string              `json:"favicon"`
	ThemeDark        string              `json:"theme_dark"`
	ThemeLight       string              `json:"theme_light"`
	EnableMermaid    bool                `json:"enable_mermaid"`
	EnableSearch     bool                `json:"enable_search"`
	Port             int                 `json:"port"`
	SharingTier      SharingTier         `json:"sharing_tier"`
	Federation       []FederationPartner `json:"federation"`
}

// AuditReport summarizes frontmatter compliance and link validation.
type AuditReport struct {
	CheckedDocuments    int      `json:"checked_documents"`
	CompliantDocuments  int      `json:"compliant_documents"`
	MissingFrontmatter  []string `json:"missing_frontmatter"`
	MissingCategories   []string `json:"missing_categories"`
	BrokenInternalLinks []string `json:"broken_internal_links"`
	UncategorizedFiles  []string `json:"uncategorized_files"`
	ComplianceScore     float64  `json:"compliance_score"`
	Passed              bool     `json:"passed"`
}

// GenerateResult reports the output of site generation.
type GenerateResult struct {
	SiteDirectory   string       `json:"site_directory"`
	DocsDirectory   string       `json:"docs_directory"`
	TotalDocs       int          `json:"total_docs"`
	CategoriesCount int          `json:"categories_count"`
	ManifestPath    string       `json:"manifest_path"`
	BundlePath      string       `json:"bundle_path,omitempty"`
	Audit           *AuditReport `json:"audit,omitempty"`
	DurationMs      int64        `json:"duration_ms"`
}
