package plane

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	frontmatterRegex  = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?(.*)$`)
	htmlTagRegex      = regexp.MustCompile(`<[^>]+>`)
	mdLinkRegex       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	orderedListRegex  = regexp.MustCompile(`^(\d+)\.\s+(.*)$`)
	tableMatcherRegex = regexp.MustCompile(`(?s)<table><thead><tr>(.*?)</tr></thead><tbody>(.*?)</tbody></table>`)
)

// SyncDocs synchronizes documentation files between the local filesystem and Plane pages.
func SyncDocs(ctx context.Context, client *Client, docsDir string, direction SyncDirection) (*DocSyncReport, error) {
	start := time.Now()
	report := &DocSyncReport{
		Direction: direction,
	}

	if docsDir == "" {
		docsDir = "docs"
	}

	switch direction {
	case DirectionToPlane:
		if err := pushDocsToPlane(ctx, client, docsDir, report); err != nil {
			return report, err
		}
	case DirectionFromPlane:
		if err := pullDocsFromPlane(ctx, client, docsDir, report); err != nil {
			return report, err
		}
	case DirectionBidirectional:
		if err := pushDocsToPlane(ctx, client, docsDir, report); err != nil {
			return report, err
		}
		if err := pullDocsFromPlane(ctx, client, docsDir, report); err != nil {
			return report, err
		}
	default:
		return report, fmt.Errorf("unsupported sync direction: %s", direction)
	}

	report.TotalSynced = report.FilesPushed + report.FilesPulled
	report.Duration = time.Since(start)
	return report, nil
}

func pushDocsToPlane(ctx context.Context, client *Client, docsDir string, report *DocSyncReport) error {
	existingPages, err := client.ListPages(ctx)
	if err != nil {
		return fmt.Errorf("failed to list plane pages: %w", err)
	}

	pageByName := make(map[string]PlanePage)
	for _, p := range existingPages {
		pageByName[strings.ToLower(strings.TrimSpace(p.Name))] = p
	}

	err = filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("read error %s: %v", path, readErr))
			return nil
		}

		rawContent := string(data)
		pageName, bodyMarkdown := extractDocTitleAndBody(rawContent, d.Name())
		pageHTML := MarkdownToHTML(bodyMarkdown)

		key := strings.ToLower(strings.TrimSpace(pageName))
		if existing, found := pageByName[key]; found {
			_, updateErr := client.UpdatePage(ctx, existing.ID, pageName, pageHTML)
			if updateErr != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("update error %s: %v", path, updateErr))
			} else {
				report.PagesUpdated++
				report.FilesPushed++
			}
		} else {
			created, createErr := client.CreatePage(ctx, pageName, pageHTML)
			if createErr != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("create error %s: %v", path, createErr))
			} else {
				report.PagesCreated++
				report.FilesPushed++
				if created != nil {
					pageByName[key] = *created
				}
			}
		}

		return nil
	})

	return err
}

type localDocInfo struct {
	path        string
	id          string
	title       string
	frontmatter string
}

func indexLocalDocs(docsDir string) (map[string]localDocInfo, error) {
	index := make(map[string]localDocInfo)
	err := filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)
		title, _ := extractDocTitleAndBody(content, d.Name())

		id := ""
		fm := ""
		if m := frontmatterRegex.FindStringSubmatch(content); len(m) > 2 {
			fm = m[1]
			for _, line := range strings.Split(fm, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "id:") {
					id = strings.ToUpper(strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "id:")), `"'`))
					break
				}
			}
		}
		if id == "" {
			if m := specRegex.FindString(title); m != "" {
				id = strings.ToUpper(m)
			} else if m := specRegex.FindString(d.Name()); m != "" {
				id = strings.ToUpper(m)
			}
		}

		info := localDocInfo{
			path:        path,
			id:          id,
			title:       title,
			frontmatter: fm,
		}

		if id != "" {
			index["id:"+id] = info
		}
		cleanTitle := strings.ToLower(strings.TrimSpace(title))
		if cleanTitle != "" {
			index["title:"+cleanTitle] = info
		}
		sanitizedName := sanitizeDocFilename(d.Name())
		index["file:"+sanitizedName] = info
		sanitizedTitle := sanitizeDocFilename(title)
		index["sanitized:"+sanitizedTitle] = info

		return nil
	})
	return index, err
}

func findMatchingLocalDoc(pageName string, index map[string]localDocInfo) (localDocInfo, bool) {
	if m := specRegex.FindString(pageName); m != "" {
		if info, ok := index["id:"+strings.ToUpper(m)]; ok {
			return info, true
		}
	}

	cleanTitle := strings.ToLower(strings.TrimSpace(pageName))
	if info, ok := index["title:"+cleanTitle]; ok {
		return info, true
	}

	sanitized := sanitizeDocFilename(pageName)
	if info, ok := index["sanitized:"+sanitized]; ok {
		return info, true
	}
	if info, ok := index["file:"+sanitized]; ok {
		return info, true
	}

	return localDocInfo{}, false
}

func determineTaxonomyPath(docsDir, pageName, bodyMD string) string {
	sanitized := sanitizeDocFilename(pageName) + ".md"
	lowerName := strings.ToLower(pageName)
	lowerBody := strings.ToLower(bodyMD)

	if strings.Contains(lowerName, "scenario") {
		return filepath.Join(docsDir, "SCENARIOS.md")
	}
	if strings.Contains(lowerName, "architecture & development environment") || lowerName == "readme" {
		return filepath.Join(docsDir, "README.md")
	}

	if m := specRegex.FindString(pageName); strings.HasPrefix(m, "FEAT-") {
		return filepath.Join(docsDir, "features", fmt.Sprintf("%s-%s.md", m, sanitizeDocFilename(pageName)))
	}
	if strings.Contains(lowerBody, "plane_epic") || strings.Contains(lowerBody, "feature") {
		return filepath.Join(docsDir, "features", sanitized)
	}

	if m := specRegex.FindString(pageName); strings.HasPrefix(m, "ADR-") {
		return filepath.Join(docsDir, "adr", fmt.Sprintf("%s-%s.md", m, sanitizeDocFilename(pageName)))
	}

	if strings.Contains(lowerName, "guide") || strings.Contains(lowerName, "setup") || strings.Contains(lowerName, "replication") || strings.Contains(lowerName, "onboarding") {
		return filepath.Join(docsDir, "guides", sanitized)
	}

	if strings.Contains(lowerName, "architecture") || strings.Contains(lowerName, "topology") {
		return filepath.Join(docsDir, "architecture", sanitized)
	}

	if strings.Contains(lowerName, "quality") || strings.Contains(lowerName, "scorecard") || strings.Contains(lowerName, "benchmark") {
		return filepath.Join(docsDir, "quality", sanitized)
	}

	if strings.Contains(lowerName, "cli") || strings.Contains(lowerName, "command reference") {
		return filepath.Join(docsDir, "cli", sanitized)
	}

	if strings.Contains(lowerName, "sharing") || strings.Contains(lowerName, "federation") || strings.Contains(lowerName, "deployment") {
		return filepath.Join(docsDir, "project-sharing", sanitized)
	}

	if strings.Contains(lowerName, "roadmap") || strings.Contains(lowerName, "development status") {
		return filepath.Join(docsDir, "roadmap", sanitized)
	}

	return filepath.Join(docsDir, "features", sanitized)
}

func pullDocsFromPlane(ctx context.Context, client *Client, targetDir string, report *DocSyncReport) error {
	pages, err := client.ListPages(ctx)
	if err != nil {
		return fmt.Errorf("failed to list plane pages: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target docs directory: %w", err)
	}

	localIndex, _ := indexLocalDocs(targetDir)

	for _, p := range pages {
		fullPage, getErr := client.GetPage(ctx, p.ID)
		if getErr != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("get page error %s: %v", p.ID, getErr))
			continue
		}

		contentHTML := fullPage.DescriptionHTML
		if strings.TrimSpace(contentHTML) == "" {
			contentHTML = fullPage.DescriptionStripped
		}

		convertedMD := strings.TrimSpace(HTMLToMarkdown(contentHTML))

		var outPath string
		var fileData string

		if matched, ok := findMatchingLocalDoc(fullPage.Name, localIndex); ok {
			outPath = matched.path
			body := convertedMD
			if matched.frontmatter != "" {
				fileData = fmt.Sprintf("---\n%s\n---\n\n%s\n", matched.frontmatter, body)
			} else {
				if !strings.HasPrefix(body, "# ") {
					fileData = fmt.Sprintf("# %s\n\n%s\n", fullPage.Name, body)
				} else {
					fileData = fmt.Sprintf("%s\n", body)
				}
			}
		} else {
			outPath = determineTaxonomyPath(targetDir, fullPage.Name, convertedMD)
			if !strings.HasPrefix(convertedMD, "# ") {
				fileData = fmt.Sprintf("# %s\n\n%s\n", fullPage.Name, convertedMD)
			} else {
				fileData = fmt.Sprintf("%s\n", convertedMD)
			}
		}

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("mkdir error %s: %v", filepath.Dir(outPath), err))
			continue
		}

		// Skip write if content is unchanged to avoid git working tree churn
		if existingBytes, readErr := os.ReadFile(outPath); readErr == nil {
			if strings.TrimSpace(string(existingBytes)) == strings.TrimSpace(fileData) {
				report.FilesPulled++
				continue
			}
			// Check if existing local content produces equivalent HTML to avoid round-trip formatting drift
			_, existingBody := extractDocTitleAndBody(string(existingBytes), filepath.Base(outPath))
			if strings.TrimSpace(MarkdownToHTML(existingBody)) == strings.TrimSpace(fullPage.DescriptionHTML) {
				report.FilesPulled++
				continue
			}
		}

		if writeErr := os.WriteFile(outPath, []byte(fileData), 0644); writeErr != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("write file error %s: %v", outPath, writeErr))
		} else {
			report.FilesPulled++
		}
	}

	return nil
}

func extractDocTitleAndBody(content, defaultFilename string) (string, string) {
	title := ""
	body := content

	if m := frontmatterRegex.FindStringSubmatch(content); len(m) > 2 {
		frontmatter := m[1]
		body = m[2]
		for _, fline := range strings.Split(frontmatter, "\n") {
			trimmed := strings.TrimSpace(fline)
			if strings.HasPrefix(trimmed, "title:") {
				title = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "title:")), `"'`)
				break
			}
		}
	}

	if title == "" {
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "# ") {
				title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
				break
			}
		}
	}

	if title == "" {
		title = strings.TrimSuffix(defaultFilename, filepath.Ext(defaultFilename))
	}

	return title, body
}
