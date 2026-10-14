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
	frontmatterRegex = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n?(.*)$`)
	htmlTagRegex     = regexp.MustCompile(`<[^>]+>`)
	mdLinkRegex      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	codeFenceRegex   = regexp.MustCompile("(?s)```([a-zA-Z0-9_-]*)\n(.*?)\n```")
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

func pullDocsFromPlane(ctx context.Context, client *Client, targetDir string, report *DocSyncReport) error {
	pages, err := client.ListPages(ctx)
	if err != nil {
		return fmt.Errorf("failed to list plane pages: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target docs directory: %w", err)
	}

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

		convertedMD := HTMLToMarkdown(contentHTML)
		filename := sanitizeDocFilename(fullPage.Name) + ".md"
		outPath := filepath.Join(targetDir, filename)

		fileData := fmt.Sprintf("# %s\n\n%s\n", fullPage.Name, strings.TrimSpace(convertedMD))
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

	md = codeFenceRegex.ReplaceAllStringFunc(md, func(match string) string {
		sub := codeFenceRegex.FindStringSubmatch(match)
		if len(sub) > 2 {
			escaped := html.EscapeString(sub[2])
			return fmt.Sprintf("\n<pre><code>%s</code></pre>\n", escaped)
		}
		return match
	})

	lines := strings.Split(md, "\n")
	inList := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inList {
				sb.WriteString("</ul>")
				inList = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "<pre>") {
			if inList {
				sb.WriteString("</ul>")
				inList = false
			}
			sb.WriteString(trimmed)
			continue
		}

		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				sb.WriteString("<ul>")
				inList = true
			}
			itemText := strings.TrimSpace(trimmed[2:])
			itemText = formatInlineMarkdown(itemText)
			sb.WriteString(fmt.Sprintf("<li>%s</li>", itemText))
			continue
		}

		if inList {
			sb.WriteString("</ul>")
			inList = false
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

	if inList {
		sb.WriteString("</ul>")
	}

	sb.WriteString("</div>")
	return sb.String()
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

	res = regexp.MustCompile(`(?s)<pre><code>(.*?)</code></pre>`).ReplaceAllStringFunc(res, func(m string) string {
		inner := regexp.MustCompile(`(?s)<pre><code>(.*?)</code></pre>`).FindStringSubmatch(m)
		if len(inner) > 1 {
			return fmt.Sprintf("\n```\n%s\n```\n", html.UnescapeString(inner[1]))
		}
		return m
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
