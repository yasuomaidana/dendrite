package plane

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

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
