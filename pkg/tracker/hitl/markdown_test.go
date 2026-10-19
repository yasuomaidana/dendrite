package hitl

import (
	"testing"

	"Dendrite/pkg/tracker/plane"
)

func TestHTMLToMarkdown(t *testing.T) {
	in := `<div><h2>Problem</h2><ul><li><code>gh</code> is &quot;hard-coded&quot; &amp; slow</li><li><strong>Bold</strong> item</li></ul><p>Para one<br/>line two</p><h3>Done</h3></div>`
	want := "### Problem\n\n- `gh` is \"hard-coded\" & slow\n- **Bold** item\n\nPara one\nline two\n\n#### Done"
	if got := htmlToMarkdown(in); got != want {
		t.Errorf("htmlToMarkdown:\n%q\nwant\n%q", got, want)
	}
	if htmlToMarkdown("") != "" {
		t.Error("empty input")
	}
}

func TestSummaryOf(t *testing.T) {
	if got := summaryOf(plane.PlaneIssue{DescriptionHTML: "<p>From <b>HTML</b></p>", DescriptionStripped: "plain"}); got != "From **HTML**" {
		t.Errorf("html summary = %q", got)
	}
	if got := summaryOf(plane.PlaneIssue{DescriptionStripped: "  plain  "}); got != "plain" {
		t.Errorf("stripped fallback = %q", got)
	}
}
