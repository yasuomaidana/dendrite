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
	for _, c := range []struct {
		name  string
		issue plane.PlaneIssue
		want  string
	}{
		{"html summary", plane.PlaneIssue{DescriptionHTML: "<p>From <b>HTML</b></p>", DescriptionStripped: "plain"}, "From **HTML**"},
		{"stripped fallback", plane.PlaneIssue{DescriptionStripped: "  plain  "}, "plain"},
	} {
		if got := summaryOf(c.issue); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}
}
