package plane

import (
	"context"
	"fmt"
	"html"
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

// MarkdownToHTML converts markdown text into semantic HTML for Plane pages.
func MarkdownToHTML(md string) string {
	var sb strings.Builder
	sb.WriteString("<div>")

	lines := strings.Split(md, "\n")
	inCodeBlock := false
	inList := false
	inOrderedList := false
	inTable := false
	tableHeaderDone := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Code block fence toggle (supports ```lang and indented ```)
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				sb.WriteString("</code></pre>")
				inCodeBlock = false
			} else {
				if inList {
					sb.WriteString("</ul>")
					inList = false
				}
				if inOrderedList {
					sb.WriteString("</ol>")
					inOrderedList = false
				}
				if inTable {
					sb.WriteString("</tbody></table>")
					inTable = false
					tableHeaderDone = false
				}
				sb.WriteString("<pre><code>")
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			sb.WriteString(html.EscapeString(line))
			sb.WriteString("\n")
			continue
		}

		// Table handling
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			if inList {
				sb.WriteString("</ul>")
				inList = false
			}
			if inOrderedList {
				sb.WriteString("</ol>")
				inOrderedList = false
			}

			cells := strings.Split(trimmed, "|")
			var rowCells []string
			for _, cell := range cells[1 : len(cells)-1] {
				rowCells = append(rowCells, strings.TrimSpace(cell))
			}

			// Check if this is a separator line (e.g. |:---|:---|)
			isSeparator := len(rowCells) > 0
			for _, cell := range rowCells {
				cleaned := strings.ReplaceAll(strings.ReplaceAll(cell, "-", ""), ":", "")
				if len(cleaned) > 0 || len(cell) == 0 {
					isSeparator = false
					break
				}
			}

			if isSeparator {
				if inTable && !tableHeaderDone {
					sb.WriteString("</thead><tbody>")
					tableHeaderDone = true
				}
				continue
			}

			if !inTable {
				sb.WriteString("<table><thead><tr>")
				for _, cell := range rowCells {
					sb.WriteString("<th>" + formatInlineMarkdown(cell) + "</th>")
				}
				sb.WriteString("</tr>")
				inTable = true
				tableHeaderDone = false
				continue
			}

			if inTable {
				sb.WriteString("<tr>")
				for _, cell := range rowCells {
					sb.WriteString("<td>" + formatInlineMarkdown(cell) + "</td>")
				}
				sb.WriteString("</tr>")
				continue
			}
		} else if inTable {
			sb.WriteString("</tbody></table>")
			inTable = false
			tableHeaderDone = false
		}

		// Blank lines
		if trimmed == "" {
			if inList {
				sb.WriteString("</ul>")
				inList = false
			}
			if inOrderedList {
				sb.WriteString("</ol>")
				inOrderedList = false
			}
			continue
		}

		// Unordered list item
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if inOrderedList {
				sb.WriteString("</ol>")
				inOrderedList = false
			}
			if !inList {
				sb.WriteString("<ul>")
				inList = true
			}
			itemText := formatInlineMarkdown(strings.TrimSpace(trimmed[2:]))
			sb.WriteString(fmt.Sprintf("<li>%s</li>", itemText))
			continue
		}

		// Ordered list item (e.g. "1. ", "2. ")
		if isOrderedListItem(trimmed) {
			if inList {
				sb.WriteString("</ul>")
				inList = false
			}
			if !inOrderedList {
				sb.WriteString("<ol>")
				inOrderedList = true
			}
			itemText := formatInlineMarkdown(extractOrderedListItemText(trimmed))
			sb.WriteString(fmt.Sprintf("<li>%s</li>", itemText))
			continue
		}

		if inList {
			sb.WriteString("</ul>")
			inList = false
		}
		if inOrderedList {
			sb.WriteString("</ol>")
			inOrderedList = false
		}

		switch {
		case strings.HasPrefix(trimmed, "###### "):
			sb.WriteString(fmt.Sprintf("<h6>%s</h6>", formatInlineMarkdown(trimmed[7:])))
		case strings.HasPrefix(trimmed, "##### "):
			sb.WriteString(fmt.Sprintf("<h5>%s</h5>", formatInlineMarkdown(trimmed[6:])))
		case strings.HasPrefix(trimmed, "#### "):
			sb.WriteString(fmt.Sprintf("<h4>%s</h4>", formatInlineMarkdown(trimmed[5:])))
		case strings.HasPrefix(trimmed, "### "):
			sb.WriteString(fmt.Sprintf("<h3>%s</h3>", formatInlineMarkdown(trimmed[4:])))
		case strings.HasPrefix(trimmed, "## "):
			sb.WriteString(fmt.Sprintf("<h2>%s</h2>", formatInlineMarkdown(trimmed[3:])))
		case strings.HasPrefix(trimmed, "# "):
			sb.WriteString(fmt.Sprintf("<h1>%s</h1>", formatInlineMarkdown(trimmed[2:])))
		case strings.HasPrefix(trimmed, "> "):
			sb.WriteString(fmt.Sprintf("<blockquote>%s</blockquote>", formatInlineMarkdown(trimmed[2:])))
		case trimmed == "---" || trimmed == "***":
			sb.WriteString("<hr/>")
		default:
			sb.WriteString(fmt.Sprintf("<p>%s</p>", formatInlineMarkdown(trimmed)))
		}
	}

	if inCodeBlock {
		sb.WriteString("</code></pre>")
	}
	if inList {
		sb.WriteString("</ul>")
	}
	if inOrderedList {
		sb.WriteString("</ol>")
	}
	if inTable {
		sb.WriteString("</tbody></table>")
	}

	sb.WriteString("</div>")
	return sb.String()
}

func isOrderedListItem(line string) bool {
	return orderedListRegex.MatchString(line)
}

func extractOrderedListItemText(line string) string {
	m := orderedListRegex.FindStringSubmatch(line)
	if len(m) > 2 {
		return m[2]
	}
	return line
}

// HTMLToMarkdown converts HTML content from Plane pages back into clean Markdown.
func HTMLToMarkdown(htmlContent string) string {
	res := htmlContent
	res = strings.ReplaceAll(res, "<div>", "")
	res = strings.ReplaceAll(res, "</div>", "\n")
	res = strings.ReplaceAll(res, "<hr/>", "\n---\n")
	res = strings.ReplaceAll(res, "<hr>", "\n---\n")
	res = strings.ReplaceAll(res, "<br/>", "\n")
	res = strings.ReplaceAll(res, "<br>", "\n")

	// Pre / Code blocks
	res = regexp.MustCompile(`(?s)<pre><code>(.*?)</code></pre>`).ReplaceAllStringFunc(res, func(m string) string {
		inner := regexp.MustCompile(`(?s)<pre><code>(.*?)</code></pre>`).FindStringSubmatch(m)
		if len(inner) > 1 {
			return fmt.Sprintf("\n```\n%s\n```\n", html.UnescapeString(inner[1]))
		}
		return m
	})

	// Tables
	res = tableMatcherRegex.ReplaceAllStringFunc(res, func(m string) string {
		sub := tableMatcherRegex.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		headerRow := sub[1]
		bodyRows := sub[2]

		var headers []string
		for _, th := range regexp.MustCompile(`<th>(.*?)</th>`).FindAllStringSubmatch(headerRow, -1) {
			if len(th) > 1 {
				headers = append(headers, strings.TrimSpace(th[1]))
			}
		}

		if len(headers) == 0 {
			return m
		}

		var sb strings.Builder
		sb.WriteString("\n| " + strings.Join(headers, " | ") + " |\n")
		var seps []string
		for range headers {
			seps = append(seps, ":---")
		}
		sb.WriteString("| " + strings.Join(seps, " | ") + " |\n")

		for _, tr := range regexp.MustCompile(`(?s)<tr>(.*?)</tr>`).FindAllStringSubmatch(bodyRows, -1) {
			if len(tr) > 1 {
				var cells []string
				for _, td := range regexp.MustCompile(`<td>(.*?)</td>`).FindAllStringSubmatch(tr[1], -1) {
					if len(td) > 1 {
						cells = append(cells, strings.TrimSpace(td[1]))
					}
				}
				if len(cells) > 0 {
					sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")
				}
			}
		}
		sb.WriteString("\n")
		return sb.String()
	})

	res = regexp.MustCompile(`<h1>(.*?)</h1>`).ReplaceAllString(res, "\n# $1\n")
	res = regexp.MustCompile(`<h2>(.*?)</h2>`).ReplaceAllString(res, "\n## $1\n")
	res = regexp.MustCompile(`<h3>(.*?)</h3>`).ReplaceAllString(res, "\n### $1\n")
	res = regexp.MustCompile(`<h4>(.*?)</h4>`).ReplaceAllString(res, "\n#### $1\n")
	res = regexp.MustCompile(`<h5>(.*?)</h5>`).ReplaceAllString(res, "\n##### $1\n")
	res = regexp.MustCompile(`<h6>(.*?)</h6>`).ReplaceAllString(res, "\n###### $1\n")

	res = regexp.MustCompile(`<li>(.*?)</li>`).ReplaceAllString(res, "- $1\n")
	res = strings.ReplaceAll(res, "<ul>", "\n")
	res = strings.ReplaceAll(res, "</ul>", "\n")
	res = strings.ReplaceAll(res, "<ol>", "\n")
	res = strings.ReplaceAll(res, "</ol>", "\n")

	res = regexp.MustCompile(`<blockquote>(.*?)</blockquote>`).ReplaceAllString(res, "\n> $1\n")
	res = regexp.MustCompile(`<p>(.*?)</p>`).ReplaceAllString(res, "\n$1\n")
	res = regexp.MustCompile(`<strong>(.*?)</strong>`).ReplaceAllString(res, "**$1**")
	res = regexp.MustCompile(`<b>(.*?)</b>`).ReplaceAllString(res, "**$1**")
	res = regexp.MustCompile(`<em>(.*?)</em>`).ReplaceAllString(res, "*$1*")
	res = regexp.MustCompile(`<i>(.*?)</i>`).ReplaceAllString(res, "*$1*")
	res = regexp.MustCompile(`<code>(.*?)</code>`).ReplaceAllString(res, "`$1`")
	res = regexp.MustCompile(`<a\s+href="([^"]+)">([^<]+)</a>`).ReplaceAllString(res, "[$2]($1)")

	res = htmlTagRegex.ReplaceAllString(res, "")
	res = html.UnescapeString(res)

	lines := strings.Split(res, "\n")
	var cleaned []string
	prevEmpty := false
	for _, l := range lines {
		trimmed := strings.TrimRight(l, " \t\r")
		if trimmed == "" {
			if !prevEmpty {
				cleaned = append(cleaned, "")
				prevEmpty = true
			}
		} else {
			cleaned = append(cleaned, trimmed)
			prevEmpty = false
		}
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func formatInlineMarkdown(text string) string {
	escaped := html.EscapeString(text)
	escaped = mdLinkRegex.ReplaceAllString(escaped, `<a href="$2">$1</a>`)
	escaped = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(escaped, "<em>$1</em>")
	escaped = regexp.MustCompile("`([^`]+)`").ReplaceAllString(escaped, "<code>$1</code>")
	return escaped
}

func sanitizeDocFilename(name string) string {
	clean := strings.ToLower(name)
	clean = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-")
	if clean == "" {
		return "doc"
	}
	return clean
}
