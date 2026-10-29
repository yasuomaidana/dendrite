package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/projectaudit"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/tracker/plane"
)

// removeAPI is a Plane that lists three projects and records the archive and delete calls the
// real dendrite-plane sidecar makes.
type removeAPI struct {
	mu    sync.Mutex
	calls []string // "POST /…/archive/" / "DELETE /…/"
}

func (f *removeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/api/v1/workspaces/ws/projects/" && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(`{"results": [
			{"id": "p-old", "name": "Old Project", "identifier": "OLD"},
			{"id": "p-two", "name": "Second Old", "identifier": "TWO"},
			{"id": "p-keep", "name": "Keep Me", "identifier": "KEEP"}]}`))
	case r.Method == http.MethodDelete || (r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/archive/")):
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (f *removeAPI) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// removeFixture builds a temp DENDRITE_HOME, a fake Plane and one repository bound to OLD.
func removeFixture(t *testing.T) (client *plane.Client, api *removeAPI, repo string, entryID string) {
	t.Helper()
	plainOutput(t)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	api = &removeAPI{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	t.Setenv("PLANE_BASE_URL", server.URL)
	var err error
	client, err = plane.NewClient(plane.Config{BaseURL: server.URL, WorkspaceSlug: "ws", ProjectID: "p-keep", DirectToken: "k"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	repo = filepath.Join(t.TempDir(), "old-repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	migrateGit(t, repo, "init", "-q")
	if err := plane.WriteLink(repo, plane.ProjectLink{Identifier: "OLD", ProjectID: "p-old", Name: "Old Project", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	indexPath, _ := projectid.DefaultIndexPath()
	ix, _ := projectid.LoadIndex(indexPath)
	e, err := ix.Register(repo)
	if err != nil {
		t.Fatal(err)
	}
	ix.SetPlane(e.ID, &projectid.PlaneBinding{Identifier: "OLD", ProjectID: "p-old", Workspace: "ws"})
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	return client, api, repo, e.ID
}

func TestPlaneProjectRemove_ArchivesByDefaultAndDetachesTheLocalBinding(t *testing.T) {
	client, api, repo, id := removeFixture(t)
	var out bytes.Buffer
	err := planeProjectRemove(context.Background(), client, planesvc.Default(), planeProjectRemoveOptions{Refs: []string{"OLD"}, Yes: true}, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("remove: %v\n%s", err, out.String())
	}

	calls := api.recorded()
	if len(calls) != 1 || calls[0] != "POST /api/v1/workspaces/ws/projects/p-old/archive/" {
		t.Fatalf("expected exactly one archive call, got %v", calls)
	}
	if _, err := os.Stat(plane.LinkPath(repo)); !os.IsNotExist(err) {
		t.Errorf("plane.json must be removed: %v", err)
	}
	if got, err := projectid.Read(repo); err != nil || got != id {
		t.Errorf("project.id must stay: %q, %v", got, err)
	}
	indexPath, _ := projectid.DefaultIndexPath()
	ix, _ := projectid.LoadIndex(indexPath)
	if _, ok := ix.Lookup(id); ok {
		t.Error("the project must leave the global index")
	}
	untrackedPath, _ := projectid.DefaultUntrackedPath()
	if u, _ := projectid.LoadUntracked(untrackedPath); !u.Has(id) {
		t.Error("the project must be remembered as intentionally untracked")
	}
	kept := withoutUntracked([]projectaudit.Repo{{Root: repo}})
	if len(kept) != 0 {
		t.Errorf("audit must skip a detached repository, kept %v", kept)
	}
}

func TestPlaneProjectRemove_PurgeNeedsTheTypedNameAndAcceptsADirectory(t *testing.T) {
	client, api, repo, _ := removeFixture(t)
	ctx := context.Background()
	var out bytes.Buffer
	opts := planeProjectRemoveOptions{Refs: []string{repo}, Purge: true, Interactive: true}

	if err := planeProjectRemove(ctx, client, planesvc.Default(), opts, strings.NewReader("wrong name\n"), &out); err == nil {
		t.Fatal("a wrong name must abort")
	}
	if len(api.recorded()) != 0 {
		t.Fatalf("nothing may be deleted after an abort, got %v", api.recorded())
	}

	out.Reset()
	if err := planeProjectRemove(ctx, client, planesvc.Default(), opts, strings.NewReader("Old Project\n"), &out); err != nil {
		t.Fatalf("purge: %v\n%s", err, out.String())
	}
	if calls := api.recorded(); len(calls) != 1 || calls[0] != "DELETE /api/v1/workspaces/ws/projects/p-old/" {
		t.Fatalf("expected one delete call, got %v", calls)
	}
}

func TestPlaneProjectRemove_RefusesWithoutAConfirmationChannel(t *testing.T) {
	client, api, _, _ := removeFixture(t)
	err := planeProjectRemove(context.Background(), client, planesvc.Default(), planeProjectRemoveOptions{Refs: []string{"OLD"}}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected a --yes hint, got %v", err)
	}
	if len(api.recorded()) != 0 {
		t.Fatalf("nothing may be removed, got %v", api.recorded())
	}
}

func TestPlaneProjectRemove_KeepLocalLeavesTheBindingAlone(t *testing.T) {
	client, _, repo, id := removeFixture(t)
	if err := planeProjectRemove(context.Background(), client, planesvc.Default(), planeProjectRemoveOptions{Refs: []string{"OLD"}, Yes: true, KeepLocal: true}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plane.LinkPath(repo)); err != nil {
		t.Errorf("--keep-local must keep plane.json: %v", err)
	}
	indexPath, _ := projectid.DefaultIndexPath()
	ix, _ := projectid.LoadIndex(indexPath)
	if _, ok := ix.Lookup(id); !ok {
		t.Error("--keep-local must keep the index entry")
	}
}

func TestPlaneProjectRemove_RefusesSynapseMembersUnlessForced(t *testing.T) {
	client, api, repo, _ := removeFixture(t)
	other, _ := syncTestRepo(t, "den")
	saveTestSynapse(t, "mesh", map[string]string{"den": other, "git": repo})

	opts := planeProjectRemoveOptions{Refs: []string{"OLD"}, Yes: true}
	err := planeProjectRemove(context.Background(), client, planesvc.Default(), opts, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "synapse mesh") {
		t.Fatalf("expected the synapse to be named, got %v", err)
	}
	if len(api.recorded()) != 0 {
		t.Fatalf("nothing may be removed, got %v", api.recorded())
	}

	opts.Force = true
	if err := planeProjectRemove(context.Background(), client, planesvc.Default(), opts, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatalf("--force: %v", err)
	}
	if len(api.recorded()) != 1 {
		t.Fatalf("--force must archive, got %v", api.recorded())
	}
}

func TestPlaneProjectRemove_UnknownReferencesAreRejected(t *testing.T) {
	client, api, _, _ := removeFixture(t)
	for _, ref := range []string{"NOPE", "ol"} { // matching is exact (identifier, name or UUID), never a substring
		err := planeProjectRemove(context.Background(), client, planesvc.Default(), planeProjectRemoveOptions{Refs: []string{ref}, Yes: true}, strings.NewReader(""), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "no Plane project matches") {
			t.Errorf("ref %q: %v", ref, err)
		}
	}
	if len(api.recorded()) != 0 {
		t.Fatalf("nothing may be removed, got %v", api.recorded())
	}
}
