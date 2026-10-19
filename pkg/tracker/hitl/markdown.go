package hitl

import (
	"html"
	"regexp"
	"strings"
)

var (
	headingRe   = regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)
	listItemRe  = regexp.MustCompile(`(?is)<li[^>]*>`)
	codeRe      = regexp.MustCompile(`(?is)</?code[^>]*>`)
	strongRe    = regexp.MustCompile(`(?is)</?(strong|b)>`)
	blockEndRe  = regexp.MustCompile(`(?is)</(p|div|ul|ol|pre|blockquote)>`)
	lineBreakRe = regexp.MustCompile(`(?is)<br\s*/?>`)
	anyTagRe    = regexp.MustCompile(`(?s)<[^>]+>`)
	blankRunsRe = regexp.MustCompile(`\n{3,}`)
)

// htmlToMarkdown turns the HTML of a Plane description into Markdown, keeping
// headings, list items, inline code and bold text; other markup is dropped.
func htmlToMarkdown(s string) string {
	s = headingRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := headingRe.FindStringSubmatch(m)
		// One level deeper than the source: the issue body already uses "## Summary".
		level := int(parts[1][0]-'0') + 1
		return "\n\n" + strings.Repeat("#", level) + " " + strings.TrimSpace(parts[2]) + "\n\n"
	})
	s = listItemRe.ReplaceAllString(s, "\n- ")
	s = codeRe.ReplaceAllString(s, "`")
	s = strongRe.ReplaceAllString(s, "**")
	s = blockEndRe.ReplaceAllString(s, "\n\n")
	s = lineBreakRe.ReplaceAllString(s, "\n")
	s = html.UnescapeString(anyTagRe.ReplaceAllString(s, ""))
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(blankRunsRe.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
