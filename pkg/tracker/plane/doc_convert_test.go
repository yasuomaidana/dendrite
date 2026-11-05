package plane

import (
	"strings"
	"testing"
)

func TestMarkdownToHTMLBlocks(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want []string
	}{
		{"every heading level", "# a\n## b\n### c\n#### d\n##### e\n###### f", []string{"<h1>a</h1>", "<h2>b</h2>", "<h3>c</h3>", "<h4>d</h4>", "<h5>e</h5>", "<h6>f</h6>"}},
		{"quote, rules and paragraph", "> q\n---\n***\ntext", []string{"<blockquote>q</blockquote>", "<hr/><hr/>", "<p>text</p>"}},
		{"lists switch kind and close on a blank line", "- a\n* b\n1. c\n2. d\n- e\n\npara", []string{"<ul><li>a</li><li>b</li></ul>", "<ol><li>c</li><li>d</li></ol>", "<ul><li>e</li></ul>", "<p>para</p>"}},
		{"a list ends at a paragraph", "- a\ntext\n1. b\nmore", []string{"</ul><p>text</p>", "</ol><p>more</p>"}},
		{"a fence closes an open list and table", "- a\n```\ncode <x>\n```\n| h |\n|---|\n| c |\n```\nz\n```", []string{"</ul><pre><code>", "code &lt;x&gt;", "</tbody></table><pre><code>"}},
		{"a table closes at the next line", "| a | b |\n|:--|--:|\n| 1 | **2** |\n\nafter", []string{"<th>a</th>", "</thead><tbody>", "<td><strong>2</strong></td>", "</tbody></table><p>after</p>"}},
		{"a table interrupts a list", "- a\n| h |\n| c |", []string{"</ul><table>"}},
		{"a separator without a table is ignored", "|---|\ntext", []string{"<p>text</p>"}},
		{"a second separator does not reopen the body", "| h |\n|---|\n|---|\n| c |", []string{"</thead><tbody><tr><td>c</td></tr>"}},
		{"unterminated blocks are closed", "```\nopen", []string{"open\n</code></pre></div>"}},
		{"open list at the end", "- a", []string{"</ul></div>"}},
		{"open ordered list at the end", "1. a", []string{"</ol></div>"}},
		{"open table at the end", "| h |\n| c |", []string{"</tbody></table></div>"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MarkdownToHTML(c.md)
			for _, want := range c.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %s", want, got)
				}
			}
		})
	}
}

func TestHTMLToMarkdownBlocks(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{"a table", `<table><thead><tr><th>a</th><th>b</th></tr></thead><tbody><tr><td>1</td><td>2</td></tr><tr></tr></tbody></table>`, "| a | b |\n| :--- | :--- |\n| 1 | 2 |"},
		{"a table without header cells is left to the tag stripper", `<table><thead><tr></tr></thead><tbody><tr><td>x</td></tr></tbody></table>`, "x"},
		{"code is unescaped", `<pre><code>a &lt; b</code></pre>`, "```\na < b\n```"},
		{"rules and breaks", `<p>a</p><hr/><hr><p>b<br/>c<br>d</p>`, "a\n\n---\n\n---\nb\nc\nd"},
		{"headings and lists", `<h1>t</h1><ul><li>x</li></ul><ol><li>y</li></ol><h6>s</h6>`, "# t\n\n- x\n\n- y\n\n###### s"},
		{"inline marks", `<p><b>a</b> <strong>b</strong> <i>c</i> <em>d</em> <code>e</code> <a href="https://x">f</a></p><blockquote>q</blockquote>`, "**a** **b** *c* *d* `e` [f](https://x)\n\n> q"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HTMLToMarkdown(c.html); got != c.want {
				t.Errorf("HTMLToMarkdown = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSanitizeDocFilenameNeverReturnsEmpty(t *testing.T) {
	for in, want := range map[string]string{"Hello World!": "hello-world", "***": "doc", "": "doc", "A_b-c": "a_b-c"} {
		if got := sanitizeDocFilename(in); got != want {
			t.Errorf("sanitizeDocFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
