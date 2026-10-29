package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// requestAPI is a workspace with three projects (DOC = this repository, OXI = a peer, TAX = a
// stranger). Issues created in a project are kept and listed back, like Plane does.
type requestAPI struct {
	mu     sync.Mutex
	issues map[string][]map[string]any // project id -> issues
}

func (f *requestAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	p := r.URL.Path
	switch {
	case p == "/api/v1/workspaces/ws/projects/":
		write(map[string]any{"results": []map[string]string{
			{"id": "p-doc", "name": "docula", "identifier": "DOC"},
			{"id": "p-oxi", "name": "oxipix", "identifier": "OXI"},
			{"id": "p-tax", "name": "tax", "identifier": "TAX"}}})
	case strings.Contains(p, "/issues/") && strings.HasSuffix(p, "/issue-relation/"):
		write(map[string]any{})
	case strings.HasSuffix(p, "/states/"):
		write(map[string]any{"results": []map[string]any{{"id": "s-todo", "name": "Todo", "group": "unstarted"}}})
	case strings.HasSuffix(p, "/issues/") && r.Method == http.MethodGet:
		list := f.issues[projectOf(p)]
		if list == nil {
			list = []map[string]any{} // Plane lists an empty project as [], never null
		}
		write(map[string]any{"results": list, "next_page_results": false})
	case strings.HasSuffix(p, "/issues/") && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		proj := projectOf(p)
		n := len(f.issues[proj]) + 1
		for k, v := range body { // Plane never answers null: drop what the request left empty
			if v == nil {
				delete(body, k)
			}
		}
		body["id"] = fmt.Sprintf("%s-uuid-%d", proj, n)
		body["sequence_id"] = n
		body["labels"] = []string{}
		f.issues[proj] = append(f.issues[proj], body)
		w.WriteHeader(http.StatusCreated)
		write(body)
	default:
		write(map[string]any{})
	}
}

func projectOf(path string) string {
	parts := strings.Split(path, "/")
	for i, s := range parts {
		if s == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (f *requestAPI) created(project string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.issues[project]...)
}

type requestFixture struct {
	client *plane.Client
	api    *requestAPI
	self   string // this repository (DOC)
	peer   string // its peer (OXI)
}

// newRequestFixture creates two repositories in one synapse "mesh" (self "doc", peer "oxipix"),
// with this one linked to Plane project DOC.
func newRequestFixture(t *testing.T) requestFixture {
	t.Helper()
	plainOutput(t)
	t.Setenv("DENDRITE_HOME", t.TempDir())
	api := &requestAPI{issues: map[string][]map[string]any{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	t.Setenv("PLANE_BASE_URL", server.URL)
	client, err := plane.NewClient(plane.Config{BaseURL: server.URL, WorkspaceSlug: "ws", ProjectID: "p-doc", DirectToken: "k"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	self, peer := filepath.Join(t.TempDir(), "docula"), filepath.Join(t.TempDir(), "oxipix")
	for _, d := range []string{self, peer} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		migrateGit(t, d, "init", "-q")
	}
	if err := plane.WriteLink(self, plane.ProjectLink{Identifier: "DOC", ProjectID: "p-doc", Name: "docula", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	saveRequestSynapse(t, "mesh", []reqMember{{"doc", self, "DOC"}, {"oxipix", peer, "OXI"}})
	return requestFixture{client: client, api: api, self: self, peer: peer}
}

type reqMember struct{ alias, dir, plane string }

func saveRequestSynapse(t *testing.T, id string, members []reqMember) {
	t.Helper()
	indexPath, _ := projectid.DefaultIndexPath()
	ix, _ := projectid.LoadIndex(indexPath)
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s := &synapse.Synapse{ID: id, Name: id, State: synapse.StateActive}
	for _, m := range members {
		e, err := ix.Register(m.dir)
		if err != nil {
			t.Fatal(err)
		}
		s.Members = append(s.Members, synapse.Member{ProjectID: e.ID, Alias: m.alias, Role: synapse.RolePeer, PathHint: m.dir, PlaneProject: m.plane})
	}
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
}

func TestPlaneRequest_CreatesTheTicketInThePeersProjectAndIsIdempotent(t *testing.T) {
	fx := newRequestFixture(t)
	var out bytes.Buffer
	o := planeRequestOptions{Target: "oxipix", Name: "Expose a batch resize API", Desc: "docula needs it\nfor thumbnails"}
	if err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, o, &out); err != nil {
		t.Fatalf("request: %v\n%s", err, out.String())
	}
	got := fx.api.created("p-oxi")
	if len(got) != 1 {
		t.Fatalf("expected one issue in the peer's project, got %d (%s)", len(got), out.String())
	}
	if got[0]["name"] != "Expose a batch resize API" || got[0]["external_source"] != "dendrite-synapse:mesh" {
		t.Errorf("issue = %v", got[0])
	}
	if desc, _ := got[0]["description_html"].(string); !strings.Contains(desc, "Requested by <b>DOC</b>") || !strings.Contains(desc, "synapse <code>mesh</code>") || !strings.Contains(desc, "thumbnails") {
		t.Errorf("description = %q", desc)
	}
	if len(fx.api.created("p-doc")) != 0 {
		t.Error("nothing may be created in this repository's own project")
	}
	if !strings.Contains(out.String(), "OXI-1") {
		t.Errorf("output should name the new ticket:\n%s", out.String())
	}

	out.Reset()
	if err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, o, &out); err != nil {
		t.Fatal(err)
	}
	if len(fx.api.created("p-oxi")) != 1 || !strings.Contains(out.String(), "already has this request") {
		t.Errorf("asking twice must not duplicate the ticket:\n%s (issues %d)", out.String(), len(fx.api.created("p-oxi")))
	}
}

func TestPlaneRequest_RefusesAProjectThatIsNotAPeer(t *testing.T) {
	fx := newRequestFixture(t)
	for _, target := range []string{"TAX", "tax-calculator", "doc"} { // a stranger, a stranger by alias, and itself
		err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, planeRequestOptions{Target: target, Name: "n", Desc: "d"}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "not a peer") {
			t.Errorf("target %q: %v", target, err)
		}
	}
	if len(fx.api.created("p-tax")) != 0 {
		t.Error("nothing may be created for a non-peer")
	}
}

func TestPlaneRequest_APeerInSeveralSynapsesNeedsTheSynapseFlag(t *testing.T) {
	fx := newRequestFixture(t)
	saveRequestSynapse(t, "other", []reqMember{{"doc", fx.self, "DOC"}, {"oxi", fx.peer, "OXI"}})
	o := planeRequestOptions{Target: "OXI", Name: "n", Desc: "d"}

	err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, o, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--synapse") || !strings.Contains(err.Error(), "mesh") || !strings.Contains(err.Error(), "other") {
		t.Fatalf("expected the ambiguity to name both synapses, got %v", err)
	}
	if len(fx.api.created("p-oxi")) != 0 {
		t.Fatal("nothing may be created before the synapse is chosen")
	}

	o.Synapse = "other"
	if err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, o, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := fx.api.created("p-oxi"); len(got) != 1 || got[0]["external_source"] != "dendrite-synapse:other" {
		t.Fatalf("the request must carry the chosen synapse: %v", got)
	}

	o.Synapse = "nope"
	if err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, o, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "not in synapse") {
		t.Fatalf("an unknown synapse must be rejected, got %v", err)
	}
}

func TestPlaneRequest_PeerWithoutAPlaneProjectGetsAHint(t *testing.T) {
	fx := newRequestFixture(t)
	saveRequestSynapse(t, "mesh", []reqMember{{"doc", fx.self, "DOC"}, {"oxipix", fx.peer, ""}})
	err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, planeRequestOptions{Target: "oxipix", Name: "n", Desc: "d"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "synapse set-plane mesh oxipix") {
		t.Fatalf("expected the set-plane hint, got %v", err)
	}
}

func TestPlaneRequest_NeedsNameAndDescription(t *testing.T) {
	fx := newRequestFixture(t)
	if err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, planeRequestOptions{Target: "oxipix", Name: "n"}, &bytes.Buffer{}); err == nil {
		t.Fatal("a request without a description must be rejected")
	}
}

func TestChronosNext_SynapseFilterKeepsOnlyRequestsFromThatSynapse(t *testing.T) {
	fx := newRequestFixture(t)
	fx.api.mu.Lock()
	fx.api.issues["p-doc"] = []map[string]any{
		{"id": "i-1", "name": "[DEN-1] ordinary urgent work", "sequence_id": 1, "state": "s-todo", "priority": "urgent"},
		{"id": "i-2", "name": "[DEN-2] asked by a peer", "sequence_id": 2, "state": "s-todo", "priority": "low", "external_source": "dendrite-synapse:mesh"},
		{"id": "i-3", "name": "[DEN-3] asked through another synapse", "sequence_id": 3, "state": "s-todo", "priority": "high", "external_source": "dendrite-synapse:other"},
	}
	fx.api.mu.Unlock()

	var out bytes.Buffer
	if err := runChronosNext(context.Background(), fx.client, "", "", "mesh", &out); err != nil {
		t.Fatalf("next --synapse: %v", err)
	}
	if !strings.Contains(out.String(), "DEN-2") || strings.Contains(out.String(), "DEN-1\n") || strings.Contains(out.String(), "DEN-3") {
		t.Errorf("only the mesh request may be considered:\n%s", out.String())
	}

	out.Reset()
	if err := runChronosNext(context.Background(), fx.client, "", "", "", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "DEN-1") {
		t.Errorf("without the filter the urgent ticket wins:\n%s", out.String())
	}
}

func TestPlaneRequest_FromLinksTheSourceTicketAndLinkResolvesPeerTickets(t *testing.T) {
	fx := newRequestFixture(t)
	fx.api.mu.Lock()
	fx.api.issues["p-doc"] = []map[string]any{{"id": "doc-1", "name": "[DEN-1] needs thumbnails", "sequence_id": 1, "state": "s-todo"}}
	fx.api.mu.Unlock()

	var out bytes.Buffer
	o := planeRequestOptions{Target: "oxipix", Name: "Batch resize", Desc: "for DEN-1", From: "DEN-1"}
	if err := planeRequest(context.Background(), fx.client, planesvc.Default(), fx.self, o, &out); err != nil {
		t.Fatalf("request: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "DEN-1 is now blocked by OXI-1") {
		t.Errorf("--from must link the source ticket:\n%s", out.String())
	}
	if desc, _ := fx.api.created("p-oxi")[0]["description_html"].(string); !strings.Contains(desc, "Needed by <b>DEN-1</b>") {
		t.Errorf("the request must name the ticket that needs it: %q", desc)
	}

	// `chronos link` can now name the peer's ticket from this project.
	uuid, err := resolvePlaneTicketUUID(context.Background(), fx.client, "OXI-1")
	if err != nil || uuid != "p-oxi-uuid-1" {
		t.Errorf("OXI-1 resolved to %q, %v", uuid, err)
	}
	if _, err := resolvePlaneTicketUUID(context.Background(), fx.client, "OXI-99"); err == nil {
		t.Error("an unknown peer ticket must not resolve")
	}
}
