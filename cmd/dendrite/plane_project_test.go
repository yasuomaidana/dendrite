package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/tracker/plane"
)

// planeAPI emulates the slice of the Plane API that `plane project` uses.
type planeAPI struct {
	mu       sync.Mutex
	projects []map[string]string
	issues   []string
}

func (f *planeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	body := map[string]interface{}{}
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	path := r.URL.Path
	switch {
	case path == "/api/v1/workspaces/ws/projects/" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": f.projects})
	case path == "/api/v1/workspaces/ws/projects/" && r.Method == http.MethodPost:
		p := map[string]string{"id": "p-" + strings.ToLower(body["identifier"].(string)), "name": body["name"].(string), "identifier": body["identifier"].(string)}
		f.projects = append(f.projects, p)
		_ = json.NewEncoder(w).Encode(p)
	case strings.HasSuffix(path, "/states/") && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(`{"results": [
			{"id": "s-b", "name": "Backlog", "group": "backlog", "sequence": 15000},
			{"id": "s-t", "name": "To Do", "group": "unstarted", "sequence": 25000},
			{"id": "s-p", "name": "In Progress", "group": "started", "sequence": 35000},
			{"id": "s-v", "name": "To Verify", "group": "started", "sequence": 40000},
			{"id": "s-d", "name": "Done", "group": "completed", "sequence": 45000},
			{"id": "s-c", "name": "Cancelled", "group": "cancelled", "sequence": 55000}]}`))
	case strings.Contains(path, "/states/"):
		_, _ = w.Write([]byte(`{"id": "s"}`))
	case strings.Contains(path, "/modules/m-") && strings.HasSuffix(path, "/issues/"):
		_, _ = w.Write([]byte(`{}`))
	case strings.HasSuffix(path, "/modules/") && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(`{"results": []}`))
	case strings.HasSuffix(path, "/modules/"):
		_, _ = w.Write([]byte(`{"id": "m-1", "name": "` + body["name"].(string) + `"}`))
	case strings.HasSuffix(path, "/issues/") && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(`{"results": []}`))
	case strings.HasSuffix(path, "/issues/"):
		f.issues = append(f.issues, body["name"].(string))
		_, _ = w.Write([]byte(`{"id": "i"}`))
	default:
		http.NotFound(w, r)
	}
}

func newPlaneProjectFixture(t *testing.T) (*plane.Client, *planeAPI, string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	api := &planeAPI{projects: []map[string]string{{"id": "p-den", "name": "Dendrite Core", "identifier": "DEN"}}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	client, err := plane.NewClient(plane.Config{BaseURL: server.URL, WorkspaceSlug: "ws", ProjectID: "p-den", DirectToken: "k"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "graph-lib")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return client, api, repo
}

func TestPlaneProjectEnsure_CreatesLinksImportsThenAdopts(t *testing.T) {
	client, api, repo := newPlaneProjectFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "DEV_STATUS.md"), []byte("### Milestone 1\n- [x] **GL-1**: Arena\n- [ ] **GL-2**: Toposort\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	var out bytes.Buffer
	if err := runPlaneProject(ctx, client, []string{"ensure", repo, "--import", "DEV_STATUS.md"}, &out); err != nil {
		t.Fatalf("ensure: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Created Plane project GRAPHLIB") || !strings.Contains(out.String(), "2 created") {
		t.Errorf("unexpected output:\n%s", out.String())
	}
	link, err := plane.ReadLink(repo)
	if err != nil || link.Identifier != "GRAPHLIB" || link.ProjectID != "p-graphlib" || link.Workspace != "ws" {
		t.Fatalf("link = %+v, err %v", link, err)
	}
	if len(api.issues) != 2 || api.issues[0] != "[GL-1] Arena" {
		t.Errorf("imported issues = %v", api.issues)
	}
	if _, err := os.Stat(filepath.Join(repo, ".dendrite", "project.id")); err != nil {
		t.Errorf("ensure must mint the project identity: %v", err)
	}

	out.Reset()
	if err := runPlaneProject(ctx, client, []string{"ensure", repo}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Adopted Plane project GRAPHLIB") || len(api.projects) != 2 {
		t.Errorf("second ensure must adopt via the link, got:\n%s (projects %d)", out.String(), len(api.projects))
	}

	out.Reset()
	if err := runPlaneProject(ctx, client, []string{"show", filepath.Join(repo, ".dendrite")}, &out); err != nil || !strings.Contains(out.String(), "GRAPHLIB") {
		t.Errorf("show = %q, %v", out.String(), err)
	}
}

func TestPlaneProject_ErrorsAndUsage(t *testing.T) {
	client, _, repo := newPlaneProjectFixture(t)
	ctx := context.Background()
	var out bytes.Buffer

	cases := [][]string{
		{"show", repo}, // before any ensure: the repository has no link yet
		{"ensure", "--identifier"},
		{"ensure", "--bogus"},
		{"ensure", t.TempDir()},
		{"ensure", repo, "--import", "missing.md"},
		{"show", t.TempDir()},
		{"audit", "--roots"},
		{"audit", "--bogus"},
		{"nope"},
	}
	for _, args := range cases {
		if err := runPlaneProject(ctx, client, args, &out); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}
	out.Reset()
	if err := runPlaneProject(ctx, client, nil, &out); err != nil || !strings.Contains(out.String(), "plane project ensure") {
		t.Errorf("no args must print usage: %v %q", err, out.String())
	}
}

func TestPlaneProjectAudit_ReportsAndPasses(t *testing.T) {
	client, _, repo := newPlaneProjectFixture(t)
	ctx := context.Background()
	roots := filepath.Dir(repo)

	var out bytes.Buffer
	err := runPlaneProject(ctx, client, []string{"audit", "--roots", roots, "--owner", "me"}, &out)
	if !errors.Is(err, errAuditViolations) || !strings.Contains(out.String(), "dendrite plane project ensure "+repo) {
		t.Fatalf("audit before ensure: %v\n%s", err, out.String())
	}

	if err := runPlaneProject(ctx, client, []string{"ensure", repo}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runPlaneProject(ctx, client, []string{"audit", "--roots", roots, "--json"}, &out); err != nil {
		t.Fatalf("audit after ensure: %v\n%s", err, out.String())
	}
	var rep struct{ Repos []struct{ Name string } }
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || len(rep.Repos) != 1 {
		t.Errorf("json report = %s (%v)", out.String(), err)
	}

	// Without --roots/--owner the audit scans the parents of known projects and the current repo.
	t.Chdir(repo)
	out.Reset()
	if err := runPlaneProject(ctx, client, []string{"audit"}, &out); err != nil || !strings.Contains(out.String(), "graph-lib") {
		t.Errorf("default-root audit: %v\n%s", err, out.String())
	}

	if got := expandHome("~/x"); !strings.HasSuffix(got, string(filepath.Separator)+"x") || strings.HasPrefix(got, "~") {
		t.Errorf("expandHome = %q", got)
	}
}
