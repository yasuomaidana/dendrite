package plane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectLink_RoundTripAndRepoRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "crates", "core")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := LinkForDir(sub); err == nil {
		t.Fatal("expected an error before the link exists")
	}
	want := ProjectLink{Identifier: "EMBEDAG", ProjectID: "p-1", Name: "embedag", Workspace: "ws"}
	if err := WriteLink(root, want); err != nil {
		t.Fatal(err)
	}
	got, gotRoot, err := LinkForDir(sub)
	if err != nil {
		t.Fatal(err)
	}
	if *got != want || gotRoot != root {
		t.Errorf("LinkForDir = %+v at %s; want %+v at %s", *got, gotRoot, want, root)
	}

	if _, err := FindRepoRoot(t.TempDir()); err != ErrNoRepoRoot {
		t.Errorf("FindRepoRoot outside a repo = %v; want ErrNoRepoRoot", err)
	}
}

func TestReadLink_RejectsMalformedAndEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".dendrite"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"{not json", `{"identifier":"X"}`} {
		if err := os.WriteFile(LinkPath(root), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadLink(root); err == nil {
			t.Errorf("ReadLink(%q) succeeded; want error", body)
		}
	}
}

func TestNewClientFromEnv_UsesRepoLink(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteLink(root, ProjectLink{Identifier: "OXI", ProjectID: "proj-oxi", Workspace: "other-ws"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLANE_PROJECT_ID", "")
	t.Setenv("PLANE_WORKSPACE_SLUG", "")
	t.Chdir(root)

	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.ProjectID() != "proj-oxi" || c.WorkspaceSlug() != "other-ws" {
		t.Errorf("client = %s/%s; want other-ws/proj-oxi", c.WorkspaceSlug(), c.ProjectID())
	}

	t.Setenv("PLANE_PROJECT_ID", "explicit")
	if c, _ := NewClientFromEnv(); c.ProjectID() != "explicit" {
		t.Errorf("PLANE_PROJECT_ID must override the link, got %s", c.ProjectID())
	}
}

func TestSuggestIdentifier(t *testing.T) {
	cases := map[string]string{
		"embedag":                          "EMBEDAG",
		"dendrite-core-rs":                 "DENDRITECO",
		"introduction-to-machine-learning": "INTRODUCTI",
		"diesel_json_path":                 "DIESELJSON",
	}
	for in, want := range cases {
		if got := SuggestIdentifier(in); got != want {
			t.Errorf("SuggestIdentifier(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestEnsureProject_AdoptsOrCreatesAndProvisionsStates(t *testing.T) {
	var created, statePosts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws/projects/":
			_, _ = w.Write([]byte(`{"results": [{"id": "p-den", "name": "Dendrite Core", "identifier": "DEN"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workspaces/ws/projects/":
			created++
			var body map[string]string
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			_, _ = w.Write([]byte(`{"id": "p-new", "name": "` + body["name"] + `", "identifier": "` + body["identifier"] + `"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/states/"):
			_, _ = w.Write([]byte(`{"results": [{"id": "s1", "name": "Todo", "group": "unstarted"}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/states/"):
			statePosts++
			_, _ = w.Write([]byte(`{"id": "s2"}`))
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/states/"):
			_, _ = w.Write([]byte(`{"id": "s1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := NewClient(Config{BaseURL: server.URL, WorkspaceSlug: "ws", ProjectID: "p-den", DirectToken: "k"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	p, isNew, err := c.EnsureProject(ctx, "Dendrite Core", "den", "")
	if err != nil || isNew || p.ID != "p-den" {
		t.Fatalf("adopt = %+v, new=%v, err=%v; want existing p-den", p, isNew, err)
	}
	p, isNew, err = c.EnsureProject(ctx, "embedag", "EMBEDAG", "graph engine")
	if err != nil || !isNew || p.ID != "p-new" || created != 1 {
		t.Fatalf("create = %+v, new=%v, err=%v, posts=%d", p, isNew, err, created)
	}
	// The board starts with only "Todo", so each ensure creates the 5 other canonical states.
	if want := 2 * (len(CanonicalStates) - 1); statePosts != want {
		t.Errorf("created %d states; want %d (canonical states minus the existing one, per ensure)", statePosts, want)
	}
}
