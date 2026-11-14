package plane

// TODO(move:dendrite-plane-rs,DPLANE-7): the Plane HTTP client code (projects, members, checklist, doc sync, markdown to HTML) belongs to dendrite-plane, where newtypes and enums rule out invalid states.
//   When: DPLANE-7 adds the typed methods and a protocol minor bump; then this package keeps only the sidecar client (pkg/planesvc) and the HTTP code is deleted.
//   Stays in Go: ticket workflow policy (canonical states, claims, lifecycle) that is Dendrite behaviour.
//   Until then: fix bugs here, but do not add features; add them in the Rust repository.

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// mdWriter builds the HTML of MarkdownToHTML and remembers which block is open.
type mdWriter struct {
	sb         strings.Builder
	inList     bool
	inOrdered  bool
	inTable    bool
	headerDone bool
}

func (w *mdWriter) closeList() {
	if w.inList {
		w.sb.WriteString("</ul>")
		w.inList = false
	}
}

func (w *mdWriter) closeOrdered() {
	if w.inOrdered {
		w.sb.WriteString("</ol>")
		w.inOrdered = false
	}
}

func (w *mdWriter) closeLists() {
	w.closeList()
	w.closeOrdered()
}

func (w *mdWriter) closeTable() {
	if w.inTable {
		w.sb.WriteString("</tbody></table>")
		w.inTable, w.headerDone = false, false
	}
}

// tableRow writes one table line: the header (opening the table), skips the separator line, or
// writes a body row.
func (w *mdWriter) tableRow(trimmed string) {
	cells := strings.Split(trimmed, "|")
	var rowCells []string
	for _, cell := range cells[1 : len(cells)-1] {
		rowCells = append(rowCells, strings.TrimSpace(cell))
	}

	// A separator line is made of dashes and colons only (|:---|:---|).
	isSeparator := len(rowCells) > 0
	for _, cell := range rowCells {
		cleaned := strings.ReplaceAll(strings.ReplaceAll(cell, "-", ""), ":", "")
		if len(cleaned) > 0 || len(cell) == 0 {
			isSeparator = false
			break
		}
	}

	switch {
	case isSeparator:
		if w.inTable && !w.headerDone {
			w.sb.WriteString("</thead><tbody>")
			w.headerDone = true
		}
	case !w.inTable:
		w.sb.WriteString("<table><thead><tr>")
		w.cells("th", rowCells)
		w.sb.WriteString("</tr>")
		w.inTable, w.headerDone = true, false
	default:
		w.sb.WriteString("<tr>")
		w.cells("td", rowCells)
		w.sb.WriteString("</tr>")
	}
}

func (w *mdWriter) cells(tag string, cells []string) {
	for _, cell := range cells {
		w.sb.WriteString("<" + tag + ">" + formatInlineMarkdown(cell) + "</" + tag + ">")
	}
}

// blockPrefixes maps a markdown line prefix to the HTML element wrapping the rest of the line.
var blockPrefixes = []struct{ prefix, tag string }{
	{"###### ", "h6"}, {"##### ", "h5"}, {"#### ", "h4"}, {"### ", "h3"}, {"## ", "h2"}, {"# ", "h1"},
	{"> ", "blockquote"},
}

// block writes a non-list line as a heading, quote, rule or paragraph.
func (w *mdWriter) block(trimmed string) {
	for _, b := range blockPrefixes {
		if strings.HasPrefix(trimmed, b.prefix) {
			w.sb.WriteString(fmt.Sprintf("<%s>%s</%s>", b.tag, formatInlineMarkdown(trimmed[len(b.prefix):]), b.tag))
			return
		}
	}
	if trimmed == "---" || trimmed == "***" {
		w.sb.WriteString("<hr/>")
		return
	}
	w.sb.WriteString(fmt.Sprintf("<p>%s</p>", formatInlineMarkdown(trimmed)))
}

// MarkdownToHTML converts markdown text into semantic HTML for Plane pages.
func MarkdownToHTML(md string) string {
	w := &mdWriter{}
	w.sb.WriteString("<div>")
	inCodeBlock := false

	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)

		// Code block fence toggle (supports ```lang and indented ```)
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				w.sb.WriteString("</code></pre>")
			} else {
				w.closeLists()
				w.closeTable()
				w.sb.WriteString("<pre><code>")
			}
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			w.sb.WriteString(html.EscapeString(line))
			w.sb.WriteString("\n")
			continue
		}

		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			w.closeLists()
			w.tableRow(trimmed)
			continue
		}
		w.closeTable()

		switch {
		case trimmed == "":
			w.closeLists()
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			w.closeOrdered()
			if !w.inList {
				w.sb.WriteString("<ul>")
				w.inList = true
			}
			w.sb.WriteString(fmt.Sprintf("<li>%s</li>", formatInlineMarkdown(strings.TrimSpace(trimmed[2:]))))
		case orderedListRegex.MatchString(trimmed): // "1. ", "2. "
			w.closeList()
			if !w.inOrdered {
				w.sb.WriteString("<ol>")
				w.inOrdered = true
			}
			w.sb.WriteString(fmt.Sprintf("<li>%s</li>", formatInlineMarkdown(orderedListRegex.FindStringSubmatch(trimmed)[2])))
		default:
			w.closeLists()
			w.block(trimmed)
		}
	}

	if inCodeBlock {
		w.sb.WriteString("</code></pre>")
	}
	w.closeLists()
	w.closeTable()
	w.sb.WriteString("</div>")
	return w.sb.String()
}

var (
	preCodeRegex = regexp.MustCompile(`(?s)<pre><code>(.*?)</code></pre>`)
	thRegex      = regexp.MustCompile(`<th>(.*?)</th>`)
	trRegex      = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	tdRegex      = regexp.MustCompile(`<td>(.*?)</td>`)
)

// htmlToMarkdownRules are applied in order, each turning one element into its markdown form.
var htmlToMarkdownRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`<h1>(.*?)</h1>`), "\n# $1\n"},
	{regexp.MustCompile(`<h2>(.*?)</h2>`), "\n## $1\n"},
	{regexp.MustCompile(`<h3>(.*?)</h3>`), "\n### $1\n"},
	{regexp.MustCompile(`<h4>(.*?)</h4>`), "\n#### $1\n"},
	{regexp.MustCompile(`<h5>(.*?)</h5>`), "\n##### $1\n"},
	{regexp.MustCompile(`<h6>(.*?)</h6>`), "\n###### $1\n"},
	{regexp.MustCompile(`<li>(.*?)</li>`), "- $1\n"},
}

// htmlInlineRules turn the remaining blocks and inline elements into markdown.
var htmlInlineRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`<blockquote>(.*?)</blockquote>`), "\n> $1\n"},
	{regexp.MustCompile(`<p>(.*?)</p>`), "\n$1\n"},
	{regexp.MustCompile(`<strong>(.*?)</strong>`), "**$1**"},
	{regexp.MustCompile(`<b>(.*?)</b>`), "**$1**"},
	{regexp.MustCompile(`<em>(.*?)</em>`), "*$1*"},
	{regexp.MustCompile(`<i>(.*?)</i>`), "*$1*"},
	{regexp.MustCompile(`<code>(.*?)</code>`), "`$1`"},
	{regexp.MustCompile(`<a\s+href="([^"]+)">([^<]+)</a>`), "[$2]($1)"},
}

// captures returns the trimmed first group of every match of re in s.
func captures(re *regexp.Regexp, s string) []string {
	var out []string
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// tableToMarkdown renders one matched <table> (header row and body rows) as a markdown table, or
// returns it unchanged when it has no header cells.
func tableToMarkdown(table string) string {
	sub := tableMatcherRegex.FindStringSubmatch(table)
	headers := captures(thRegex, sub[1])
	if len(headers) == 0 {
		return table
	}
	seps := strings.TrimSuffix(strings.Repeat(":--- | ", len(headers)), " | ")

	var sb strings.Builder
	sb.WriteString("\n| " + strings.Join(headers, " | ") + " |\n")
	sb.WriteString("| " + seps + " |\n")
	for _, tr := range captures(trRegex, sub[2]) {
		if cells := captures(tdRegex, tr); len(cells) > 0 {
			sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
	}
	sb.WriteString("\n")
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

	res = preCodeRegex.ReplaceAllStringFunc(res, func(m string) string {
		return fmt.Sprintf("\n```\n%s\n```\n", html.UnescapeString(preCodeRegex.FindStringSubmatch(m)[1]))
	})
	res = tableMatcherRegex.ReplaceAllStringFunc(res, tableToMarkdown)

	for _, rule := range htmlToMarkdownRules {
		res = rule.re.ReplaceAllString(res, rule.repl)
	}
	for _, tag := range []string{"ul", "ol"} {
		res = strings.ReplaceAll(res, "<"+tag+">", "\n")
		res = strings.ReplaceAll(res, "</"+tag+">", "\n")
	}
	for _, rule := range htmlInlineRules {
		res = rule.re.ReplaceAllString(res, rule.repl)
	}

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

var (
	boldRegex      = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRegex    = regexp.MustCompile(`\*([^*]+)\*`)
	inlineCodeExpr = regexp.MustCompile("`([^`]+)`")
	unsafeNameExpr = regexp.MustCompile(`[^a-z0-9_-]+`)
)

func formatInlineMarkdown(text string) string {
	escaped := html.EscapeString(text)
	escaped = mdLinkRegex.ReplaceAllString(escaped, `<a href="$2">$1</a>`)
	escaped = boldRegex.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = italicRegex.ReplaceAllString(escaped, "<em>$1</em>")
	return inlineCodeExpr.ReplaceAllString(escaped, "<code>$1</code>")
}

func sanitizeDocFilename(name string) string {
	clean := unsafeNameExpr.ReplaceAllString(strings.ToLower(name), "-")
	clean = strings.Trim(clean, "-")
	if clean == "" {
		return "doc"
	}
	return clean
}
