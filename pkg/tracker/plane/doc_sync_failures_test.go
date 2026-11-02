package plane

import (
	"Dendrite/pkg/testkit"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// pagesPlane serves the Plane pages API from memory. A status other than 0 makes that call fail.
type pagesPlane struct {
	*httptest.Server
	pages      map[string]PlanePage
	list       int            // status for GET .../pages/
	listAfter  int32          // lists answered normally before list applies
	get        map[string]int // status per page id for GET .../pages/<id>/
	create     int
	update     int
	listCalled atomic.Int32
}

func newPagesPlane(t *testing.T, pages ...PlanePage) *pagesPlane {
	t.Helper()
	p := &pagesPlane{pages: map[string]PlanePage{}, get: map[string]int{}}
	for _, pg := range pages {
		p.pages[pg.ID] = pg
	}
	p.Server = httptest.NewServer(http.HandlerFunc(p.handle))
	t.Cleanup(p.Close)
	return p
}

func (p *pagesPlane) handle(w http.ResponseWriter, r *http.Request) {
	id := ""
	if rest := strings.SplitN(r.URL.Path, "/pages/", 2); len(rest) == 2 {
		id = strings.Trim(rest[1], "/")
	}
	fail := func(code int) bool {
		if code == 0 {
			return false
		}
		w.WriteHeader(code)
		return true
	}
	switch {
	case r.Method == http.MethodGet && id == "":
		if p.listCalled.Add(1) > p.listAfter && fail(p.list) {
			return
		}
		var list []PlanePage
		for _, pg := range p.pages {
			list = append(list, pg)
		}
		_ = json.NewEncoder(w).Encode(list)
	case r.Method == http.MethodGet:
		if fail(p.get[id]) {
			return
		}
		_ = json.NewEncoder(w).Encode(p.pages[id])
	case r.Method == http.MethodPost:
		if fail(p.create) {
			return
		}
		var body struct{ Name, HTML string }
		_ = json.NewDecoder(r.Body).Decode(&map[string]interface{}{})
		_ = json.NewEncoder(w).Encode(PlanePage{ID: "new", Name: body.Name})
	case r.Method == http.MethodPatch:
		if fail(p.update) {
			return
		}
		_ = json.NewEncoder(w).Encode(p.pages[id])
	}
}

func (p *pagesPlane) client(t *testing.T) *Client {
	t.Helper()
	return planeAt(t, p.URL)
}

func writeDoc(t *testing.T, path, content string) { testkit.WriteFile(t, path, content, 0o644) }

func TestSyncDocsReportsPageListFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("push", func(t *testing.T) {
		p := newPagesPlane(t)
		p.list = 500
		if _, err := SyncDocs(ctx, p.client(t), t.TempDir(), DirectionToPlane); err == nil {
			t.Fatal("expected the list error")
		}
	})
	t.Run("pull", func(t *testing.T) {
		p := newPagesPlane(t)
		p.list = 500
		if _, err := SyncDocs(ctx, p.client(t), t.TempDir(), DirectionFromPlane); err == nil {
			t.Fatal("expected the list error")
		}
	})
	t.Run("bidirectional fails in its push", func(t *testing.T) {
		p := newPagesPlane(t)
		p.list = 500
		if _, err := SyncDocs(ctx, p.client(t), t.TempDir(), DirectionBidirectional); err == nil {
			t.Fatal("expected the push error")
		}
	})
	t.Run("bidirectional fails in its pull", func(t *testing.T) {
		p := newPagesPlane(t)
		p.list, p.listAfter = 500, 1
		if _, err := SyncDocs(ctx, p.client(t), t.TempDir(), DirectionBidirectional); err == nil {
			t.Fatal("expected the pull error")
		}
	})
	t.Run("the default docs directory is docs", func(t *testing.T) {
		t.Chdir(t.TempDir())
		p := newPagesPlane(t)
		if _, err := SyncDocs(ctx, p.client(t), "", DirectionToPlane); err == nil {
			t.Fatal("a missing docs directory must be an error")
		}
	})
}

func TestPushDocsRecordsPerFileFailures(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "existing.md"), "# Existing\n\nbody\n")
	writeDoc(t, filepath.Join(dir, "fresh.md"), "# Fresh\n\nbody\n")
	writeDoc(t, filepath.Join(dir, "locked.md"), "# Locked\n")
	writeDoc(t, filepath.Join(dir, "notes.txt"), "ignored")
	if err := os.Chmod(filepath.Join(dir, "locked.md"), 0); err != nil {
		t.Fatal(err)
	}

	p := newPagesPlane(t, PlanePage{ID: "p1", Name: "Existing"})
	p.update, p.create = 500, 500
	report, err := SyncDocs(context.Background(), p.client(t), dir, DirectionToPlane)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(report.Errors, "\n")
	for _, want := range []string{"read error", "update error", "create error"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, report.Errors)
		}
	}
	if report.FilesPushed != 0 {
		t.Errorf("nothing should count as pushed, got %d", report.FilesPushed)
	}
}

func TestIndexLocalDocsReadsIdentityFromFrontmatterTitleOrFilename(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "a.md"), "---\nid: \"feat-1\"\ntitle: Alpha\n---\n\nbody\n")
	writeDoc(t, filepath.Join(dir, "b.md"), "# ADR-7 Beta\n")
	writeDoc(t, filepath.Join(dir, "FEAT-9-gamma.md"), "# Gamma\n")
	writeDoc(t, filepath.Join(dir, "c.md"), "---\ntitle: No id\n---\nx\n")
	writeDoc(t, filepath.Join(dir, "locked.md"), "# Locked\n")
	if err := os.Chmod(filepath.Join(dir, "locked.md"), 0); err != nil {
		t.Fatal(err)
	}

	index, err := indexLocalDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id:FEAT-1", "id:ADR-7", "id:FEAT-9", "title:alpha", "sanitized:no-id"} {
		if _, ok := index[key]; !ok {
			t.Errorf("index has no %q: %v", key, keys(index))
		}
	}
	if _, ok := index["title:locked"]; ok {
		t.Error("an unreadable file must be skipped")
	}
	if _, err := indexLocalDocs(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing directory must be an error")
	}
}

func keys(m map[string]localDocInfo) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestFindMatchingLocalDoc(t *testing.T) {
	index := map[string]localDocInfo{
		"id:FEAT-1":       {path: "by-id"},
		"title:the title": {path: "by-title"},
		"sanitized:slug":  {path: "by-slug"},
		"file:file-slug":  {path: "by-file"},
	}
	cases := map[string]string{
		"FEAT-1 anything": "by-id",
		"The Title":       "by-title",
		"Slug":            "by-slug",
		"File Slug":       "by-file",
		"FEAT-2 unknown":  "",
	}
	for name, want := range cases {
		got, ok := findMatchingLocalDoc(name, index)
		if (want != "") != ok || got.path != want {
			t.Errorf("findMatchingLocalDoc(%q) = %q, %v; want %q", name, got.path, ok, want)
		}
	}
}

func TestDetermineTaxonomyPath(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"Scenarios overview", "", "SCENARIOS.md"},
		{"README", "", "README.md"},
		{"Architecture & Development Environment", "", "README.md"},
		{"FEAT-12 Search", "", "features/FEAT-12-feat-12-search.md"},
		{"Misc", "mentions a feature", "features/misc.md"},
		{"ADR-3 Choice", "", "adr/ADR-3-adr-3-choice.md"},
		{"Setup guide", "", "guides/setup-guide.md"},
		{"Topology", "", "architecture/topology.md"},
		{"Benchmark results", "", "quality/benchmark-results.md"},
		{"Command reference", "", "cli/command-reference.md"},
		{"Federation", "", "project-sharing/federation.md"},
		{"Roadmap", "", "roadmap/roadmap.md"},
		{"Something else", "", "features/something-else.md"},
	}
	for _, c := range cases {
		if got := determineTaxonomyPath("docs", c.name, c.body); got != filepath.Join("docs", c.want) {
			t.Errorf("determineTaxonomyPath(%q) = %q, want %q", c.name, got, filepath.Join("docs", c.want))
		}
	}
}

func TestPullDocsHandlesEveryPageShape(t *testing.T) {
	dir := t.TempDir()
	// Local docs a pulled page can match: one with front matter, one plain with a heading.
	writeDoc(t, filepath.Join(dir, "framed.md"), "---\ntitle: Framed\n---\n\nold\n")
	writeDoc(t, filepath.Join(dir, "plain.md"), "# Plain\n\nold\n")
	writeDoc(t, filepath.Join(dir, "same.md"), "# Same\n\nkept\n")
	writeDoc(t, filepath.Join(dir, "equal.md"), "# Equal\n\n* item\n")

	p := newPagesPlane(t,
		PlanePage{ID: "1", Name: "Framed", DescriptionHTML: "<p>new framed</p>"},
		PlanePage{ID: "2", Name: "Plain", DescriptionHTML: "<p>new plain</p>"},
		PlanePage{ID: "3", Name: "Same", DescriptionHTML: "<h1>Same</h1><p>kept</p>"},
		PlanePage{ID: "4", Name: "Equal", DescriptionHTML: MarkdownToHTML("# Equal\n\n* item\n")},
		PlanePage{ID: "5", Name: "Stripped", DescriptionStripped: "only stripped text"},
		PlanePage{ID: "6", Name: "Titled", DescriptionHTML: "<h1>Titled</h1><p>body</p>"},
		PlanePage{ID: "7", Name: "Broken", DescriptionHTML: "<p>x</p>"},
	)
	p.get["7"] = 500

	report, err := SyncDocs(context.Background(), p.client(t), dir, DirectionFromPlane)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) != 1 || !strings.Contains(report.Errors[0], "get page error 7") {
		t.Errorf("errors = %v, want only the broken page", report.Errors)
	}
	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return string(data)
	}
	if got := read("framed.md"); !strings.HasPrefix(got, "---\ntitle: Framed\n---\n\nnew framed") {
		t.Errorf("framed.md = %q", got)
	}
	if got := read("plain.md"); !strings.HasPrefix(got, "# Plain\n\nnew plain") {
		t.Errorf("plain.md = %q", got)
	}
	if got := read("equal.md"); got != "# Equal\n\n* item\n" {
		t.Errorf("a page that renders the same HTML must not rewrite the file, got %q", got)
	}
	if got := read("same.md"); got != "# Same\n\nkept\n" {
		t.Errorf("an unchanged page must not rewrite the file, got %q", got)
	}
	if got := read("features/stripped.md"); !strings.Contains(got, "# Stripped\n\nonly stripped text") {
		t.Errorf("stripped.md = %q", got)
	}
	if got := read("features/titled.md"); got != "# Titled\n\nbody\n" {
		t.Errorf("titled.md = %q", got)
	}
}

func TestPullDocsReportsFileSystemFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("target is a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "docs")
		writeDoc(t, file, "x")
		p := newPagesPlane(t)
		if _, err := SyncDocs(ctx, p.client(t), file, DirectionFromPlane); err == nil {
			t.Fatal("expected the directory error")
		}
	})

	t.Run("a subdirectory cannot be created", func(t *testing.T) {
		dir := t.TempDir()
		writeDoc(t, filepath.Join(dir, "features"), "a file where the directory should go")
		p := newPagesPlane(t, PlanePage{ID: "1", Name: "Blocked", DescriptionHTML: "<p>x</p>"})
		report, err := SyncDocs(ctx, p.client(t), dir, DirectionFromPlane)
		if err != nil || len(report.Errors) != 1 || !strings.Contains(report.Errors[0], "mkdir error") {
			t.Fatalf("report = %+v, err = %v", report, err)
		}
	})

	t.Run("a file cannot be written", func(t *testing.T) {
		dir := t.TempDir()
		writeDoc(t, filepath.Join(dir, "features", "keep.md"), "x")
		readOnly := filepath.Join(dir, "features")
		if err := os.Chmod(readOnly, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })
		p := newPagesPlane(t, PlanePage{ID: "1", Name: "Locked", DescriptionHTML: "<p>x</p>"})
		report, err := SyncDocs(ctx, p.client(t), dir, DirectionFromPlane)
		if err != nil || len(report.Errors) != 1 || !strings.Contains(report.Errors[0], "write file error") {
			t.Fatalf("report = %+v, err = %v", report, err)
		}
	})
}
