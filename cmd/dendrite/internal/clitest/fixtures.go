package clitest

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/store/segmented"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/hitl"
	"Dendrite/pkg/tracker/hitl/hitltest"
	"Dendrite/pkg/tracker/plane"
	"Dendrite/pkg/workcopy"
	"Dendrite/pkg/workspace"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// boardIssues is what the board lists: DEN-101 starts after DEN-100.
var boardIssues = map[string]any{
	"results": []map[string]any{
		{"id": "uuid-100", "name": "[DEN-100] First", "sequence_id": 100, "state": "s-todo", "priority": "high"},
		{"id": "uuid-101", "name": "[DEN-101] Second", "sequence_id": 101, "state": "s-todo", "priority": "low",
			"start_date": "2026-10-01", "target_date": "2026-10-05"},
	},
	"next_page_results": false,
}

// boardLists are the collections the board answers by the end of the request path.
var boardLists = []struct {
	suffix string
	body   any
}{
	{"/workspaces/ws/projects/", map[string]any{"results": []map[string]any{{"id": "proj", "identifier": "DEN"}}}},
	{"/states/", []map[string]any{
		{"id": "s-todo", "name": "Todo", "group": "unstarted"},
		{"id": "s-done", "name": "Done", "group": "completed"},
	}},
	{"/modules/", []map[string]any{{"id": "mod-1", "name": "M1", "status": "in-progress"}}},
	{"/pages/", []map[string]any{{"id": "page-1", "name": "Roadmap", "updated_at": "2026-10-01T10:00:00Z"}}},
}

// boardServer serves a minimal Plane API over HTTP: two issues where DEN-101 is blocked by DEN-100.
type boardServer struct {
	Mu        sync.Mutex
	Fail      bool
	Relations []string
	Patches   int
}

func (b *boardServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if b.Fail {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	p := r.URL.Path
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	for _, l := range boardLists {
		if strings.HasSuffix(p, l.suffix) {
			write(l.body)
			return
		}
	}
	switch {
	case strings.HasSuffix(p, "/issue-relation/"):
		b.serveRelation(w, r, p, write)
	case strings.HasSuffix(p, "/issues/") && r.Method == http.MethodGet:
		write(boardIssues)
	case r.Method == http.MethodPatch:
		b.Patches++
		write(map[string]any{"id": "uuid-100", "name": "[DEN-100] First", "sequence_id": 100})
	default:
		write(map[string]any{})
	}
}

// serveRelation records the relations posted and answers the one DEN-101 holds.
func (b *boardServer) serveRelation(w http.ResponseWriter, r *http.Request, p string, write func(any)) {
	if r.Method == http.MethodPost {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		b.Relations = append(b.Relations, body["relation_type"].(string))
		w.WriteHeader(http.StatusCreated)
		write([]any{})
		return
	}
	if strings.Contains(p, "uuid-101") {
		write(map[string]any{"blocked_by": []map[string]any{{"id": "uuid-100", "name": "[DEN-100] First"}}})
		return
	}
	write(map[string]any{})
}

func FreePort(t *testing.T) int { return testkit.FreePort(t) }

func GitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// isolateEngines points every inference engine at a closed port so probes fail fast.
func IsolateEngines(t *testing.T) {
	t.Helper()
	for _, e := range []string{"OLLAMA", "OXILLAMA", "VLLM", "HF_TGI", "HF_TEI", "CANDLE", "LLAMACPP"} {
		t.Setenv("DENDRITE_"+e+"_ENDPOINT", "http://127.0.0.1:1")
	}
}

func MigrateGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// offlineSyncer points github-sync and hitl-sync at a local Plane server and a
// scripted gh so tests never reach the real Plane instance or GitHub.
func OfflineSyncer(t *testing.T, issues ...plane.PlaneIssue) (*hitltest.PlaneServer, *hitltest.GHScript) {
	t.Helper()
	ps := hitltest.NewPlaneServer(issues...)
	t.Cleanup(ps.Close)
	gh := &hitltest.GHScript{}
	prev := cli.NewHITLSyncer
	cli.NewHITLSyncer = func(_ context.Context, repo string) (*hitl.Syncer, error) {
		if repo == "fail/me" {
			return nil, errors.New("no plane")
		}
		s := hitl.New(ps.Client(), github.NewGitHubClient("o/r").WithExecutor(gh), ".")
		s.Git = gh
		return s, nil
	}
	t.Cleanup(func() { cli.NewHITLSyncer = prev })
	return ps, gh
}

type ReqMember struct{ Alias, Dir, Plane string }

func SaveRequestSynapse(t *testing.T, id string, members []ReqMember) {
	t.Helper()
	indexPath, _ := projectid.DefaultIndexPath()
	ix, _ := projectid.LoadIndex(indexPath)
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s := &synapse.Synapse{ID: id, Name: id, State: synapse.StateActive}
	for _, m := range members {
		e, err := ix.Register(m.Dir)
		if err != nil {
			t.Fatal(err)
		}
		s.Members = append(s.Members, synapse.Member{ProjectID: e.ID, Alias: m.Alias, Role: synapse.RolePeer, PathHint: m.Dir, PlaneProject: m.Plane})
	}
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
}

func SaveTestSynapse(t *testing.T, id string, dirs map[string]string) {
	t.Helper()
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s := &synapse.Synapse{ID: id, Name: id, State: synapse.StateActive}
	for _, alias := range []string{"den", "git", "rag"} {
		dir, ok := dirs[alias]
		if !ok {
			continue
		}
		pid, err := projectid.Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		s.Members = append(s.Members, synapse.Member{ProjectID: pid, Alias: alias, Role: synapse.RolePeer, PathHint: dir})
	}
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
}

func ServeBoard(t *testing.T) *boardServer {
	t.Helper()
	b := &boardServer{}
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	t.Setenv("PLANE_BASE_URL", srv.URL)
	t.Setenv("PLANE_API_KEY", "test-token")
	t.Setenv("PLANE_WORKSPACE_SLUG", "ws")
	t.Setenv("PLANE_PROJECT_ID", "proj")
	return b
}

func SetupCommitRepo(t *testing.T) string {
	t.Setenv("DENDRITE_TICKET_GUARD", "off") // the tests use made-up ticket ids; never ask the real Plane
	t.Helper()
	repo := t.TempDir()
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	RunGitT(t, repo, "init", "-q", "-b", "feat/x")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RunGitT(t, repo, "add", ".")
	RunGitT(t, repo, "commit", "-q", "-m", "init")
	t.Chdir(repo)
	return repo
}

// setupWorkRepo creates a repo with one commit and points the work CLI at it.
func SetupWorkRepo(t *testing.T) (repo, root string) {
	t.Setenv("DENDRITE_TICKET_GUARD", "off") // the tests use made-up ticket ids; never ask the real Plane
	t.Helper()
	base := workcopy.Canon(t.TempDir())
	repo = filepath.Join(base, "Proj")
	root = filepath.Join(base, "work")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(workcopy.EnvWorkDir, root)
	t.Setenv("DENDRITE_STATE_DIR", filepath.Join(base, "state"))
	RunGitT(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RunGitT(t, repo, "add", ".")
	RunGitT(t, repo, "commit", "-q", "-m", "init")

	prev := cli.NewWorkManager
	cli.NewWorkManager = func() *workcopy.Manager { return workcopy.New(repo) }
	t.Cleanup(func() { cli.NewWorkManager = prev })
	return repo, root
}

func SyncTestFile(t *testing.T, dir, name string, commit bool) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	if commit {
		RunGitT(t, dir, "add", "-A")
		RunGitT(t, dir, "commit", "-q", "-m", "add "+name)
	}
}

// syncTestRepo creates a repo on "feature" with one pushed commit, a bare origin and a project.id.
func SyncTestRepo(t *testing.T, name string) (dir, bare string) {
	t.Helper()
	root := t.TempDir()
	dir, bare = filepath.Join(root, name), filepath.Join(root, name+".git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	RunGitT(t, root, "init", "-q", "--bare", "-b", "feature", bare)
	RunGitT(t, dir, "init", "-q", "-b", "feature")
	RunGitT(t, dir, "config", "user.email", "t@x")
	RunGitT(t, dir, "config", "user.name", "t")
	RunGitT(t, dir, "remote", "add", "origin", bare)
	if _, err := projectid.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	SyncTestFile(t, dir, "README.md", true)
	RunGitT(t, dir, "push", "-q", "-u", "origin", "feature")
	return dir, bare
}

// useTempRegistry points the ganglion commands at a registry under t.TempDir().
func UseTempRegistry(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := cli.NewProjectRegistry
	cli.NewProjectRegistry = func() (*workspace.Registry, error) {
		return workspace.NewRegistry(filepath.Join(dir, "config.json"), filepath.Join(dir, "projects"))
	}
	t.Cleanup(func() { cli.NewProjectRegistry = prev })
	return dir
}

// useTempStore points engram and cognition at a segmented store under t.TempDir().
func UseTempStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	prev := cli.OpenSegmentedStore
	cli.OpenSegmentedStore = func() (*segmented.SegmentedStore, error) { return segmented.NewSegmentedStore(dir) }
	t.Cleanup(func() { cli.OpenSegmentedStore = prev })
}

func WriteTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		testkit.WriteFile(t, filepath.Join(root, name), content, 0o644)
	}
}
