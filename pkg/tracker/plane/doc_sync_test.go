package plane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownToHTML_Conversions(t *testing.T) {
	md := `# Main Title

## Sub Heading

Here is **bold** text and *italic* text and ` + "`code inline`" + `.

> This is a blockquote.

- Item 1
- Item 2 with [link](https://example.com)

` + "```go\nfunc main() {\n    println(\"hello\")\n}\n```" + `

---
`

	htmlOut := MarkdownToHTML(md)
	if !strings.Contains(htmlOut, "<h1>Main Title</h1>") {
		t.Errorf("missing h1 tag in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<h2>Sub Heading</h2>") {
		t.Errorf("missing h2 tag in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<strong>bold</strong>") {
		t.Errorf("missing strong tag in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<em>italic</em>") {
		t.Errorf("missing em tag in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<code>code inline</code>") {
		t.Errorf("missing code tag in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<blockquote>This is a blockquote.</blockquote>") {
		t.Errorf("missing blockquote tag in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<ul><li>Item 1</li>") {
		t.Errorf("missing ul/li tags in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<a href=\"https://example.com\">link</a>") {
		t.Errorf("missing a tag in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<pre><code>") {
		t.Errorf("missing pre/code tags in html: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<hr/>") {
		t.Errorf("missing hr tag in html: %s", htmlOut)
	}
}

func TestHTMLToMarkdown_Conversions(t *testing.T) {
	htmlIn := `<div>
<h1>Architecture Overview</h1>
<p>Dendrite uses <strong>LiteLLM</strong> for <em>budgeting</em> and <code>caching</code>.</p>
<blockquote>Important note</blockquote>
<ul>
<li>Feature 1</li>
<li>Feature 2 with <a href="https://example.com">doc link</a></li>
</ul>
<pre><code>let x = 42;</code></pre>
<hr/>
</div>`

	mdOut := HTMLToMarkdown(htmlIn)
	if !strings.Contains(mdOut, "# Architecture Overview") {
		t.Errorf("missing # heading in markdown: %s", mdOut)
	}
	if !strings.Contains(mdOut, "**LiteLLM**") {
		t.Errorf("missing bold in markdown: %s", mdOut)
	}
	if !strings.Contains(mdOut, "*budgeting*") {
		t.Errorf("missing italic in markdown: %s", mdOut)
	}
	if !strings.Contains(mdOut, "`caching`") {
		t.Errorf("missing code in markdown: %s", mdOut)
	}
	if !strings.Contains(mdOut, "> Important note") {
		t.Errorf("missing blockquote in markdown: %s", mdOut)
	}
	if !strings.Contains(mdOut, "- Feature 1") {
		t.Errorf("missing list item in markdown: %s", mdOut)
	}
	if !strings.Contains(mdOut, "[doc link](https://example.com)") {
		t.Errorf("missing link in markdown: %s", mdOut)
	}
	if !strings.Contains(mdOut, "```") || !strings.Contains(mdOut, "let x = 42;") {
		t.Errorf("missing code block in markdown: %s", mdOut)
	}
	if !strings.Contains(mdOut, "---") {
		t.Errorf("missing hr in markdown: %s", mdOut)
	}
}

func TestSyncDocs_PushAndPull(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dendrite_docs_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	docFile := filepath.Join(tempDir, "FEAT-008.md")
	content := "---\nid: FEAT-008\ntitle: Pluggable Issue Trackers\n---\n\n# Pluggable Trackers\n\nStrategy pattern implementation.\n"
	if err := os.WriteFile(docFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	pagesStore := make(map[string]PlanePage)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pages/"):
			var list []PlanePage
			for _, p := range pagesStore {
				list = append(list, p)
			}
			data, _ := json.Marshal(list)
			_, _ = w.Write(data)

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pages/"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			name := body["name"].(string)
			htmlC, _ := body["description_html"].(string)
			p := PlanePage{
				ID:              "page-101",
				Name:            name,
				DescriptionHTML: htmlC,
			}
			pagesStore[p.ID] = p
			w.WriteHeader(http.StatusCreated)
			data, _ := json.Marshal(p)
			_, _ = w.Write(data)

		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/pages/"):
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			pageID := parts[len(parts)-1]
			p := pagesStore[pageID]
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if n, ok := body["name"].(string); ok {
				p.Name = n
			}
			if h, ok := body["description_html"].(string); ok {
				p.DescriptionHTML = h
			}
			pagesStore[pageID] = p
			data, _ := json.Marshal(p)
			_, _ = w.Write(data)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pages/"):
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			pageID := parts[len(parts)-1]
			p := pagesStore[pageID]
			data, _ := json.Marshal(p)
			_, _ = w.Write(data)

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:       server.URL,
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		DirectToken:   "key",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Push to plane
	pushRep, err := SyncDocs(ctx, client, tempDir, DirectionToPlane)
	if err != nil {
		t.Fatalf("push docs failed: %v", err)
	}
	if pushRep.FilesPushed != 1 || pushRep.PagesCreated != 1 {
		t.Errorf("unexpected push report: %+v", pushRep)
	}

	// 2. Push again to test update path
	pushRep2, err := SyncDocs(ctx, client, tempDir, DirectionToPlane)
	if err != nil {
		t.Fatalf("push update failed: %v", err)
	}
	if pushRep2.PagesUpdated != 1 {
		t.Errorf("expected 1 page updated on second push, got: %+v", pushRep2)
	}

	// 3. Pull from plane
	pullDir, err := os.MkdirTemp("", "pull_docs_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(pullDir)

	pullRep, err := SyncDocs(ctx, client, pullDir, DirectionFromPlane)
	if err != nil {
		t.Fatalf("pull docs failed: %v", err)
	}
	if pullRep.FilesPulled != 1 {
		t.Errorf("expected 1 file pulled, got: %d", pullRep.FilesPulled)
	}

	// 4. Test Bidirectional
	biRep, err := SyncDocs(ctx, client, tempDir, DirectionBidirectional)
	if err != nil {
		t.Fatalf("bidirectional sync failed: %v", err)
	}
	if biRep.TotalSynced < 1 {
		t.Errorf("expected at least 1 synced item, got: %d", biRep.TotalSynced)
	}

	// 5. Test Invalid direction
	_, err = SyncDocs(ctx, client, tempDir, SyncDirection("invalid"))
	if err == nil {
		t.Errorf("expected error for invalid direction")
	}
}
