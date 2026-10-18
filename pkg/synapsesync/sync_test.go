package synapsesync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo creates a repo on branch "feature" with one pushed commit and a project.id.
func newRepo(t *testing.T, name string) (dir, bare string) {
	t.Helper()
	root := t.TempDir()
	bare = filepath.Join(root, name+".git")
	dir = filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "--bare", "-b", "feature", bare)
	git(t, dir, "init", "-q", "-b", "feature")
	git(t, dir, "config", "user.email", "t@x")
	git(t, dir, "config", "user.name", "t")
	git(t, dir, "remote", "add", "origin", bare)
	if _, err := projectid.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, dir, "README.md")
	git(t, dir, "push", "-q", "-u", "origin", "feature")
	return dir, bare
}

func writeAndCommit(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "add "+name)
}

func member(t *testing.T, dir, alias, planeIdent string) synapse.Member {
	t.Helper()
	id, err := projectid.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	return synapse.Member{ProjectID: id, Alias: alias, Role: synapse.RolePeer, PathHint: dir, PlaneProject: planeIdent}
}

// planeServer is a permissive in-memory Plane workspace that records every write.
type planeServer struct {
	mu     sync.Mutex
	writes []string
}

func (p *planeServer) start(t *testing.T) *plane.Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/ws/projects/" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]string{
				{"id": "uuid-den", "identifier": "DEN"},
				{"id": "uuid-git", "identifier": "DGIT"},
			}})
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		p.mu.Lock()
		p.writes = append(p.writes, r.Method+" "+r.URL.Path)
		p.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new"}`))
	}))
	t.Cleanup(ts.Close)
	c, err := plane.NewClient(plane.Config{BaseURL: ts.URL, WorkspaceSlug: "ws", ProjectID: "uuid-den", DirectToken: "tok"}, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (p *planeServer) writesTo(project string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, w := range p.writes {
		if strings.Contains(w, "/projects/"+project+"/") {
			n++
		}
	}
	return n
}

func (p *planeServer) reset() {
	p.mu.Lock()
	p.writes = nil
	p.mu.Unlock()
}

const devStatus = "# DEV STATUS\n\n## Milestone 1: Core\n\n- [ ] [DEN-1] feat: something\n"

func TestRun_FansOutAcrossSynapse(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_COMMIT_PUSH", "")
	ctx := context.Background()

	den, denBare := newRepo(t, "den")
	gitRepo, gitBare := newRepo(t, "git")
	rag, _ := newRepo(t, "rag")

	// den: one unpushed commit + DEV_STATUS; git: unpushed commit + dirty file; rag: will go missing.
	if err := os.WriteFile(filepath.Join(den, "DEV_STATUS.md"), []byte(devStatus), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, den, "den.txt")
	writeAndCommit(t, gitRepo, "git.txt")
	if err := os.WriteFile(filepath.Join(gitRepo, "wip.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	syn := &synapse.Synapse{ID: "mesh", Name: "mesh", State: synapse.StateActive, Members: []synapse.Member{
		member(t, gitRepo, "git", "DGIT"),
		member(t, den, "den", "DEN"),
		member(t, rag, "rag", ""),
	}}
	if err := st.Save(syn); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(rag); err != nil {
		t.Fatal(err)
	}

	ps := &planeServer{}
	client := ps.start(t)

	// Dry run: nothing pushed, nothing written, synapse state untouched on disk.
	rep, err := Run(ctx, Options{StartDir: den, Plane: client, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Members) != 3 || rep.Members[0].Alias != "den" {
		t.Fatalf("expected den first of 3 members, got %+v", rep.Members)
	}
	if rep.Members[0].Push == nil || rep.Members[0].Push.Pushed {
		t.Fatalf("dry run must not push: %+v", rep.Members[0].Push)
	}
	if n := ps.writesTo("uuid-den") + ps.writesTo("uuid-git"); n != 0 {
		t.Fatalf("dry run wrote to Plane %d times", n)
	}
	if got, _ := st.Load("mesh"); got.State != synapse.StateActive {
		t.Fatalf("dry run must not persist state, got %s", got.State)
	}

	// Real run.
	rep, err = Run(ctx, Options{StartDir: den, Plane: client})
	if err != nil {
		t.Fatal(err)
	}
	byAlias := map[string]MemberReport{}
	for _, m := range rep.Members {
		byAlias[m.Alias] = m
	}

	d := byAlias["den"]
	if d.Push == nil || !d.Push.Pushed || len(d.Errors) != 0 {
		t.Fatalf("den should be pushed: %+v", d)
	}
	if git(t, denBare, "rev-parse", "feature") != git(t, den, "rev-parse", "HEAD") {
		t.Error("den remote not updated")
	}
	if d.Plane == nil || !d.Plane.StatusPushed || d.Plane.StateChanges != len(plane.CanonicalStates) {
		t.Errorf("den Plane sync: %+v", d.Plane)
	}

	g := byAlias["git"]
	if g.Push == nil || !g.Push.Pushed || len(g.Repo.Dirty) != 1 {
		t.Fatalf("git should be pushed and report its dirty file: %+v %+v", g.Push, g.Repo)
	}
	if git(t, gitBare, "rev-parse", "feature") != git(t, gitRepo, "rev-parse", "HEAD") {
		t.Error("git remote not updated")
	}
	if g.Plane == nil || g.Plane.StatusPushed || g.Plane.Skipped != "no DEV_STATUS.md" || ps.writesTo("uuid-git") == 0 {
		t.Errorf("git Plane states should reconcile without status push: %+v", g.Plane)
	}

	r := byAlias["rag"]
	if r.Status != synapse.StatusMissing || len(r.Errors) == 0 {
		t.Errorf("rag should be reported missing: %+v", r)
	}
	if !rep.Failed() {
		t.Error("report with a missing member should be marked failed")
	}
	if len(rep.Synapses) != 1 || rep.Synapses[0].After != synapse.StateBroken {
		t.Fatalf("synapse should flip to broken: %+v", rep.Synapses)
	}
	if got, _ := st.Load("mesh"); got.State != synapse.StateBroken {
		t.Fatalf("broken state should be persisted, got %s", got.State)
	}

	// Second run: DEV_STATUS unchanged -> no status push; nothing ahead -> no git push.
	ps.reset()
	rep, err = Run(ctx, Options{StartDir: den, Plane: client, GitOnly: false})
	if err != nil {
		t.Fatal(err)
	}
	d = rep.Members[0]
	if d.Push != nil || d.Plane.StatusPushed || d.Plane.Skipped != "DEV_STATUS unchanged since last sync" {
		t.Errorf("second run should be a no-op for den: push=%+v plane=%+v", d.Push, d.Plane)
	}

	// Heal: drop the missing member -> synapse returns to active.
	syn, _ = st.Load("mesh")
	syn.Members = syn.Members[:2]
	if err := st.Save(syn); err != nil {
		t.Fatal(err)
	}
	rep, err = Run(ctx, Options{SynapseID: "mesh", Plane: client, GitOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Synapses[0].After != synapse.StateActive || rep.Members[0].Plane != nil {
		t.Errorf("expected active synapse and git-only run: %+v %+v", rep.Synapses, rep.Members[0])
	}
}

func TestRun_StandaloneAndAll(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_COMMIT_PUSH", "false")
	ctx := context.Background()

	solo, _ := newRepo(t, "solo")
	writeAndCommit(t, solo, "x.txt")

	rep, err := Run(ctx, Options{StartDir: solo, PlaneOnly: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Members) != 1 || rep.Members[0].Synapse != "" {
		t.Fatalf("expected a single standalone member: %+v", rep.Members)
	}
	m := rep.Members[0]
	if m.Push == nil || m.Push.Pushed || m.Push.Reason != "DENDRITE_COMMIT_PUSH=false" {
		t.Errorf("push must honor DENDRITE_COMMIT_PUSH=false: %+v", m.Push)
	}
	if m.Plane == nil || m.Plane.Skipped == "" {
		t.Errorf("standalone repo without a bound Plane project must skip Plane: %+v", m.Plane)
	}

	// Plane bound but client unavailable.
	rep, _ = Run(ctx, Options{StartDir: solo, PlaneOnly: true, DefaultPlaneProject: "DEN"})
	if rep.Members[0].Plane.Skipped != "Plane client unavailable" || rep.Members[0].Repo != nil {
		t.Errorf("plane-only without client: %+v", rep.Members[0])
	}

	// --all with archived synapse skipped.
	st, _ := synapse.DefaultStore()
	peer, _ := newRepo(t, "peer")
	arch := &synapse.Synapse{ID: "old", Name: "old", State: synapse.StateArchived, Members: []synapse.Member{
		member(t, solo, "solo", ""), member(t, peer, "peer", ""),
	}}
	if err := st.Save(arch); err != nil {
		t.Fatal(err)
	}
	rep, err = Run(ctx, Options{All: true, GitOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Members) != 0 || len(rep.Synapses) != 0 {
		t.Errorf("archived synapses must be skipped: %+v", rep)
	}

	if _, err := Run(ctx, Options{SynapseID: "nope"}); err == nil {
		t.Error("expected error for unknown synapse")
	}
}

func TestRun_DivergedAndUnknownPlaneProject(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_COMMIT_PUSH", "")
	ctx := context.Background()

	repo, bare := newRepo(t, "div")
	other := filepath.Join(t.TempDir(), "other")
	git(t, filepath.Dir(other), "clone", "-q", "-b", "feature", bare, other)
	git(t, other, "config", "user.email", "o@x")
	git(t, other, "config", "user.name", "o")
	writeAndCommit(t, other, "remote.txt")
	git(t, other, "push", "-q")
	writeAndCommit(t, repo, "local.txt")

	ps := &planeServer{}
	rep, err := Run(ctx, Options{StartDir: repo, Fetch: true, Plane: ps.start(t), DefaultPlaneProject: "NOPE"})
	if err != nil {
		t.Fatal(err)
	}
	m := rep.Members[0]
	if m.Push == nil || m.Push.Pushed || !strings.Contains(m.Push.Reason, "diverged") {
		t.Errorf("diverged repo must not be pushed: %+v", m.Push)
	}
	if len(m.Errors) != 1 || !strings.Contains(m.Errors[0], "NOPE") {
		t.Errorf("unknown Plane project should be reported: %v", m.Errors)
	}
}
