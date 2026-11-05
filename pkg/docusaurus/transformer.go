package docusaurus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	frontmatterRegex = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n`)
	headingOneRegex  = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	mermaidRegex     = regexp.MustCompile("(?s)```mermaid")
)

// DocumentTransformer handles ingestion, frontmatter normalization, and category layout.
type DocumentTransformer struct {
	sourceDir string
	targetDir string
}

// NewDocumentTransformer creates a new transformer instance.
func NewDocumentTransformer(sourceDir, targetDir string) *DocumentTransformer {
	return &DocumentTransformer{
		sourceDir: sourceDir,
		targetDir: targetDir,
	}
}

// Transform traverses sourceDir and writes categorized Docusaurus documents.
func (t *DocumentTransformer) Transform() ([]DocItem, *AuditReport, error) {
	if t.sourceDir == "" {
		return nil, nil, ErrEmptyDocDir
	}
	if t.targetDir == "" {
		return nil, nil, ErrEmptyOutputDir
	}

	items, report, err := t.collectDocuments()
	if err != nil {
		return nil, nil, err
	}

	for _, item := range items {
		if err := t.writeDocument(item); err != nil {
			return nil, nil, err
		}
	}

	if err := t.writeCategoryDescriptors(); err != nil {
		return nil, nil, err
	}

	if err := t.writeSharingDocumentation(); err != nil {
		return nil, nil, err
	}

	report.ComplianceScore = calculateComplianceScore(report)
	report.Passed = report.ComplianceScore >= 0.85
	return items, report, nil
}

func (t *DocumentTransformer) collectDocuments() ([]DocItem, *AuditReport, error) {
	report := &AuditReport{
		MissingFrontmatter:  make([]string, 0),
		MissingCategories:   make([]string, 0),
		BrokenInternalLinks: make([]string, 0),
		UncategorizedFiles:  make([]string, 0),
	}
	var items []DocItem

	err := filepath.Walk(t.sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || (!strings.HasSuffix(info.Name(), ".md") && !strings.HasSuffix(info.Name(), ".markdown")) {
			return nil
		}

		item, err := t.processFile(path, report)
		if err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})

	if err != nil {
		return nil, nil, fmt.Errorf("failed scanning source directory: %w", err)
	}
	return items, report, nil
}

func (t *DocumentTransformer) processFile(path string, report *AuditReport) (DocItem, error) {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return DocItem{}, fmt.Errorf("cannot read file %s: %w", path, err)
	}
	content := string(contentBytes)

	rel, _ := filepath.Rel(t.sourceDir, path)
	fm, body, hasFM := parseFrontmatter(content)
	report.CheckedDocuments++

	if !hasFM {
		report.MissingFrontmatter = append(report.MissingFrontmatter, rel)
	} else {
		report.CompliantDocuments++
	}

	category := detectCategory(rel, content, fm)
	if category == "" {
		category = CategoryOverview
		report.UncategorizedFiles = append(report.UncategorizedFiles, rel)
	}

	normalizeFrontmatter(&fm, path, body, category)

	item := DocItem{
		Frontmatter:  fm,
		Category:     category,
		SourcePath:   path,
		RelativePath: rel,
		Content:      body,
		WordCount:    len(strings.Fields(body)),
		HasMermaid:   mermaidRegex.MatchString(body),
	}
	return item, nil
}

func parseFrontmatter(content string) (Frontmatter, string, bool) {
	matches := frontmatterRegex.FindStringSubmatch(content)
	if len(matches) < 2 {
		return Frontmatter{}, content, false
	}

	rawYAML := matches[1]
	body := content[len(matches[0]):]
	fm := extractSimpleYAMLFields(rawYAML)
	return fm, body, true
}

func extractSimpleYAMLFields(yamlStr string) Frontmatter {
	var fm Frontmatter
	scanner := bufio.NewScanner(strings.NewReader(yamlStr))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")

		switch key {
		case "id":
			fm.ID = val
		case "title":
			fm.Title = val
		case "description":
			fm.Description = val
		case "sidebar_label":
			fm.SidebarLabel = val
		case "sidebar_position":
			if pos, err := strconv.Atoi(val); err == nil {
				fm.SidebarPosition = pos
			}
		case "slug":
			fm.Slug = val
		}
	}
	return fm
}

func detectCategory(relPath, content string, fm Frontmatter) DocCategoryType {
	lowerRel := strings.ToLower(relPath)
	lowerContent := strings.ToLower(content)

	switch {
	case strings.Contains(lowerRel, "feature") || strings.HasPrefix(filepath.Base(relPath), "FEAT-"):
		return CategoryFeatures
	case strings.Contains(lowerRel, "architecture") || strings.HasPrefix(filepath.Base(relPath), "ADR-"):
		return CategoryArchitecture
	case strings.Contains(lowerRel, "guide") || strings.Contains(lowerRel, "bootstrap") || strings.Contains(lowerRel, "replication"):
		return CategoryGuides
	case strings.Contains(lowerRel, "cli") || strings.Contains(lowerRel, "command"):
		return CategoryCLI
	case strings.Contains(lowerRel, "quality") || strings.Contains(lowerRel, "scorecard"):
		return CategoryQuality
	case strings.Contains(lowerRel, "roadmap") || strings.Contains(lowerRel, "overview"):
		return CategoryOverview
	case strings.Contains(lowerContent, "user story") || strings.Contains(lowerContent, "acceptance criteria"):
		return CategoryFeatures
	default:
		return CategoryOverview
	}
}

func normalizeFrontmatter(fm *Frontmatter, path, body string, cat DocCategoryType) {
	base := filepath.Base(path)
	nameWithoutExt := strings.TrimSuffix(base, filepath.Ext(base))

	if fm.ID == "" {
		fm.ID = nameWithoutExt
	}
	if fm.Title == "" {
		if h1 := headingOneRegex.FindStringSubmatch(body); len(h1) > 1 {
			fm.Title = strings.TrimSpace(h1[1])
		} else {
			fm.Title = formatTitleFromFilename(nameWithoutExt)
		}
	}
	if fm.SidebarLabel == "" {
		fm.SidebarLabel = fm.Title
	}
	if fm.SidebarPosition == 0 {
		fm.SidebarPosition = derivePosition(nameWithoutExt)
	}
}

func formatTitleFromFilename(name string) string {
	cleaned := strings.ReplaceAll(name, "-", " ")
	cleaned = strings.ReplaceAll(cleaned, "_", " ")
	words := strings.Fields(cleaned)
	for i, w := range words {
		if len(w) > 0 {
			r := []rune(w)
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return strings.Join(words, " ")
}

func derivePosition(name string) int {
	re := regexp.MustCompile(`[0-9]+`)
	if match := re.FindString(name); match != "" {
		if val, err := strconv.Atoi(match); err == nil {
			return val
		}
	}
	return 100
}

func (t *DocumentTransformer) writeDocument(item DocItem) error {
	destCategoryDir := filepath.Join(t.targetDir, "docs", string(item.Category))
	if err := os.MkdirAll(destCategoryDir, 0755); err != nil {
		return err
	}

	destPath := filepath.Join(destCategoryDir, filepath.Base(item.SourcePath))
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", item.Frontmatter.ID))
	sb.WriteString(fmt.Sprintf("title: %q\n", item.Frontmatter.Title))
	sb.WriteString(fmt.Sprintf("sidebar_label: %q\n", item.Frontmatter.SidebarLabel))
	sb.WriteString(fmt.Sprintf("sidebar_position: %d\n", item.Frontmatter.SidebarPosition))
	if item.Frontmatter.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %q\n", item.Frontmatter.Description))
	}
	sb.WriteString("---\n\n")

	cleanBody := sanitizeContentForDocusaurus(item.Content)
	sb.WriteString(cleanBody)

	return os.WriteFile(destPath, []byte(sb.String()), 0644)
}

func sanitizeContentForDocusaurus(body string) string {
	cleaned := cleanHTMLEntities(body)
	reExt := regexp.MustCompile(`[\t\\]+ext\{([^}]+)\}`)
	cleaned = reExt.ReplaceAllString(cleaned, " $1")
	reText := regexp.MustCompile(`\\text\{([^}]+)\}`)
	cleaned = reText.ReplaceAllString(cleaned, " $1")
	cleaned = sanitizeInternalLinks(cleaned)
	return wrapUnfencedMermaid(cleaned)
}

func cleanHTMLEntities(body string) string {
	r := strings.NewReplacer(
		"&#34;", "\"",
		"&#39;", "'",
		"&quot;", "\"",
		"&amp;gt;", ">",
		"&amp;lt;", "<",
		"&amp;amp;", "&",
		"&amp;", "&",
		"&lt;br/&gt;", "<br />",
		"&lt;br /&gt;", "<br />",
		"</code></pre>", "",
		"<pre><code>", "",
	)
	return r.Replace(body)
}

func sanitizeInternalLinks(body string) string {
	reDocLink := regexp.MustCompile(`\(\.?\/?(?:docs\/)?(?:[a-zA-Z0-9_\-]+\/)*([a-zA-Z0-9_\-]+)\.md\)`)
	return reDocLink.ReplaceAllString(body, "($1)")
}

func wrapUnfencedMermaid(body string) string {
	lines := strings.Split(body, "\n")
	var result []string
	inCodeBlock := false
	inBareMermaid := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			if inBareMermaid {
				result = append(result, "```")
				inBareMermaid = false
			}
			result = append(result, line)
			continue
		}

		if !inCodeBlock {
			isDiagramStart := strings.HasPrefix(trimmed, "flowchart ") ||
				strings.HasPrefix(trimmed, "sequenceDiagram") ||
				strings.HasPrefix(trimmed, "stateDiagram") ||
				strings.HasPrefix(trimmed, "classDiagram")

			if isDiagramStart && !inBareMermaid {
				inBareMermaid = true
				result = append(result, "```mermaid")
			} else if inBareMermaid && (trimmed == "---" || strings.HasPrefix(trimmed, "#") || (trimmed == "" && len(result) > 0 && result[len(result)-1] == "")) {
				result = append(result, "```")
				inBareMermaid = false
			}
		}

		result = append(result, line)
	}

	if inBareMermaid {
		result = append(result, "```")
	}

	return strings.Join(result, "\n")
}

func (t *DocumentTransformer) writeCategoryDescriptors() error {
	categories := map[DocCategoryType]CategoryConfig{
		CategoryOverview: {
			Label:       "Overview & Roadmap",
			Position:    1,
			Link:        map[string]interface{}{"type": "generated-index"},
			Collapsible: true,
			Collapsed:   false,
		},
		CategoryArchitecture: {
			Label:       "Architecture & Decision Records",
			Position:    2,
			Link:        map[string]interface{}{"type": "generated-index"},
			Collapsible: true,
			Collapsed:   false,
		},
		CategoryFeatures: {
			Label:       "Features & Specifications",
			Position:    3,
			Link:        map[string]interface{}{"type": "generated-index"},
			Collapsible: true,
			Collapsed:   false,
		},
		CategoryGuides: {
			Label:       "Guides & Operations",
			Position:    4,
			Link:        map[string]interface{}{"type": "generated-index"},
			Collapsible: true,
			Collapsed:   false,
		},
		CategoryCLI: {
			Label:       "CLI & Tooling Reference",
			Position:    5,
			Link:        map[string]interface{}{"type": "generated-index"},
			Collapsible: true,
			Collapsed:   false,
		},
		CategoryQuality: {
			Label:       "Quality & Complexity Audits",
			Position:    6,
			Link:        map[string]interface{}{"type": "generated-index"},
			Collapsible: true,
			Collapsed:   false,
		},
		CategorySharing: {
			Label:       "Project Sharing & Ecosystem",
			Position:    7,
			Link:        map[string]interface{}{"type": "generated-index"},
			Collapsible: true,
			Collapsed:   false,
		},
	}

	for cat, cfg := range categories {
		catDir := filepath.Join(t.targetDir, "docs", string(cat))
		if err := os.MkdirAll(catDir, 0755); err != nil {
			return err
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		cfgPath := filepath.Join(catDir, "_category_.json")
		if err := os.WriteFile(cfgPath, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (t *DocumentTransformer) writeSharingDocumentation() error {
	sharingDir := filepath.Join(t.targetDir, "docs", string(CategorySharing))
	if err := os.MkdirAll(sharingDir, 0755); err != nil {
		return err
	}

	files := map[string]string{
		"project-sharing-overview.md": renderSharingOverviewDoc(),
		"federated-ecosystem.md":      renderFederatedEcosystemDoc(),
		"export-and-deployment.md":    renderExportAndDeploymentDoc(),
	}

	for filename, content := range files {
		fullPath := filepath.Join(sharingDir, filename)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

func calculateComplianceScore(r *AuditReport) float64 {
	if r.CheckedDocuments == 0 {
		return 1.0
	}
	compliant := float64(r.CompliantDocuments)
	total := float64(r.CheckedDocuments)
	return compliant / total
}
